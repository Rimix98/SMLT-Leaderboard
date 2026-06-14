package handler

import (
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/golang-jwt/jwt/v5"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mojocn/base64Captcha"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ──────────────────────────────────────────────
// TYPES
// ──────────────────────────────────────────────

type StaffPlayer struct {
	Nickname string `json:"nickname" firestore:"nickname"`
	Discord  string `json:"discord" firestore:"discord"`
}

type StaffRole struct {
	Name         string        `json:"name" firestore:"name"`
	Color        string        `json:"color" firestore:"color"`
	Players      []StaffPlayer `json:"players" firestore:"players"`
	TiersEnabled bool          `json:"tiersEnabled" firestore:"tiersEnabled"`
}

type StaffTierEntry struct {
	Nickname string `json:"nickname" firestore:"nickname"`
	Tier     string `json:"tier" firestore:"tier"`
}

type Project struct {
	Name         string   `json:"name" firestore:"name"`
	VideoID      string   `json:"videoId" firestore:"videoId"`
	ID           string   `json:"id" firestore:"id"`
	Comment      string   `json:"comment" firestore:"comment"`
	Status       string   `json:"status" firestore:"status"`
	Verifier     string   `json:"verifier" firestore:"verifier"`
	Participants []string `json:"participants" firestore:"participants"`
}

type Player struct {
	Name string `json:"name" firestore:"name"`
}

type FullPlayerData struct {
	Name    string      `json:"name"`
	Data    interface{} `json:"data"`
	Records interface{} `json:"records"`
}

type AuditEntry struct {
	Action    string      `json:"action" firestore:"action"`
	Details   interface{} `json:"details" firestore:"details"`
	CreatedAt time.Time   `json:"createdAt" firestore:"createdAt"`
}

type jwtKey struct {
	Secret []byte
	ID     string
}

type memoryBucket struct {
	count   int
	resetAt time.Time
}

type tokenVersionCacheEntry struct {
	version   int64
	expiresAt time.Time
}

// ──────────────────────────────────────────────
// GLOBALS
// ──────────────────────────────────────────────

var (
	fsClient *firestore.Client
	fsOnce   sync.Once
	fsErr    error

	httpClient = &http.Client{Timeout: 15 * time.Second}

	trustProxy     bool
	maxRequestBody = int64(1024 * 1024)
	primaryJWTKey  []byte
	primaryJWTID   string

	globalRateLimiter rateLimiter
	rlOnce            sync.Once
	rlStop            func()

	rateLimitSalt string
	saltOnce      sync.Once

	jwtSecrets     []jwtKey
	jwtSecretsMu   sync.RWMutex
	jwtSecretsOnce sync.Once

	captchaStore base64Captcha.Store
	captchaInst  *base64Captcha.Captcha
	captchaOnce  sync.Once

	tokenVerCache sync.Map

	adminKnockStore knockStore
	adminKnockOnce  sync.Once

	captchaEscalation sync.Map
)

type knockStore interface {
	get(ip string) (string, bool)
	set(ip, key string, ttl time.Duration)
	delete(ip string)
	stop()
}

type adminKnockEntry struct {
	key       string
	expiresAt time.Time
}

type adminKnockStoreT struct {
	mu     sync.Mutex
	store  map[string]*adminKnockEntry
	stopCh chan struct{}
}

func newAdminKnockStore() *adminKnockStoreT {
	s := &adminKnockStoreT{
		store:  make(map[string]*adminKnockEntry),
		stopCh: make(chan struct{}),
	}
	go s.cleanup()
	return s
}

func (s *adminKnockStoreT) stop() {
	close(s.stopCh)
}

func (s *adminKnockStoreT) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for ip, e := range s.store {
				if now.After(e.expiresAt) {
					delete(s.store, ip)
				}
			}
			s.mu.Unlock()
		case <-s.stopCh:
			return
		}
	}
}

func (s *adminKnockStoreT) set(ip, key string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[ip] = &adminKnockEntry{
		key:       key,
		expiresAt: time.Now().Add(ttl),
	}
}

func (s *adminKnockStoreT) get(ip string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.store[ip]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		delete(s.store, ip)
		return "", false
	}
	return e.key, true
}

func (s *adminKnockStoreT) delete(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, ip)
}

type firestoreKnockStore struct{}

func (s *firestoreKnockStore) get(ip string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	docRef := fsClient.Collection("admin_knocks").Doc(ipToDocID(ip))
	doc, err := docRef.Get(ctx)
	if err != nil {
		return "", false
	}
	var entry struct {
		Key       string    `firestore:"key"`
		ExpiresAt time.Time `firestore:"expiresAt"`
	}
	if err := doc.DataTo(&entry); err != nil {
		return "", false
	}
	if time.Now().After(entry.ExpiresAt) {
		docRef.Delete(context.Background())
		return "", false
	}
	return entry.Key, true
}

func (s *firestoreKnockStore) set(ip, key string, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := fsClient.Collection("admin_knocks").Doc(ipToDocID(ip)).Set(ctx, map[string]interface{}{
		"key":       key,
		"expiresAt": time.Now().Add(ttl),
	})
	if err != nil {
		log.Printf("[knock] firestore set: %v", err)
	}
}

func (s *firestoreKnockStore) delete(ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := fsClient.Collection("admin_knocks").Doc(ipToDocID(ip)).Delete(ctx)
	if err != nil {
		log.Printf("[knock] firestore delete: %v", err)
	}
}

func (s *firestoreKnockStore) stop() {}

func ipToDocID(ip string) string {
	return strings.NewReplacer(".", "-", ":", "-").Replace(ip)
}

