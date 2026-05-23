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
	"github.com/microcosm-cc/bluemonday"
	"github.com/mojocn/base64Captcha"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	fsClient *firestore.Client
	fsOnce   sync.Once
	fsErr    error

	httpClient = &http.Client{Timeout: 10 * time.Second}

	trustProxy      bool
	maxRequestBody  = int64(1024 * 1024)
	primaryJWTKey   []byte
	primaryJWTID    string

	globalRateLimiter rateLimiter
	rlOnce            sync.Once

	rateLimitSalt string
	saltOnce      sync.Once

	jwtSecrets     []jwtKey
	jwtSecretsMu   sync.RWMutex
	jwtSecretsOnce sync.Once

	captchaStore base64Captcha.Store
	captchaInst  *base64Captcha.Captcha
	captchaOnce  sync.Once
)

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
		return err
	}
	return nil
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
	doc.DataTo(&data)
	if time.Now().After(data.ExpiresAt) {
		if clear {
			s.client.Collection("captcha").Doc(id).Delete(ctx)
		}
		return ""
	}
	if clear {
		s.client.Collection("captcha").Doc(id).Delete(ctx)
	}
	return data.Value
}

func (s *firestoreCaptchaStore) Verify(id, answer string, clear bool) bool {
	return s.Get(id, clear) == answer
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

type AuditEntry struct {
	Action    string      `json:"action" firestore:"action"`
	AdminIP   string      `json:"adminIp" firestore:"adminIp"`
	Details   interface{} `json:"details" firestore:"details"`
	CreatedAt time.Time   `json:"createdAt" firestore:"createdAt"`
}

type jwtKey struct {
	Secret []byte
	ID     string
}

var defaultPlayerNames = []string{
	"samoletik", "paradoxiz", "clokman", "itzslxnq", "H30n41k_GmD",
	"Filkoty", "DarBeast", "Florned", "Marzyiiik", "euphoriak8",
	"npoctou_gamer", "NopanicGD", "CandyCloud22", "Vakum", "Daggit",
	"Loran", "tapxyhh", "SerGio", "Fanim59", "prostoymofficial",
	"toxik blaze", "NatrixGMD", "toxatort", "SpaceRS", "yeahme",
	"Спини", "Linqwq", "RossceorpGD", "69liqu69",
}

var (
	reProjectID     = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)
	reAlphanumeric  = regexp.MustCompile(`^[a-zA-Z0-9 _.\-]+$`)
	reDiscord       = regexp.MustCompile(`^[a-zA-Z0-9 _.\-#]+$`)
	reVideoID       = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	reRoleName      = regexp.MustCompile(`^[a-zA-Z0-9 _.\-]+$`)
	reHexColor      = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

func init() {
	trustProxy = os.Getenv("TRUST_PROXY") == "true" || os.Getenv("VERCEL") == "1"
	initFirestore()
	initRateLimiter()
	initRateLimitSalt()
	initJWTSecrets()
}

func initRateLimitSalt() {
	saltOnce.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			rateLimitSalt = fmt.Sprintf("%x", time.Now().UnixNano())
			return
		}
		rateLimitSalt = hex.EncodeToString(buf)
	})
}

func initJWTSecrets() {
	jwtSecretsOnce.Do(func() {
		primary := os.Getenv("JWT_SECRET")
		if primary == "" {
			log.Println("[jwt] JWT_SECRET not set, auth will fail")
			return
		}
		primaryJWTKey = []byte(primary)
		primaryJWTID = "1"
		jwtSecrets = append(jwtSecrets, jwtKey{Secret: primaryJWTKey, ID: primaryJWTID})
		for i := 2; ; i++ {
			key := os.Getenv(fmt.Sprintf("JWT_SECRET_%d", i))
			if key == "" {
				break
			}
			jwtSecrets = append(jwtSecrets, jwtKey{Secret: []byte(key), ID: strconv.Itoa(i)})
		}
	})
}

