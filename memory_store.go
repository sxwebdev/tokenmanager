package tokenmanager

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrKeyNotFound = errors.New("key not found")
	ErrKeyExpired  = errors.New("key expired")
)

// MemoryTokenStore is an in‑memory implementation of ITokenStore.
// It can be replaced with a Redis-based store or another external storage.
type MemoryTokenStore struct {
	mu              sync.RWMutex
	store           map[string]memoryItem
	done            chan struct{}
	cleanupInterval time.Duration
}

var _ ITokenStore = (*MemoryTokenStore)(nil)

type memoryItem struct {
	value     []byte
	expiresAt time.Time
}

// NewMemoryTokenStore creates a new in‑memory store.
// An optional cleanup interval can be provided; defaults to 1 minute.
func NewMemoryTokenStore(cleanupInterval ...time.Duration) *MemoryTokenStore {
	interval := 1 * time.Minute
	if len(cleanupInterval) > 0 && cleanupInterval[0] > 0 {
		interval = cleanupInterval[0]
	}
	mts := &MemoryTokenStore{
		store:           make(map[string]memoryItem),
		done:            make(chan struct{}),
		cleanupInterval: interval,
	}
	go mts.cleanupLoop()
	return mts
}

// Close stops the background cleanup goroutine.
func (mts *MemoryTokenStore) Close() {
	close(mts.done)
}

// Set stores the key and value for the specified duration.
func (mts *MemoryTokenStore) Set(_ context.Context, key []byte, value []byte, duration time.Duration) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()
	mts.store[string(key)] = memoryItem{
		value:     value,
		expiresAt: time.Now().Add(duration),
	}
	return nil
}

// Get returns the value for the given key if it exists and is not expired.
func (mts *MemoryTokenStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	mts.mu.RLock()
	item, exists := mts.store[string(key)]
	mts.mu.RUnlock()
	if !exists {
		return nil, ErrKeyNotFound
	}
	if time.Now().After(item.expiresAt) {
		if err := mts.Delete(ctx, key); err != nil {
			return nil, err
		}
		return nil, ErrKeyExpired
	}
	return item.value, nil
}

// Delete removes the key from the storage.
func (mts *MemoryTokenStore) Delete(_ context.Context, key []byte) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()
	delete(mts.store, string(key))
	return nil
}

// Keys returns all non-expired keys that start with the given prefix.
func (mts *MemoryTokenStore) Keys(_ context.Context, prefix []byte) ([]string, error) {
	mts.mu.RLock()
	defer mts.mu.RUnlock()
	var keys []string
	pfx := string(prefix)
	now := time.Now()
	for k, item := range mts.store {
		if strings.HasPrefix(k, pfx) {
			if now.After(item.expiresAt) {
				continue
			}
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// KeysAndValues returns a map of all keys that start with the given prefix and their corresponding values.
// Expired items are skipped.
func (mts *MemoryTokenStore) KeysAndValues(_ context.Context, prefix []byte) (map[string][]byte, error) {
	mts.mu.RLock()
	defer mts.mu.RUnlock()
	result := make(map[string][]byte)
	pfx := string(prefix)
	now := time.Now()
	for k, item := range mts.store {
		if strings.HasPrefix(k, pfx) {
			if now.After(item.expiresAt) {
				continue
			}
			result[k] = item.value
		}
	}
	return result, nil
}

// GetFromJSON retrieves the value for the given key and unmarshals it into dst.
func (mts *MemoryTokenStore) GetFromJSON(ctx context.Context, key []byte, dst any) error {
	data, err := mts.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// SetJSON marshals the given value to JSON and stores it with the specified expiration.
func (mts *MemoryTokenStore) SetJSON(ctx context.Context, key []byte, value any, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return mts.Set(ctx, key, data, expiration)
}

// Exists checks if a key exists in the store and is not expired.
func (mts *MemoryTokenStore) Exists(ctx context.Context, key []byte) (bool, error) {
	mts.mu.RLock()
	item, exists := mts.store[string(key)]
	mts.mu.RUnlock()
	if !exists {
		return false, nil
	}
	if time.Now().After(item.expiresAt) {
		// Если ключ просрочен, удаляем его и возвращаем false.
		if err := mts.Delete(ctx, key); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// cleanupLoop periodically removes expired items from the store.
func (mts *MemoryTokenStore) cleanupLoop() {
	ticker := time.NewTicker(mts.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			mts.mu.Lock()
			for k, item := range mts.store {
				if now.After(item.expiresAt) {
					delete(mts.store, k)
				}
			}
			mts.mu.Unlock()
		case <-mts.done:
			return
		}
	}
}
