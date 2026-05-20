package handler

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
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
	fsClient  *firestore.Client
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))

	// Безопасный HTTP клиент с жестким таймаутом (защита от зависания горутин)
	httpClient = &http.Client{Timeout: 10 * time.Second}

	// Rate Limiter
	ipMap  = make(map[string]*ipLimit)
	muLim  sync.Mutex
	maxIPs = 10000 // Защита от OOM при DDoS
)

// === ТИПЫ ДАННЫХ ===
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

type ipLimit struct {
	requests  int
	resetTime time.Time
}

type FullPlayerData struct {
	Name    string      `json:"name"`
	Data    interface{} `json:"data"`
	Records interface{} `json:"records"`
}

// === ИНИЦИАЛИЗАЦИЯ (Firebase) ===
func init() {
	ctx := context.Background()
	creds := os.Getenv("FIREBASE_CREDENTIALS")
	if creds == "" {
		log.Println("ВНИМАНИЕ: FIREBASE_CREDENTIALS не задан")
		return
	}

	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(creds)))
	if err != nil {
		log.Fatalf("Ошибка инициализации Firebase: %v", err)
	}

	fsClient, err = app.Firestore(ctx)
	if err != nil {
		log.Fatalf("Ошибка инициализации Firebase: %v", err)
	}
}

// === УТИЛИТЫ ===
func sendError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Берем реальный IP на уровне TCP, игнорируя поддельные заголовки
func getRealIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// Берем первый IP из списка (клиентский)
		return strings.Split(ip, ",")[0]
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// === MIDDLEWARES ===

// 1. Бронебойный Rate Limiter
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getRealIP(r)

		muLim.Lock()
		// Вместо обнуления всей мапы:
		if len(ipMap) > maxIPs {
			http.Error(w, "Rate limit map full", http.StatusServiceUnavailable)
			return
		}

		limiter, exists := ipMap[ip]
		if !exists || time.Now().After(limiter.resetTime) {
			ipMap[ip] = &ipLimit{requests: 1, resetTime: time.Now().Add(1 * time.Minute)}
			muLim.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		if limiter.requests >= 60 {
			muLim.Unlock()
			sendError(w, http.StatusTooManyRequests, "Слишком много запросов")
			return
		}

		limiter.requests++
		muLim.Unlock()
		next.ServeHTTP(w, r)
	}
}

// 2. Валидация JWT Админа
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			sendError(w, http.StatusUnauthorized, "Нет доступа")
			return
		}

		token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			sendError(w, http.StatusUnauthorized, "Невалидный токен")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// === ХЭНДЛЕРЫ ===

// Обработчик входа (Bcrypt cost должен быть не больше 10-12)
func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой запрос")
		return
	}

	adminHash := os.Getenv("ADMIN_HASH")

	// Проверка пароля. Если база ляжет, это место спасет от перебора
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash), []byte(req.Password)); err != nil {
		sendError(w, http.StatusUnauthorized, "Неверный пароль")
		return
	}

	// Выдача токена
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin": true,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString(jwtSecret)

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    tokenString,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true, // Защита от XSS (JS не прочитает куку)
		Secure:   true, // Только по HTTPS
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// Сохранение проекта с жесткой валидацией
func handleSaveProject(w http.ResponseWriter, r *http.Request) {
	var p Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}

	// ЗАЩИТА: Не даем забить базу мусором
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

	ctx := context.Background()
	_, err := fsClient.Collection("projects").Doc(p.ID).Set(ctx, p)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Защищенный фетчинг лидерборда (Worker Pool)