func initFirestore() {
	fsOnce.Do(func() {
		ctx := context.Background()
		creds := os.Getenv("FIREBASE_CREDENTIALS")
		if creds == "" {
			fsErr = errors.New("FIREBASE_CREDENTIALS not set")
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

func requireFirestore(w http.ResponseWriter) bool {
	if fsErr != nil || fsClient == nil {
		sendError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return false
	}
	return true
}

func sendError(w http.ResponseWriter, status int, msg string) {
	log.Printf("[error] %d %s", status, msg)
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
		for i := len(parts) - 1; i >= 0; i-- {
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

func decodeRequestJSON(w http.ResponseWriter, r *http.Request, dest interface{}) error {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
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

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth_token")
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

		if err := verifyTokenVersion(r.Context(), claims); err != nil {
			sendError(w, http.StatusUnauthorized, "Сессия устарела, войдите заново")
			return
		}

		next.ServeHTTP(w, r)
	}
}

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
	if fsClient == nil {
		return nil
	}

	v, ok := (*claims)["ver"].(float64)
	if !ok {
		return errors.New("no token version")
	}
	requiredVer := int64(v)

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
	doc.DataTo(&cfg)

	if cfg.TokenVersion > requiredVer {
		return errors.New("token version too old")
	}
	return nil
}

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

func handleVerify(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]bool{"success": false})
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
		json.NewEncoder(w).Encode(map[string]bool{"success": false})
		return
	}

	if err := verifyTokenVersion(r.Context(), claims); err != nil {
		json.NewEncoder(w).Encode(map[string]bool{"success": false})
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

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

	json.NewEncoder(w).Encode(map[string]string{
		"captchaId":    id,
		"captchaImage": b64s,
	})
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

	if !captchaStore.Verify(req.CaptchaID, req.CaptchaValue, true) {
		sendError(w, http.StatusUnauthorized, "Неверный код с картинки")
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
		sendError(w, http.StatusUnauthorized, "Неверный пароль")
		return
	}

	tokenVersion := getCurrentTokenVersion(r.Context())
	exp := time.Now().Add(24 * time.Hour)
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin": true,
		"exp":   exp.Unix(),
		"iat":   now.Unix(),
		"ver":   tokenVersion,
	})

	token.Header["kid"] = primaryJWTID

	tokenString, err := token.SignedString(primaryJWTKey)
	if err != nil {
		log.Printf("[login] token signing: %v", err)
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

func getCurrentTokenVersion(ctx context.Context) int64 {
	if fsClient == nil {
		return 1
	}
	doc, err := fsClient.Collection("config").Doc("auth").Get(ctx)
	if err != nil {
		return 1
	}
	var cfg struct {
		TokenVersion int64 `json:"tokenVersion" firestore:"tokenVersion"`
	}
	doc.DataTo(&cfg)
	if cfg.TokenVersion < 1 {
		return 1
	}
	return cfg.TokenVersion
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
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

	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
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

	for i, p := range projectList {
		projectList[i].Name = sanitizeString(p.Name)
		projectList[i].VideoID = sanitizeString(p.VideoID)
		projectList[i].Comment = sanitizeString(p.Comment)
		projectList[i].Verifier = sanitizeString(p.Verifier)
		for j, part := range projectList[i].Participants {
			projectList[i].Participants[j] = sanitizeString(part)
		}

		if err := validateProjectID(p.ID); err != nil {
			sendError(w, http.StatusBadRequest, "Некорректные данные проекта")
			return
		}
		if len(projectList[i].Name) == 0 || len(projectList[i].Name) > 100 {
			sendError(w, http.StatusBadRequest, "Некорректные данные проекта")
			return
		}
		if projectList[i].VideoID != "" && !reVideoID.MatchString(projectList[i].VideoID) {
			sendError(w, http.StatusBadRequest, "Некорректные данные проекта")
			return
		}
		if len(projectList[i].Comment) > 1000 {
			sendError(w, http.StatusBadRequest, "Некорректные данные проекта")
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
		log.Printf("[projects] batch commit: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	auditLog(r.Context(), AuditEntry{
		Action:  "projects.save",
		AdminIP: getRealIP(r),
		Details: map[string]int{"count": len(projectList)},
	})

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

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

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()

		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
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
			for j := range jobs {
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
		}()
	}

	for _, p := range players {
		jobs <- job{name: p.Name}
	}
	close(jobs)

	wg.Wait()
	json.NewEncoder(w).Encode(result)
}

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
			json.NewEncoder(w).Encode([]StaffRole{})
			return
		}
		log.Printf("[staff] Get staff doc: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	var data struct {
		Roles []StaffRole `json:"roles" firestore:"roles"`
	}
	doc.DataTo(&data)
	json.NewEncoder(w).Encode(data.Roles)
}

