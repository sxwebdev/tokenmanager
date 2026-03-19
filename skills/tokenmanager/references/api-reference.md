# TokenManager API Reference

## Table of Contents

- [Types](#types)
- [Manager](#manager)
- [ITokenStore Interface](#itokenstore-interface)
- [MemoryTokenStore](#memorytokenstore)
- [Token Format](#token-format)
- [Storage Key Pattern](#storage-key-pattern)
- [Error Types](#error-types)
- [Helper Functions](#helper-functions)

## Types

### TokenType

```go
type TokenType string

const (
    AccessTokenType  TokenType = "access_token"
    RefreshTokenType TokenType = "refresh_token"
)
```

**Methods:**

- `String() string` — returns the string representation
- `IsValid() bool` — returns `true` only for `AccessTokenType` or `RefreshTokenType`

### Data[T]

Token metadata stored alongside each token. The generic parameter `T` is the user-defined additional data type.

```go
type Data[TAdditionalData any] struct {
    UserID         string          `json:"user_id"`
    IssuedAt       time.Time       `json:"issued_at"`
    Expiry         time.Time       `json:"expiry"`
    TokenType      TokenType       `json:"token_type"`
    AdditionalData TAdditionalData `json:"additional_data"`
}
```

This struct is serialized to JSON and stored in the backend. The `Expiry` field is set based on the `tokenDuration` passed to `New`.

## Manager

### Constructor

```go
func New[TAdditionalData any](
    store ITokenStore,
    secretKey string,
    tokenDuration time.Duration,
) *Manager[TAdditionalData]
```

Creates a new token manager. Parameters:

- `store` — storage backend implementing `ITokenStore`
- `secretKey` — HMAC-SHA256 signing key. Use at least 32 random bytes. `GenerateKey()` produces a suitable key.
- `tokenDuration` — TTL for tokens created by this manager

Create separate managers for different token types/durations:

```go
accessMgr  := tokenmanager.New[Claims](store, secret, 15*time.Minute)
refreshMgr := tokenmanager.New[Claims](store, secret, 7*24*time.Hour)
```

### CreateToken

```go
func (tm *Manager[T]) CreateToken(
    ctx context.Context,
    userID string,
    additionalData T,
    tokenType TokenType,
) (string, Data[T], error)
```

Generates a new signed token. Steps:

1. Generates 64 cryptographically random bytes (512-bit entropy)
2. Computes HMAC-SHA256 signature over the payload using the secret key
3. Stores `Data[T]` as JSON under key `tokenmanager:{payload_hex}` with the configured TTL
4. Returns the signed token string `{payload_hex}.{signature_hex}`, the token data, and any error

**Returns:** `(tokenString, tokenData, error)`

### ValidateToken

```go
func (tm *Manager[T]) ValidateToken(
    ctx context.Context,
    signedToken string,
    expectedType TokenType,
) (*Data[T], bool)
```

Validates a token's integrity and status. Checks performed in order:

1. **Format** — token must contain exactly one `.` separator
2. **Signature** — recomputes HMAC-SHA256 and uses `hmac.Equal` for constant-time comparison (prevents timing attacks)
3. **Storage** — retrieves token data from the store
4. **Type** — `TokenType` must match `expectedType`
5. **Expiry** — `Expiry` must be after `time.Now()`

**Returns:** `(*Data[T], true)` if all checks pass, `(nil, false)` for ANY failure. Never returns an error — this is intentional to simplify auth middleware.

### RevokeToken

```go
func (tm *Manager[T]) RevokeToken(
    ctx context.Context,
    signedToken string,
) error
```

Deletes the token from storage. After revocation, `ValidateToken` returns `(nil, false)` even if the token hasn't expired. The token string itself remains cryptographically valid but cannot be validated because its data is gone.

### UpdateAdditionalData

```go
func (tm *Manager[T]) UpdateAdditionalData(
    ctx context.Context,
    signedToken string,
    newAdditionalData T,
) error
```

Replaces the `AdditionalData` field of an existing token. Steps:

1. Extracts the payload from the signed token
2. Retrieves current `Data[T]` from storage
3. Replaces `AdditionalData` with `newAdditionalData`
4. Computes remaining TTL from the original `Expiry`
5. Re-stores the updated data with the remaining TTL

Returns an error if the token doesn't exist or storage fails.

## ITokenStore Interface

```go
type ITokenStore interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, expiration time.Duration) error
    Delete(ctx context.Context, key string) error
    Keys(ctx context.Context, prefix string) ([]string, error)
    KeysAndValues(ctx context.Context, prefix string) (map[string][]byte, error)
    GetFromJSON(ctx context.Context, key string, dst any) error
    SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error
    Exists(ctx context.Context, key string) (bool, error)
}
```

### Method contracts

| Method          | Behavior                                    | Error cases                                             |
| --------------- | ------------------------------------------- | ------------------------------------------------------- |
| `Get`           | Return raw bytes for key                    | `ErrKeyNotFound` if missing, `ErrKeyExpired` if expired |
| `Set`           | Store bytes with TTL                        | Storage failure                                         |
| `Delete`        | Remove key (no error if missing)            | Storage failure                                         |
| `Keys`          | Return all keys matching prefix             | Storage failure                                         |
| `KeysAndValues` | Return key-value map for prefix             | Storage failure                                         |
| `GetFromJSON`   | Get + `json.Unmarshal` into `dst`           | `ErrKeyNotFound`, `ErrKeyExpired`, unmarshal error      |
| `SetJSON`       | `json.Marshal` + Set                        | Marshal error, storage failure                          |
| `Exists`        | Return `true` if key exists and not expired | Storage failure                                         |

The manager primarily uses `GetFromJSON`, `SetJSON`, and `Delete`. The other methods (`Get`, `Set`, `Keys`, `KeysAndValues`, `Exists`) are part of the interface for general-purpose use and must still be implemented.

## MemoryTokenStore

```go
func NewMemoryTokenStore() *MemoryTokenStore
```

Built-in in-memory implementation:

- Thread-safe via `sync.RWMutex` (read-optimized)
- Stores items with expiration timestamps
- Runs automatic cleanup goroutine every 60 seconds removing expired items
- Suitable for testing and development; not for production (data lost on restart)

## Token Format

```text
{payload_hex}.{signature_hex}

payload_hex:   128 characters (64 random bytes, hex-encoded)
signature_hex: 64 characters (HMAC-SHA256 of payload bytes, hex-encoded)
```

Total token length: 193 characters (128 + 1 + 64).

## Storage Key Pattern

```text
tokenmanager:{payload_hex}
```

The prefix `tokenmanager:` is hardcoded. When implementing `Keys` and `KeysAndValues`, ensure prefix matching works with this pattern.

## Error Types

```go
var ErrKeyNotFound = errors.New("key not found")
var ErrKeyExpired  = errors.New("key expired")
```

These sentinel errors must be returned by `ITokenStore` implementations. The manager checks for these to distinguish between "token doesn't exist" and "storage failure".

## Helper Functions

### GenerateKey

```go
func GenerateKey() string
```

Generates a 64-character hex string (32 random bytes) suitable for use as a secret key. Uses `crypto/rand` for cryptographic randomness.
