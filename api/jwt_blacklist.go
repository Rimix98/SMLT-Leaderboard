package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

// ============================================
// JWT BLACKLIST INTERFACE
// ============================================

type JWTBlacklist interface {
	// Добавить токен в черный список
	Add(token string, expiresAt time.Time) error
	// Проверить, не в черном ли списке
	IsBlacklisted(token string) bool
	// Очистить просроченные токены
	Cleanup() error
}

// ============================================
// IN-MEMORY IMPLEMENTATION (для разработки)
// ============================================

type MemoryBlacklist struct {
	mu      sync.RWMutex
	tokens  map[string]time.Time // token_hash -> expires_at
	cleaner *time.Ticker
	done    chan bool
}

func NewMemoryBlacklist() *MemoryBlacklist {
	bl := &MemoryBlacklist{
		tokens:  make(map[string]time.Time),
		cleaner: time.NewTicker(1 * time.Hour), // Чистим каждый час
		done:    make(chan bool),
	}
	go bl.cleanupLoop()
	return bl
}

// Хэшируем токен перед сохранением (безопасность)
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (b *MemoryBlacklist) Add(token string, expiresAt time.Time) error {
	if token == "" {
		return nil
	}

	tokenHash := hashToken(token)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.tokens[tokenHash] = expiresAt
	log.Printf("[blacklist] Token added, expires at %v", expiresAt)
	return nil
}

func (b *MemoryBlacklist) IsBlacklisted(token string) bool {
	if token == "" {
		return false
	}

	tokenHash := hashToken(token)

	b.mu.RLock()
	expiresAt, exists := b.tokens[tokenHash]
	b.mu.RUnlock()

	if !exists {
		return false
	}

	// Если токен просрочен, удаляем из blacklist (он уже невалидный по времени)
	if time.Now().After(expiresAt) {
		go b.Remove(tokenHash) // Асинхронно удаляем
		return false
	}

	return true
}

func (b *MemoryBlacklist) Remove(tokenHash string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.tokens, tokenHash)
}

func (b *MemoryBlacklist) Cleanup() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	for hash, expiresAt := range b.tokens {
		if now.After(expiresAt) {
			delete(b.tokens, hash)
		}
	}
	return nil
}

func (b *MemoryBlacklist) cleanupLoop() {
	for {
		select {
		case <-b.cleaner.C:
			if err := b.Cleanup(); err != nil {
				log.Printf("[blacklist] Cleanup error: %v", err)
			}
		case <-b.done:
			return
		}
	}
}

func (b *MemoryBlacklist) Stop() {
	b.cleaner.Stop()
	close(b.done)
}

// ============================================
// REDIS IMPLEMENTATION
// ============================================

type RedisBlacklist struct {
	client interface{} // *redis.Client или другой клиент
	// Добавь реальную реализацию если используешь Redis
}

func NewRedisBlacklist(client interface{}) *RedisBlacklist {
	return &RedisBlacklist{client: client}
}

// Глобальный экземпляр blacklist
var blacklist JWTBlacklist

// Инициализация blacklist (вызвать в init())
func initBlacklist() {
	// Для начала используем memory версию
	blacklist = NewMemoryBlacklist()
	log.Println("[blacklist] Initialized with memory storage")

	// Если есть Redis - можно использовать его
	// redisClient := redis.NewClient(&redis.Options{...})
	// blacklist = NewRedisBlacklist(redisClient)
}
