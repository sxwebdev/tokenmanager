# Authentication Patterns with TokenManager

## Table of Contents

- [Access and Refresh Token Flow](#access-and-refresh-token-flow)
- [HTTP Middleware](#http-middleware)
- [Token Refresh Endpoint](#token-refresh-endpoint)
- [Token Rotation](#token-rotation)
- [Logout and Revocation](#logout-and-revocation)
- [Revoking All User Tokens](#revoking-all-user-tokens)

## Access and Refresh Token Flow

Use two separate `Manager` instances with different durations. This separation ensures a refresh token cannot be used as an access token (type confusion prevention).

```go
type Claims struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

type AuthService struct {
	accessMgr  *tokenmanager.Manager[Claims]
	refreshMgr *tokenmanager.Manager[Claims]
}

func NewAuthService(store tokenmanager.ITokenStore, secret string) *AuthService {
	return &AuthService{
		accessMgr:  tokenmanager.New[Claims](store, secret, 15*time.Minute),
		refreshMgr: tokenmanager.New[Claims](store, secret, 7*24*time.Hour),
	}
}

// Login creates both tokens.
func (s *AuthService) Login(ctx context.Context, userID string, claims Claims) (accessToken, refreshToken string, err error) {
	accessToken, _, err = s.accessMgr.CreateToken(ctx, userID, claims, tokenmanager.AccessTokenType)
	if err != nil {
		return "", "", fmt.Errorf("create access token: %w", err)
	}

	refreshToken, _, err = s.refreshMgr.CreateToken(ctx, userID, claims, tokenmanager.RefreshTokenType)
	if err != nil {
		// Clean up the access token if refresh creation fails.
		_ = s.accessMgr.RevokeToken(ctx, accessToken)
		return "", "", fmt.Errorf("create refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}
```

## HTTP Middleware

Extract the token from the `Authorization` header and inject validated claims into the request context.

```go
type contextKey string

const userClaimsKey contextKey = "user_claims"

func AuthMiddleware(accessMgr *tokenmanager.Manager[Claims]) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, "missing authorization", http.StatusUnauthorized)
				return
			}

			data, valid := accessMgr.ValidateToken(r.Context(), token, tokenmanager.AccessTokenType)
			if !valid {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			// Inject claims into context for downstream handlers.
			ctx := context.WithValue(r.Context(), userClaimsKey, data)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// GetClaims retrieves validated claims from the request context.
func GetClaims(ctx context.Context) *tokenmanager.Data[Claims] {
	data, _ := ctx.Value(userClaimsKey).(*tokenmanager.Data[Claims])
	return data
}
```

**Usage with a router (e.g., chi):**

```go
r := chi.NewRouter()
r.Use(AuthMiddleware(accessMgr))
r.Get("/profile", profileHandler)
```

**Usage with net/http:**

```go
mux := http.NewServeMux()
mux.Handle("/profile", AuthMiddleware(accessMgr)(http.HandlerFunc(profileHandler)))
```

## Token Refresh Endpoint

Accept a refresh token, validate it, revoke it, and issue a new token pair. Always revoke the old refresh token to implement rotation.

```go
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (newAccess, newRefresh string, err error) {
	// Validate the refresh token (note: RefreshTokenType, not AccessTokenType).
	data, valid := s.refreshMgr.ValidateToken(ctx, refreshToken, tokenmanager.RefreshTokenType)
	if !valid {
		return "", "", errors.New("invalid refresh token")
	}

	// Revoke old refresh token (rotation — prevents reuse).
	if err := s.refreshMgr.RevokeToken(ctx, refreshToken); err != nil {
		return "", "", fmt.Errorf("revoke old refresh token: %w", err)
	}

	// Issue new token pair with the same claims.
	return s.Login(ctx, data.UserID, data.AdditionalData)
}
```

## Token Rotation

Token rotation means issuing a new refresh token on every refresh and revoking the old one. This limits the window of opportunity if a refresh token is compromised:

1. Attacker steals refresh token R1
2. Legitimate user refreshes with R1 → gets R2 (R1 revoked)
3. Attacker tries to use R1 → fails (revoked)

If the attacker refreshes first:

1. Attacker refreshes with R1 → gets R2 (R1 revoked)
2. Legitimate user tries to refresh with R1 → fails
3. User must re-authenticate, which signals a potential compromise

**Detect reuse for extra security:**

```go
func (s *AuthService) RefreshWithReuseDectection(ctx context.Context, refreshToken string) (string, string, error) {
	data, valid := s.refreshMgr.ValidateToken(ctx, refreshToken, tokenmanager.RefreshTokenType)
	if !valid {
		// Token invalid — could be a reuse attempt.
		// Optionally: revoke ALL tokens for this user as a precaution.
		// s.RevokeAllUserTokens(ctx, extractUserIDFromToken(refreshToken))
		return "", "", errors.New("invalid refresh token — possible reuse detected")
	}

	if err := s.refreshMgr.RevokeToken(ctx, refreshToken); err != nil {
		return "", "", err
	}

	return s.Login(ctx, data.UserID, data.AdditionalData)
}
```

## Logout and Revocation

Revoke both access and refresh tokens on logout:

```go
func (s *AuthService) Logout(ctx context.Context, accessToken, refreshToken string) error {
	// Revoke both tokens. Errors are non-fatal — the tokens will expire naturally.
	accessErr := s.accessMgr.RevokeToken(ctx, accessToken)
	refreshErr := s.refreshMgr.RevokeToken(ctx, refreshToken)

	return errors.Join(accessErr, refreshErr)
}
```

**Client-side:** Delete stored tokens from cookies/localStorage regardless of server response.

## Revoking All User Tokens

Use the `Keys` method on the store to find and delete all tokens for a user. This is useful for "log out everywhere" or when a user changes their password:

```go
func (s *AuthService) RevokeAllUserTokens(ctx context.Context, store tokenmanager.ITokenStore, userID string) error {
	// Get all token keys.
	kvs, err := store.KeysAndValues(ctx, "tokenmanager:")
	if err != nil {
		return fmt.Errorf("list tokens: %w", err)
	}

	for key, value := range kvs {
		// Unmarshal to check the user ID.
		var data struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(value, &data); err != nil {
			continue
		}
		if data.UserID == userID {
			if err := store.Delete(ctx, key); err != nil {
				return fmt.Errorf("delete token %s: %w", key, err)
			}
		}
	}

	return nil
}
```

**Performance note:** This scans all tokens. For high-volume systems, maintain a secondary index (e.g., `user_tokens:{userID}` → set of token keys) to make per-user revocation O(1) per token instead of O(n) total tokens.
