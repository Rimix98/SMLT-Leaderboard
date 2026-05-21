package handler

import (
	"context"
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
)

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

// [FIXED] Без прокси — только RemoteAddr; за доверенным прокси — заголовки от LB
func getRealIP(r *http.Request) string {
	remoteIP := remoteAddrIP(r)
	if !trustProxy {
		return remoteIP
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		candidate := strings.TrimSpace(strings.Split(xff, ",")[0])
		if parsed := net.ParseIP(candidate); parsed != nil {
			return candidate
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		candidate := strings.TrimSpace(xri)
		if parsed := net.ParseIP(candidate); parsed != nil {
			return candidate
		}
	}
	return remoteIP
}

// [FIXED] Ограничение размера тела + запрет неизвестных полей JSON
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

		token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
			return jwtSecretKey(), nil
		})

		if err != nil || !token.Valid {
			sendError(w, http.StatusUnauthorized, "Невалидный токен")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// === ХЭНДЛЕРЫ ===

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

	if os.Getenv("JWT_SECRET") == "" {
		sendError(w, http.StatusInternalServerError, "Сервер не настроен: JWT_SECRET")
		return
	}
	adminHash := os.Getenv("ADMIN_HASH")
	if adminHash == "" {
		sendError(w, http.StatusInternalServerError, "Сервер не настроен: ADMIN_HASH")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash), []byte(req.Password)); err != nil {
		sendError(w, http.StatusUnauthorized, "Неверный пароль")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin": true,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenString, err := token.SignedString(jwtSecretKey())
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка выдачи токена")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    tokenString,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
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
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   -1,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// [SECURITY FIX] Проверка JWT из HttpOnly-куки для синхронизации с фронтендом
func handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	cookie, err := r.Cookie("auth_token")
	if err != nil {
		sendError(w, http.StatusUnauthorized, "Нет токена")
		return
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
		return jwtSecretKey(), nil
	})

	if err != nil || !token.Valid {
		sendError(w, http.StatusUnauthorized, "Невалидный токен")
		return
	}

	w.WriteHeader(http.StatusOK)
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

	ctx := context.Background()
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

	if len(projectList) > 500 {
		sendError(w, http.StatusBadRequest, "Слишком много проектов")
		return
	}

	for _, p := range projectList {
		if len(p.Name) == 0 || len(p.Name) > 100 {
			sendError(w, http.StatusBadRequest, "Недопустимая длина имени")
			return
		}
		if len(p.Comment) > 1000 {
			sendError(w, http.StatusBadRequest, "Слишком длинный комментарий")
			return
		}
		if len(p.VideoID) > 50 {
			sendError(w, http.StatusBadRequest, "Слишком длинный VideoID")
			return
		}
		if p.ID == "" {
			sendError(w, http.StatusBadRequest, "ID проекта обязателен")
			return
		}
	}

	ctx := context.Background()
	batch := fsClient.Batch()
	for _, p := range projectList {
		ref := fsClient.Collection("projects").Doc(p.ID)
		batch.Set(ref, p)
	}
	if _, err := batch.Commit(ctx); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func formatUserID(id interface{}) string {
	switch v := id.(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case json.Number:
		return v.String()
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func extractUserID(data interface{}, playerName string) string {
	m, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	d, ok := m["data"].(map[string]interface{})
	if !ok {
		return ""
	}
	users, ok := d["users"].([]interface{})
	if !ok || len(users) == 0 {
		return ""
	}

	nl := strings.ToLower(strings.TrimSpace(playerName))
	for _, u := range users {
		user, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		username, _ := user["username"].(string)
		if strings.ToLower(strings.TrimSpace(username)) == nl {
			return formatUserID(user["id"])
		}
	}
	return ""
}

func fetchAPIWithRetry(ctx context.Context, url string, maxRetries int) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			if attempt < maxRetries-1 {
				time.Sleep(time.Second * time.Duration(attempt+1))
			}
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}

		if len(body) == 0 {
			lastErr = errors.New("empty response")
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

	var wg sync.WaitGroup
	var mu sync.Mutex
	result := make([]FullPlayerData, 0, len(players))
	sem := make(chan struct{}, 8)

	for _, p := range players {
		wg.Add(1)
		sem <- struct{}{}

		go func(playerName string) {
			defer wg.Done()
			defer func() { <-sem }()

			entry := FullPlayerData{Name: playerName}
			search := url.QueryEscape(playerName)

			body1, err := fetchAPIWithRetry(ctx, "https://api.demonlist.org/leaderboard/user/list?search="+search+"&limit=50", 3)
			if err != nil {
				log.Printf("[leaderboard] fetch user list for %s: %v", playerName, err)
				mu.Lock()
				result = append(result, entry)
				mu.Unlock()
				return
			}

			var data interface{}
			if err := json.Unmarshal(body1, &data); err != nil {
				log.Printf("[leaderboard] parse user list for %s: %v", playerName, err)
				mu.Lock()
				result = append(result, entry)
				mu.Unlock()
				return
			}

			entry.Data = data
			userID := extractUserID(data, playerName)
			if userID == "" {
				log.Printf("[leaderboard] no user id found for %s", playerName)
				mu.Lock()
				result = append(result, entry)
				mu.Unlock()
				return
			}

			body2, err := fetchAPIWithRetry(ctx, "https://api.demonlist.org/user/record/list?user_id="+url.QueryEscape(userID)+"&limit=50", 3)
			if err != nil {
				log.Printf("[leaderboard] fetch records for %s (id=%s): %v", playerName, userID, err)
				mu.Lock()
				result = append(result, entry)
				mu.Unlock()
				return
			}

			var recs interface{}
			if err := json.Unmarshal(body2, &recs); err != nil {
				log.Printf("[leaderboard] parse records for %s: %v", playerName, err)
				mu.Lock()
				result = append(result, entry)
				mu.Unlock()
				return
			}

			entry.Records = recs
			mu.Lock()
			result = append(result, entry)
			mu.Unlock()
		}(p.Name)
	}

	wg.Wait()
	json.NewEncoder(w).Encode(result)
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
	initFirestore()
	if fsClient != nil {
		players, err := loadPlayersFromFirestore(ctx)
		if err == nil && len(players) > 0 {
			return players
		}
	}
	return defaultPlayersList()
}

func savePlayersToFirestore(ctx context.Context, players []Player) error {
	_, err := fsClient.Collection("config").Doc("players").Set(ctx, map[string]interface{}{
		"players": players,
	})
	return err
}

func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	ctx := context.Background()
	players := playersForLeaderboard(ctx)
	names := make([]string, len(players))
	for i, p := range players {
		names[i] = p.Name
	}
	json.NewEncoder(w).Encode(names)
}

func handleSavePlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}

	var playerList []Player
	if err := decodeRequestJSON(w, r, &playerList); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}

	if len(playerList) > 1000 {
		sendError(w, http.StatusBadRequest, "Слишком много игроков (макс 1000)")
		return
	}

	for _, p := range playerList {
		if len(p.Name) == 0 || len(p.Name) > 100 {
			sendError(w, http.StatusBadRequest, "Недопустимая длина имени игрока")
			return
		}
	}

	ctx := context.Background()
	if err := savePlayersToFirestore(ctx, playerList); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// [CONCURRENCY FIX] Использование транзакций Firestore вместо read-modify-write
func handleDeletePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	if !requireFirestore(w) {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		sendError(w, http.StatusBadRequest, "Имя игрока обязательно")
		return
	}

	ctx := context.Background()
	lowerName := strings.ToLower(name)

	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(fsClient.Collection("config").Doc("players"))
		if err != nil {
			return err
		}

		var players []Player
		if err := doc.DataTo(&players); err != nil {
			if raw, ok := doc.Data()["players"]; ok {
				b, _ := json.Marshal(raw)
				json.Unmarshal(b, &players)
			}
		}

		filtered := make([]Player, 0, len(players))
		removed := false
		for _, p := range players {
			if strings.ToLower(strings.TrimSpace(p.Name)) == lowerName {
				removed = true
				continue
			}
			filtered = append(filtered, p)
		}

		if !removed {
			return errors.New("player_not_found")
		}

		return tx.Set(fsClient.Collection("config").Doc("players"), map[string]interface{}{
			"players": filtered,
		})
	})

	if err != nil {
		if err.Error() == "player_not_found" {
			sendError(w, http.StatusNotFound, "Игрок не найден")
		} else {
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// [FIXED] Vercel rewrite /api/* → /api оставляет Path=/api; берём путь из RequestURI
func requestPath(r *http.Request) string {
	path := r.URL.Path
	if path != "/api" && path != "/api/" {
		return path
	}
	uri := r.RequestURI
	if i := strings.Index(uri, "?"); i >= 0 {
		uri = uri[:i]
	}
	if uri != "" && uri != path {
		return uri
	}
	return path
}

func handleGetStaff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}

	ctx := context.Background()
	doc, err := fsClient.Collection("config").Doc("staff").Get(ctx)
	if err != nil {
		// Если документа нет, возвращаем пустой список
		json.NewEncoder(w).Encode([]StaffRole{})
		return
	}

	var data struct {
		Roles []StaffRole `json:"roles" firestore:"roles"`
	}
	if err := doc.DataTo(&data); err != nil {
		json.NewEncoder(w).Encode([]StaffRole{})
		return
	}

	json.NewEncoder(w).Encode(data.Roles)
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
		RoleName string `json:"roleName"`
		Nickname string `json:"nickname"`
		Discord  string `json:"discord"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}

	if req.RoleName == "" || req.Nickname == "" {
		sendError(w, http.StatusBadRequest, "roleName и nickname обязательны")
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

		found := false
		for i, role := range data.Roles {
			if strings.EqualFold(role.Name, req.RoleName) {
				found = true
				// Check if player already exists
				for _, p := range role.Players {
					if strings.EqualFold(p.Nickname, req.Nickname) {
						return errors.New("player_already_exists")
					}
				}
				// Add player
				data.Roles[i].Players = append(data.Roles[i].Players, StaffPlayer{
					Nickname: req.Nickname,
					Discord:  req.Discord,
				})
				break
			}
		}

		if !found {
			return errors.New("role_not_found")
		}

		return tx.Set(docRef, map[string]interface{}{"roles": data.Roles})
	})

	if err != nil {
		if err.Error() == "player_already_exists" {
			sendError(w, http.StatusConflict, "Игрок уже в этой роли")
		} else if err.Error() == "role_not_found" {
			sendError(w, http.StatusNotFound, "Роль не найдена")
		} else {
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleStaffRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		methodNotAllowed(w, "POST, DELETE")
		return
	}
	if !requireFirestore(w) {
		return
	}

	var req struct {
		RoleName string `json:"roleName"`
		Nickname string `json:"nickname"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
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

		found := false
		for i, role := range data.Roles {
			if strings.EqualFold(role.Name, req.RoleName) {
				found = true
				newPlayers := make([]StaffPlayer, 0, len(role.Players))
				for _, p := range role.Players {
					if !strings.EqualFold(p.Nickname, req.Nickname) {
						newPlayers = append(newPlayers, p)
					}
				}
				if len(newPlayers) == len(role.Players) {
					return errors.New("player_not_found")
				}
				data.Roles[i].Players = newPlayers
				break
			}
		}

		if !found {
			return errors.New("role_not_found")
		}

		return tx.Set(docRef, map[string]interface{}{"roles": data.Roles})
	})

	if err != nil {
		if err.Error() == "player_not_found" {
			sendError(w, http.StatusNotFound, "Игрок не найден")
		} else if err.Error() == "role_not_found" {
			sendError(w, http.StatusNotFound, "Роль не найдена")
		} else {
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// Handler — точка входа Vercel Go (api/index.go)
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	switch requestPath(r) {
	case "/api/login":
		rateLimitLoginMiddleware(handleLogin)(w, r)
	case "/api/logout":
		rateLimitMiddleware(handleLogout)(w, r)
	case "/api/auth/verify":
		rateLimitMiddleware(handleAuthVerify)(w, r)
	case "/api/leaderboard":
		rateLimitMiddleware(handleLeaderboard)(w, r)
	case "/api/staff/add":
		rateLimitMiddleware(authMiddleware(handleStaffAdd))(w, r)
	case "/api/staff/remove":
		rateLimitMiddleware(authMiddleware(handleStaffRemove))(w, r)
	case "/api/staff":
		switch r.Method {
		case http.MethodGet:
			rateLimitMiddleware(handleGetStaff)(w, r)
		default:
			methodNotAllowed(w, "GET")
		}
	case "/api/players":
		switch r.Method {
		case http.MethodGet:
			rateLimitMiddleware(handleGetPlayers)(w, r)
		case http.MethodPost:
			rateLimitMiddleware(authMiddleware(handleSavePlayers))(w, r)
		case http.MethodDelete:
			rateLimitMiddleware(authMiddleware(handleDeletePlayer))(w, r)
		default:
			methodNotAllowed(w, "GET, POST, DELETE")
		}
	case "/api/projects":
		switch r.Method {
		case http.MethodGet:
			rateLimitMiddleware(handleGetProjects)(w, r)
		case http.MethodPost:
			rateLimitMiddleware(authMiddleware(handleSaveProjects))(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
	default:
		sendError(w, http.StatusNotFound, "Роут не найден")
	}
}

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
		globalRateLimiter = newUpstashLimiter()
		if globalRateLimiter == nil {
			globalRateLimiter = newMemoryLimiter()
		}
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