var (
	reProjectID    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)
	reAlphanumeric = regexp.MustCompile(`^[\p{L}0-9 _.\-]+$`)
	reDiscord      = regexp.MustCompile(`^[a-zA-Z0-9 _.\-#]+$`)
	reVideoID      = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	reRoleName     = regexp.MustCompile(`^[\p{L}0-9 _.\-]+$`)
	reHexColor     = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$`)
	reCaptchaID    = regexp.MustCompile(`^[a-zA-Z0-9]{8,64}$`)
)

var defaultPlayerNames = []string{
	"samoletik", "paradoxiz", "clokman", "itzslxnq", "H30n41k_GmD",
	"Filkoty", "DarBeast", "Florned", "Marzyiiik", "euphoriak8",
	"npoctou_gamer", "NopanicGD", "CandyCloud22", "Vakum", "Daggit",
	"Loran", "tapxyhh", "SerGio", "Fanim59", "prostoymofficial",
	"toxik blaze", "NatrixGMD", "toxatort", "SpaceRS", "yeahme",
	"Спини", "Linqwq", "RossceorpGD", "69liqu69",
}

var errRateLimitExceeded = fmt.Errorf("rate limit exceeded")

// ──────────────────────────────────────────────
// INIT
// ──────────────────────────────────────────────

func init() {
	trustProxy = os.Getenv("TRUST_PROXY") == "true" || os.Getenv("VERCEL") == "1"
	initFirestore()
	initRateLimiter()
	initRateLimitSalt()
	initJWTSecrets()
	initAdminKnock()
	startTokenBlacklistCleanup()
	StartAlertWorker()
}

func initAdminKnock() {
	adminKnockOnce.Do(func() {
		if fsClient != nil {
			log.Println("[knock] using firestore store")
			adminKnockStore = &firestoreKnockStore{}
		} else {
			log.Println("[knock] using memory store")
			adminKnockStore = newAdminKnockStore()
		}
	})
}

func initRateLimitSalt() {
	saltOnce.Do(func() {
		buf := make([]byte, 32)
		for tries := 0; tries < 3; tries++ {
			if _, err := rand.Read(buf); err == nil {
				rateLimitSalt = hex.EncodeToString(buf)
				return
			}
		}
		rateLimitSalt = fmt.Sprintf("%x|%d", time.Now().UnixNano(), os.Getpid())
	})
}

func initJWTSecrets() {
	jwtSecretsOnce.Do(func() {
		primary := os.Getenv("JWT_SECRET")
		if primary == "" {
			log.Println("[jwt] JWT_SECRET not set, auth will fail")
			return
		}
		if len(primary) < 32 {
			log.Fatalf("[jwt] JWT_SECRET too short (%d bytes), need at least 32", len(primary))
		}
		primaryJWTKey = []byte(primary)
		primaryJWTID = "1"
		jwtSecrets = append(jwtSecrets, jwtKey{Secret: primaryJWTKey, ID: primaryJWTID})
		for i := 2; ; i++ {
			key := os.Getenv(fmt.Sprintf("JWT_SECRET_%d", i))
			if key == "" {
				break
			}
			if len(key) < 32 {
				log.Printf("[jwt] JWT_SECRET_%d too short (%d bytes), skipping", i, len(key))
				continue
			}
			jwtSecrets = append(jwtSecrets, jwtKey{Secret: []byte(key), ID: fmt.Sprintf("%d", i)})
		}
	})
}

func initFirestore() {
	fsOnce.Do(func() {
		ctx := context.Background()
		creds := os.Getenv("FIRESTORE_CREDENTIALS")
		if creds == "" {
			fsErr = errors.New("FIRESTORE_CREDENTIALS not set")
			log.Printf("[firestore] %v", fsErr)
			return
		}
		app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(creds)))
		if err != nil {
			fsErr = err
			log.Printf("[firestore] init app: %v", err)
			return
		}
		fsClient, err = app.Firestore(ctx)
		if err != nil {
			fsErr = err
			log.Printf("[firestore] connect: %v", err)
		}
	})
}

// ──────────────────────────────────────────────
// RATE LIMITER
// ──────────────────────────────────────────────

type rateLimiter interface {
	allow(ctx context.Context, key string, max int, window time.Duration) (bool, error)
}

type memoryLimiter struct {
	mu     sync.Mutex
	keys   map[string]*memoryBucket
	stopCh chan struct{}
}

func newMemoryLimiter() *memoryLimiter {
	m := &memoryLimiter{keys: make(map[string]*memoryBucket), stopCh: make(chan struct{})}
	go m.cleanup()
	return m
}

// WARNING: in-memory rate limiter is scoped to a single serverless instance.
// On Vercel, concurrent instances each have their own counter,
// so effective limit = instances × max. Use Upstash Redis in production.

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
		log.Println("[ratelimit] Upstash Redis not configured, using memory limiter")
		return nil
	}
	return &upstashLimiter{
		url:   strings.TrimRight(redisURL, "/"),
		token: redisToken,
		http:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (u *upstashLimiter) allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	windowSeconds := int(window.Seconds())
	if windowSeconds < 1 {
		windowSeconds = 1
	}
	_, count, err := u.getOrCreate(ctx, key, max, windowSeconds)
	if err != nil {
		return true, err
	}
	return count <= max, nil
}

func (u *upstashLimiter) getOrCreate(ctx context.Context, key string, _ int, windowSec int) (ttl int, count int, err error) {
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
	if result.Error != nil && *result.Error == "ERR no such key" {
		return -1, 0, nil
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
		getCount, getErr := u.getKey(ctx, key)
		if getErr != nil {
			return 0, fmt.Errorf("upstash count parse: %w; get: %v", err, getErr)
		}
		return getCount, nil
	}
	return count, nil
}

func (u *upstashLimiter) getKey(ctx context.Context, key string) (int, error) {
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
		return 0, errors.New("key not found after incr")
	}
	return strconv.Atoi(*gr.Result)
}

func (u *upstashLimiter) ping(ctx context.Context) error {
	cmd := url.PathEscape("PING")
	reqURL := fmt.Sprintf("%s/%s", u.url, cmd)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth("default", u.token)
	resp, err := u.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	var result struct {
		Result *string `json:"result"`
		Error  *string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if result.Error != nil && *result.Error != "" {
		return fmt.Errorf("upstash ping: %s", *result.Error)
	}
	return nil
}

func initRateLimiter() {
	rlOnce.Do(func() {
		ul := newUpstashLimiter()
		if ul != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := ul.ping(ctx); err != nil {
				log.Printf("[ratelimit] Upstash Redis connection failed: %v, falling back to memory limiter", err)
				ml := newMemoryLimiter()
				globalRateLimiter = ml
				rlStop = ml.stop
				log.Println("[ratelimit] using in-memory limiter (fallback)")
				return
			}
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

// ──────────────────────────────────────────────
// CAPTCHA
// ──────────────────────────────────────────────

type firestoreCaptchaStore struct {
	client *firestore.Client
}

func (s *firestoreCaptchaStore) Set(id string, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.client.Collection("captcha").Doc(id).Set(ctx, map[string]interface{}{
		"value":     value,
		"expiresAt": time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		log.Printf("[captcha] firestore set: %v", err)
	}
	return err
}

func (s *firestoreCaptchaStore) Get(id string, clear bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	doc, err := s.client.Collection("captcha").Doc(id).Get(ctx)
	if err != nil {
		return ""
	}
	var data struct {
		Value     string    `firestore:"value"`
		ExpiresAt time.Time `firestore:"expiresAt"`
	}
	if err := doc.DataTo(&data); err != nil {
		return ""
	}
	if time.Now().After(data.ExpiresAt) {
		if clear {
			if _, delErr := s.client.Collection("captcha").Doc(id).Delete(ctx); delErr != nil {
				log.Printf("[captcha] delete expired: %v", delErr)
			}
		}
		return ""
	}
	if clear {
		if _, delErr := s.client.Collection("captcha").Doc(id).Delete(ctx); delErr != nil {
			log.Printf("[captcha] delete after get: %v", delErr)
		}
	}
	return data.Value
}

func (s *firestoreCaptchaStore) Verify(id, answer string, clear bool) bool {
	stored := s.Get(id, clear)
	if stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(answer)) == 1
}

func ensureCaptcha() {
	captchaOnce.Do(func() {
		if fsClient != nil {
			log.Println("[captcha] using firestore store")
			captchaStore = &firestoreCaptchaStore{client: fsClient}
		} else {
			log.Println("[captcha] using default memory store")
			captchaStore = base64Captcha.DefaultMemStore
		}
		captchaInst = base64Captcha.NewCaptcha(
			base64Captcha.NewDriverDigit(80, 240, 5, 0.7, 80),
			captchaStore,
		)
	})
}

// ──────────────────────────────────────────────
// MIDDLEWARE HELPERS
// ──────────────────────────────────────────────

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	statusCode int
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.statusCode = code
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.Writer.Write(b)
}

func gzipMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}
		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			next(w, r)
			return
		}
		defer gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		gw := &gzipResponseWriter{Writer: gz, ResponseWriter: w, statusCode: http.StatusOK}
		next(gw, r)
	}
}

func remoteAddrIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func getRealIP(r *http.Request) string {
	if !trustProxy {
		return remoteAddrIP(r)
	}
	xfvf := r.Header.Get("X-Vercel-Forwarded-For")
	if xfvf != "" {
		if ip := net.ParseIP(strings.TrimSpace(xfvf)); ip != nil {
			return ip.String()
		}
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		for i := 0; i < len(parts); i++ {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip != nil {
				return ip.String()
			}
		}
	}
	return remoteAddrIP(r)
}

func hashIP(ip string) string {
	if rateLimitSalt == "" {
		initRateLimitSalt()
	}
	hash := sha256.Sum256([]byte(ip + rateLimitSalt))
	return hex.EncodeToString(hash[:16])
}

func sendError(w http.ResponseWriter, status int, msg string) {
	log.Printf("[error] %d %s", status, msg)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	sendError(w, http.StatusMethodNotAllowed, "Метод не разрешен")
}

func decodeRequestJSON(w http.ResponseWriter, r *http.Request, dest interface{}) error {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return errors.New("unsupported content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dest)
}

func sanitizeString(s string) string {
	return strings.TrimSpace(bluemonday.UGCPolicy().Sanitize(s))
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func setSecureCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-" + name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	})
}

func setCSRFCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-csrf_token",
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-" + name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func requestPath(r *http.Request) string {
	p := r.URL.Path
	if p == "/api" || p == "/api/" {
		return r.RequestURI
	}
	return p
}

func requireFirestore(w http.ResponseWriter) bool {
	if fsErr != nil || fsClient == nil {
		sendError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return false
	}
	return true
}

// ──────────────────────────────────────────────
// BOT DETECTION
// ──────────────────────────────────────────────

var blockedBotPatterns = []string{
	"sqlmap",
	"nikto",
	"nessus",
	"openvas",
	"w3af",
	"arachni",
	"skipfish",
	"whatweb",
	"dirbuster",
	"gobuster",
	"ffuf",
	"wfuzz",
	"masscan",
	"zgrab",
	"httpx",
	"nuclei",
	"jaeles",
	"xray",
	"vulmap",
	"pocsuite",
	"hydra",
	"medusa",
	"ncrack",
	"patator",
	"brutus",
	"metasploit",
	"burpsuite",
	"owasp",
	"acunetix",
	"appscan",
	"webinspect",
	"paros",
	"wparos",
	"webscarab",
	"mitmproxy",
	"charles",
	"fiddler",
	"grabber",
	"wapiti",
	"havij",
	"canari",
	"slowloris",
	"goldeneye",
	"slowhttptest",
	"rudy",
	"tor",
	"curl",
	"wget",
	"python",
	"perl",
	"ruby",
	"php/",
	"scrapy",
	"crawler",
	"spider",
	"scraper",
	"harvest",
	"extract",
	"scan",
	"exploit",
	"hack",
	"crack",
	"brute",
	"go-http-client",
	"java/",
	"libwww",
	"lwp-trivial",
	"webbandit",
	"webcopier",
	"webzip",
	"teleport",
	"sitecopy",
	"httrack",
	"clixboard",
	"cms探测",
	"dirbuster",
	"nmap",
	"masscan",
	"zmap",
	"unicornsql",
	"sqlbf",
	"sqlbrute",
	"sqlsmack",
	"sqlfury",
	"sqlninja",
	"bbqsql",
	"jsql",
	"wapiti",
	"arachni",
	"skipfish",
	"nikto",
	"openvas",
	"nessus",
	"qualys",
	"ratproxy",
	"w3af",
	"websecurify",
	"netsparker",
}

var blockedPathPatterns = []string{
	"wp-admin",
	"wp-login",
	"xmlrpc.php",
	"wp-content",
	"wp-includes",
	"administrator",
	"config.php",
	"config.inc",
	"setup.php",
	"install.php",
	"shell.php",
	"cmd.php",
	"backdoor",
	"webshell",
	"c99",
	"r57",
	"b374k",
	" FilesMan",
	"File manager",
	".htaccess",
	"web.config",
	"crossdomain.xml",
}

func isBlockedBot(ua string) bool {
	if ua == "" {
		return true
	}
	lower := strings.ToLower(ua)
	for _, pattern := range blockedBotPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	browserPrefixes := []string{
		"mozilla/", "chrome/", "safari/", "firefox/", "edge/", "opera/",
		"webkit/", "blink/",
	}
	isBrowser := false
	for _, prefix := range browserPrefixes {
		if strings.HasPrefix(lower, prefix) {
			isBrowser = true
			break
		}
	}
	if !isBrowser {
		return true
	}
	return false
}

func isBlockedPath(path string) bool {
	lower := strings.ToLower(path)
	for _, pattern := range blockedPathPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func botDetectionMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ua := r.UserAgent()
		ip := getRealIP(r)

		if isDeviceBanned(r.Context(), generateFingerprint(r)) {
			securityEvent(r.Context(), "device_banned", ip, r.URL.Path, map[string]string{
				"ua":          ua,
				"fingerprint": generateFingerprint(r),
			})
			sendError(w, http.StatusForbidden, "Устройство заблокировано")
			return
		}

		if isBlockedBot(ua) {
			fp := generateFingerprint(r)
			securityEvent(r.Context(), "bot_blocked", ip, r.URL.Path, map[string]string{
				"ua":          ua,
				"fingerprint": fp,
			})
			alertWithBanButtons("bot_blocked", ip, r.URL.Path, ua, fp)
			time.Sleep(time.Duration(mathrand.IntN(500)+200) * time.Millisecond)
			sendError(w, http.StatusForbidden, "Доступ запрещен")
			return
		}

		if isBlockedPath(r.URL.Path) {
			fp := generateFingerprint(r)
			securityEvent(r.Context(), "blocked_path", ip, r.URL.Path, map[string]string{
				"ua":          ua,
				"fingerprint": fp,
			})
			alertWithBanButtons("blocked_path", ip, r.URL.Path, ua, fp)
			time.Sleep(time.Duration(mathrand.IntN(500)+200) * time.Millisecond)
			sendError(w, http.StatusForbidden, "Доступ запрещен")
			return
		}

		if r.ContentLength > 5*1024*1024 {
			securityEvent(r.Context(), "oversized_request", ip, r.URL.Path, map[string]int{
				"size": int(r.ContentLength),
			})
			sendError(w, http.StatusRequestEntityTooLarge, "Слишком большой запрос")
			return
		}

		next(w, r)
	}
}

// ──────────────────────────────────────────────
// JWT / AUTH
// ──────────────────────────────────────────────

func lookupJWTSecret(kid string) ([]byte, error) {
	jwtSecretsMu.RLock()
	defer jwtSecretsMu.RUnlock()
	if kid == "" && len(jwtSecrets) > 0 {
		return jwtSecrets[0].Secret, nil
	}
	for _, k := range jwtSecrets {
		if k.ID == kid {
			return k.Secret, nil
		}
	}
	return nil, errors.New("jwt secret not found")
}

func verifyTokenVersion(ctx context.Context, claims *jwt.MapClaims) error {
	v, ok := (*claims)["ver"].(float64)
	if !ok {
		return errors.New("no token version")
	}
	requiredVer := int64(v)

	cached, ok := tokenVerCache.Load("tokenVersion")
	if ok {
		entry := cached.(*tokenVersionCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			if entry.version > requiredVer {
				return errors.New("token version too old")
			}
			return nil
		}
	}
	if fsClient == nil {
		// Use last known version from cache (even if slightly expired)
		if cached, ok := tokenVerCache.Load("tokenVersion"); ok {
			entry := cached.(*tokenVersionCacheEntry)
			if entry.version > requiredVer {
				return errors.New("token version too old")
			}
			return nil
		}
		// No cache and no Firestore — deny to be safe during outages
		return errors.New("token version cannot be verified")
	}
	doc, err := fsClient.Collection("config").Doc("auth").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return err
	}
	var cfg struct {
		TokenVersion int64 `json:"tokenVersion" firestore:"tokenVersion"`
	}
	if err := doc.DataTo(&cfg); err != nil {
		return nil
	}
	tokenVerCache.Store("tokenVersion", &tokenVersionCacheEntry{
		version:   cfg.TokenVersion,
		expiresAt: time.Now().Add(60 * time.Second),
	})
	if cfg.TokenVersion > requiredVer {
		return errors.New("token version too old")
	}
	return nil
}

func hashIPWithSalt(ip string) string {
	if rateLimitSalt == "" {
		initRateLimitSalt()
	}
	h := sha256.Sum256([]byte("jwt-bind:" + ip + ":" + rateLimitSalt))
	return hex.EncodeToString(h[:16])
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("__Host-auth_token")
		if err != nil {
			sendError(w, http.StatusUnauthorized, "Нет доступа")
			return
		}
		tokenString := cookie.Value
		claims := &jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			kid, _ := t.Header["kid"].(string)
			return lookupJWTSecret(kid)
		})
		if err != nil || !token.Valid {
			sendError(w, http.StatusUnauthorized, "Невалидный токен")
			return
		}

		if jti, ok := (*claims)["jti"].(string); ok && jti != "" {
			if isTokenBlacklisted(r.Context(), jti) {
				sendError(w, http.StatusUnauthorized, "Сессия завершена, войдите заново")
				return
			}
		}

		if err := verifyTokenVersion(r.Context(), claims); err != nil {
			sendError(w, http.StatusUnauthorized, "Сессия устарела, войдите заново")
			return
		}

		if ipHash, ok := (*claims)["ip"].(string); ok && ipHash != "" {
			currentHash := hashIPWithSalt(getRealIP(r))
			if subtle.ConstantTimeCompare([]byte(ipHash), []byte(currentHash)) != 1 {
				if jti, ok := (*claims)["jti"].(string); ok && jti != "" {
					blacklistToken(r.Context(), jti)
				}
				securityEvent(r.Context(), "ip_mismatch", getRealIP(r), r.URL.Path, nil)
				alertIPMismatch(getRealIP(r), r.URL.Path)
				sendError(w, http.StatusUnauthorized, "Сессия недействительна, войдите заново")
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

func csrfMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		headerToken := r.Header.Get("X-CSRF-Token")
		cookie, err := r.Cookie("__Host-csrf_token")
		if err != nil || cookie.Value == "" || headerToken == "" || headerToken != cookie.Value {
			sendError(w, http.StatusForbidden, "Доступ запрещен")
			return
		}
		// Rotate CSRF token after successful verification (single-use)
		newToken := make([]byte, 32)
		if _, randErr := rand.Read(newToken); randErr == nil {
			tokenStr := hex.EncodeToString(newToken)
			setCSRFCookie(w, tokenStr, 3600)
			w.Header().Set("X-CSRF-Token", tokenStr)
		}
		next.ServeHTTP(w, r)
	}
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

// ──────────────────────────────────────────────
// ADMIN KNOCK MIDDLEWARE
// ──────────────────────────────────────────────

const adminKnockTTL = 15 * time.Minute

func generateAdminKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func knockMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getRealIP(r)
		headerKey := r.Header.Get("X-Admin-Path-Key")
		if headerKey == "" {
			sendError(w, http.StatusNotFound, "Роут не найден")
			return
		}
		storedKey, ok := adminKnockStore.get(ip)
		if !ok || subtle.ConstantTimeCompare([]byte(headerKey), []byte(storedKey)) != 1 {
			sendError(w, http.StatusNotFound, "Роут не найден")
			return
		}
		next.ServeHTTP(w, r)
	}
}

func handleAdminKnock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	key, err := generateAdminKey()
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка генерации ключа")
		return
	}
	ip := getRealIP(r)
	adminKnockStore.set(ip, key, adminKnockTTL)
	log.Printf("[knock] admin key issued (TTL=%v)", adminKnockTTL)
	writeJSON(w, map[string]interface{}{
		"key":        key,
		"ttl":        int(adminKnockTTL.Seconds()),
		"expires_in": adminKnockTTL.String(),
	})
}

// ──────────────────────────────────────────────
// HANDLER: CAPTCHA
// ──────────────────────────────────────────────

func getCaptchaDifficulty(ip string) (int, int, float64, int) {
	val, _ := captchaEscalation.Load(ip)
	failures := 0
	if v, ok := val.(*int); ok {
		failures = *v
	}

	switch {
	case failures >= 10:
		return 80, 240, 0.5, 8
	case failures >= 5:
		return 80, 240, 0.6, 7
	case failures >= 3:
		return 80, 240, 0.65, 6
	default:
		return 80, 240, 0.7, 5
	}
}

func recordCaptchaFailure(ip string) {
	val, loaded := captchaEscalation.LoadOrStore(ip, new(int))
	if !loaded {
		p := val.(*int)
		p = new(int)
		captchaEscalation.Store(ip, p)
		val = p
	}
	p := val.(*int)
	*p++
}

func clearCaptchaEscalation(ip string) {
	captchaEscalation.Delete(ip)
}

func handleCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ensureCaptcha()

	ip := getRealIP(r)
	h, cw, noise, len := getCaptchaDifficulty(ip)

	drv := base64Captcha.NewDriverDigit(h, cw, len, noise, 80)
	c := base64Captcha.NewCaptcha(drv, captchaStore)
	id, b64s, _, err := c.Generate()
	if err != nil {
		log.Printf("[captcha] generate: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка генерации капчи")
		return
	}
	writeJSON(w, map[string]interface{}{
		"captchaId":    id,
		"captchaImage": b64s,
		"difficulty":   len,
	})
}

// ──────────────────────────────────────────────
// HANDLER: CSRF
// ──────────────────────────────────────────────

func handleGetCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка генерации токена")
		return
	}
	tokenStr := hex.EncodeToString(token)
	setCSRFCookie(w, tokenStr, 3600)
	writeJSON(w, map[string]bool{"success": true})
}

// ──────────────────────────────────────────────
// HANDLER: VERIFY
// ──────────────────────────────────────────────

func handleVerify(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("__Host-auth_token")
	if err != nil {
		writeJSON(w, map[string]bool{"success": false})
		return
	}
	claims := &jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		return lookupJWTSecret(kid)
	})
	if err != nil || !token.Valid {
		writeJSON(w, map[string]bool{"success": false})
		return
	}
	if jti, ok := (*claims)["jti"].(string); ok && jti != "" {
		if isTokenBlacklisted(r.Context(), jti) {
			writeJSON(w, map[string]bool{"success": false})
			return
		}
	}
	if err := verifyTokenVersion(r.Context(), claims); err != nil {
		writeJSON(w, map[string]bool{"success": false})
		return
	}
	writeJSON(w, map[string]bool{"success": true})
}

// ──────────────────────────────────────────────
// HANDLER: LOGIN
// ──────────────────────────────────────────────

func getCurrentTokenVersion(ctx context.Context) int64 {
	if fsClient == nil {
		return 1
	}
	cached, ok := tokenVerCache.Load("tokenVersion")
	if ok {
		entry := cached.(*tokenVersionCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			if entry.version < 1 {
				return 1
			}
			return entry.version
		}
	}
	doc, err := fsClient.Collection("config").Doc("auth").Get(ctx)
	if err != nil {
		return 1
	}
	var cfg struct {
		TokenVersion int64 `json:"tokenVersion" firestore:"tokenVersion"`
	}
	if err := doc.DataTo(&cfg); err != nil {
		return 1
	}
	tokenVerCache.Store("tokenVersion", &tokenVersionCacheEntry{
		version:   cfg.TokenVersion,
		expiresAt: time.Now().Add(60 * time.Second),
	})
	if cfg.TokenVersion < 1 {
		return 1
	}
	return cfg.TokenVersion
}

func generateJTI() (string, error) {
	buf := make([]byte, 16)
	for tries := 0; tries < 3; tries++ {
		if _, err := rand.Read(buf); err == nil {
			return hex.EncodeToString(buf), nil
		}
	}
	return "", errors.New("crypto/rand unavailable")
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !checkLoginRateLimit(w, r) {
		return
	}
	var req struct {
		Password     string `json:"password"`
		CaptchaID    string `json:"captchaId"`
		CaptchaValue string `json:"captchaValue"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой запрос")
		return
	}
	ensureCaptcha()
	if !reCaptchaID.MatchString(req.CaptchaID) {
		sendError(w, http.StatusBadRequest, "Некорректный ID капчи")
		return
	}
	if !captchaStore.Verify(req.CaptchaID, req.CaptchaValue, true) {
		recordCaptchaFailure(getRealIP(r))
		securityEvent(r.Context(), "captcha_failed", getRealIP(r), "/api/login", nil)
		sendError(w, http.StatusUnauthorized, "Неверные учетные данные")
		return
	}
	adminHash := os.Getenv("ADMIN_HASH")
	if adminHash == "" {
		log.Println("[login] ADMIN_HASH not set")
		sendError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		return
	}
	if primaryJWTKey == nil {
		log.Println("[login] JWT secrets not initialized")
		sendError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash), []byte(req.Password)); err != nil {
		recordCaptchaFailure(getRealIP(r))
		securityEvent(r.Context(), "login_failed", getRealIP(r), "/api/login", nil)
		alertLoginFailure(getRealIP(r), "wrong_password")
		sendError(w, http.StatusUnauthorized, "Неверные учетные данные")
		return
	}

	clearCaptchaEscalation(getRealIP(r))
	tokenVersion := getCurrentTokenVersion(r.Context())
	exp := time.Now().Add(24 * time.Hour)
	now := time.Now()
	jti, err := generateJTI()
	if err != nil {
		log.Printf("[login] jti generation failed: %v", err)
		sendError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin": true,
		"exp":   exp.Unix(),
		"iat":   now.Unix(),
		"ver":   tokenVersion,
		"jti":   jti,
		"ip":    hashIPWithSalt(getRealIP(r)),
	})
	token.Header["kid"] = primaryJWTID
	tokenString, err := token.SignedString(primaryJWTKey)
	if err != nil {
		log.Printf("[login] token signing: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка выдачи токена")
		return
	}
	setSecureCookie(w, "auth_token", tokenString, 86400)
	securityEvent(r.Context(), "login_success", getRealIP(r), "/api/login", nil)
	writeJSON(w, map[string]bool{"success": true})
}

