package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/option"
)

// === КОНФИГ И ГЛОБАЛКИ ===
var (
	fsClient *firestore.Client
	fsOnce   sync.Once
	fsErr    error

	httpClient = &http.Client{Timeout: 10 * time.Second}

	trustProxy     bool
	maxRequestBody = int64(1024 * 1024)

	jwtBlacklist  Blacklist
	blacklistOnce sync.Once

	globalRateLimiter rateLimiter
	rlOnce            sync.Once
)

type Blacklist interface {
	IsBlacklisted(token string) bool
	Add(token string, exp time.Time)
}

type memoryBlacklist struct {
	mu     sync.RWMutex
	tokens map[string]time.Time
}

func newMemoryBlacklist() Blacklist {
	bl := &memoryBlacklist{tokens: make(map[string]time.Time)}
	// [SECURITY FIX] Фоновая очистка blacklist
	go bl.cleanup()
	return bl
}

func (b *memoryBlacklist) IsBlacklisted(token string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	exp, ok := b.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		return false // Уже протух, можно не считать в блэклисте
	}
	return true
}

func (b *memoryBlacklist) Add(token string, exp time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens[token] = exp
}

func (b *memoryBlacklist) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		now := time.Now()
		b.mu.Lock()
		for t, exp := range b.tokens {
			if now.After(exp) {
				delete(b.tokens, t)
			}
		}
		b.mu.Unlock()
	}
}

// === ТИПЫ ДАННЫХ ===
type StaffPlayer struct {
	Nickname string `json:"nickname" firestore:"nickname"`
	Discord  string `json:"discord" firestore:"discord"`
}

type StaffRole struct {
	Name    string        `json:"name" firestore:"name"`
	Color   string        `json:"color" firestore:"color"`
	Players []StaffPlayer `json:"players" firestore:"players"`
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

// Список по умолчанию (если Firestore пуст)
var defaultPlayerNames = []string{
	"samoletik", "paradoxiz", "clokman", "itzslxnq", "H30n41k_GmD",
	"Filkoty", "DarBeast", "Florned", "Marzyiiik", "euphoriak8",
	"npoctou_gamer", "NopanicGD", "CandyCloud22", "Vakum", "Daggit",
	"Loran", "tapxyhh", "SerGio", "Fanim59", "prostoymofficial",
	"toxik blaze", "NatrixGMD", "toxatort", "SpaceRS", "yeahme",
	"Спини", "Linqwq", "RossceorpGD", "69liqu69",
}

// === ИНИЦИАЛИЗАЦИЯ ===
func init() {
	trustProxy = os.Getenv("TRUST_PROXY") == "true" || os.Getenv("VERCEL") == "1"
	initFirestore()
	initJWTBlacklist()
	initRateLimiter()
}

func initFirestore() {
	fsOnce.Do(func() {
		ctx := context.Background()
		creds := os.Getenv("FIREBASE_CREDENTIALS")
		if creds == "" {
			fsErr = errors.New("FIREBASE_CREDENTIALS не задан")
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

func initJWTBlacklist() {
	blacklistOnce.Do(func() {
		jwtBlacklist = newMemoryBlacklist()
		log.Println("[jwt] Blacklist initialized")
	})
}

func requireFirestore(w http.ResponseWriter) bool {
	if fsErr != nil || fsClient == nil {
		sendError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return false
	}
	return true
}

func jwtSecretKey() []byte {
	return []byte(os.Getenv("JWT_SECRET"))
}

// === УТИЛИТЫ ===
func sendError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	sendError(w, http.StatusMethodNotAllowed, "Метод не разрешен")
}

func remoteAddrIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// [SECURITY FIX] Более надежное определение IP
func getRealIP(r *http.Request) string {
	if trustProxy {
		// Vercel / Cloudflare специфичные заголовки
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
		if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
			return cf
		}
		// X-Forwarded-For берем последний надежный сегмент, если прокси доверенный
		// Но на Vercel X-Real-IP обычно достаточно.
	}
	return remoteAddrIP(r)
}

func hashIP(ip string) string {
	salt := os.Getenv("RATE_LIMIT_SALT")
	if salt == "" {
		salt = "default-salt-12345"
	}
	hash := sha256.Sum256([]byte(ip + salt))
	return hex.EncodeToString(hash[:16])
}

func decodeRequestJSON(w http.ResponseWriter, r *http.Request, dest interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dest)
}

// === MIDDLEWARES ===

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			sendError(w, http.StatusUnauthorized, "Нет доступа")
			return
		}

		tokenString := cookie.Value

		if jwtBlacklist.IsBlacklisted(tokenString) {
			sendError(w, http.StatusUnauthorized, "Сессия аннулирована")
			return
		}

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return jwtSecretKey(), nil
		})

		if err != nil || !token.Valid {
			sendError(w, http.StatusUnauthorized, "Невалидный токен")
			return
		}

		next.ServeHTTP(w, r)
	}
}

