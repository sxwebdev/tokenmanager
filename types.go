package tokenmanager

import (
	"context"
	"time"
)

// ITokenStore is the interface for an abstract token storage.
// In a real system, this could be Redis, Memcached, etc.
type ITokenStore interface {
	Get(ctx context.Context, key []byte) ([]byte, error)
	Set(ctx context.Context, key []byte, value []byte, expiration time.Duration) error
	Delete(ctx context.Context, key []byte) error
	Keys(ctx context.Context, prefix []byte) ([]string, error)
	KeysAndValues(ctx context.Context, prefix []byte) (map[string][]byte, error)
	GetFromJSON(ctx context.Context, key []byte, dst any) error
	SetJSON(ctx context.Context, key []byte, value any, expiration time.Duration) error
	Exists(ctx context.Context, key []byte) (bool, error)
}

// TokenType represents the type of token.
type TokenType string

func (t TokenType) String() string { return string(t) }
func (t TokenType) IsValid() bool {
	switch t {
	case AccessTokenType, RefreshTokenType:
		return true
	}
	return false
}

const (
	AccessTokenType  TokenType = "access_token"
	RefreshTokenType TokenType = "refresh_token"
)