func isTokenBlacklisted(ctx context.Context, jti string) bool {
	if fsClient == nil || jti == "" {
		return false
	}
	doc, err := fsClient.Collection("token_blacklist").Doc(jti).Get(ctx)
	if err != nil {
		return false
	}
	return doc.Exists()
}

func blacklistToken(ctx context.Context, jti string) {
	if fsClient == nil || jti == "" {
		return
	}
	_, err := fsClient.Collection("token_blacklist").Doc(jti).Set(ctx, map[string]interface{}{
		"blacklistedAt": time.Now(),
		"expiresAt":     time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		log.Printf("[auth] failed to blacklist token: %v", err)
	}
}

var tokenBlacklistCleanupOnce sync.Once

func startTokenBlacklistCleanup() {
	tokenBlacklistCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				cleanupExpiredBlacklistEntries()
			}
		}()
	})
}

func cleanupExpiredBlacklistEntries() {
	if fsClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	iter := fsClient.Collection("token_blacklist").Where("expiresAt", "<", time.Now()).Documents(ctx)
	defer iter.Stop()
	batch := fsClient.BulkWriter(ctx)
	defer batch.End()
	deleted := int64(0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("[auth] blacklist cleanup iter: %v", err)
			break
		}
		batch.Delete(doc.Ref)
		atomic.AddInt64(&deleted, 1)
	}
	if n := atomic.LoadInt64(&deleted); n > 0 {
		log.Printf("[auth] cleaned up %d expired blacklist entries", n)
	}
}

