package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	rateLimitWindow     = time.Minute
	rateLimitDefaultMax = 60
	rateLimitLoginMax   = 10
)

// rateLimiter — единый контракт; в serverless нужен внешний store (Upstash), не RAM.
type rateLimiter interface {
	allow(ctx context.Context, key string, max int, window time.Duration) (bool, error)
}

var (
	globalRateLimiter rateLimiter
	rlOnce            sync.Once
)

func initRateLimiter() {
	rlOnce.Do(func() {
		if u := newUpstashLimiter(); u != nil {
			globalRateLimiter = u
			log.Println("[ratelimit] Upstash Redis (распределённый лимит для serverless)")
			return
		}
		globalRateLimiter = newMemoryLimiter()
		log.Println("[ratelimit] UPSTASH_REDIS_REST_URL/TOKEN не заданы — in-memory лимит только внутри одного контейнера (не защита на Vercel)")
	})
}

// --- Upstash (REST), работает между инстансами Vercel ---

type upstashLimiter struct {
	restURL string
	token   string
}

func newUpstashLimiter() rateLimiter {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("UPSTASH_REDIS_REST_URL")), "/")
	token := strings.TrimSpace(os.Getenv("UPSTASH_REDIS_REST_TOKEN"))
	if base == "" || token == "" {
		return nil
	}
	return &upstashLimiter{restURL: base, token: token}
}

func (u *upstashLimiter) allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	bucket := time.Now().Unix() / int64(window.Seconds())
	redisKey := fmt.Sprintf("rl:%s:%d", hashRateKey(key), bucket)
	sec := int(window.Seconds())

	payload := fmt.Sprintf(
		`[["INCR","%s"],["EXPIRE","%s",%d]]`,
		escapeRedisKey(redisKey),
		escapeRedisKey(redisKey),
		sec,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.restURL+"/pipeline", strings.NewReader(payload))
	if err != nil {
		return true, err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return true, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return true, fmt.Errorf("upstash status %d: %s", resp.StatusCode, string(body))
	}

	var results []struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		return true, fmt.Errorf("upstash decode: %w", err)
	}

	var count int64
	if err := json.Unmarshal(results[0].Result, &count); err != nil {
		return true, err
	}
	return count <= int64(max), nil
}

func escapeRedisKey(k string) string {
	return strings.ReplaceAll(k, `"`, `\"`)
}

func hashRateKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// --- In-memory: только best-effort в рамках одного warm-контейнера ---

type memoryLimiter struct {
	mu   sync.Mutex
	keys map[string]*memBucket
}

type memBucket struct {
	count   int
	resetAt time.Time
}

func newMemoryLimiter() rateLimiter {
	return &memoryLimiter{keys: make(map[string]*memBucket)}
}

func (m *memoryLimiter) allow(_ context.Context, key string, max int, window time.Duration) (bool, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.keys[key]
	if !ok || now.After(b.resetAt) {
		m.keys[key] = &memBucket{count: 1, resetAt: now.Add(window)}
		return true, nil
	}
	if b.count >= max {
		return false, nil
	}
	b.count++
	return true, nil
}

func checkRateLimit(w http.ResponseWriter, r *http.Request, max int) bool {
	initRateLimiter()
	key := requestPath(r) + "|" + getRealIP(r)
	ok, err := globalRateLimiter.allow(r.Context(), key, max, rateLimitWindow)
	if err != nil {
		log.Printf("[ratelimit] %v", err)
		return true
	}
	if !ok {
		sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
		return false
	}
	return true
}

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkRateLimit(w, r, rateLimitDefaultMax) {
			return
		}
		next(w, r)
	}
}

func rateLimitLoginMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkRateLimit(w, r, rateLimitLoginMax) {
			return
		}
		next(w, r)
	}
}
