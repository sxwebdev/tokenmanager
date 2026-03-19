# Custom Storage Backends for TokenManager

## Table of Contents

- [Interface Contract](#interface-contract)
- [Redis Implementation](#redis-implementation)
- [PostgreSQL Implementation](#postgresql-implementation)
- [Testing Custom Stores](#testing-custom-stores)

## Interface Contract

Every `ITokenStore` implementation must satisfy these rules:

1. **`Get`** — return `ErrKeyNotFound` if the key does not exist, `ErrKeyExpired` if the key exists but has expired. Never return `nil, nil` for a missing key.
2. **`Set`** — store with the given TTL. A zero or negative duration means no expiration.
3. **`Delete`** — idempotent. Do not return an error if the key doesn't exist.
4. **`Keys`** — return all keys matching the given prefix. The manager uses the prefix `tokenmanager:`.
5. **`KeysAndValues`** — return a map of key-to-raw-bytes for all keys matching the prefix.
6. **`GetFromJSON`** — equivalent to `Get` + `json.Unmarshal(data, dst)`. Return the same errors as `Get` plus any unmarshal error.
7. **`SetJSON`** — equivalent to `json.Marshal(value)` + `Set`. Return marshal errors or storage errors.
8. **`Exists`** — return `true` only if the key exists AND has not expired.
9. **Thread safety** — all methods must be safe for concurrent use.
10. **Context propagation** — respect context cancellation and deadlines.

## Redis Implementation

A production Redis backend using `github.com/redis/go-redis/v9`:

```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sxwebdev/tokenmanager"
)

type RedisTokenStore struct {
	client *redis.Client
}

func NewRedisTokenStore(client *redis.Client) *RedisTokenStore {
	return &RedisTokenStore{client: client}
}

func (s *RedisTokenStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, tokenmanager.ErrKeyNotFound
	}
	return val, err
}

func (s *RedisTokenStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return s.client.Set(ctx, key, value, expiration).Err()
}

func (s *RedisTokenStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

func (s *RedisTokenStore) Keys(ctx context.Context, prefix string) ([]string, error) {
	// SCAN is preferred over KEYS in production to avoid blocking Redis.
	var keys []string
	iter := s.client.Scan(ctx, 0, prefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *RedisTokenStore) KeysAndValues(ctx context.Context, prefix string) (map[string][]byte, error) {
	keys, err := s.Keys(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return map[string][]byte{}, nil
	}

	// Use pipeline for efficient bulk retrieval.
	pipe := s.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd, len(keys))
	for _, key := range keys {
		cmds[key] = pipe.Get(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	result := make(map[string][]byte, len(keys))
	for key, cmd := range cmds {
		val, err := cmd.Bytes()
		if err == nil {
			result[key] = val
		}
	}
	return result, nil
}

func (s *RedisTokenStore) GetFromJSON(ctx context.Context, key string, dst any) error {
	data, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func (s *RedisTokenStore) SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.Set(ctx, key, data, expiration)
}

func (s *RedisTokenStore) Exists(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
```

**Key decisions:**

- `Keys` uses `SCAN` instead of `KEYS` to avoid blocking Redis on large datasets.
- `KeysAndValues` uses a pipeline for efficient bulk retrieval.
- Redis handles TTL natively, so no manual expiration logic is needed.
- `redis.Nil` maps to `ErrKeyNotFound`.

## PostgreSQL Implementation

A PostgreSQL backend using `github.com/jackc/pgx/v5`:

```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sxwebdev/tokenmanager"
)

// Schema:
// CREATE TABLE token_store (
//     key        TEXT PRIMARY KEY,
//     value      BYTEA NOT NULL,
//     expires_at TIMESTAMPTZ
// );
// CREATE INDEX idx_token_store_prefix ON token_store (key text_pattern_ops);
// CREATE INDEX idx_token_store_expires ON token_store (expires_at);

type PgTokenStore struct {
	pool *pgxpool.Pool
}

func NewPgTokenStore(pool *pgxpool.Pool) *PgTokenStore {
	return &PgTokenStore{pool: pool}
}

func (s *PgTokenStore) Get(ctx context.Context, key string) ([]byte, error) {
	var value []byte
	var expiresAt *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT value, expires_at FROM token_store WHERE key = $1`, key,
	).Scan(&value, &expiresAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tokenmanager.ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		// Clean up expired key asynchronously.
		go s.Delete(context.Background(), key)
		return nil, tokenmanager.ErrKeyExpired
	}
	return value, nil
}

func (s *PgTokenStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	var expiresAt *time.Time
	if expiration > 0 {
		t := time.Now().Add(expiration)
		expiresAt = &t
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO token_store (key, value, expires_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (key) DO UPDATE SET value = $2, expires_at = $3`,
		key, value, expiresAt,
	)
	return err
}

func (s *PgTokenStore) Delete(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM token_store WHERE key = $1`, key,
	)
	return err
}

func (s *PgTokenStore) Keys(ctx context.Context, prefix string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key FROM token_store
		 WHERE key LIKE $1 AND (expires_at IS NULL OR expires_at > NOW())`,
		prefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *PgTokenStore) KeysAndValues(ctx context.Context, prefix string) (map[string][]byte, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, value FROM token_store
		 WHERE key LIKE $1 AND (expires_at IS NULL OR expires_at > NOW())`,
		prefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, rows.Err()
}

func (s *PgTokenStore) GetFromJSON(ctx context.Context, key string, dst any) error {
	data, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func (s *PgTokenStore) SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.Set(ctx, key, data, expiration)
}

func (s *PgTokenStore) Exists(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM token_store
			WHERE key = $1 AND (expires_at IS NULL OR expires_at > NOW())
		)`, key,
	).Scan(&exists)
	return exists, err
}
```

**Key decisions:**

- Uses `UPSERT` (`ON CONFLICT DO UPDATE`) for idempotent `Set`.
- `text_pattern_ops` index enables efficient prefix queries with `LIKE`.
- Expired rows are cleaned lazily on `Get` and filtered in queries. Set up a periodic `DELETE FROM token_store WHERE expires_at < NOW()` job for bulk cleanup.
- `pgx.ErrNoRows` maps to `ErrKeyNotFound`.

**Required schema migration:**

```sql
CREATE TABLE IF NOT EXISTS token_store (
    key        TEXT PRIMARY KEY,
    value      BYTEA NOT NULL,
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_token_store_prefix
    ON token_store (key text_pattern_ops);

CREATE INDEX IF NOT EXISTS idx_token_store_expires
    ON token_store (expires_at)
    WHERE expires_at IS NOT NULL;
```

## Testing Custom Stores

Test any custom `ITokenStore` implementation by running it through the tokenmanager test scenarios:

```go
func TestCustomStore(t *testing.T) {
	store := NewYourCustomStore(/* config */)
	ctx := context.Background()

	// 1. Basic set/get
	err := store.Set(ctx, "test:key1", []byte("value1"), time.Minute)
	require.NoError(t, err)

	val, err := store.Get(ctx, "test:key1")
	require.NoError(t, err)
	assert.Equal(t, []byte("value1"), val)

	// 2. Missing key returns ErrKeyNotFound
	_, err = store.Get(ctx, "test:nonexistent")
	assert.ErrorIs(t, err, tokenmanager.ErrKeyNotFound)

	// 3. Delete is idempotent
	err = store.Delete(ctx, "test:nonexistent")
	assert.NoError(t, err)

	// 4. JSON round-trip
	type payload struct {
		Name string `json:"name"`
	}
	err = store.SetJSON(ctx, "test:json1", payload{Name: "test"}, time.Minute)
	require.NoError(t, err)

	var dst payload
	err = store.GetFromJSON(ctx, "test:json1", &dst)
	require.NoError(t, err)
	assert.Equal(t, "test", dst.Name)

	// 5. Keys with prefix
	keys, err := store.Keys(ctx, "test:")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(keys), 2)

	// 6. Exists
	exists, err := store.Exists(ctx, "test:key1")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = store.Exists(ctx, "test:nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	// 7. Integration with Manager
	mgr := tokenmanager.New[payload](store, tokenmanager.GenerateKey(), 5*time.Second)
	token, _, err := mgr.CreateToken(ctx, "user1", payload{Name: "alice"}, tokenmanager.AccessTokenType)
	require.NoError(t, err)

	data, valid := mgr.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	assert.True(t, valid)
	assert.Equal(t, "alice", data.AdditionalData.Name)
}
```
