package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
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
	firebase "firebase.google.com/go"
	"github.com/golang-jwt/jwt/v5"
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

var defaultPlayerNames = []string{
	"samoletik", "paradoxiz", "clokman", "itzslxnq", "H30n41k_GmD",
	"Filkoty", "DarBeast", "Florned", "Marzyiiik", "euphoriak8",
	"npoctou_gamer", "NopanicGD", "CandyCloud22", "Vakum", "Daggit",
	"Loran", "tapxyhh", "SerGio", "Fanim59", "prostoymofficial",
	"toxik blaze", "NatrixGMD", "toxatort", "SpaceRS", "yeahme",
	"Спини", "Linqwq", "RossceorpGD", "69liqu69",
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
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
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
		batch.Set(ref, p, firestore.MergeAll)
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

	writeJSON(w, map[string]bool{"success": true})
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

func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	players := playersForLeaderboard(r.Context())
	writeJSON(w, players)
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

		player := StaffPlayer{
			Nickname: req.Nickname,
			Discord:  req.Discord,
		}
		data.Roles[req.RoleIndex].Players = append(data.Roles[req.RoleIndex].Players, player)
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

	writeJSON(w, map[string]bool{"success": true})
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

	writeJSON(w, map[string]bool{"success": true})
}

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-src https://www.youtube.com; object-src 'none'; base-uri 'none'; form-action 'self'")

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
		"/api/captcha":       rateLimitMiddleware(30)(handleCaptcha),
		"/api/login":         rateLimitLoginMiddleware(handleLogin),
		"/api/logout":        rateLimitLoginMiddleware(handleLogout),
		"/api/verify":        rateLimitMiddleware(60)(handleVerify),
		"/api/csrf-token":    rateLimitMiddleware(30)(handleGetCSRFToken),
		"/api/leaderboard":   rateLimitMiddleware(30)(handleLeaderboard),
		"/api/staff":         rateLimitMiddleware(60)(handleGetStaff),
		"/api/staff/add":     rateLimitMiddleware(30)(authMiddleware(csrfMiddleware(handleStaffAdd))),
		"/api/staff/role":    rateLimitMiddleware(30)(authMiddleware(csrfMiddleware(handleStaffRole))),
		"/api/staff/remove":  rateLimitMiddleware(30)(authMiddleware(csrfMiddleware(handleStaffRemove))),
		"/api/projects":      rateLimitMiddleware(60)(handleGetProjects),
		"/api/projects/save": rateLimitMiddleware(30)(authMiddleware(csrfMiddleware(handleSaveProjects))),
		"/api/players":       rateLimitMiddleware(60)(handleGetPlayers),
	}

	if h, ok := mux[path]; ok {
		gzipMiddleware(h)(w, r)
		return
	}

	path = strings.TrimSuffix(path, "/")
	if h, ok := mux[path]; ok {
		gzipMiddleware(h)(w, r)
		return
	}

	sendError(w, http.StatusNotFound, "Роут не найден")
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