// ──────────────────────────────────────────────
// HANDLER: LOGOUT
// ──────────────────────────────────────────────

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if cookie, err := r.Cookie("__Host-auth_token"); err == nil {
		claims := &jwt.MapClaims{}
		if _, parseErr := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			kid, _ := t.Header["kid"].(string)
			return lookupJWTSecret(kid)
		}); parseErr == nil {
			if jti, ok := (*claims)["jti"].(string); ok && jti != "" {
				blacklistToken(r.Context(), jti)
			}
		}
	}
	clearCookie(w, "auth_token")
	clearCookie(w, "csrf_token")
	writeJSON(w, map[string]bool{"success": true})
}

// ──────────────────────────────────────────────
// HANDLER: PROJECTS
// ──────────────────────────────────────────────

func handleGetProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}
	ctx := r.Context()
	iter := fsClient.Collection("projects").Documents(ctx)
	projects := make([]Project, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("[projects] iter error: %v", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
			return
		}
		var p Project
		if err := doc.DataTo(&p); err != nil {
			continue
		}
		projects = append(projects, p)
	}
	writeJSON(w, projects)
}

func handleSaveProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var projectList []Project
	if err := decodeRequestJSON(w, r, &projectList); err != nil {
		log.Printf("[projects] decode error: %v", err)
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}
	ctx := r.Context()
	for i, p := range projectList {
		projectList[i].Name = sanitizeString(p.Name)
		projectList[i].VideoID = sanitizeString(p.VideoID)
		projectList[i].Comment = sanitizeString(p.Comment)
		projectList[i].Verifier = sanitizeString(p.Verifier)
		for j, part := range projectList[i].Participants {
			projectList[i].Participants[j] = sanitizeString(part)
		}
		if len(projectList[i].Name) > 100 ||
			len(projectList[i].VideoID) > 200 ||
			len(projectList[i].Comment) > 500 ||
			len(projectList[i].Verifier) > 50 {
			sendError(w, http.StatusBadRequest, "Слишком длинное поле в проекте")
			return
		}
		for _, part := range projectList[i].Participants {
			if len(part) > 5000 {
				sendError(w, http.StatusBadRequest, "Слишком длинное имя участника")
				return
			}
		}
	}

	seen := make(map[string]bool)
	for _, p := range projectList {
		if p.ID == "" {
			continue
		}
		var docID string
		if p.ID == "-" {
			b := make([]byte, 8)
			if _, err := rand.Read(b); err != nil {
				log.Printf("[projects] rand error: %v", err)
				sendError(w, http.StatusInternalServerError, "Ошибка генерации ID")
				return
			}
			docID = fmt.Sprintf("-%x", b)
		} else {
			if err := validateProjectID(p.ID); err != nil {
				log.Printf("[projects] invalid id %q: %v", p.ID, err)
				sendError(w, http.StatusBadRequest, "Некорректный ID проекта")
				return
			}
			if seen[p.ID] {
				log.Printf("[projects] duplicate id %q", p.ID)
				sendError(w, http.StatusBadRequest, "ID проекта уже существует")
				return
			}
			docID = p.ID
		}
		seen[docID] = true
		ref := fsClient.Collection("projects").Doc(docID)
		if _, err := ref.Set(ctx, p); err != nil {
			log.Printf("[projects] set %q: %v", docID, err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
			return
		}
	}

	iter := fsClient.Collection("projects").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("[projects] delete iter: %v", err)
			break
		}
		if !seen[doc.Ref.ID] {
			if _, err := doc.Ref.Delete(ctx); err != nil {
				log.Printf("[projects] delete %q: %v", doc.Ref.ID, err)
			}
		}
	}

	auditLog(r.Context(), AuditEntry{
		Action:  "projects.save",
		Details: map[string]int{"count": len(projectList)},
	})
	writeJSON(w, map[string]bool{"success": true})
}

// ──────────────────────────────────────────────
// DEVICE FINGERPRINTING & BAN SYSTEM
// ──────────────────────────────────────────────

type DeviceBan struct {
	Fingerprint string    `firestore:"fingerprint" json:"fingerprint"`
	IP          string    `firestore:"ip" json:"ip"`
	UA          string    `firestore:"ua" json:"ua"`
	Reason      string    `firestore:"reason" json:"reason"`
	BannedAt    time.Time `firestore:"bannedAt" json:"bannedAt"`
	ExpiresAt   time.Time `firestore:"expiresAt" json:"expiresAt"`
	BannedBy    string    `firestore:"bannedBy" json:"bannedBy"`
}

func generateFingerprint(r *http.Request) string {
	ua := r.UserAgent()
	al := r.Header.Get("Accept-Language")
	ae := r.Header.Get("Accept-Encoding")
	accept := r.Header.Get("Accept")

	raw := strings.ToLower(ua + "|" + al + "|" + ae + "|" + accept)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:12])
}

func isDeviceBanned(ctx context.Context, fingerprint string) bool {
	if fsClient == nil || fingerprint == "" {
		return false
	}
	doc, err := fsClient.Collection("device_bans").Doc(fingerprint).Get(ctx)
	if err != nil {
		return false
	}
	var ban DeviceBan
	if err := doc.DataTo(&ban); err != nil {
		return false
	}
	if time.Now().Before(ban.ExpiresAt) {
		return true
	}
	doc.Ref.Delete(ctx)
	return false
}

func banDevice(ctx context.Context, fingerprint, ip, ua, reason, bannedBy string, duration time.Duration) error {
	if fsClient == nil {
		return errors.New("firestore not available")
	}
	ban := DeviceBan{
		Fingerprint: fingerprint,
		IP:          ip,
		UA:          ua,
		Reason:      reason,
		BannedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(duration),
		BannedBy:    bannedBy,
	}
	_, err := fsClient.Collection("device_bans").Doc(fingerprint).Set(ctx, ban)
	return err
}

func unbanDevice(ctx context.Context, fingerprint string) error {
	if fsClient == nil {
		return errors.New("firestore not available")
	}
	_, err := fsClient.Collection("device_bans").Doc(fingerprint).Delete(ctx)
	return err
}

// ──────────────────────────────────────────────
// DISCORD INTERACTION HANDLER
// ──────────────────────────────────────────────

var (
	discordPublicKey     ed25519.PublicKey
	discordPublicKeyOnce sync.Once
)

func getDiscordPublicKey() ed25519.PublicKey {
	discordPublicKeyOnce.Do(func() {
		keyHex := os.Getenv("DISCORD_PUBLIC_KEY")
		if keyHex == "" {
			log.Println("[discord] DISCORD_PUBLIC_KEY not set, interactions disabled")
			return
		}
		keyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			log.Printf("[discord] invalid DISCORD_PUBLIC_KEY: %v", err)
			return
		}
		discordPublicKey = ed25519.PublicKey(keyBytes)
	})
	return discordPublicKey
}

func verifyDiscordSignature(signature, timestamp, body string) bool {
	pubKey := getDiscordPublicKey()
	if pubKey == nil {
		return false
	}
	msg := []byte(timestamp + body)
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return ed25519.Verify(pubKey, msg, sig)
}

