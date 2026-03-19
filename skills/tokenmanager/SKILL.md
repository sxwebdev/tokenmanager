---
name: tokenmanager
description: >
  Go token management package (github.com/sxwebdev/tokenmanager) with HMAC-SHA256
  signing, Go generics, and pluggable storage backends. Use this skill whenever code
  imports or references "tokenmanager", "sxwebdev/tokenmanager", ITokenStore,
  MemoryTokenStore, TokenType, AccessTokenType, RefreshTokenType, or when the user
  asks about: creating/validating/revoking tokens with this package, implementing
  custom token stores (Redis, PostgreSQL, MongoDB), access/refresh token flows using
  tokenmanager, token-based auth middleware in Go, HMAC token signing patterns, or
  generic token data management. Also triggers when working with files that contain
  tokenmanager imports or when the user mentions "token manager" in the context of
  a Go project.
user-invocable: false
---

# TokenManager — Go Token Management Package

## Overview

`tokenmanager` is a zero-dependency Go package providing secure, type-safe token management using HMAC-SHA256 signatures and Go generics. Its architecture separates concerns into three layers:

1. **`Manager[T]`** — core engine handling token creation, validation, revocation, and data updates. The generic parameter `T` allows attaching arbitrary typed data (claims, roles, permissions) to each token.
2. **`ITokenStore`** — abstract storage interface enabling pluggable backends (Redis, PostgreSQL, in-memory, etc.).
3. **`MemoryTokenStore`** — built-in thread-safe in-memory implementation with automatic expiration cleanup, suitable for testing and development.

**Token format:** `{payload_hex}.{hmac_signature_hex}` where payload is 64 random bytes (512-bit entropy) and signature is HMAC-SHA256. Tokens are stored under the key pattern `tokenmanager:{payload_hex}`.

## Instructions

### Helping users integrate tokenmanager

1. Read the user's existing code to understand their storage layer and auth requirements.
2. Determine the appropriate `TAdditionalData` type — use a struct for structured claims, `map[string]any` for flexibility, or `struct{}` when no additional data is needed.
3. Guide users to create separate `Manager` instances for access vs. refresh tokens with different durations because the token type is validated and prevents type confusion attacks.
4. For custom storage backends, ensure the implementation satisfies the full `ITokenStore` interface. See `references/storage-backends.md` for complete implementation guides.
5. For auth middleware and token flow patterns, see `references/auth-patterns.md`.
6. For detailed API signatures and behavior, see `references/api-reference.md`.

### Implementing custom ITokenStore backends

When a user needs a production storage backend (Redis, PostgreSQL, etc.):

1. Read `references/storage-backends.md` for the interface contract and implementation examples.
2. Ensure all 8 methods are implemented: `Get`, `Set`, `Delete`, `Keys`, `KeysAndValues`, `GetFromJSON`, `SetJSON`, `Exists`.
3. `GetFromJSON` and `SetJSON` handle JSON marshaling — implement them using `Get`/`Set` with `json.Marshal`/`json.Unmarshal`.
4. `Keys` and `KeysAndValues` must support prefix-based filtering because the manager uses the `tokenmanager:` prefix.
5. Return `ErrKeyNotFound` when a key does not exist and `ErrKeyExpired` when a key has expired, because the manager relies on these sentinel errors.
6. All operations must be safe for concurrent use.

### Setting up authentication flows

When a user asks about auth patterns:

1. Read `references/auth-patterns.md` for access/refresh token flows, middleware, and revocation strategies.
2. Recommend separate `Manager` instances with different durations: 5-15 minutes for access tokens, 7-30 days for refresh tokens.
3. Always validate the expected `TokenType` to prevent a refresh token from being used as an access token.

## Key API Quick Reference

```go
// Constructor
func New[T any](store ITokenStore, secretKey string, tokenDuration time.Duration) *Manager[T]

// Core operations
func (m *Manager[T]) CreateToken(ctx context.Context, userID string, additionalData T, tokenType TokenType) (string, Data[T], error)
func (m *Manager[T]) ValidateToken(ctx context.Context, signedToken string, expectedType TokenType) (*Data[T], bool)
func (m *Manager[T]) RevokeToken(ctx context.Context, signedToken string) error
func (m *Manager[T]) UpdateAdditionalData(ctx context.Context, signedToken string, newAdditionalData T) error

// Token types
const AccessTokenType  TokenType = "access_token"
const RefreshTokenType TokenType = "refresh_token"

// Sentinel errors
var ErrKeyNotFound = errors.New("key not found")
var ErrKeyExpired  = errors.New("key expired")

// Key generation helper
func GenerateKey() string  // Returns 64-char hex string suitable as secret key
```

## Examples

**Example 1: User asks "How do I use tokenmanager with Redis?"**

Input: User wants to integrate tokenmanager with a Redis backend in their Go service.

Output: Claude reads `references/storage-backends.md`, then provides a complete `RedisTokenStore` implementation satisfying `ITokenStore`, wires it into `Manager`, and shows usage:

```go
type UserClaims struct {
    Role        string   `json:"role"`
    Permissions []string `json:"permissions"`
}

store := NewRedisTokenStore(redisClient)
accessMgr := tokenmanager.New[UserClaims](store, os.Getenv("TOKEN_SECRET"), 15*time.Minute)
refreshMgr := tokenmanager.New[UserClaims](store, os.Getenv("TOKEN_SECRET"), 7*24*time.Hour)

// Create tokens on login
accessToken, _, err := accessMgr.CreateToken(ctx, userID, claims, tokenmanager.AccessTokenType)
refreshToken, _, err := refreshMgr.CreateToken(ctx, userID, claims, tokenmanager.RefreshTokenType)
```

**Example 2: User asks "Why is my token validation failing?"**

Input: User reports `ValidateToken` always returns `false`.

Output: Claude checks common issues:

1. Token type mismatch — created with `AccessTokenType` but validated with `RefreshTokenType`
2. Token expired — check duration passed to `New`
3. Different secret keys between creation and validation managers
4. Storage backend not persisting correctly — verify `ITokenStore.Get` returns the stored data
5. Token string modified in transit — ensure no whitespace or encoding changes

## Key Principles

- **Fail-secure validation**: `ValidateToken` returns `(nil, false)` for ANY failure (expired, revoked, tampered, wrong type, storage error). It never returns an error — this is by design to simplify auth middleware logic. Log storage errors at the store level if observability is needed.
- **Type confusion prevention**: Always pass the expected `TokenType` to `ValidateToken`. A valid refresh token must NOT grant access when validated as an access token.
- **Secret key management**: Use `tokenmanager.GenerateKey()` or at least 32 random bytes for the secret. Store in environment variables or a secrets manager, never in source code.
- **Short-lived access tokens**: Keep access token duration at 5-15 minutes. Use refresh tokens (7-30 days) to obtain new access tokens without re-authentication.
- **Thread safety**: `Manager` operations are safe for concurrent use. Custom `ITokenStore` implementations must also be thread-safe because the manager does not add synchronization.
- **Storage cleanup**: `MemoryTokenStore` runs automatic cleanup every 60 seconds. Production backends (Redis, PostgreSQL) should handle TTL/expiration natively or implement periodic cleanup.
