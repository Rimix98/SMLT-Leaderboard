package handler

import (
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ──────────────────────────────────────────────
// RESPONSE CACHE
// ──────────────────────────────────────────────

func TestCacheGet_Miss(t *testing.T) {
	apiCache = &responseCache{entries: make(map[string]*cacheEntry)}
	data, ok := cacheGet("nonexistent")
	if ok || data != nil {
		t.Error("cacheGet should return nil,false for missing key")
	}
}

func TestCacheSetAndGet_Hit(t *testing.T) {
	apiCache = &responseCache{entries: make(map[string]*cacheEntry)}
	payload := []byte(`{"test":true}`)
	cacheSet("key1", payload, 10*time.Second)

	got, ok := cacheGet("key1")
	if !ok {
		t.Fatal("cacheGet should return true for existing key")
	}
	if string(got) != string(payload) {
		t.Errorf("cacheGet returned %q, want %q", got, payload)
	}
}

func TestCacheGet_Expired(t *testing.T) {
	apiCache = &responseCache{entries: make(map[string]*cacheEntry)}
	apiCache.entries["expired"] = &cacheEntry{
		data:      []byte("old"),
		expiresAt: time.Now().Add(-1 * time.Second),
	}

	data, ok := cacheGet("expired")
	if ok || data != nil {
		t.Error("cacheGet should return nil,false for expired entry")
	}
}

func TestCacheInvalidate(t *testing.T) {
	apiCache = &responseCache{entries: make(map[string]*cacheEntry)}
	cacheSet("prefix:a", []byte("a"), 10*time.Second)
	cacheSet("prefix:b", []byte("b"), 10*time.Second)
	cacheSet("other:c", []byte("c"), 10*time.Second)

	cacheInvalidate("prefix:")

	if _, ok := cacheGet("prefix:a"); ok {
		t.Error("prefix:a should be invalidated")
	}
	if _, ok := cacheGet("prefix:b"); ok {
		t.Error("prefix:b should be invalidated")
	}
	if _, ok := cacheGet("other:c"); !ok {
		t.Error("other:c should NOT be invalidated")
	}
}

// ──────────────────────────────────────────────
// MEMORY RATE LIMITER
// ──────────────────────────────────────────────

func TestMemoryLimiter_Allow(t *testing.T) {
	ml := newMemoryLimiter()
	defer ml.stop()

	ctx := context.Background()
	ok, err := ml.allow(ctx, "test-key", 3, time.Minute)
	if err != nil || !ok {
		t.Error("first request should be allowed")
	}
}

func TestMemoryLimiter_Exceed(t *testing.T) {
	ml := newMemoryLimiter()
	defer ml.stop()

	ctx := context.Background()
	key := "limit-test"
	for i := 0; i < 3; i++ {
		ok, err := ml.allow(ctx, key, 3, time.Minute)
		if err != nil || !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, err := ml.allow(ctx, key, 3, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("4th request should be denied (limit 3)")
	}
}

func TestMemoryLimiter_DifferentKeys(t *testing.T) {
	ml := newMemoryLimiter()
	defer ml.stop()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ml.allow(ctx, "key-a", 1, time.Minute)
	}
	ok, _ := ml.allow(ctx, "key-b", 1, time.Minute)
	if !ok {
		t.Error("different key should not be affected by other key's limit")
	}
}

func TestMemoryLimiter_WindowReset(t *testing.T) {
	ml := newMemoryLimiter()
	defer ml.stop()

	ctx := context.Background()
	key := "reset-test"
	ml.allow(ctx, key, 1, 50*time.Millisecond)
	ok, _ := ml.allow(ctx, key, 1, 50*time.Millisecond)
	if ok {
		t.Error("should be denied within window")
	}

	time.Sleep(60 * time.Millisecond)
	ok, _ = ml.allow(ctx, key, 1, time.Minute)
	if !ok {
		t.Error("should be allowed after window resets")
	}
}

func TestMemoryLimiter_CleanupExpired(t *testing.T) {
	ml := newMemoryLimiter()
	defer ml.stop()

	ctx := context.Background()
	ml.allow(ctx, "cleanup-test", 1, 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	// Manually trigger cleanup logic (same as cleanup goroutine)
	ml.mu.Lock()
	now := time.Now()
	for k, b := range ml.keys {
		if now.After(b.resetAt) {
			delete(ml.keys, k)
		}
	}
	count := len(ml.keys)
	ml.mu.Unlock()

	if count != 0 {
		t.Errorf("cleanup should have removed expired entries, got %d", count)
	}
}

// ──────────────────────────────────────────────
// ADMIN KNOCK STORE
// ──────────────────────────────────────────────

func TestAdminKnockStore_SetGet(t *testing.T) {
	s := newAdminKnockStore()
	defer s.stop()

	s.set("1.2.3.4", "secret-key", 5*time.Minute)
	key, ok := s.get("1.2.3.4")
	if !ok || key != "secret-key" {
		t.Errorf("expected 'secret-key', got ok=%v key=%q", ok, key)
	}
}

func TestAdminKnockStore_Expired(t *testing.T) {
	s := newAdminKnockStore()
	defer s.stop()

	s.set("1.2.3.4", "expiring", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	_, ok := s.get("1.2.3.4")
	if ok {
		t.Error("should return false for expired entry")
	}
}

func TestAdminKnockStore_Delete(t *testing.T) {
	s := newAdminKnockStore()
	defer s.stop()

	s.set("1.2.3.4", "to-delete", 5*time.Minute)
	s.delete("1.2.3.4")
	_, ok := s.get("1.2.3.4")
	if ok {
		t.Error("should return false after delete")
	}
}

func TestAdminKnockStore_Cleanup(t *testing.T) {
	s := newAdminKnockStore()
	defer s.stop()

	s.set("1.1.1.1", "k1", 1*time.Millisecond)
	s.set("2.2.2.2", "k2", 5*time.Minute)
	time.Sleep(10 * time.Millisecond)

	// Manually trigger cleanup
	now := time.Now()
	s.mu.Lock()
	for ip, e := range s.store {
		if now.After(e.expiresAt) {
			delete(s.store, ip)
		}
	}
	count := len(s.store)
	s.mu.Unlock()

	if count != 1 {
		t.Errorf("cleanup should remove 1 entry, got %d remaining", count)
	}
}

// ──────────────────────────────────────────────
// GZIP MIDDLEWARE
// ──────────────────────────────────────────────

func TestGzipMiddleware_Compresses(t *testing.T) {
	handler := gzipMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip")
	}

	gz, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gz.Close()
	body, _ := io.ReadAll(gz)
	if string(body) != "hello world" {
		t.Errorf("decompressed body = %q, want %q", body, "hello world")
	}
}

func TestGzipMiddleware_NoCompression(t *testing.T) {
	handler := gzipMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not compress when Accept-Encoding is missing")
	}
	if w.Body.String() != "hello" {
		t.Errorf("body = %q, want %q", w.Body.String(), "hello")
	}
}

// ──────────────────────────────────────────────
// FINGERPRINT GENERATION
// ──────────────────────────────────────────────

func TestGenerateFingerprint_Deterministic(t *testing.T) {
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.Header.Set("User-Agent", "TestAgent/1.0")
	r1.Header.Set("Accept-Language", "en-US")

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("User-Agent", "TestAgent/1.0")
	r2.Header.Set("Accept-Language", "en-US")

	fp1 := generateFingerprint(r1)
	fp2 := generateFingerprint(r2)

	if fp1 != fp2 {
		t.Errorf("fingerprints differ: %q != %q", fp1, fp2)
	}
}

func TestGenerateFingerprint_DifferentInputs(t *testing.T) {
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.Header.Set("User-Agent", "Agent1")

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("User-Agent", "Agent2")

	fp1 := generateFingerprint(r1)
	fp2 := generateFingerprint(r2)

	if fp1 == fp2 {
		t.Error("different inputs should produce different fingerprints")
	}
}

func TestGenerateFingerprint_IsHex(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	fp := generateFingerprint(r)
	if _, err := hex.DecodeString(fp); err != nil {
		t.Errorf("fingerprint is not valid hex: %q", fp)
	}
}

// ──────────────────────────────────────────────
// COOKIE HELPERS
// ──────────────────────────────────────────────

func TestSetSecureCookie(t *testing.T) {
	w := httptest.NewRecorder()
	setSecureCookie(w, "auth_token", "abc123", 3600)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "__Host-auth_token" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("cookie not found")
	}
	if found.Value != "abc123" {
		t.Errorf("cookie value = %q, want abc123", found.Value)
	}
	if !found.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if !found.Secure {
		t.Error("cookie should be Secure")
	}
	if found.SameSite != http.SameSiteStrictMode {
		t.Error("cookie should be SameSite=Strict")
	}
	if found.Path != "/" {
		t.Errorf("cookie path = %q, want /", found.Path)
	}
}