func handleStaffAdd(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("[staff] add player: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	auditLog(r.Context(), AuditEntry{
		Action:  "staff.add",
		AdminIP: getRealIP(r),
		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"nickname":  req.Nickname,
		},
	})

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
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
	}

	ctx := r.Context()
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
			doc.DataTo(&data)
		}

		data.Roles = append(data.Roles, StaffRole{Name: req.Name, Color: req.Color, Players: []StaffPlayer{}})
		return tx.Set(docRef, data)
	})

	if err != nil {
		log.Printf("[staff] create role: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	auditLog(r.Context(), AuditEntry{
		Action:  "staff.createRole",
		AdminIP: getRealIP(r),
		Details: map[string]string{"name": req.Name, "color": req.Color},
	})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "name": req.Name, "color": req.Color, "players": []StaffPlayer{}})
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
		doc.DataTo(&data)

		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errors.New("invalid role index")
		}

		data.Roles = append(data.Roles[:req.RoleIndex], data.Roles[req.RoleIndex+1:]...)
		return tx.Set(docRef, data)
	})

	if err != nil {
		log.Printf("[staff] delete role: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	auditLog(r.Context(), AuditEntry{
		Action:  "staff.deleteRole",
		AdminIP: getRealIP(r),
		Details: map[string]int{"roleIndex": req.RoleIndex},
	})

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleStaffRole(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleCreateStaffRole(w, r)
	case http.MethodDelete:
		handleDeleteStaffRole(w, r)
	default:
		methodNotAllowed(w, "POST, DELETE")
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
		doc.DataTo(&data)

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
		return tx.Set(docRef, data)
	})

	if err != nil {
		log.Printf("[staff] remove player: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	auditLog(r.Context(), AuditEntry{
		Action:  "staff.removePlayer",
		AdminIP: getRealIP(r),
		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"nickname":  req.Nickname,
		},
	})

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-src https://www.youtube.com; object-src 'none'; base-uri 'none'; form-action 'self'")

	origin := r.Header.Get("Origin")
	if origin != "" {
		allowedOrigins := map[string]bool{
			"https://smlt-demonlist.vercel.app": true,
			"https://smlt-demonlist.ru":         true,
			"https://www.smlt-demonlist.ru":     true,
		}
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Requested-With")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := requestPath(r)

	mux := map[string]http.HandlerFunc{
		"/api/captcha":       rateLimitMiddleware(handleCaptcha),
		"/api/login":         rateLimitLoginMiddleware(handleLogin),
		"/api/logout":        rateLimitLoginMiddleware(handleLogout),
		"/api/verify":        rateLimitMiddleware(handleVerify),
		"/api/csrf-token":    rateLimitMiddleware(handleGetCSRFToken),
		"/api/leaderboard":   rateLimitMiddleware(handleLeaderboard),
		"/api/staff":         rateLimitMiddleware(handleGetStaff),
		"/api/staff/add":     rateLimitMiddleware(authMiddleware(csrfMiddleware(handleStaffAdd))),
		"/api/staff/role":    rateLimitMiddleware(authMiddleware(csrfMiddleware(handleStaffRole))),
		"/api/staff/remove":  rateLimitMiddleware(authMiddleware(csrfMiddleware(handleStaffRemove))),
		"/api/projects":      rateLimitMiddleware(handleGetProjects),
		"/api/projects/save": rateLimitMiddleware(authMiddleware(csrfMiddleware(handleSaveProjects))),
	}

	if h, ok := mux[path]; ok {
		h(w, r)
		return
	}

	path = strings.TrimSuffix(path, "/")
	if h, ok := mux[path]; ok {
		h(w, r)
		return
	}

	sendError(w, http.StatusNotFound, "Роут не найден")
}

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
		if !checkRateLimit(w, r, 60) {
			return
		}
		next(w, r)
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

	if fsClient != nil {
		return checkFirestoreLoginLimit(w, r, key)
	}

	ok, _ := globalRateLimiter.allow(r.Context(), key, 5, time.Minute)
	if !ok {
		sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
		return false
	}
	return true
}

func checkFirestoreLoginLimit(w http.ResponseWriter, r *http.Request, key string) bool {
	ctx := r.Context()
	docRef := fsClient.Collection("rate_limits").Doc(key)

	maxAttempts := 5
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
		doc.DataTo(&data)

		if time.Now().After(data.ResetAt) {
			return tx.Set(docRef, map[string]interface{}{
				"count":   1,
				"resetAt": time.Now().Add(window),
			})
		}

		if data.Count >= maxAttempts {
			return errors.New("rate limit exceeded")
		}

		return tx.Set(docRef, map[string]interface{}{
			"count":   data.Count + 1,
			"resetAt": data.ResetAt,
		})
	})

	if err != nil {
		if err.Error() == "rate limit exceeded" {
			sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
			return false
		}
		log.Printf("[ratelimit] firestore error: %v", err)
		sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
		return false
	}

	return true
}

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

func validateProjectID(id string) error {
	if id == "" || !reProjectID.MatchString(id) {
		return errors.New("invalid project id")
	}
	return nil
}

func validateNickname(n string) error {
	if len(n) < 2 || len(n) > 32 {
		return errors.New("invalid nickname length")
	}
	if !reAlphanumeric.MatchString(n) {
		return errors.New("invalid nickname characters")
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

func requestPath(r *http.Request) string {
	p := r.URL.Path
	if p == "/api" || p == "/api/" {
		return r.RequestURI
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
