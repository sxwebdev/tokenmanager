package tokenmanager_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sxwebdev/tokenmanager"
)

const secretKey = "test_secret_key"

func TestCreateAndValidateAccessToken(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
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
	defer store.Close()
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
	defer store.Close()
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
	defer store.Close()
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
	defer store.Close()
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
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	// Try with invalid token format
	err := manager.UpdateAdditionalData(ctx, "invalid-token", map[string]any{"role": "admin"})
	if err == nil || err.Error() != "invalid token format" {
		t.Fatalf("expected 'invalid token format' error, got: %v", err)
	}

	// Try with non-existent token (valid hex payload but wrong signature)
	err = manager.UpdateAdditionalData(ctx, "abcd.efgh", map[string]any{"role": "admin"})
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
}

func TestValidateTokenWrongType(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	// Создаём access token
	token, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Валидируем как refresh — должен быть невалидным
	_, valid := manager.ValidateToken(ctx, token, tokenmanager.RefreshTokenType)
	if valid {
		t.Fatal("access token should not be valid as refresh token")
	}

	// Валидируем как access — должен быть валидным
	_, valid = manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if !valid {
		t.Fatal("access token should be valid as access token")
	}
}

func TestEmptySecretKey(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, "", 5*time.Minute)

	token, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token with empty secret: %v", err)
	}

	td, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if !valid {
		t.Fatal("token with empty secret should still be valid")
	}
	if td.UserID != "user123" {
		t.Fatalf("expected userID 'user123', got %s", td.UserID)
	}
}

func TestUpdateAdditionalDataExpiredToken(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 1*time.Second)

	token, _, err := manager.CreateToken(ctx, "user123", map[string]any{"role": "user"}, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Ждём истечения токена
	time.Sleep(2 * time.Second)

	err = manager.UpdateAdditionalData(ctx, token, map[string]any{"role": "admin"})
	if err == nil {
		t.Fatal("expected error when updating expired token, got nil")
	}
}

func TestRevokeTokenInvalidSignature(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	token, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Подделываем подпись
	parts := strings.Split(token, ".")
	tamperedToken := parts[0] + ".0000000000000000000000000000000000000000000000000000000000000000"

	err = manager.RevokeToken(ctx, tamperedToken)
	if err == nil {
		t.Fatal("expected error when revoking token with invalid signature")
	}
	if err.Error() != "invalid token signature" {
		t.Fatalf("expected 'invalid token signature' error, got: %v", err)
	}

	// Оригинальный токен должен по-прежнему быть валидным
	_, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if !valid {
		t.Fatal("original token should still be valid after failed revoke")
	}
}

func TestUpdateAdditionalDataInvalidSignature(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	token, _, err := manager.CreateToken(ctx, "user123", map[string]any{"role": "user"}, tokenmanager.AccessTokenType)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Подделываем подпись
	parts := strings.Split(token, ".")
	tamperedToken := parts[0] + ".0000000000000000000000000000000000000000000000000000000000000000"

	err = manager.UpdateAdditionalData(ctx, tamperedToken, map[string]any{"role": "admin"})
	if err == nil {
		t.Fatal("expected error when updating token with invalid signature")
	}
	if err.Error() != "invalid token signature" {
		t.Fatalf("expected 'invalid token signature' error, got: %v", err)
	}

	// Данные оригинального токена должны остаться прежними
	td, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
	if !valid {
		t.Fatal("original token should still be valid")
	}
	if role, ok := td.AdditionalData["role"].(string); !ok || role != "user" {
		t.Fatalf("expected role 'user', got %v", td.AdditionalData["role"])
	}
}

func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()

	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	tokens := make([]string, goroutines)
	errs := make([]error, goroutines)

	// Параллельное создание токенов
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			token, _, err := manager.CreateToken(ctx, "user"+strings.Repeat("0", idx), nil, tokenmanager.AccessTokenType)
			tokens[idx] = token
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: CreateToken failed: %v", i, err)
		}
	}

	// Параллельная валидация
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, valid := manager.ValidateToken(ctx, tokens[idx], tokenmanager.AccessTokenType)
			if !valid {
				t.Errorf("goroutine %d: token should be valid", idx)
			}
		}(i)
	}
	wg.Wait()

	// Параллельный revoke
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			if err := manager.RevokeToken(ctx, tokens[idx]); err != nil {
				t.Errorf("goroutine %d: RevokeToken failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	// Все токены должны быть отозваны
	for i := 0; i < goroutines; i++ {
		_, valid := manager.ValidateToken(ctx, tokens[i], tokenmanager.AccessTokenType)
		if valid {
			t.Fatalf("goroutine %d: token should be revoked", i)
		}
	}
}

func BenchmarkCreateToken(b *testing.B) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
		if err != nil {
			b.Fatalf("CreateToken failed: %v", err)
		}
	}
}

func BenchmarkValidateToken(b *testing.B) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	manager := tokenmanager.New[map[string]any](store, secretKey, 5*time.Minute)

	token, _, err := manager.CreateToken(ctx, "user123", nil, tokenmanager.AccessTokenType)
	if err != nil {
		b.Fatalf("CreateToken failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
		if !valid {
			b.Fatal("token should be valid")
		}
	}
}