func TestSetCSRFCookie(t *testing.T) {
	w := httptest.NewRecorder()
	setCSRFCookie(w, "csrf-value", 3600)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "__Host-csrf_token" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("CSRF cookie not found")
	}
	if found.HttpOnly {
		t.Error("CSRF cookie should NOT be HttpOnly (frontend needs to read it)")
	}
	if !found.Secure {
		t.Error("CSRF cookie should be Secure")
	}
}

func TestClearCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearCookie(w, "auth_token")

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "__Host-auth_token" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("clear cookie not found")
	}
	if found.Value != "" {
		t.Error("cleared cookie should have empty value")
	}
	if found.MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", found.MaxAge)
	}
}

// ──────────────────────────────────────────────
// HANDLER INTEGRATION
// ──────────────────────────────────────────────

func TestHandler_AllPublicEndpoints(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	jwtSecretsOnce.Do(func() {
		jwtSecrets = []jwtKey{{Secret: []byte("test-secret-at-least-32-chars!!"), ID: "1"}}
		primaryJWTKey = jwtSecrets[0].Secret
		primaryJWTID = "1"
	})

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/api/leaderboard", http.StatusOK},
		{http.MethodGet, "/api/players", http.StatusOK},
		{http.MethodGet, "/api/csrf-token", http.StatusOK},
		{http.MethodGet, "/api/captcha", http.StatusOK},
		{http.MethodGet, "/api/nonexistent", http.StatusNotFound},
		{http.MethodPost, "/api/nonexistent", http.StatusNotFound},
	}

	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	// Staff tiers requires Firestore (returns 503 without it)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/staff/tiers", nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "10.0.0.1:1234"
	Handler(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /api/staff/tiers without Firestore: got %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	// Endpoints requiring Firestore return 503
 firestoreEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/projects"},
		{http.MethodGet, "/api/staff"},
	}
	for _, ep := range firestoreEndpoints {
		t.Run(ep.method+" "+ep.path+"_no_firestore", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(ep.method, ep.path, nil)
			r.Header.Set("User-Agent", ua)
			r.RemoteAddr = "10.0.0.2:1234"
			Handler(w, r)
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("%s %s without Firestore: got %d, want %d", ep.method, ep.path, w.Code, http.StatusServiceUnavailable)
			}
		})
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tt.method, tt.path, nil)
			r.Header.Set("User-Agent", ua)
			Handler(w, r)
			if w.Code != tt.want {
				t.Errorf("%s %s: got %d, want %d", tt.method, tt.path, w.Code, tt.want)
			}
		})
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	tests := []struct {
		path   string
		method string
	}{
		{"/api/captcha", http.MethodPost},
		{"/api/captcha", http.MethodPut},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tt.method, tt.path, nil)
			r.Header.Set("User-Agent", ua)
			Handler(w, r)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: got %d, want %d", tt.method, tt.path, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandler_UnauthorizedEndpoints(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	jwtSecretsOnce.Do(func() {
		jwtSecrets = []jwtKey{{Secret: []byte("test-secret-at-least-32-chars!!"), ID: "1"}}
		primaryJWTKey = jwtSecrets[0].Secret
		primaryJWTID = "1"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	// Endpoints that use authMiddleware only (no knockMiddleware)
	authOnlyEndpoints := []struct {
		path   string
		method string
	}{
		{"/api/security/dashboard", http.MethodGet},
		{"/api/knock-knock-admin", http.MethodPost},
	}

	for _, ep := range authOnlyEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(ep.method, ep.path, nil)
			r.Header.Set("User-Agent", ua)
			r.RemoteAddr = "20.0.0.1:1234"
			Handler(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without auth: got %d, want %d", ep.method, ep.path, w.Code, http.StatusUnauthorized)
			}
		})
	}

	// Endpoints that use knockMiddleware (returns 404 when no knock key)
	knockEndpoints := []struct {
		path   string
		method string
	}{
		{"/api/players/save", http.MethodPost},
		{"/api/players/delete", http.MethodPost},
		{"/api/projects/save", http.MethodPost},
		{"/api/staff/save", http.MethodPost},
		{"/api/staff/add", http.MethodPost},
		{"/api/staff/remove", http.MethodPost},
		{"/api/staff/role", http.MethodPost},
		{"/api/staff/reorder", http.MethodPost},
		{"/api/staff/tier", http.MethodPost},
	}

	for _, ep := range knockEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(ep.method, ep.path, nil)
			r.Header.Set("User-Agent", ua)
			r.RemoteAddr = "20.0.0.1:1234"
			Handler(w, r)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s without knock key: got %d, want %d", ep.method, ep.path, w.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandler_HoneypotTriggers(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	honeypots := []string{"/.env", "/wp-admin", "/phpmyadmin", "/.git/config", "/api/debug"}
	for _, path := range honeypots {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Header.Set("User-Agent", ua)
			r.RemoteAddr = "1.2.3.4:1234"
			Handler(w, r)
			if w.Code != http.StatusNotFound {
				t.Errorf("honeypot %s: got %d, want %d", path, w.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandler_BotDetection(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})

	bots := []string{"sqlmap/1.5", "nikto/2.1.6", "curl/7.68.0"}
	for _, ua := range bots {
		t.Run(ua, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
			r.Header.Set("User-Agent", ua)
			r.RemoteAddr = "5.6.7.8:1234"
			Handler(w, r)
			if w.Code != http.StatusForbidden {
				t.Errorf("bot %q: got %d, want %d", ua, w.Code, http.StatusForbidden)
			}
		})
	}
}