// [SECURITY FIX] CSRF Protection
func csrfMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		headerToken := r.Header.Get("X-CSRF-Token")
		cookie, err := r.Cookie("csrf_token")
		if err != nil || cookie.Value == "" || headerToken == "" || headerToken != cookie.Value {
			sendError(w, http.StatusForbidden, "Ошибка CSRF: неверный токен")
			return
		}

		next.ServeHTTP(w, r)
	}
}

// === ХЭНДЛЕРЫ ===

func handleGetCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка генерации токена")
		return
	}
	tokenStr := hex.EncodeToString(token)

	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})

	json.NewEncoder(w).Encode(map[string]string{"token": tokenStr})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var req struct {
		Password     string `json:"password"`
		CaptchaToken string `json:"captchaToken"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой запрос")
		return
	}

	adminHash := os.Getenv("ADMIN_HASH")
	if adminHash == "" || os.Getenv("JWT_SECRET") == "" {
		sendError(w, http.StatusInternalServerError, "Сервер не настроен")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(adminHash), []byte(req.Password)); err != nil {
		sendError(w, http.StatusUnauthorized, "Неверный пароль")
		return
	}

	exp := time.Now().Add(24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin": true,
		"exp":   exp.Unix(),
	})
	tokenString, err := token.SignedString(jwtSecretKey())
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка выдачи токена")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    tokenString,
		Expires:  exp,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if cookie, err := r.Cookie("auth_token"); err == nil {
		// [SECURITY FIX] Добавление в blacklist при логауте
		jwtBlacklist.Add(cookie.Value, time.Now().Add(24*time.Hour))
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

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
		if err != nil {
			break
		}
		var p Project
		if err := doc.DataTo(&p); err != nil {
			continue
		}
		projects = append(projects, p)
	}

	json.NewEncoder(w).Encode(projects)
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
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}

	// [SECURITY FIX] Валидация всех полей проекта
	for _, p := range projectList {
		if err := validateProjectID(p.ID); err != nil {
			sendError(w, http.StatusBadRequest, "Неверный ID проекта: "+err.Error())
			return
		}
		if len(p.Name) == 0 || len(p.Name) > 100 {
			sendError(w, http.StatusBadRequest, "Недопустимая длина имени")
			return
		}
		if len(p.Comment) > 1000 {
			sendError(w, http.StatusBadRequest, "Слишком длинный комментарий")
			return
		}
	}

	ctx := r.Context()
	batch := fsClient.Batch()
	for _, p := range projectList {
		ref := fsClient.Collection("projects").Doc(p.ID)
		batch.Set(ref, p)
	}
	if _, err := batch.Commit(ctx); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// [SECURITY FIX] Валидация URL Demonlist для предотвращения SSRF
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
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			continue
		}

		return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	}
	return nil, lastErr
}

func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	players := playersForLeaderboard(ctx)

	var wg sync.WaitGroup
	var mu sync.Mutex
	result := make([]FullPlayerData, 0, len(players))
	sem := make(chan struct{}, 5) // Ограничиваем параллелизм

	for _, p := range players {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			entry := FullPlayerData{Name: name}
			
			// User Info
			u1 := fmt.Sprintf("https://api.demonlist.org/leaderboard/user/list?search=%s&limit=1", url.QueryEscape(name))
			if body, err := fetchAPIWithRetry(ctx, u1, 2); err == nil {
				json.Unmarshal(body, &entry.Data)
			}

			// Records (если ID найден)
			userID := extractUserID(entry.Data, name)
			if userID != "" {
				u2 := fmt.Sprintf("https://api.demonlist.org/user/record/list?user_id=%s&limit=50", userID)
				if body, err := fetchAPIWithRetry(ctx, u2, 2); err == nil {
					json.Unmarshal(body, &entry.Records)
				}
			}

			mu.Lock()
			result = append(result, entry)
			mu.Unlock()
		}(p.Name)
	}

	wg.Wait()
	json.NewEncoder(w).Encode(result)
}

func handleGetStaff(w http.ResponseWriter, r *http.Request) {
	if !requireFirestore(w) {
		return
	}
	ctx := r.Context()
	doc, err := fsClient.Collection("config").Doc("staff").Get(ctx)
	if err != nil {
		json.NewEncoder(w).Encode([]StaffRole{})
		return
	}
	var data struct {
		Roles []StaffRole `json:"roles" firestore:"roles"`
	}
	doc.DataTo(&data)
	json.NewEncoder(w).Encode(data.Roles)
}

func handleStaffAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RoleIndex int    `json:"roleIndex"`
		Nickname  string `json:"nickname"`
		Discord   string `json:"discord"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}

	if err := validateNickname(req.Nickname); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateDiscord(req.Discord); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
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
		doc.DataTo(&data)

		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errors.New("invalid role index")
		}

		data.Roles[req.RoleIndex].Players = append(data.Roles[req.RoleIndex].Players, StaffPlayer{
			Nickname: req.Nickname,
			Discord:  req.Discord,
		})
		return tx.Set(docRef, data)
	})

	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// Handler — точка входа
