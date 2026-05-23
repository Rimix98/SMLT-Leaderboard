package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type rateLimiter interface {
	allow(ctx context.Context, key string, max int, window time.Duration) (bool, error)
}

type memoryLimiter struct {
	mu     sync.Mutex
	keys   map[string]*memoryBucket
	stopCh chan struct{}
}

func newMemoryLimiter() *memoryLimiter {
	m := &memoryLimiter{
		keys:   make(map[string]*memoryBucket),
		stopCh: make(chan struct{}),
	}
	go m.cleanup()
	return m
}

func (m *memoryLimiter) stop() {
	close(m.stopCh)
}

func (m *memoryLimiter) allow(_ context.Context, key string, max int, window time.Duration) (bool, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.keys[key]
	if !ok || now.After(b.resetAt) {
		m.keys[key] = &memoryBucket{count: 1, resetAt: now.Add(window)}
		return true, nil
	}
	if b.count >= max {
		return false, nil
	}
	b.count++
	return true, nil
}

func (m *memoryLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			m.mu.Lock()
			for k, b := range m.keys {
				if now.After(b.resetAt) {
					delete(m.keys, k)
				}
			}
			m.mu.Unlock()
		case <-m.stopCh:
			return
		}
	}
}

type upstashLimiter struct {
	url   string
	token string
	http  *http.Client
}

func newUpstashLimiter() *upstashLimiter {
	redisURL := os.Getenv("UPSTASH_REDIS_REST_URL")
	redisToken := os.Getenv("UPSTASH_REDIS_REST_TOKEN")
	if redisURL == "" || redisToken == "" {
		log.Println("[ratelimit] Upstash Redis not configured, falling back to memory limiter")
		return nil
	}
	return &upstashLimiter{
		url:   strings.TrimRight(redisURL, "/"),
		token: redisToken,
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (u *upstashLimiter) allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	windowSeconds := int(window.Seconds())
	if windowSeconds < 1 {
		windowSeconds = 1
	}

	ttl, count, err := u.getOrCreate(ctx, key, max, windowSeconds)
	if err != nil {
		return false, err
	}
	if ttl < 0 {
		return true, nil
	}
	if count > max {
		return false, nil
	}
	return true, nil
}

func (u *upstashLimiter) getOrCreate(ctx context.Context, key string, max int, windowSec int) (ttl int, count int, err error) {
	cmd := url.PathEscape(fmt.Sprintf("SET %s 1 EX %d NX", key, windowSec))
	reqURL := fmt.Sprintf("%s/%s", u.url, cmd)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, 0, err
	}
	req.SetBasicAuth("default", u.token)

	resp, err := u.http.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	var result struct {
		Result *string `json:"result"`
		Error  *string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, 0, fmt.Errorf("upstash parse: %w", err)
	}
	if result.Error != nil && *result.Error != "" {
		if *result.Error == "ERR no such key" {
			return -1, 0, nil
		}
		return 0, 0, fmt.Errorf("upstash: %s", *result.Error)
	}
	if result.Result != nil && *result.Result == "OK" {
		return -1, 1, nil
	}

	count, err = u.incrAndTTL(ctx, key)
	if err != nil {
		return 0, 0, err
	}
	return 0, count, nil
}

