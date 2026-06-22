package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

func cacheGet(key string) ([]byte, bool) {
	apiCache.mu.RLock()
	defer apiCache.mu.RUnlock()
	e, ok := apiCache.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

func cacheSet(key string, data []byte, ttl time.Duration) {
	apiCache.mu.Lock()
	defer apiCache.mu.Unlock()
	h := sha256.Sum256(data)
	apiCache.entries[key] = &cacheEntry{
		data:      data,
		hash:      hex.EncodeToString(h[:]),
		expiresAt: time.Now().Add(ttl),
		lastSet:   time.Now(),
	}
}

func cacheGetWithMeta(key string) ([]byte, string, time.Time, bool) {
	apiCache.mu.RLock()
	defer apiCache.mu.RUnlock()
	e, ok := apiCache.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, "", time.Time{}, false
	}
	return e.data, e.hash, e.lastSet, true
}

func cacheInvalidate(prefix string) {
	apiCache.mu.Lock()
	defer apiCache.mu.Unlock()
	for k := range apiCache.entries {
		if strings.HasPrefix(k, prefix) {
			delete(apiCache.entries, k)
		}
	}
}
