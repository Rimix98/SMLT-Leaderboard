package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// ============================================
// JWT BLACKLIST INTERFACE (Теперь с Context)
// ============================================

type JWTBlacklist interface {
	Add(ctx context.Context, token string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, token string) (bool, error)
}

// Хэшируем токен перед сохранением (чтобы не хранить сырые JWT в базе)
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// ============================================
// UPSTASH REDIS IMPLEMENTATION (Для Vercel)
// ============================================

type UpstashBlacklist struct {
	url   string
	token string
}

func NewUpstashBlacklist(url, token string) *UpstashBlacklist {
	return &UpstashBlacklist{url: url, token: token}
}

func (u *UpstashBlacklist) Add(ctx context.Context, token string, expiresAt time.Time) error {
	if token == "" {
		return nil
	}
	
	ttl := int64(time.Until(expiresAt).Seconds())
	if ttl <= 0 {
		return nil // Токен уже просрочен, в базу писать нет смысла
	}

	tokenHash := hashToken(token)
	// Ключ в Redis делаем с префиксом, чтобы не путать с рейт-лимитами
	key := "blacklist:" + tokenHash

	// Используем Upstash REST API: SET key true EX seconds
	reqURL := fmt.Sprintf("%s/set/%s/true/EX/%d", u.url, key, ttl)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)

	// Юзаем httpClient, который объявлен глобально в index.go
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstash error: status %d", resp.StatusCode)
	}

	log.Printf("[blacklist] Токен успешно забанен в Redis на %d сек.", ttl)
	return nil
}

func (u *UpstashBlacklist) IsBlacklisted(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}

	tokenHash := hashToken(token)
	key := "blacklist:" + tokenHash

	reqURL := fmt.Sprintf("%s/get/%s", u.url, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("upstash error: status %d", resp.StatusCode)
	}

	var upstashResp struct {
		Result *string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&upstashResp); err != nil {
		return false, err
	}

	// Если Redis вернул "true", значит токен в черном списке
	return upstashResp.Result != nil && *upstashResp.Result == "true", nil
}

// ============================================
// IN-MEMORY IMPLEMENTATION (Только для локальной разработки)
// ============================================

type MemoryBlacklist struct {
	mu     sync.Mutex
	tokens map[string]time.Time // token_hash -> expires_at
}

func NewMemoryBlacklist() *MemoryBlacklist {
	return &MemoryBlacklist{
		tokens: make(map[string]time.Time),
	}
}

func (b *MemoryBlacklist) Add(_ context.Context, token string, expiresAt time.Time) error {
	if token == "" {
		return nil
	}
	b.mu.Lock()
	b.tokens[hashToken(token)] = expiresAt
	b.mu.Unlock()
	return nil
}

func (b *MemoryBlacklist) IsBlacklisted(_ context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	tokenHash := hashToken(token)

	b.mu.Lock() // Берем обычный Lock, так как внутри возможна синхронная очистка
	defer b.mu.Unlock()

	expiresAt, exists := b.tokens[tokenHash]
	if !exists {
		return false, nil
	}

	// Если токен просрочен, удаляем его прямо во время запроса без фоновых горутин
	if time.Now().After(expiresAt) {
		delete(b.tokens, tokenHash)
		return false, nil
	}

	return true, nil
}

// ============================================
// ИНИЦИАЛИЗАЦИЯ (Вызывать внутри глобального init() в index.go)
// ============================================

var blacklist JWTBlacklist

func InitBlacklist() {
	redisURL := os.Getenv("UPSTASH_REDIS_REST_URL")
	redisToken := os.Getenv("UPSTASH_REDIS_REST_TOKEN")

	if redisURL != "" && redisToken != "" {
		blacklist = NewUpstashBlacklist(redisURL, redisToken)
		log.Println("[blacklist] Успешно инициализирован Upstash Redis")
	} else {
		blacklist = NewMemoryBlacklist()
		log.Println("[blacklist] ВНИМАНИЕ: Переменные Redis не найдены. Откат на MemoryBlacklist (Не для продакшена!)")
	}
}