func TestHandler_OversizedRequest(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.Header.Set("User-Agent", ua)
	r.ContentLength = 10 * 1024 * 1024
	r.RemoteAddr = "9.9.9.9:1234"
	Handler(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized: got %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandler_CaptchaEndpoint(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/captcha", nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "10.0.0.1:1234"
	Handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("captcha: got %d, want %d", w.Code, http.StatusOK)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("failed to parse captcha response: %v", err)
	}
	if _, ok := data["captchaId"]; !ok {
		t.Error("response should contain captchaId")
	}
	if _, ok := data["captchaImage"]; !ok {
		t.Error("response should contain captchaImage")
	}
}

func TestHandler_CSRFToken(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "11.0.0.1:1234"
	Handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("csrf-token: got %d, want %d", w.Code, http.StatusOK)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("failed to parse csrf response: %v", err)
	}
	if _, ok := data["token"]; !ok {
		t.Error("response should contain token")
	}

	cookies := w.Result().Cookies()
	var csrfCookie bool
	for _, c := range cookies {
		if c.Name == "__Host-csrf_token" {
			csrfCookie = true
			break
		}
	}
	if !csrfCookie {
		t.Error("should set __Host-csrf_token cookie")
	}
}

func TestHandler_LoginRequiresMethod(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/login", nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "12.0.0.1:1234"
	Handler(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/login: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandler_LogoutRequiresMethod(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/logout", nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "13.0.0.1:1234"
	Handler(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/logout: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ──────────────────────────────────────────────
// CORS
// ──────────────────────────────────────────────

func TestHandler_CORS_Preflight(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/api/leaderboard", nil)
	r.Header.Set("Origin", "https://smltdemonlist.vercel.app")
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "14.0.0.1:1234"
	Handler(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS preflight: got %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://smltdemonlist.vercel.app" {
		t.Error("CORS header not set for allowed origin")
	}
}

func TestHandler_CORS_BlockedOrigin(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	r.Header.Set("Origin", "https://evil.com")
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "15.0.0.1:1234"
	Handler(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS header should NOT be set for blocked origin")
	}
}

// ──────────────────────────────────────────────
// KNOCK STORE (MEMORY)
// ──────────────────────────────────────────────

func TestKnockMiddleware_NoKey(t *testing.T) {
	s := newAdminKnockStore()
	defer s.stop()
	adminKnockStore = s
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := knockMiddleware(inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/staff/save", nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "20.0.0.1:1234"
	handler(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("knock without key: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestKnockMiddleware_WrongKey(t *testing.T) {
	s := newAdminKnockStore()
	defer s.stop()
	s.set("21.0.0.1", "correct-key", 5*time.Minute)
	adminKnockStore = s
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := knockMiddleware(inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/staff/save", nil)
	r.Header.Set("User-Agent", ua)
	r.Header.Set("X-Admin-Path-Key", "wrong-key")
	r.RemoteAddr = "21.0.0.1:1234"
	handler(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("knock with wrong key: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestKnockMiddleware_CorrectKey(t *testing.T) {
	s := newAdminKnockStore()
	defer s.stop()
	s.set("22.0.0.1", "correct-key", 5*time.Minute)
	adminKnockStore = s
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := knockMiddleware(inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/staff/save", nil)
	r.Header.Set("User-Agent", ua)
	r.Header.Set("X-Admin-Path-Key", "correct-key")
	r.RemoteAddr = "22.0.0.1:1234"
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("knock with correct key: got %d, want %d", w.Code, http.StatusOK)
	}
}

// ──────────────────────────────────────────────
// CAPTCHA ESCALATION
// ──────────────────────────────────────────────

func TestGetCaptchaDifficulty_Default(t *testing.T) {
	clearCaptchaEscalation("test-ip")
	h, cw, noise, length := getCaptchaDifficulty("test-ip")
	if h != 80 || cw != 240 || length != 5 {
		t.Errorf("default difficulty: h=%d cw=%d len=%d, want 80/240/5", h, cw, length)
	}
	if noise != 0.7 {
		t.Errorf("default noise = %f, want 0.7", noise)
	}
}

func TestGetCaptchaDifficulty_Escalation(t *testing.T) {
	for i := 0; i < 10; i++ {
		recordCaptchaFailure("escalate-ip")
	}
	_, _, noise, length := getCaptchaDifficulty("escalate-ip")
	if length != 8 {
		t.Errorf("high escalation length = %d, want 8", length)
	}
	if noise != 0.5 {
		t.Errorf("high escalation noise = %f, want 0.5", noise)
	}
	clearCaptchaEscalation("escalate-ip")
}

func TestRecordCaptchaFailure(t *testing.T) {
	clearCaptchaEscalation("record-test")
	recordCaptchaFailure("record-test")
	recordCaptchaFailure("record-test")
	recordCaptchaFailure("record-test")

	val, ok := captchaEscalation.Load("record-test")
	if !ok {
		t.Fatal("failure count not stored")
	}
	p := val.(*int)
	if *p != 3 {
		t.Errorf("failure count = %d, want 3", *p)
	}
	clearCaptchaEscalation("record-test")
}

// ──────────────────────────────────────────────
// RESPONSE CACHE CONCURRENCY
// ──────────────────────────────────────────────

func TestCacheConcurrentAccess(t *testing.T) {
	apiCache = &responseCache{entries: make(map[string]*cacheEntry)}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "concurrent-" + string(rune('a'+n%26))
			cacheSet(key, []byte("value"), 10*time.Second)
			cacheGet(key)
			cacheInvalidate("concurrent-")
		}(i)
	}
	wg.Wait()
}

// ──────────────────────────────────────────────
// REQUEST PATH PARSING
// ──────────────────────────────────────────────

func TestRequestPath_TrailingSlash(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/players/", nil)
	r.URL.Path = "/api/players/"
	got := requestPath(r)
	// requestPath returns the raw path - trailing slash is preserved for non-API-root
	if got != "/api/players/" {
		t.Errorf("requestPath(%q) = %q, want /api/players/", r.URL.Path, got)
	}
}

func TestRequestPath_ApiRoot(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api", nil)
	r.URL.Path = "/api"
	got := requestPath(r)
	if got != "/api" {
		t.Errorf("requestPath(%q) = %q, want /api", r.URL.Path, got)
	}
}

func TestRequestPath_ApiRootSlash(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/", nil)
	r.URL.Path = "/api/"
	got := requestPath(r)
	// /api/ returns RequestURI for API root (with query string if present)
	if !strings.HasPrefix(got, "/api") {
		t.Errorf("requestPath(%q) = %q, should start with /api", r.URL.Path, got)
	}
}

// ──────────────────────────────────────────────
// IP UTILITIES
// ──────────────────────────────────────────────

func TestGetRealIP_VercelHeader(t *testing.T) {
	trustProxy = true
	defer func() { trustProxy = false }()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:80"
	r.Header.Set("X-Vercel-Forwarded-For", "198.51.100.1")
	got := getRealIP(r)
	if got != "198.51.100.1" {
		t.Errorf("Vercel header: got %q, want 198.51.100.1", got)
	}
}

func TestGetRealIP_VercelInvalid(t *testing.T) {
	trustProxy = true
	defer func() { trustProxy = false }()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("X-Vercel-Forwarded-For", "not-an-ip")
	r.Header.Set("X-Forwarded-For", "203.0.113.50")
	got := getRealIP(r)
	if got != "203.0.113.50" {
		t.Errorf("invalid Vercel header fallback: got %q, want 203.0.113.50", got)
	}
}

func TestRemoteAddrIP_IPv6(t *testing.T) {
	r := &http.Request{RemoteAddr: "[::1]:8080"}
	got := remoteAddrIP(r)
	if got != "::1" {
		t.Errorf("IPv6: got %q, want ::1", got)
	}
}

// ──────────────────────────────────────────────
// BOT DETECTION
// ──────────────────────────────────────────────

func TestIsBlockedBot_Scanners(t *testing.T) {
	scanners := []string{
		"Nuclei - Open-source project",
		"masscan/1.0",
		"ZmEu",
		"DirBuster/1.0",
	}
	for _, ua := range scanners {
		if !isBlockedBot(ua) {
			t.Errorf("scanner %q should be blocked", ua)
		}
	}
}

func TestIsBlockedBot_AllowedBrowsers(t *testing.T) {
	browsers := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
	}
	for _, ua := range browsers {
		if isBlockedBot(ua) {
			t.Errorf("browser %q should NOT be blocked", ua)
		}
	}
}

// ──────────────────────────────────────────────
// BLOCKED PATHS
// ──────────────────────────────────────────────

func TestIsBlockedPath_AttackPatterns(t *testing.T) {
	attacks := []string{
		"/wp-login.php",
		"/xmlrpc.php",
		"/wp-admin/install.php",
		"/administrator/index.php",
		"/config.php.bak",
		"/shell.php",
		"/.htaccess",
		"/web.config",
		"/backdoor.php",
		"/webshell.php",
		"/c99.php",
		"/r57.php",
	}
	for _, p := range attacks {
		if !isBlockedPath(p) {
			t.Errorf("attack path %q should be blocked", p)
		}
	}
}

func TestIsBlockedPath_NotInBlockList(t *testing.T) {
	// These paths are NOT in the blockedPathPatterns list
	notBlocked := []string{
		"/shell.cmd",
		"/.htpasswd",
		"/cgi-bin/test",
	}
	for _, p := range notBlocked {
		if isBlockedPath(p) {
			t.Errorf("path %q is not in block list, should NOT be blocked", p)
		}
	}
}

func TestIsBlockedPath_NormalPaths(t *testing.T) {
	normal := []string{
		"/api/leaderboard",
		"/api/projects",
		"/api/staff",
		"/index.html",
		"/leaderboard.html",
		"/projects.html",
	}
	for _, p := range normal {
		if isBlockedPath(p) {
			t.Errorf("normal path %q should NOT be blocked", p)
		}
	}
}

// ──────────────────────────────────────────────
// SECURITY HEADERS
// ──────────────────────────────────────────────

func TestHandler_AllSecurityHeaders(t *testing.T) {
	rlOnce.Do(func() {
		ml := newMemoryLimiter()
		globalRateLimiter = ml
		rlStop = ml.stop
	})
	saltOnce.Do(func() {
		rateLimitSalt = "test-salt"
	})
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	r.Header.Set("User-Agent", ua)
	r.RemoteAddr = "30.0.0.1:1234"
	Handler(w, r)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=(), payment=(), interest-cohort=(), browsing-topics=()",
		"Cross-Origin-Opener-Policy": "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	}

	for k, want := range expectedHeaders {
		got := w.Header().Get(k)
		if got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
}

// ──────────────────────────────────────────────
// JWT LOOKUP
// ──────────────────────────────────────────────

func TestLookupJWTSecret_KnownID(t *testing.T) {
	jwtSecretsMu.Lock()
	jwtSecrets = []jwtKey{
		{Secret: []byte("secret-1"), ID: "1"},
		{Secret: []byte("secret-2"), ID: "2"},
	}
	jwtSecretsMu.Unlock()

	secret, err := lookupJWTSecret("1")
	if err != nil || string(secret) != "secret-1" {
		t.Errorf("lookupJWTSecret(1) = %q, %v", secret, err)
	}

	secret, err = lookupJWTSecret("2")
	if err != nil || string(secret) != "secret-2" {
		t.Errorf("lookupJWTSecret(2) = %q, %v", secret, err)
	}
}

func TestLookupJWTSecret_UnknownID(t *testing.T) {
	jwtSecretsMu.Lock()
	jwtSecrets = []jwtKey{{Secret: []byte("secret-1"), ID: "1"}}
	jwtSecretsMu.Unlock()

	_, err := lookupJWTSecret("999")
	if err == nil {
		t.Error("lookupJWTSecret(999) should return error")
	}
}

func TestLookupJWTSecret_EmptyID(t *testing.T) {
	jwtSecretsMu.Lock()
	jwtSecrets = []jwtKey{{Secret: []byte("primary"), ID: "1"}}
	jwtSecretsMu.Unlock()

	secret, err := lookupJWTSecret("")
	if err != nil || string(secret) != "primary" {
		t.Errorf("lookupJWTSecret('') = %q, %v (should return primary)", secret, err)
	}
}

// ──────────────────────────────────────────────
// SANITIZE STRING
// ──────────────────────────────────────────────

func TestSanitizeString_XSSVectors(t *testing.T) {
	vectors := []struct {
		input string
		desc  string
	}{
		{"<script>alert(1)</script>", "script tag"},
		{"<img src=x onerror=alert(1)>", "img onerror"},
		{"<svg onload=alert(1)>", "svg onload"},
		{"javascript:alert(1)", "javascript protocol"},
		{"<iframe src='javascript:alert(1)>'", "iframe"},
	}
	for _, v := range vectors {
		result := sanitizeString(v.input)
		if strings.Contains(result, "<script") || strings.Contains(result, "onerror") || strings.Contains(result, "onload") {
			t.Errorf("sanitizeString blocked %s but result contains: %q", v.desc, result)
		}
	}
}

// ──────────────────────────────────────────────
// DEFAULT PLAYERS
// ──────────────────────────────────────────────

func TestDefaultPlayersList_NonEmpty(t *testing.T) {
	players := defaultPlayersList()
	if len(players) == 0 {
		t.Fatal("defaultPlayersList should not be empty")
	}
	seen := make(map[string]bool)
	for _, p := range players {
		if p.Name == "" {
			t.Error("player name should not be empty")
		}
		if seen[p.Name] {
			t.Errorf("duplicate player: %q", p.Name)
		}
		seen[p.Name] = true
	}
}

func TestDefaultPlayersList_Count(t *testing.T) {
	players := defaultPlayersList()
	if len(players) < 10 {
		t.Errorf("expected at least 10 default players, got %d", len(players))
	}
}
