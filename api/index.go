package handler

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
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
			setSecureCookie(w, "csrf_token", tokenStr, 3600)
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

func handleCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ensureCaptcha()
	id, b64s, _, err := captchaInst.Generate()
	if err != nil {
		log.Printf("[captcha] generate: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка генерации капчи")
		return
	}
	writeJSON(w, map[string]string{
		"captchaId":    id,
		"captchaImage": b64s,
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
	setSecureCookie(w, "csrf_token", tokenStr, 3600)
	writeJSON(w, map[string]string{"token": tokenStr})
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

func generateJTI() string {
	buf := make([]byte, 16)
	for tries := 0; tries < 3; tries++ {
		if _, err := rand.Read(buf); err == nil {
			return hex.EncodeToString(buf)
		}
	}
	return fmt.Sprintf("%x-%d", time.Now().UnixNano(), os.Getpid())
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
		sendError(w, http.StatusUnauthorized, "Неверные учетные данные")
		return
	}
	tokenVersion := getCurrentTokenVersion(r.Context())
	exp := time.Now().Add(24 * time.Hour)
	now := time.Now()
	jti := generateJTI()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin": true,
		"exp":   exp.Unix(),
		"iat":   now.Unix(),
		"ver":   tokenVersion,
		"jti":   jti,
	})
	token.Header["kid"] = primaryJWTID
	tokenString, err := token.SignedString(primaryJWTKey)
	if err != nil {
		log.Printf("[login] token signing: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка выдачи токена")
		return
	}
	setSecureCookie(w, "auth_token", tokenString, 86400)
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
	if cookie, err := r.Cookie("auth_token"); err == nil {
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
		Action:  "staff.add",

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
		Action:  "staff.createRole",

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
		Action:  "staff.deleteRole",

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
		Action:  "staff.updateRole",

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
		Action:  "staff.removePlayer",

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
		Action:  "staff.reorder",

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
			Roles    []StaffRole     `json:"roles" firestore:"roles"`
			GPTiers  []StaffTierEntry `json:"gp_tiers" firestore:"gp_tiers"`
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
		Action:  "staff.setTier",

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
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-src https://www.youtube.com; object-src 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Del("Server")

	origin := r.Header.Get("Origin")
	if origin != "" {
		allowedOrigins := map[string]bool{
			"https://smltdemonlist.vercel.app": true,
			"https://smlt-demonlist.ru":         true,
			"https://www.smlt-demonlist.ru":     true,
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

	mux := map[string]http.HandlerFunc{
		"/api/captcha":            rateLimitMiddleware(30)(handleCaptcha),
		"/api/login":              rateLimitLoginMiddleware(handleLogin),
		"/api/logout":             rateLimitLoginMiddleware(handleLogout),
		"/api/verify":             rateLimitMiddleware(60)(handleVerify),
		"/api/csrf-token":         rateLimitMiddleware(30)(handleGetCSRFToken),
		"/api/leaderboard":        rateLimitMiddleware(30)(handleLeaderboard),
		"/api/staff":              rateLimitMiddleware(60)(handleGetStaff),
		"/api/knock-knock-admin":  rateLimitMiddleware(10)(authMiddleware(handleAdminKnock)),
		"/api/staff/add":          rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffAdd)))),
		"/api/staff/role":         rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffRole)))),
		"/api/staff/remove":       rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffRemove)))),
		"/api/staff/reorder":      rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleReorderStaffRoles)))),
		"/api/staff/tiers":         rateLimitMiddleware(60)(handleGetStaffTiers),
		"/api/staff/tier":          rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSetStaffTier)))),
		"/api/projects":            rateLimitMiddleware(60)(handleGetProjects),
		"/api/projects/save":      rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSaveProjects)))),
		"/api/players":             rateLimitMiddleware(60)(handleGetPlayers),
		"/api/players/save":        rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSavePlayers)))),
		"/api/players/delete":      rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleDeletePlayer)))),
	}

	h, ok := mux[path]
	if !ok {
		path = strings.TrimSuffix(path, "/")
		h, ok = mux[path]
	}
	if ok {
		gzipMiddleware(h)(w, r)
		return
	}
	sendError(w, http.StatusNotFound, "Роут не найден")
}