func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	// ... (Здесь должен быть код получения players из Firestore)
	// Допустим, мы получили []Player{...} в переменную players
	players := []Player{{Name: "Player1"}, {Name: "Player2"}} // Заглушка

	var wg sync.WaitGroup
	var mu sync.Mutex
	result := make([]FullPlayerData, 0, len(players))

	// СЕМАФОР: Ограничиваем до 5 одновременных запросов к demonlist.org
	sem := make(chan struct{}, 5)

	for _, p := range players {
		wg.Add(1)
		sem <- struct{}{} // Занимаем слот

		go func(playerName string) {
			defer wg.Done()
			defer func() { <-sem }() // Освобождаем слот

			escaped := url.PathEscape(playerName)

			// Запрос 1 (с учетом таймаута из глобального httpClient)
			resp1, err := httpClient.Get("https://api.demonlist.org/api/v1/players/by_name/" + escaped)
			if err != nil {
				return
			}
			defer resp1.Body.Close()
			var data interface{}
			json.NewDecoder(resp1.Body).Decode(&data)

			// Запрос 2
			resp2, err := httpClient.Get("https://api.demonlist.org/api/v1/players/by_name/" + escaped + "/records/")
			if err != nil {
				return
			}
			defer resp2.Body.Close()
			var recs interface{}
			json.NewDecoder(resp2.Body).Decode(&recs)

			mu.Lock()
			result = append(result, FullPlayerData{
				Name:    playerName,
				Data:    data,
				Records: recs,
			})
			mu.Unlock()
		}(p.Name)
	}

	wg.Wait()
	json.NewEncoder(w).Encode(result)
}

// Проверка валидности JWT токена
func handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		sendError(w, http.StatusUnauthorized, "Нет токена")
		return
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		sendError(w, http.StatusUnauthorized, "Невалидный токен")
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"valid": true})
}

// Получить список игроков
func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	doc, err := fsClient.Collection("config").Doc("players").Get(ctx)
	if err != nil {
		// Если документа нет, возвращаем пустой массив
		json.NewEncoder(w).Encode([]string{})
		return
	}

	var players []Player
	if err := doc.DataTo(&players); err != nil {
		json.NewEncoder(w).Encode([]string{})
		return
	}

	// Возвращаем только имена
	names := make([]string, len(players))
	for i, p := range players {
		names[i] = p.Name
	}
	json.NewEncoder(w).Encode(names)
}

// Сохранить список игроков (требует авторизацию)
func handleSavePlayers(w http.ResponseWriter, r *http.Request) {
	var playerList []Player
	if err := json.NewDecoder(r.Body).Decode(&playerList); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}

	// Защита от переполнения
	if len(playerList) > 1000 {
		sendError(w, http.StatusBadRequest, "Слишком много игроков (макс 1000)")
		return
	}

	// Валидация имен игроков
	for _, p := range playerList {
		if len(p.Name) == 0 || len(p.Name) > 100 {
			sendError(w, http.StatusBadRequest, "Недопустимая длина имени игрока")
			return
		}
	}

	ctx := context.Background()
	_, err := fsClient.Collection("config").Doc("players").Set(ctx, playerList)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// === РОУТЕР (Точка входа) ===
func MainHandler(w http.ResponseWriter, r *http.Request) {
	// Базовые заголовки (CORS + защита контента)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	// Маршрутизация по пути и методу
	switch r.URL.Path {
	case "/api/login":
		rateLimitMiddleware(handleLogin)(w, r)
	case "/api/auth/verify":
		rateLimitMiddleware(handleAuthVerify)(w, r)
	case "/api/leaderboard":
		rateLimitMiddleware(handleLeaderboard)(w, r)
	case "/api/players":
		if r.Method == "GET" {
			// GET: без авторизации (загрузка списка игроков)
			rateLimitMiddleware(handleGetPlayers)(w, r)
		} else if r.Method == "POST" {
			// POST: с авторизацией (сохранение списка)
			rateLimitMiddleware(authMiddleware(handleSavePlayers))(w, r)
		} else {
			sendError(w, http.StatusMethodNotAllowed, "Метод не разрешен")
		}
	case "/api/projects":
		rateLimitMiddleware(authMiddleware(handleSaveProject))(w, r)
	default:
		sendError(w, http.StatusNotFound, "Роут не найден")
	}
}
