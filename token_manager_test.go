package tokenmanager_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sxwebdev/tokenmanager"
)

const secretKey = "test_secret_key"

func TestCreateAndValidateAccessToken(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	userID := "user123"
	token, _, err := manager.CreateToken(ctx, userID, nil, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	td, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if !valid {
		t.Fatal("token should be valid")
	}
	if td.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, td.UserID)
	}
}

func TestTokenExpiration(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	manager := tokenmanager.New[map[string]any](store, secretKey, 1*time.Second)

	token, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	time.Sleep(2 * time.Second)
	_, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if valid {
		t.Fatal("token should be expired")
	}
}

func TestRevokeToken(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	token, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	if _, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType); !valid {
		t.Fatal("token should be valid")
	}
	err = manager.RevokeToken(ctx, token)
	if err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}
	if _, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType); valid {
		t.Fatal("token should be revoked")
	}
}

func TestInvalidSignature(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	token, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	// Изменяем последний символ подписи, чтобы токен стал недействительным.
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatal("invalid token format")
	}
	signature := parts[1]
	if len(signature) == 0 {
		t.Fatal("empty signature")
	}
	var newChar byte
	if signature[len(signature)-1] == 'a' {
		newChar = 'b'
	} else {
		newChar = 'a'
	}
	tamperedSig := signature[:len(signature)-1] + string(newChar)
	tamperedToken := parts[0] + "." + tamperedSig

	if _, valid := manager.ValidateToken(ctx, tamperedToken, tokenmanager.AccessTokenType); valid {
		t.Fatal("tampered token should be invalid")
	}
}

func TestUpdateAdditionalData(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	// Initial data
	userID := "user123"
	initialData := map[string]any{"role": "user"}
	token, _, err := manager.CreateToken(ctx, userID, initialData, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Verify initial data
	td, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if !valid {
		t.Fatal("token should be valid")
	}
	if role, ok := td.AdditionalData["role"].(string); !ok || role != "user" {
		t.Fatalf("expected role 'user', got %v", td.AdditionalData["role"])
	}

	// Update the additional data
	newData := map[string]any{"role": "admin", "permissions": []string{"read", "write"}}
	err = manager.UpdateAdditionalData(ctx, token, newData)
	if err != nil {
		t.Fatalf("failed to update additional data: %v", err)
	}

	// Verify updated data
	td, valid = manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if !valid {
		t.Fatal("token should be valid after update")
	}
	if role, ok := td.AdditionalData["role"].(string); !ok || role != "admin" {
		t.Fatalf("expected updated role 'admin', got %v", td.AdditionalData["role"])
	}
	if permissions, ok := td.AdditionalData["permissions"].([]interface{}); !ok || len(permissions) != 2 {
		t.Fatalf("expected permissions array with 2 elements, got %v", td.AdditionalData["permissions"])
	}
}

func TestUpdateAdditionalDataWithInvalidToken(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	// Try with invalid token format
	err := manager.UpdateAdditionalData(ctx, "invalid-token", map[string]any{"role": "admin"})
	if err == nil || err.Error() != "invalid token format" {
		t.Fatalf("expected 'invalid token format' error, got: %v", err)
	}

	// Try with non-existent token
	err = manager.UpdateAdditionalData(ctx, "payload.signature", map[string]any{"role": "admin"})
	if err == nil {
		t.Fatal("expected error for non-existent token, got nil")
	}
}