func Handler(w http.ResponseWriter, r *http.Request) {
	// [SECURITY FIX] Security Headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-src https://www.youtube.com;")

	path := requestPath(r)
	
	// Роутинг с применением middleware
	mux := map[string]http.HandlerFunc{
		"/api/login":        rateLimitLoginMiddleware(handleLogin),
		"/api/logout":       rateLimitMiddleware(handleLogout),
		"/api/csrf-token":   rateLimitMiddleware(handleGetCSRFToken),
		"/api/leaderboard":  rateLimitMiddleware(handleLeaderboard),
		"/api/staff":        rateLimitMiddleware(authMiddleware(handleGetStaff)),
		"/api/staff/add":    rateLimitMiddleware(authMiddleware(csrfMiddleware(handleStaffAdd))),
		"/api/projects":     rateLimitMiddleware(handleGetProjects),
		"/api/projects/save": rateLimitMiddleware(authMiddleware(csrfMiddleware(handleSaveProjects))),
	}

	if h, ok := mux[path]; ok {
		h(w, r)
		return
	}

	sendError(w, http.StatusNotFound, "Роут не найден")
}

// --- RATE LIMITING ---

type rateLimiter interface {
	allow(ctx context.Context, key string, max int, window time.Duration) (bool, error)
}

type memoryLimiter struct {
	mu   sync.Mutex
	keys map[string]*memBucket
}

type memBucket struct {
	count   int
	resetAt time.Time
}

func newMemoryLimiter() rateLimiter {
	m := &memoryLimiter{keys: make(map[string]*memBucket)}
	// [SECURITY FIX] Фоновая очистка лимитера
	go m.cleanup()
	return m
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

func (m *memoryLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		now := time.Now()
		m.mu.Lock()
		for k, b := range m.keys {
			if now.After(b.resetAt) {
				delete(m.keys, k)
			}
		}
		m.mu.Unlock()
	}
}

func initRateLimiter() {
	rlOnce.Do(func() {
		globalRateLimiter = newMemoryLimiter()
	})
}

func checkRateLimit(w http.ResponseWriter, r *http.Request, max int) bool {
	ip := hashIP(getRealIP(r))
	key := requestPath(r) + ":" + ip
	ok, _ := globalRateLimiter.allow(r.Context(), key, max, time.Minute)
	if !ok {
		sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
		return false
	}
	return true
}

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkRateLimit(w, r, 60) { return }
		next(w, r)
	}
}

func rateLimitLoginMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkRateLimit(w, r, 5) { return }
		next(w, r)
	}
}

// --- VALIDATION ---

func validateProjectID(id string) error {
	if id == "" { return errors.New("empty id") }
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{1,50}$`, id); !matched {
		return errors.New("invalid id format")
	}
	return nil
}

func validateNickname(n string) error {
	if len(n) < 2 || len(n) > 32 { return errors.New("invalid nickname length") }
	return nil
}

func validateDiscord(d string) error {
	if d == "" { return nil }
	if len(d) > 64 { return errors.New("discord too long") }
	return nil
}

func validateRoleName(n string) error {
	if len(n) < 2 || len(n) > 32 { return errors.New("invalid role name length") }
	return nil
}

// --- UTILS ---

func requestPath(r *http.Request) string {
	p := r.URL.Path
	if p == "/api" || p == "/api/" {
		return r.RequestURI // Vercel workaround
	}
	return p
}

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
	// [SECURITY FIX] Safe navigation through nested maps
	d, ok := m["data"].(map[string]interface{})
	if !ok {
		// Fallback if structure is different
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
			id := user["id"]
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