func (u *upstashLimiter) incrAndTTL(ctx context.Context, key string) (int, error) {
	cmd := url.PathEscape(fmt.Sprintf("INCR %s", key))
	reqURL := fmt.Sprintf("%s/%s", u.url, cmd)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth("default", u.token)

	resp, err := u.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	var result struct {
		Result json.Number `json:"result"`
		Error  *string     `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("upstash incr parse: %w", err)
	}
	if result.Error != nil && *result.Error != "" {
		return 0, fmt.Errorf("upstash incr: %s", *result.Error)
	}

	count, err := strconv.Atoi(result.Result.String())
	if err != nil {
		ttlCount, ttlErr := u.fixTTLKey(ctx, key)
		if ttlErr != nil {
			return 0, fmt.Errorf("upstash count parse: %w; fixTTL: %v", err, ttlErr)
		}
		return ttlCount, nil
	}
	return count, nil
}

func (u *upstashLimiter) fixTTLKey(ctx context.Context, key string) (int, error) {
	cmd := url.PathEscape(fmt.Sprintf("GET %s", key))
	reqURL := fmt.Sprintf("%s/%s", u.url, cmd)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth("default", u.token)
	resp, err := u.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	type getResult struct {
		Result *string `json:"result"`
	}
	var gr getResult
	if err := json.Unmarshal(body, &gr); err != nil {
		return 0, err
	}
	if gr.Result == nil {
		return 0, fmt.Errorf("key not found after incr")
	}
	return strconv.Atoi(*gr.Result)
}

var (
	globalRateLimiter rateLimiter
	rlOnce            sync.Once
	rlStop            func()
)

func initRateLimiter() {
	rlOnce.Do(func() {
		ul := newUpstashLimiter()
		if ul != nil {
			globalRateLimiter = ul
			log.Println("[ratelimit] using Upstash Redis limiter")
		} else {
			ml := newMemoryLimiter()
			globalRateLimiter = ml
			rlStop = ml.stop
			log.Println("[ratelimit] using in-memory limiter")
		}
	})
}

func checkRateLimit(w http.ResponseWriter, r *http.Request, max int) bool {
	ip := hashIP(getRealIP(r))
	key := requestPath(r) + ":" + ip
	ok, err := globalRateLimiter.allow(r.Context(), key, max, time.Minute)
	if err != nil {
		log.Printf("[ratelimit] error: %v", err)
	}
	if !ok {
		sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
		return false
	}
	return true
}

func rateLimitMiddleware(max int) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !checkRateLimit(w, r, max) {
				return
			}
			next(w, r)
		}
	}
}

func rateLimitLoginMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkLoginRateLimit(w, r) {
			return
		}
		next(w, r)
	}
}

func checkLoginRateLimit(w http.ResponseWriter, r *http.Request) bool {
	ip := hashIP(getRealIP(r))
	key := "login:" + ip
	maxLoginAttempts := 5

	if fsClient != nil {
		return checkFirestoreLoginLimit(w, r, key, maxLoginAttempts)
	}

	ok, err := globalRateLimiter.allow(r.Context(), key, maxLoginAttempts, time.Minute)
	if err != nil {
		log.Printf("[ratelimit] login error: %v", err)
	}
	if !ok {
		sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
		return false
	}
	return true
}

func checkFirestoreLoginLimit(w http.ResponseWriter, r *http.Request, key string, maxAttempts int) bool {
	ctx := r.Context()
	docRef := fsClient.Collection("rate_limits").Doc(key)
	window := 1 * time.Minute

	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return tx.Set(docRef, map[string]interface{}{
					"count":   1,
					"resetAt": time.Now().Add(window),
				})
			}
			return err
		}
		var data struct {
			Count   int       `firestore:"count"`
			ResetAt time.Time `firestore:"resetAt"`
		}
		if err := doc.DataTo(&data); err != nil {
			return tx.Set(docRef, map[string]interface{}{
				"count":   1,
				"resetAt": time.Now().Add(window),
			})
		}

		if time.Now().After(data.ResetAt) {
			return tx.Set(docRef, map[string]interface{}{
				"count":   1,
				"resetAt": time.Now().Add(window),
			})
		}

		if data.Count >= maxAttempts {
			return errRateLimitExceeded
		}

		return tx.Set(docRef, map[string]interface{}{
			"count":   data.Count + 1,
			"resetAt": data.ResetAt,
		})
	})

	if err != nil {
		if errors.Is(err, errRateLimitExceeded) {
			sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
			return false
		}
		log.Printf("[ratelimit] firestore error: %v", err)
		sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
		return false
	}
	return true
}

var errRateLimitExceeded = fmt.Errorf("rate limit exceeded")