type discordInteraction struct {
	ID            string                 `json:"id"`
	Type          int                    `json:"type"`
	Token         string                 `json:"token"`
	Member        *discordMember         `json:"member,omitempty"`
	Data          *discordInteractionData `json:"data,omitempty"`
}

type discordMember struct {
	User discordUser `json:"user"`
}

type discordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type discordInteractionData struct {
	CustomID string `json:"custom_id"`
	Values   []string `json:"values,omitempty"`
}

type discordInteractionResponse struct {
	Type int         `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

func handleDiscordInteraction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	signature := r.Header.Get("X-Signature-Ed25519")
	timestamp := r.Header.Get("X-Signature-Timestamp")
	if signature == "" || timestamp == "" {
		sendError(w, http.StatusUnauthorized, "Missing signature")
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
	if err != nil {
		sendError(w, http.StatusBadRequest, "Cannot read body")
		return
	}
	body := string(bodyBytes)

	if !verifyDiscordSignature(signature, timestamp, body) {
		sendError(w, http.StatusUnauthorized, "Invalid signature")
		return
	}

	var interaction discordInteraction
	if err := json.Unmarshal(bodyBytes, &interaction); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if interaction.Type == 1 {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"type":1}`))
		return
	}

	if interaction.Type == 3 && interaction.Data != nil {
		customID := interaction.Data.CustomID
		parts := strings.SplitN(customID, "|", 3)
		if len(parts) == 3 && parts[0] == "ban" {
			action := parts[1]
			fingerprint := parts[2]

			var duration time.Duration
			var label string
			switch action {
			case "1h":
				duration = 1 * time.Hour
				label = "1 час"
			case "24h":
				duration = 24 * time.Hour
				label = "24 часа"
			case "7d":
				duration = 7 * 24 * time.Hour
				label = "7 дней"
			case "perm":
				duration = 365 * 24 * time.Hour
				label = "навсегда"
			case "unban":
				err := unbanDevice(r.Context(), fingerprint)
				if err != nil {
				 respondInteraction(w, "❌ Ошибка: "+err.Error())
				 return
				}
				respondInteraction(w, fmt.Sprintf("✅ Устройство `%s` разбанено", fingerprint[:12]))
				return
			default:
				respondInteraction(w, "❌ Неизвестное действие")
				return
			}

			user := "unknown"
			if interaction.Member != nil {
				user = interaction.Member.User.Username
			}

			err := banDevice(r.Context(), fingerprint, "", "", "Discord ban", user, duration)
			if err != nil {
				respondInteraction(w, "❌ Ошибка: "+err.Error())
				return
			}

			respondInteraction(w, fmt.Sprintf("🔨 Устройство `%s` забанено на %s", fingerprint[:12], label))
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"type":5}`))
}

func respondInteraction(w http.ResponseWriter, message string) {
	resp := discordInteractionResponse{
		Type: 4,
		Data: map[string]interface{}{
			"content":    message,
			"flags":      64,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ──────────────────────────────────────────────
// HANDLER: SECURITY DASHBOARD
// ──────────────────────────────────────────────

func handleSecurityDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}

	type eventCount struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}
	type recentEvent struct {
		Type      string    `json:"type"`
		IP        string    `json:"ip"`
		Path      string    `json:"path"`
		CreatedAt time.Time `json:"createdAt"`
	}

	ctx := r.Context()
	since := time.Now().Add(-24 * time.Hour)

	iter := fsClient.Collection("security_events").
		Where("createdAt", ">=", since).
		Documents(ctx)
	defer iter.Stop()

	total := 0
	byType := make(map[string]int)
	topIPs := make(map[string]int)
	var recent []recentEvent

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("[dashboard] iter error: %v", err)
			break
		}
		var ev SecurityEvent
		if err := doc.DataTo(&ev); err != nil {
			continue
		}
		total++
		byType[ev.Type]++
		topIPs[ev.IP]++
		if len(recent) < 20 {
			recent = append(recent, recentEvent{
				Type:      ev.Type,
				IP:        ev.IP,
				Path:      ev.Path,
				CreatedAt: ev.CreatedAt,
			})
		}
	}

	typeCounts := make([]eventCount, 0, len(byType))
	for t, c := range byType {
		typeCounts = append(typeCounts, eventCount{Type: t, Count: c})
	}
	sort.Slice(typeCounts, func(i, j int) bool {
		return typeCounts[i].Count > typeCounts[j].Count
	})

	type ipCount struct {
		IP    string `json:"ip"`
		Count int    `json:"count"`
	}
	ipCounts := make([]ipCount, 0, len(topIPs))
	for ip, c := range topIPs {
		ipCounts = append(ipCounts, ipCount{IP: ip, Count: c})
	}
	sort.Slice(ipCounts, func(i, j int) bool {
		return ipCounts[i].Count > ipCounts[j].Count
	})
	if len(ipCounts) > 10 {
		ipCounts = ipCounts[:10]
	}

	writeJSON(w, map[string]interface{}{
		"period": "24h",
		"total":  total,
		"byType": typeCounts,
		"topIPs": ipCounts,
		"recent": recent,
	})
}

// ──────────────────────────────────────────────
// HANDLER: LEADERBOARD
// ──────────────────────────────────────────────

func validateDemonlistURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return errors.New("only https allowed")
	}
	if parsed.Host != "api.demonlist.org" {
		return errors.New("only api.demonlist.org allowed")
	}
	return nil
}

func fetchAPIWithRetry(ctx context.Context, apiURL string, maxRetries int) ([]byte, error) {
	if err := validateDemonlistURL(apiURL); err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
			continue
		}
		return body, nil
	}
	return nil, lastErr
}

func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ctx := r.Context()
	players := playersForLeaderboard(ctx)

	type job struct {
		name string
	}
	jobs := make(chan job, len(players))
	var mu sync.Mutex
	result := make([]FullPlayerData, 0, len(players))

	var wg sync.WaitGroup
	workerCount := 5
	if len(players) < workerCount {
		workerCount = len(players)
	}

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-jobs:
					if !ok {
						return
					}
					entry := FullPlayerData{Name: j.name}
					u1 := fmt.Sprintf("https://api.demonlist.org/leaderboard/user/list?search=%s&limit=1", url.QueryEscape(j.name))
					if body, err := fetchAPIWithRetry(ctx, u1, 2); err == nil {
						json.Unmarshal(body, &entry.Data)
					}
					userID := extractUserID(entry.Data, j.name)
					if userID != "" {
						u2 := fmt.Sprintf("https://api.demonlist.org/user/record/list?user_id=%s&limit=50", userID)
						if body, err := fetchAPIWithRetry(ctx, u2, 2); err == nil {
							json.Unmarshal(body, &entry.Records)
						}
					}
					mu.Lock()
					result = append(result, entry)
					mu.Unlock()
				}
			}
		}()
	}
	for _, p := range players {
		jobs <- job{name: p.Name}
	}
	close(jobs)
	wg.Wait()
	writeJSON(w, result)
}

// ──────────────────────────────────────────────
// HANDLER: PLAYERS
// ──────────────────────────────────────────────

func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	players := playersForLeaderboard(r.Context())
	writeJSON(w, players)
}

func handleSavePlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var players []Player
	if err := decodeRequestJSON(w, r, &players); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}
	if len(players) > 200 {
		sendError(w, http.StatusBadRequest, "Слишком много игроков")
		return
	}
	for i, p := range players {
		players[i].Name = sanitizeString(p.Name)
		if len(p.Name) == 0 || len(p.Name) > 32 {
			sendError(w, http.StatusBadRequest, "Некорректные данные игрока")
			return
		}
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("players")
		return tx.Set(docRef, map[string]interface{}{"players": players})
	})
	if err != nil {
		log.Printf("[players] save: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action:  "players.save",
		Details: map[string]int{"count": len(players)},
	})
	writeJSON(w, map[string]bool{"success": true})
}

func handleDeletePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Name = sanitizeString(req.Name)
	if req.Name == "" {
		sendError(w, http.StatusBadRequest, "Имя игрока обязательно")
		return
	}
	if len(req.Name) > 32 {
		sendError(w, http.StatusBadRequest, "Слишком длинное имя игрока")
		return
	}
	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("players")
		doc, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return errors.New("player not found")
			}
			return err
		}
		var data struct {
			Players []Player `json:"players" firestore:"players"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		found := false
		for i, p := range data.Players {
			if p.Name == req.Name {
				data.Players = append(data.Players[:i], data.Players[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return errors.New("player not found")
		}
		return tx.Set(docRef, map[string]interface{}{"players": data.Players})
	})
	if err != nil {
		log.Printf("[players] delete: %v", err)
		if err.Error() == "player not found" {
			sendError(w, http.StatusNotFound, "Игрок не найден")
		} else {
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action:  "players.delete",
		Details: map[string]string{"name": req.Name},
	})
	writeJSON(w, map[string]bool{"success": true})
}

// ──────────────────────────────────────────────
// HANDLER: STAFF
// ──────────────────────────────────────────────

func handleGetStaff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}
	ctx := r.Context()
	doc, err := fsClient.Collection("config").Doc("staff").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			writeJSON(w, []StaffRole{})
			return
		}
		log.Printf("[staff] Get staff doc: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	var data struct {
		Roles []StaffRole `json:"roles" firestore:"roles"`
	}
	if err := doc.DataTo(&data); err != nil {
		log.Printf("[staff] DataTo error: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	writeJSON(w, data.Roles)
}

func handleSaveStaff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var roles []StaffRole
	if err := decodeRequestJSON(w, r, &roles); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	if len(roles) > 50 {
		sendError(w, http.StatusBadRequest, "Слишком много ролей")
		return
	}
	for i := range roles {
		roles[i].Name = sanitizeString(roles[i].Name)
		roles[i].Color = sanitizeString(roles[i].Color)
		if err := validateRoleName(roles[i].Name); err != nil {
			sendError(w, http.StatusBadRequest, "Некорректные данные")
			return
		}
		if roles[i].Color == "" {
			roles[i].Color = "#3b82f6"
		} else if !reHexColor.MatchString(roles[i].Color) {
			sendError(w, http.StatusBadRequest, "Некорректный цвет")
			return
		} else if !strings.HasPrefix(roles[i].Color, "#") {
			roles[i].Color = "#" + roles[i].Color
		}
		for j := range roles[i].Players {
			roles[i].Players[j].Nickname = sanitizeString(roles[i].Players[j].Nickname)
			roles[i].Players[j].Discord = sanitizeString(roles[i].Players[j].Discord)
		}
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		return tx.Set(docRef, map[string]interface{}{"roles": roles})
	})
	if err != nil {
		log.Printf("[staff] save roles: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action:  "staff.save",
		Details: map[string]int{"count": len(roles)},
	})
	writeJSON(w, map[string]bool{"success": true})
}

func handleStaffAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int    `json:"roleIndex"`
		Nickname  string `json:"nickname"`
		Discord   string `json:"discord"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Nickname = sanitizeString(req.Nickname)
	req.Discord = sanitizeString(req.Discord)
	if err := validateNickname(req.Nickname); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}
	if err := validateDiscord(req.Discord); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errors.New("invalid role index")
		}
		player := StaffPlayer{Nickname: req.Nickname, Discord: req.Discord}
		data.Roles[req.RoleIndex].Players = append(data.Roles[req.RoleIndex].Players, player)
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})

	if err != nil {
		log.Printf("[staff] add player: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.add",

		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"nickname":  req.Nickname,
		},
	})
	writeJSON(w, map[string]interface{}{
		"success":  true,
		"nickname": req.Nickname,
		"discord":  req.Discord,
	})
}

func handleCreateStaffRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Name = sanitizeString(req.Name)
	req.Color = sanitizeString(req.Color)
	if err := validateRoleName(req.Name); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}
	if req.Color == "" {
		req.Color = "#3b82f6"
	} else if !reHexColor.MatchString(req.Color) {
		sendError(w, http.StatusBadRequest, "Некорректный цвет")
		return
	} else if !strings.HasPrefix(req.Color, "#") {
		req.Color = "#" + req.Color
	}

	ctx := r.Context()
	newRole := StaffRole{Name: req.Name, Color: req.Color, Players: []StaffPlayer{}}
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err != nil {
			if status.Code(err) == codes.NotFound {
				data.Roles = []StaffRole{}
			} else {
				return err
			}
		} else {
			if err := doc.DataTo(&data); err != nil {
				data.Roles = []StaffRole{}
			}
		}
		data.Roles = append(data.Roles, newRole)
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] create role: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.createRole",

		Details: map[string]string{"name": req.Name, "color": req.Color},
	})
	writeJSON(w, map[string]interface{}{
		"success": true,
		"name":    req.Name,
		"color":   req.Color,
		"players": []StaffPlayer{},
	})
}

func handleDeleteStaffRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int `json:"roleIndex"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errors.New("invalid role index")
		}
		data.Roles = append(data.Roles[:req.RoleIndex], data.Roles[req.RoleIndex+1:]...)
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] delete role: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.deleteRole",

		Details: map[string]int{"roleIndex": req.RoleIndex},
	})
	writeJSON(w, map[string]bool{"success": true})
}

func handleUpdateStaffRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex    int           `json:"roleIndex"`
		Name         string        `json:"name"`
		Color        string        `json:"color"`
		Players      []StaffPlayer `json:"players"`
		TiersEnabled *bool         `json:"tiersEnabled"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Name = sanitizeString(req.Name)
	req.Color = sanitizeString(req.Color)
	if err := validateRoleName(req.Name); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}
	if req.Color == "" {
		req.Color = "#3b82f6"
	} else if !reHexColor.MatchString(req.Color) {
		sendError(w, http.StatusBadRequest, "Некорректный цвет")
		return
	} else if !strings.HasPrefix(req.Color, "#") {
		req.Color = "#" + req.Color
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errors.New("invalid role index")
		}
		data.Roles[req.RoleIndex].Name = req.Name
		data.Roles[req.RoleIndex].Color = req.Color
		if req.Players != nil {
			data.Roles[req.RoleIndex].Players = req.Players
		}
		if req.TiersEnabled != nil {
			data.Roles[req.RoleIndex].TiersEnabled = *req.TiersEnabled
		}
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] update role: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.updateRole",

		Details: map[string]interface{}{"roleIndex": req.RoleIndex, "name": req.Name, "color": req.Color},
	})
	writeJSON(w, map[string]bool{"success": true})
}

func handleStaffRole(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleCreateStaffRole(w, r)
	case http.MethodPut:
		handleUpdateStaffRole(w, r)
	case http.MethodDelete:
		handleDeleteStaffRole(w, r)
	default:
		methodNotAllowed(w, "POST, PUT, DELETE")
	}
}

func handleStaffRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int    `json:"roleIndex"`
		Nickname  string `json:"nickname"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Nickname = sanitizeString(req.Nickname)

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errors.New("invalid role index")
		}
		players := data.Roles[req.RoleIndex].Players
		found := false
		for i, p := range players {
			if p.Nickname == req.Nickname {
				data.Roles[req.RoleIndex].Players = append(players[:i], players[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return errors.New("player not found")
		}
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] remove player: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.removePlayer",

		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"nickname":  req.Nickname,
		},
	})
	writeJSON(w, map[string]bool{"success": true})
}

func handleReorderStaffRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int    `json:"roleIndex"`
		Direction string `json:"direction"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		idx := req.RoleIndex
		target := idx - 1
		if req.Direction == "down" {
			target = idx + 1
		}
		if target < 0 || target >= len(data.Roles) {
			return errors.New("invalid move")
		}
		data.Roles[idx], data.Roles[target] = data.Roles[target], data.Roles[idx]
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] reorder: %v", err)
		if err.Error() == "invalid move" {
			sendError(w, http.StatusBadRequest, "Некорректное перемещение")
		} else {
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.reorder",

		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"direction": req.Direction,
		},
	})
	writeJSON(w, map[string]bool{"success": true})
}

func handleGetStaffTiers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}
	ctx := r.Context()
	doc, err := fsClient.Collection("config").Doc("staff").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			writeJSON(w, map[string]interface{}{"gp": []StaffTierEntry{}, "deco": []StaffTierEntry{}})
			return
		}
		log.Printf("[staff] Get tiers doc: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	var raw map[string]interface{}
	if err := doc.DataTo(&raw); err != nil {
		log.Printf("[staff] tiers DataTo error: %v", err)
		writeJSON(w, map[string]interface{}{"gp": []StaffTierEntry{}, "deco": []StaffTierEntry{}})
		return
	}
	gpTiers := []StaffTierEntry{}
	decoTiers := []StaffTierEntry{}
	if gpRaw, ok := raw["gp_tiers"]; ok {
		if gpArr, ok := gpRaw.([]interface{}); ok {
			for _, item := range gpArr {
				if m, ok := item.(map[string]interface{}); ok {
					nickname, _ := m["nickname"].(string)
					tier, _ := m["tier"].(string)
					gpTiers = append(gpTiers, StaffTierEntry{Nickname: nickname, Tier: tier})
				}
			}
		}
	}
	if decoRaw, ok := raw["deco_tiers"]; ok {
		if decoArr, ok := decoRaw.([]interface{}); ok {
			for _, item := range decoArr {
				if m, ok := item.(map[string]interface{}); ok {
					nickname, _ := m["nickname"].(string)
					tier, _ := m["tier"].(string)
					decoTiers = append(decoTiers, StaffTierEntry{Nickname: nickname, Tier: tier})
				}
			}
		}
	}
	writeJSON(w, map[string]interface{}{"gp": gpTiers, "deco": decoTiers})
}

func handleSetStaffTier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		Category string `json:"category"`
		Nickname string `json:"nickname"`
		Tier     string `json:"tier"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Category = sanitizeString(req.Category)
	req.Nickname = sanitizeString(req.Nickname)
	req.Tier = sanitizeString(req.Tier)

	if req.Category != "gp" && req.Category != "deco" {
		sendError(w, http.StatusBadRequest, "Некорректная категория")
		return
	}
	if req.Nickname == "" || len(req.Nickname) > 32 {
		sendError(w, http.StatusBadRequest, "Некорректный ник")
		return
	}
	validTiers := map[string]bool{"priority": true, "base": true, "reserve": true, "na": true}
	if !validTiers[req.Tier] {
		sendError(w, http.StatusBadRequest, "Некорректный тир")
		return
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return tx.Set(docRef, map[string]interface{}{
					req.Category + "_tiers": []StaffTierEntry{{Nickname: req.Nickname, Tier: req.Tier}},
				})
			}
			return err
		}
		var data struct {
			Roles     []StaffRole      `json:"roles" firestore:"roles"`
			GPTiers   []StaffTierEntry `json:"gp_tiers" firestore:"gp_tiers"`
			DecoTiers []StaffTierEntry `json:"deco_tiers" firestore:"deco_tiers"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}

		var tiers *[]StaffTierEntry
		if req.Category == "gp" {
			tiers = &data.GPTiers
		} else {
			tiers = &data.DecoTiers
		}

		found := false
		for i, entry := range *tiers {
			if entry.Nickname == req.Nickname {
				(*tiers)[i].Tier = req.Tier
				found = true
				break
			}
		}
		if !found {
			*tiers = append(*tiers, StaffTierEntry{Nickname: req.Nickname, Tier: req.Tier})
		}

		return tx.Set(docRef, map[string]interface{}{
			req.Category + "_tiers": *tiers,
		}, firestore.Merge(firestore.FieldPath{req.Category + "_tiers"}))
	})

	if err != nil {
		log.Printf("[staff] set tier: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.setTier",

		Details: map[string]interface{}{
			"category": req.Category,
			"nickname": req.Nickname,
			"tier":     req.Tier,
		},
	})
	writeJSON(w, map[string]bool{"success": true})
}

// ──────────────────────────────────────────────
// VALIDATION
// ──────────────────────────────────────────────

func validateProjectID(id string) error {
	if id == "" || !reProjectID.MatchString(id) {
		return errors.New("invalid project id")
	}
	return nil
}

func validateNickname(n string) error {
	if len(n) < 1 || len(n) > 32 {
		return errors.New("invalid nickname length")
	}
	return nil
}

func validateDiscord(d string) error {
	if d == "" {
		return nil
	}
	if len(d) > 64 {
		return errors.New("discord too long")
	}
	if !reDiscord.MatchString(d) {
		return errors.New("invalid discord characters")
	}
	return nil
}

func validateRoleName(n string) error {
	if len(n) < 2 || len(n) > 32 {
		return errors.New("invalid role name length")
	}
	if !reRoleName.MatchString(n) {
		return errors.New("invalid role name characters")
	}
	return nil
}

// ──────────────────────────────────────────────
// PLAYERS HELPERS
// ──────────────────────────────────────────────

func loadPlayersFromFirestore(ctx context.Context) ([]Player, error) {
	doc, err := fsClient.Collection("config").Doc("players").Get(ctx)
	if err != nil {
		return nil, err
	}
	var players []Player
	if err := doc.DataTo(&players); err == nil && len(players) > 0 {
		return players, nil
	}
	if raw, ok := doc.Data()["players"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &players); err != nil {
			return nil, err
		}
		if len(players) > 0 {
			return players, nil
		}
	}
	return players, nil
}

func defaultPlayersList() []Player {
	out := make([]Player, len(defaultPlayerNames))
	for i, name := range defaultPlayerNames {
		out[i] = Player{Name: name}
	}
	return out
}

func playersForLeaderboard(ctx context.Context) []Player {
	if fsClient != nil {
		players, err := loadPlayersFromFirestore(ctx)
		if err == nil && len(players) > 0 {
			return players
		}
	}
	return defaultPlayersList()
}

func extractUserID(data interface{}, playerName string) string {
	m, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	d, ok := m["data"].(map[string]interface{})
	if !ok {
		if users, ok := m["users"].([]interface{}); ok {
			return findUserID(users, playerName)
		}
		return ""
	}
	users, ok := d["users"].([]interface{})
	if !ok || len(users) == 0 {
		return ""
	}
	return findUserID(users, playerName)
}

func findUserID(users []interface{}, playerName string) string {
	nl := strings.ToLower(strings.TrimSpace(playerName))
	for _, u := range users {
		user, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		username, _ := user["username"].(string)
		if strings.ToLower(strings.TrimSpace(username)) == nl {
			id, ok := user["id"]
			if !ok {
				continue
			}
			switch v := id.(type) {
			case float64:
				return strconv.FormatInt(int64(v), 10)
			case string:
				return v
			case json.Number:
				return v.String()
			}
		}
	}
	return ""
}

// ──────────────────────────────────────────────
// AUDIT
// ──────────────────────────────────────────────

func auditLog(ctx context.Context, entry AuditEntry) {
	if fsClient == nil {
		return
	}
	entry.CreatedAt = time.Now()
	_, err := fsClient.Collection("audit_log").NewDoc().Set(ctx, entry)
	if err != nil {
		log.Printf("[audit] failed to write log: %v", err)
	}
}

// ──────────────────────────────────────────────
// SECURITY EVENT LOG
// ──────────────────────────────────────────────

type SecurityEvent struct {
	Type      string      `firestore:"type" json:"type"`
	IP        string      `firestore:"ip" json:"ip"`
	Path      string      `firestore:"path" json:"path"`
	Detail    interface{} `firestore:"detail,omitempty" json:"detail,omitempty"`
	CreatedAt time.Time   `firestore:"createdAt" json:"createdAt"`
}

func securityEvent(ctx context.Context, eventType, ip, path string, detail interface{}) {
	log.Printf("[security] %s ip=%s path=%s", eventType, ip, path)
	if fsClient == nil {
		return
	}
	_, err := fsClient.Collection("security_events").NewDoc().Set(ctx, SecurityEvent{
		Type:      eventType,
		IP:        ip,
		Path:      path,
		Detail:    detail,
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Printf("[security] write failed: %v", err)
	}

	alertSecurityEvent(eventType, ip, path, detail)
}

// ──────────────────────────────────────────────
// DISCORD WEBHOOK ALERTS (Worker Pool)
// ──────────────────────────────────────────────

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type alertMessage struct {
	eventType string
	ip        string
	path      string
	detail    interface{}
	body      []byte
}

var (
	alertQueue    chan alertMessage
	discordHTTP   = &http.Client{Timeout: 5 * time.Second}
	discordActive atomic.Bool
)

const (
	alertQueueCapacity = 300
	discordRateDelay   = 220 * time.Millisecond
)

var alertColors = map[string]int{
	"honeypot_triggered":         0xff0000,
	"honeypot_token_blacklisted": 0xff4444,
	"ip_mismatch":                0xff6600,
	"login_failed":               0xffaa00,
	"rate_limit_exceeded":        0xffcc00,
	"backoff_blocked":            0xff0000,
	"bot_blocked":                0x9933ff,
	"blocked_path":               0x9933ff,
	"captcha_failed":             0xffaa00,
	"oversized_request":          0xff3333,
}

var alertEmoji = map[string]string{
	"honeypot_triggered":         "🚨",
	"honeypot_token_blacklisted": "🔒",
	"ip_mismatch":                "⚠️",
	"login_failed":               "🔐",
	"rate_limit_exceeded":        "🚫",
	"backoff_blocked":            "⛔",
	"bot_blocked":                "🤖",
	"blocked_path":               "🗂️",
	"captcha_failed":             "🧩",
	"oversized_request":          "📦",
}

var alertTitles = map[string]string{
	"honeypot_triggered":         "Обнаружена попытка взлома",
	"honeypot_token_blacklisted": "Токен заблокирован",
	"ip_mismatch":                "Смена IP-адреса",
	"login_failed":               "Неудачный вход",
	"rate_limit_exceeded":        "Превышен лимит запросов",
	"backoff_blocked":            "Заблокирован за нарушения",
	"bot_blocked":                "Заблокирован бот",
	"blocked_path":               "Заблокирован путь",
	"captcha_failed":             "Неверная CAPTCHA",
	"oversized_request":          "Слишком большой запрос",
}

var alertFieldNames = map[string]string{
	"method": "Метод",
	"ua":     "Браузер",
	"reason": "Причина",
	"jti":    "ID токена",
	"max":    "Лимит",
}

var criticalEvents = map[string]bool{
	"honeypot_triggered":         true,
	"honeypot_token_blacklisted": true,
	"ip_mismatch":                true,
	"backoff_blocked":            true,
	"bot_blocked":                true,
	"blocked_path":               true,
}

func StartAlertWorker() {
	webhookURL := os.Getenv("DISCORD_SECURITY_WEBHOOK")
	if webhookURL == "" {
		log.Println("[discord] DISCORD_SECURITY_WEBHOOK not set, alerts disabled")
		return
	}

	alertQueue = make(chan alertMessage, alertQueueCapacity)
	discordActive.Store(true)
	log.Printf("[discord] worker started (queue=%d, rate=%v)", alertQueueCapacity, discordRateDelay)

	go func() {
		for msg := range alertQueue {
			if err := sendDiscordPayload(msg.body); err != nil {
				log.Printf("[discord] send failed: %v", err)
			}
			time.Sleep(discordRateDelay)
		}
	}()
}

func sendDiscordPayload(body []byte) error {
	webhookURL := os.Getenv("DISCORD_SECURITY_WEBHOOK")
	if webhookURL == "" {
		return nil
	}
	resp, err := discordHTTP.Post(webhookURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func alertSecurityEvent(eventType, ip, path string, detail interface{}) {
	log.Printf("[security] %s ip=%s path=%s", eventType, ip, path)

	if !discordActive.Load() {
		return
	}

	emoji := alertEmoji[eventType]
	if emoji == "" {
		emoji = "🛡️"
	}
	color := alertColors[eventType]
	if color == 0 {
		color = 0x3b82f6
	}
	title := alertTitles[eventType]
	if title == "" {
		title = eventType
	}

	fields := []discordField{
		{Name: "IP", Value: "`" + ip + "`", Inline: true},
		{Name: "Путь", Value: "`" + path + "`", Inline: true},
	}

	if detailMap, ok := detail.(map[string]string); ok {
		for k, v := range detailMap {
			fieldName := alertFieldNames[k]
			if fieldName == "" {
				fieldName = k
			}
			fields = append(fields, discordField{Name: fieldName, Value: "`" + v + "`", Inline: true})
		}
	} else if detailMap, ok := detail.(map[string]int); ok {
		for k, v := range detailMap {
			fieldName := alertFieldNames[k]
			if fieldName == "" {
				fieldName = k
			}
			fields = append(fields, discordField{Name: fieldName, Value: fmt.Sprintf("`%d`", v), Inline: true})
		}
	}

	embed := discordEmbed{
		Title:       emoji + " " + title,
		Description: "SMLT Leaderboard — Тревога безопасности",
		Color:       color,
		Fields:      fields,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	payload := discordPayload{Embeds: []discordEmbed{embed}}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[discord] marshal failed: %v", err)
		return
	}

	select {
	case alertQueue <- alertMessage{eventType: eventType, body: body}:
	default:
		log.Printf("[discord] queue full, dropping alert: %s", eventType)
	}
}

func alertLoginFailure(ip string, reason string) {
	alertSecurityEvent("login_failed", ip, "/api/login", map[string]string{"reason": reason})
}

func alertHoneypot(ip, path, method, ua string) {
	alertSecurityEvent("honeypot_triggered", ip, path, map[string]string{
		"method": method,
		"ua":     ua,
	})
}

func alertIPMismatch(ip, path string) {
	alertSecurityEvent("ip_mismatch", ip, path, nil)
}

type discordComponent struct {
	Type       int              `json:"type"`
	Style      int              `json:"style,omitempty"`
	Label      string           `json:"label,omitempty"`
	CustomID   string           `json:"custom_id,omitempty"`
	Components []discordComponent `json:"components,omitempty"`
}

type discordPayloadWithComponents struct {
	Embeds     []discordEmbed      `json:"embeds"`
	Components []discordComponent  `json:"components,omitempty"`
}

func alertWithBanButtons(eventType, ip, path, ua, fingerprint string) {
	if !discordActive.Load() {
		return
	}

	emoji := alertEmoji[eventType]
	if emoji == "" {
		emoji = "🛡️"
	}
	color := alertColors[eventType]
	if color == 0 {
		color = 0x3b82f6
	}
	title := alertTitles[eventType]
	if title == "" {
		title = eventType
	}

	fields := []discordField{
		{Name: "IP", Value: "`" + ip + "`", Inline: true},
		{Name: "Путь", Value: "`" + path + "`", Inline: true},
		{Name: "Устройство", Value: "`" + fingerprint + "`", Inline: false},
	}
	if ua != "" {
		fields = append(fields, discordField{Name: "Браузер", Value: "`" + ua + "`", Inline: false})
	}

	embed := discordEmbed{
		Title:       emoji + " " + title,
		Description: "SMLT Leaderboard — Тревога безопасности",
		Color:       color,
		Fields:      fields,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	fp := fingerprint
	components := []discordComponent{
		{
			Type: 1,
			Components: []discordComponent{
				{Type: 2, Style: 1, Label: "ban 1ч", CustomID: "ban|1h|" + fp},
				{Type: 2, Style: 1, Label: "ban 24ч", CustomID: "ban|24h|" + fp},
				{Type: 2, Style: 1, Label: "ban 7д", CustomID: "ban|7d|" + fp},
				{Type: 2, Style: 4, Label: "ban навсегда", CustomID: "ban|perm|" + fp},
				{Type: 2, Style: 3, Label: "unban", CustomID: "ban|unban|" + fp},
			},
		},
	}

	payload := discordPayloadWithComponents{
		Embeds:     []discordEmbed{embed},
		Components: components,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[discord] marshal failed: %v", err)
		return
	}

	select {
	case alertQueue <- alertMessage{eventType: eventType, body: body}:
	default:
		log.Printf("[discord] queue full, dropping alert: %s", eventType)
	}
}

// ──────────────────────────────────────────────
// HONEYPOT TRAPS
// ──────────────────────────────────────────────

var honeypotPaths = map[string]bool{
	"/api/admin":       true,
	"/api/admin/":      true,
	"/api/admin/login": true,
	"/api/admin/panel": true,
	"/api/debug":       true,
	"/api/debug/":      true,
	"/api/internal":    true,
	"/api/internal/":   true,
	"/api/health":      true,
	"/api/config":      true,
	"/api/users":       true,
	"/api/users/":      true,
	"/api/user":        true,
	"/api/auth":        true,
	"/api/auth/":       true,
	"/api/v1":          true,
	"/api/v1/":         true,
	"/_debug":          true,
	"/wp-admin":        true,
	"/wp-login.php":    true,
	"/.env":            true,
	"/.git/config":     true,
	"/.git/HEAD":       true,
	"/phpmyadmin":      true,
	"/admin":           true,
	"/admin/":          true,
	"/debug":           true,
	"/server-status":   true,
	"/.well-known":     true,
	"/api/swagger":     true,
	"/api/docs":        true,
	"/api/graphql":     true,
}

func isHoneypot(path string) bool {
	cleaned := strings.TrimSuffix(path, "/")
	if honeypotPaths[cleaned] || honeypotPaths[cleaned+"/"] {
		return true
	}
	for p := range honeypotPaths {
		if strings.HasPrefix(cleaned, p) {
			return true
		}
	}
	return false
}

func handleHoneypot(w http.ResponseWriter, r *http.Request) {
	ip := getRealIP(r)

	securityEvent(r.Context(), "honeypot_triggered", ip, r.URL.Path, map[string]string{
		"method": r.Method,
		"ua":     r.UserAgent(),
	})
	alertHoneypot(ip, r.URL.Path, r.Method, r.UserAgent())

	if cookie, err := r.Cookie("__Host-auth_token"); err == nil && cookie.Value != "" {
		claims := &jwt.MapClaims{}
		if _, parseErr := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			kid, _ := t.Header["kid"].(string)
			return lookupJWTSecret(kid)
		}); parseErr == nil {
			if jti, ok := (*claims)["jti"].(string); ok && jti != "" {
				blacklistToken(r.Context(), jti)
				securityEvent(r.Context(), "honeypot_token_blacklisted", ip, r.URL.Path, map[string]string{
					"jti": jti,
				})
				alertSecurityEvent("honeypot_token_blacklisted", ip, r.URL.Path, map[string]string{
					"jti": jti,
				})
			}
		}
	}

	time.Sleep(time.Duration(50+mathrand.IntN(200)) * time.Millisecond)
	sendError(w, http.StatusNotFound, "Роут не найден")
}

// ──────────────────────────────────────────────
// EXPONENTIAL BACKOFF TRACKER
// ──────────────────────────────────────────────

type backoffEntry struct {
	violations   int
	blockedUntil time.Time
}

type backoffTracker struct {
	mu      sync.Mutex
	entries map[string]*backoffEntry
	stopCh  chan struct{}
}

var globalBackoff = newBackoffTracker()

func newBackoffTracker() *backoffTracker {
	t := &backoffTracker{
		entries: make(map[string]*backoffEntry),
		stopCh:  make(chan struct{}),
	}
	go t.cleanup()
	return t
}

func (t *backoffTracker) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			t.mu.Lock()
			for ip, e := range t.entries {
				if now.After(e.blockedUntil) && e.violations < 3 {
					delete(t.entries, ip)
				}
			}
			t.mu.Unlock()
		case <-t.stopCh:
			return
		}
	}
}

func (t *backoffTracker) recordViolation(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	e, ok := t.entries[ip]
	if !ok {
		t.entries[ip] = &backoffEntry{violations: 1}
		return false
	}
	if now.Before(e.blockedUntil) {
		return true
	}
	e.violations++
	if e.violations >= 5 {
		e.blockedUntil = now.Add(30 * time.Minute)
	} else if e.violations >= 3 {
		e.blockedUntil = now.Add(time.Duration(e.violations) * time.Minute)
	}
	return now.Before(e.blockedUntil)
}

func backoffMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getRealIP(r)
		if globalBackoff.recordViolation(ip) {
			securityEvent(r.Context(), "backoff_blocked", ip, r.URL.Path, nil)
			sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
			return
		}
		next(w, r)
	}
}

// ──────────────────────────────────────────────
// HANDLER: SILENT TOKEN REFRESH
// ──────────────────────────────────────────────

func handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	cookie, err := r.Cookie("__Host-auth_token")
	if err != nil {
		sendError(w, http.StatusUnauthorized, "Нет доступа")
		return
	}

	claims := &jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		return lookupJWTSecret(kid)
	})
	if err != nil || !token.Valid {
		sendError(w, http.StatusUnauthorized, "Невалидный токен")
		return
	}

	if jti, ok := (*claims)["jti"].(string); ok && jti != "" {
		if isTokenBlacklisted(r.Context(), jti) {
			sendError(w, http.StatusUnauthorized, "Сессия завершена")
			return
		}
	}

	if err := verifyTokenVersion(r.Context(), claims); err != nil {
		sendError(w, http.StatusUnauthorized, "Сессия устарела")
		return
	}

	exp, _ := (*claims)["exp"].(float64)
	remaining := time.Until(time.Unix(int64(exp), 0))
	if remaining > 1*time.Hour {
		writeJSON(w, map[string]interface{}{"success": true, "refreshed": false, "remaining_seconds": int(remaining.Seconds())})
		return
	}

	ipHash, _ := (*claims)["ip"].(string)
	newJTI, err := generateJTI()
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка обновления")
		return
	}

	tokenVersion := getCurrentTokenVersion(r.Context())
	newExp := time.Now().Add(24 * time.Hour)
	now := time.Now()

	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin": true,
		"exp":   newExp.Unix(),
		"iat":   now.Unix(),
		"ver":   tokenVersion,
		"jti":   newJTI,
		"ip":    ipHash,
	})
	newToken.Header["kid"] = primaryJWTID
	tokenString, err := token.SignedString(primaryJWTKey)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка подписи")
		return
	}

	if oldJTI, ok := (*claims)["jti"].(string); ok && oldJTI != "" {
		blacklistToken(r.Context(), oldJTI)
	}

	setSecureCookie(w, "auth_token", tokenString, 86400)
	securityEvent(r.Context(), "token_refreshed", getRealIP(r), "/api/auth/refresh", map[string]string{
		"old_jti": func() string { j, _ := (*claims)["jti"].(string); return j }(),
		"new_jti": newJTI,
	})
	writeJSON(w, map[string]interface{}{"success": true, "refreshed": true})
}

// ──────────────────────────────────────────────
// MAIN HANDLER (Vercel entry point)
// ──────────────────────────────────────────────

func Handler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[panic] %v\n%s", rec, debug.Stack())
			sendError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-src https://www.youtube.com; object-src 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Del("Server")

	origin := r.Header.Get("Origin")
	if origin != "" {
		allowedOrigins := map[string]bool{
			"https://smltdemonlist.vercel.app": true,
			"https://smlt-demonlist.ru":        true,
			"https://www.smlt-demonlist.ru":    true,
		}
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Requested-With, X-Admin-Path-Key")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := requestPath(r)

	if path == "/api/discord/interactions" {
		handleDiscordInteraction(w, r)
		return
	}

	if isHoneypot(path) {
		handleHoneypot(w, r)
		return
	}

	mux := map[string]http.HandlerFunc{
		"/api/captcha":           rateLimitMiddleware(30)(handleCaptcha),
		"/api/login":             rateLimitLoginMiddleware(handleLogin),
		"/api/logout":            rateLimitLoginMiddleware(handleLogout),
		"/api/verify":            rateLimitMiddleware(60)(handleVerify),
		"/api/csrf-token":        rateLimitMiddleware(30)(handleGetCSRFToken),
		"/api/auth/refresh":      rateLimitMiddleware(10)(handleRefreshToken),
		"/api/leaderboard":       rateLimitMiddleware(30)(handleLeaderboard),
		"/api/staff":             rateLimitMiddleware(60)(handleGetStaff),
		"/api/security/dashboard": rateLimitMiddleware(10)(authMiddleware(handleSecurityDashboard)),
		"/api/knock-knock-admin": rateLimitMiddleware(10)(authMiddleware(csrfMiddleware(handleAdminKnock))),
		"/api/staff/add":         rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffAdd)))),
		"/api/staff/role":        rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffRole)))),
		"/api/staff/remove":      rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffRemove)))),
		"/api/staff/reorder":     rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleReorderStaffRoles)))),
		"/api/staff/tiers":       rateLimitMiddleware(60)(handleGetStaffTiers),
		"/api/staff/tier":        rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSetStaffTier)))),
		"/api/staff/save":        rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSaveStaff)))),
		"/api/projects":          rateLimitMiddleware(60)(handleGetProjects),
		"/api/projects/save":     rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSaveProjects)))),
		"/api/players":           rateLimitMiddleware(60)(handleGetPlayers),
		"/api/players/save":      rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSavePlayers)))),
		"/api/players/delete":    rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleDeletePlayer)))),
	}

	h, ok := mux[path]
	if !ok {
		path = strings.TrimSuffix(path, "/")
		h, ok = mux[path]
	}
	if ok {
		gzipMiddleware(botDetectionMiddleware(h))(w, r)
		return
	}
	sendError(w, http.StatusNotFound, "Роут не найден")
}
