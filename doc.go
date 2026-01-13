// Package tokenmanager provides a secure, flexible, and type-safe token management
// system for Go applications. It supports creating, validating, revoking, and updating
// tokens with customizable additional data using Go generics.
//
// # Overview
//
// The tokenmanager package implements a token-based authentication system with the
// following key features:
//
//   - HMAC-SHA256 signed tokens for cryptographic security
//   - Generic type support for custom token payload data
//   - Pluggable storage backend via the [ITokenStore] interface
//   - Built-in in-memory storage implementation ([MemoryTokenStore])
//   - Support for access and refresh token types
//   - Automatic token expiration handling
//   - Thread-safe operations
//
// # Token Format
//
// Tokens are generated as signed strings in the format:
//
//	{payload}.{signature}
//
// Where:
//   - payload: 128-character hex string (64 random bytes from crypto/rand)
//   - signature: 64-character hex string (HMAC-SHA256 of the payload bytes)
//
// This format provides 512 bits of entropy in the payload, making tokens
// practically impossible to guess or brute-force.
//
// # Architecture
//
// The package follows a clean separation of concerns:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                    Manager[TAdditionalData]                 │
//	│  - CreateToken()    - ValidateToken()                       │
//	│  - RevokeToken()    - UpdateAdditionalData()                │
//	└─────────────────────────────────────────────────────────────┘
//	                              │
//	                              ▼
//	┌─────────────────────────────────────────────────────────────┐
//	│                      ITokenStore                            │
//	│  Interface for pluggable storage backends                   │
//	└─────────────────────────────────────────────────────────────┘
//	          │                                    │
//	          ▼                                    ▼
//	┌──────────────────────┐          ┌──────────────────────────┐
//	│  MemoryTokenStore    │          │  Custom Implementation   │
//	│  (built-in)          │          │  (Redis, PostgreSQL...)  │
//	└──────────────────────┘          └──────────────────────────┘
//
// # Basic Usage
//
// Creating a token manager with custom additional data:
//
//	// Define your custom data structure
//	type UserClaims struct {
//	    Role        string   `json:"role"`
//	    Permissions []string `json:"permissions"`
//	}
//
//	// Create storage and manager
//	store := tokenmanager.NewMemoryTokenStore()
//	manager := tokenmanager.New[UserClaims](
//	    store,
//	    "your-secret-key-here",
//	    15*time.Minute,
//	)
//
//	// Create a token
//	ctx := context.Background()
//	token, data, err := manager.CreateToken(
//	    ctx,
//	    "user-123",
//	    UserClaims{Role: "admin", Permissions: []string{"read", "write"}},
//	    tokenmanager.AccessTokenType,
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf("Token: %s\n", token)
//	fmt.Printf("Expires: %s\n", data.Expiry)
//
// # Token Validation
//
// Validating a token and retrieving its data:
//
//	data, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
//	if !valid {
//	    // Token is invalid, expired, revoked, or wrong type
//	    return errors.New("invalid token")
//	}
//
//	fmt.Printf("User ID: %s\n", data.UserID)
//	fmt.Printf("Role: %s\n", data.AdditionalData.Role)
//
// Validation checks performed:
//   - Token format (must contain exactly one dot separator)
//   - Payload is valid hex encoding
//   - HMAC-SHA256 signature matches (constant-time comparison)
//   - Token exists in storage
//   - Token type matches expected type
//   - Token has not expired
//
// # Token Revocation
//
// Revoking a token (e.g., on logout):
//
//	err := manager.RevokeToken(ctx, token)
//	if err != nil {
//	    log.Printf("Failed to revoke token: %v", err)
//	}
//
// After revocation, the token will fail validation even if it hasn't expired.
//
// # Updating Token Data
//
// Updating the additional data of an existing token:
//
//	newClaims := UserClaims{
//	    Role:        "superadmin",
//	    Permissions: []string{"read", "write", "delete", "admin"},
//	}
//	err := manager.UpdateAdditionalData(ctx, token, newClaims)
//	if err != nil {
//	    log.Printf("Failed to update token: %v", err)
//	}
//
// This is useful for updating user permissions without requiring re-authentication.
//
// # Token Types
//
// The package provides two predefined token types:
//
//   - [AccessTokenType]: Short-lived tokens for API authentication
//   - [RefreshTokenType]: Long-lived tokens for obtaining new access tokens
//
// Token type validation prevents type confusion attacks where an attacker
// might try to use a refresh token as an access token or vice versa.
//
// Example of implementing a refresh token flow:
//
//	// Create separate managers for access and refresh tokens
//	accessManager := tokenmanager.New[UserClaims](store, secret, 15*time.Minute)
//	refreshManager := tokenmanager.New[UserClaims](store, secret, 7*24*time.Hour)
//
//	// Issue both tokens on login
//	accessToken, _, _ := accessManager.CreateToken(ctx, userID, claims, tokenmanager.AccessTokenType)
//	refreshToken, _, _ := refreshManager.CreateToken(ctx, userID, claims, tokenmanager.RefreshTokenType)
//
//	// Refresh endpoint
//	func RefreshTokens(ctx context.Context, refreshToken string) (string, string, error) {
//	    data, valid := refreshManager.ValidateToken(ctx, refreshToken, tokenmanager.RefreshTokenType)
//	    if !valid {
//	        return "", "", errors.New("invalid refresh token")
//	    }
//
//	    // Revoke old refresh token
//	    refreshManager.RevokeToken(ctx, refreshToken)
//
//	    // Issue new tokens
//	    newAccess, _, _ := accessManager.CreateToken(ctx, data.UserID, data.AdditionalData, tokenmanager.AccessTokenType)
//	    newRefresh, _, _ := refreshManager.CreateToken(ctx, data.UserID, data.AdditionalData, tokenmanager.RefreshTokenType)
//
//	    return newAccess, newRefresh, nil
//	}
//
// # Custom Storage Backend
//
// Implement the [ITokenStore] interface to use a custom storage backend:
//
//	type RedisTokenStore struct {
//	    client *redis.Client
//	}
//
//	func (r *RedisTokenStore) Get(ctx context.Context, key []byte) ([]byte, error) {
//	    return r.client.Get(ctx, string(key)).Bytes()
//	}
//
//	func (r *RedisTokenStore) Set(ctx context.Context, key, value []byte, exp time.Duration) error {
//	    return r.client.Set(ctx, string(key), value, exp).Err()
//	}
//
//	func (r *RedisTokenStore) Delete(ctx context.Context, key []byte) error {
//	    return r.client.Del(ctx, string(key)).Err()
//	}
//
//	// ... implement remaining interface methods
//
//	// Use with manager
//	redisStore := &RedisTokenStore{client: redisClient}
//	manager := tokenmanager.New[UserClaims](redisStore, secret, duration)
//
// # Security Considerations
//
// The package implements several security best practices:
//
// Cryptographic Security:
//   - Uses crypto/rand for generating random payloads (cryptographically secure)
//   - HMAC-SHA256 for token signing (industry standard)
//   - Constant-time signature comparison via hmac.Equal (prevents timing attacks)
//   - 512-bit entropy in token payloads (impossible to brute-force)
//
// Token Validation:
//   - Fail-secure design: returns false for ANY validation failure
//   - Token type validation prevents type confusion attacks
//   - Automatic expiration checking
//   - Storage-backed validation (revoked tokens fail immediately)
//
// Recommendations:
//   - Use a strong, randomly generated secret key (at least 32 bytes)
//   - Store the secret key securely (environment variable, secrets manager)
//   - Use short expiration times for access tokens (5-15 minutes)
//   - Implement token refresh for long sessions
//   - Use HTTPS to prevent token interception
//   - Consider implementing rate limiting for token creation
//
// # Thread Safety
//
// All operations in [Manager] are thread-safe. The built-in [MemoryTokenStore]
// uses sync.RWMutex for safe concurrent access. Custom storage implementations
// should also ensure thread safety.
//
// # Memory Management
//
// The [MemoryTokenStore] includes an automatic cleanup goroutine that runs every
// minute to remove expired tokens. This prevents memory leaks from accumulated
// expired tokens.
//
// For production environments with high token volumes, consider using a
// dedicated storage backend like Redis, which handles expiration natively
// and provides persistence.
//
// # Error Handling
//
// The package defines the following sentinel errors:
//
//   - [ErrKeyNotFound]: Returned when a key does not exist in storage
//   - [ErrKeyExpired]: Returned when a key has expired
//
// The [ValidateToken] method does not return errors; instead, it returns
// a boolean indicating validity. This fail-secure design ensures that any
// unexpected condition results in token rejection.
//
// # Data Storage Format
//
// Token data is stored as JSON with the following structure:
//
//	{
//	    "user_id": "user-123",
//	    "issued_at": "2024-01-15T10:30:00Z",
//	    "expiry": "2024-01-15T10:45:00Z",
//	    "token_type": "access_token",
//	    "additional_data": { ... }
//	}
//
// Storage keys use the prefix "tokenmanager:" followed by the token payload:
//
//	tokenmanager:{payload_hex}
//
// # Performance
//
// The package is designed for high performance:
//   - O(1) token validation (single storage lookup after signature check)
//   - Minimal allocations during token operations
//   - Read-write mutex in MemoryTokenStore optimizes concurrent reads
//
// For benchmarks, run:
//
//	go test -bench=. -benchmem
//
// # Installation
//
//	go get github.com/sxwebdev/tokenmanager@latest
//
// # Requirements
//
//   - Go 1.25.1 or later (for generics support)
//   - No external dependencies (standard library only)
package tokenmanager
