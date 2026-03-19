package tokenmanager_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/sxwebdev/tokenmanager"
)

func TestSetAndGet(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	key := []byte("testKey")
	value := []byte("testValue")
	duration := 2 * time.Second

	if err := store.Set(ctx, key, value, duration); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retVal, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(retVal) != string(value) {
		t.Fatalf("expected value %q, got %q", value, retVal)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	key := []byte("deleteKey")
	value := []byte("valueToDelete")
	duration := 5 * time.Second

	if err := store.Set(ctx, key, value, duration); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Удаляем ключ
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// После удаления ключ должен возвращать ErrKeyNotFound
	_, err := store.Get(ctx, key)
	if !errors.Is(err, tokenmanager.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound after deletion, got: %v", err)
	}
}

func TestExpiration(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	key := []byte("expireKey")
	value := []byte("valueToExpire")
	duration := 100 * time.Millisecond

	if err := store.Set(ctx, key, value, duration); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Ждем, пока ключ истечет
	time.Sleep(150 * time.Millisecond)
	_, err := store.Get(ctx, key)
	if !errors.Is(err, tokenmanager.ErrKeyExpired) {
		t.Fatalf("expected ErrKeyExpired, got: %v", err)
	}
}

func TestKeys(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	// Добавляем ключи с префиксом "prefix"
	if err := store.Set(ctx, []byte("prefix1"), []byte("value1"), 5*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Set(ctx, []byte("prefix2"), []byte("value2"), 5*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Set(ctx, []byte("other"), []byte("value3"), 5*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	keys, err := store.Keys(ctx, []byte("prefix"))
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}

	expected := map[string]bool{"prefix1": true, "prefix2": true}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
	}
	for _, k := range keys {
		if !expected[k] {
			t.Fatalf("unexpected key: %s", k)
		}
	}
}

func TestKeysAndValues(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	if err := store.Set(ctx, []byte("test1"), []byte("val1"), 5*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Set(ctx, []byte("test2"), []byte("val2"), 5*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Set(ctx, []byte("other"), []byte("val3"), 5*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	result, err := store.KeysAndValues(ctx, []byte("test"))
	if err != nil {
		t.Fatalf("KeysAndValues failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}

	if string(result["test1"]) != "val1" {
		t.Fatalf("expected value 'val1' for key 'test1', got %q", result["test1"])
	}
	if string(result["test2"]) != "val2" {
		t.Fatalf("expected value 'val2' for key 'test2', got %q", result["test2"])
	}
}

// TestSetJSONAndGetFromJSON проверяет методы SetJSON и GetFromJSON,
// обеспечивая корректное сохранение и извлечение структур, маршалленных в JSON.
func TestSetJSONAndGetFromJSON(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	// Определяем тестовую структуру
	type sample struct {
		Field string `json:"field"`
	}

	key := []byte("jsonKey")
	original := sample{Field: "testValue"}
	expiration := 5 * time.Second

	// Сохраняем значение в виде JSON
	if err := store.SetJSON(ctx, key, original, expiration); err != nil {
		t.Fatalf("SetJSON failed: %v", err)
	}

	// Извлекаем значение и анмаршалим в структуру
	var retrieved sample
	if err := store.GetFromJSON(ctx, key, &retrieved); err != nil {
		t.Fatalf("GetFromJSON failed: %v", err)
	}

	if retrieved.Field != original.Field {
		t.Fatalf("expected field %q, got %q", original.Field, retrieved.Field)
	}
}

// TestExists проверяет корректную работу метода Exists: для несуществующего ключа,
// для существующего, а также для истекшего.
func TestExists(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	// Проверяем несуществующий ключ
	exists, err := store.Exists(ctx, []byte("nonexistent"))
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Fatalf("expected key 'nonexistent' to not exist")
	}

	// Устанавливаем ключ с коротким временем жизни
	key := []byte("existKey")
	value := []byte("some value")
	expiration := 200 * time.Millisecond

	if err := store.Set(ctx, key, value, expiration); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Проверяем, что ключ существует сразу после установки
	exists, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected key to exist")
	}

	// Ждем истечения срока действия ключа
	time.Sleep(250 * time.Millisecond)
	exists, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Fatalf("expected key to be expired and not exist")
	}
}

func TestKeysFiltersExpired(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	// Добавляем ключ с коротким TTL и ключ с длинным TTL
	if err := store.Set(ctx, []byte("prefix_short"), []byte("val1"), 100*time.Millisecond); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Set(ctx, []byte("prefix_long"), []byte("val2"), 5*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Оба ключа должны быть видны
	keys, err := store.Keys(ctx, []byte("prefix_"))
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys before expiration, got %d", len(keys))
	}

	// Ждём истечения короткого ключа
	time.Sleep(150 * time.Millisecond)

	keys, err = store.Keys(ctx, []byte("prefix_"))
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key after expiration, got %d", len(keys))
	}
	if keys[0] != "prefix_long" {
		t.Fatalf("expected key 'prefix_long', got %q", keys[0])
	}
}

func TestClose(t *testing.T) {
	store := tokenmanager.NewMemoryTokenStore()

	// Close должен завершиться без паники
	store.Close()

	// Store продолжает работать для чтения/записи после Close (cleanup просто останавливается)
	ctx := context.Background()
	if err := store.Set(ctx, []byte("key"), []byte("val"), 5*time.Second); err != nil {
		t.Fatalf("Set after Close failed: %v", err)
	}
	val, err := store.Get(ctx, []byte("key"))
	if err != nil {
		t.Fatalf("Get after Close failed: %v", err)
	}
	if string(val) != "val" {
		t.Fatalf("expected 'val', got %q", val)
	}
}

func TestNewMemoryTokenStoreWithCustomInterval(t *testing.T) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore(100 * time.Millisecond)
	defer store.Close()

	// Устанавливаем ключ с коротким TTL
	if err := store.Set(ctx, []byte("fast_key"), []byte("val"), 50*time.Millisecond); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Ждём, пока cleanup сработает (>100ms interval + >50ms TTL)
	time.Sleep(200 * time.Millisecond)

	// Ключ должен быть удалён cleanup-горутиной
	exists, err := store.Exists(ctx, []byte("fast_key"))
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Fatal("expected key to be cleaned up by custom interval cleanup")
	}
}

func BenchmarkSet(b *testing.B) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()

	// Остановка таймера до начала цикла
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte("benchKey" + strconv.Itoa(i))
		value := []byte("value")
		if err := store.Set(ctx, key, value, 10*time.Minute); err != nil {
			b.Fatalf("Set failed: %v", err)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	ctx := context.Background()
	store := tokenmanager.NewMemoryTokenStore()
	defer store.Close()
	numKeys := 1000
	keys := make([][]byte, numKeys)

	// Предварительное заполнение хранилища
	for i := 0; i < numKeys; i++ {
		key := []byte("benchKey" + strconv.Itoa(i))
		value := []byte("value")
		if err := store.Set(ctx, key, value, 10*time.Minute); err != nil {
			b.Fatalf("Set failed: %v", err)
		}
		keys[i] = key
	}

	b.ResetTimer()
	// Чтение ключей циклически
	for i := 0; i < b.N; i++ {
		key := keys[i%numKeys]
		if _, err := store.Get(ctx, key); err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}
