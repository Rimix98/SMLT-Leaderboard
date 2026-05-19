package handler

import (
	"context"
	"encoding/json"
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
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// === ТИПЫ ДАННЫХ ===

type Player struct {
	Name string `json:"name" firestore:"name"`
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

type LoginRequest struct {
	Password string `json:"password"`
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

// === ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ (ЛИМИТЕР И КЭШ) ===

var (
	limiterMu   sync.Mutex
	ipMap       = make(map[string]*ipLimit)
	cleanerOnce sync.Once

	cacheMu     sync.Mutex
	cachedData  []byte
	cacheExpiry time.Time
)

// === ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ===

func sendError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func startCleaner() {
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		for range ticker.C {
			limiterMu.Lock()
			now := time.Now()
			for ip, limit := range ipMap {
				if now.After(limit.resetTime) {
					delete(ipMap, ip)
				}
			}
			limiterMu.Unlock()
		}
	}()
}

func isRateLimited(ip string) bool {
	cleanerOnce.Do(startCleaner)

	limiterMu.Lock()
	defer limiterMu.Unlock()

	now := time.Now()
	lim, exists := ipMap[ip]
	if !exists || now.After(lim.resetTime) {
		ipMap[ip] = &ipLimit{
			requests:  1,
			resetTime: now.Add(1 * time.Minute),
		}
		return false
	}

	lim.requests++
	return lim.requests > 60
}

func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func getFirestore(ctx context.Context) (*firestore.Client, error) {
	credsJSON := os.Getenv("FIREBASE_CREDENTIALS")
	if credsJSON != "" {
		app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(credsJSON)))
		if err != nil {
			return nil, err
		}
		return app.Firestore(ctx)
	}

	opt := option.WithCredentialsFile("serviceAccountKey.json")
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, err
	}
	return app.Firestore(ctx)
}

func checkAdminAuth(r *http.Request) bool {
	if r.Method != "GET" && r.Method != "OPTIONS" {
		origin := r.Header.Get("Origin")
		isLocal := strings.HasPrefix(origin, "http://localhost:") || origin == "http://127.0.0.1:5500"

		if origin != "https://smltdemonlist.vercel.app" && !isLocal {
			return false
		}
	}

	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return false
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return false
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	return ok && claims["role"] == "admin"
}

// === ОСНОВНОЙ ХЕНДЛЕР С СИСТЕМОЙ РОУТИНГА ===

func Handler(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowedOrigin := "https://smltdemonlist.vercel.app"

	if origin == "http://localhost:3000" || origin == "http://127.0.0.1:5500" || strings.HasPrefix(origin, "http://localhost:") {
		allowedOrigin = origin
	}

	if origin != "" && (origin == allowedOrigin) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	}
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Честный и безопасный рейтлимитер
	if isRateLimited(getClientIP(r)) {
		sendError(w, http.StatusTooManyRequests, "Too many requests. Slow down!")
		return
	}

	ctx := context.Background()
	client, err := getFirestore(ctx)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка подключения к Firestore")
		return
	}
	defer client.Close()

	// Чистый роутинг без дублирующих проверок на суффиксы
	switch r.URL.Path {

	case "/auth/login":
		if r.Method != "POST" {
			sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, http.StatusBadRequest, "Некорректный запрос")
			return
		}

		adminDoc, err := client.Collection("config").Doc("admin").Get(ctx)
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Ошибка БД: не найден конфиг админа")
			return
		}
		var adminData struct {
			PasswordHash string `firestore:"password_hash"`
		}
		adminDoc.DataTo(&adminData)

		if adminData.PasswordHash == "" {
			sendError(w, http.StatusInternalServerError, "Ошибка конфигурации: пароль админа пуст")
			return
		}

		if err = bcrypt.CompareHashAndPassword([]byte(adminData.PasswordHash), []byte(req.Password)); err != nil {
			sendError(w, http.StatusUnauthorized, "Неверный пароль")
			return
		}

		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			sendError(w, http.StatusInternalServerError, "Ошибка сервера: JWT_SECRET не настроен")
			return
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"role": "admin",
			"exp":  time.Now().Add(24 * time.Hour).Unix(),
		})

		tokenString, err := token.SignedString([]byte(jwtSecret))
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Ошибка создания токена")
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})

	case "/auth/logout":
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    "",
			Expires:  time.Now().Add(-1 * time.Hour),
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case "/auth/verify":
		if checkAdminAuth(r) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "authorized"})
		} else {
			sendError(w, http.StatusUnauthorized, "Unauthorized")
		}

	case "/stats/countries":
		if r.Method != "GET" {
			sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		iter := client.Collection("players").Documents(ctx)
		stats := make(map[string]int)

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
				return
			}

			var p struct {
				Country string `firestore:"country"`
			}
			doc.DataTo(&p)

			if p.Country != "" {
				stats[p.Country]++
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)

	case "/players":
		docRef := client.Collection("list_data").Doc("players")

		if r.Method == "GET" {
			doc, err := docRef.Get(ctx)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]Player{})
				return
			}
			var data struct {
				List []Player `firestore:"list"`
			}
			doc.DataTo(&data)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(data.List)
			return
		}

		if r.Method == "POST" {
			if !checkAdminAuth(r) {
				sendError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			var list []Player
			if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
				sendError(w, http.StatusBadRequest, "Некорректный JSON")
				return
			}

			if len(list) == 0 {
				sendError(w, http.StatusBadRequest, "Список игроков не может быть пустым")
				return
			}
			for _, p := range list {
				trimmed := strings.TrimSpace(p.Name)
				if trimmed == "" || len(trimmed) > 50 {
					sendError(w, http.StatusBadRequest, "Некорректное или слишком длинное имя игрока")
					return
				}
			}

			_, err = docRef.Set(ctx, map[string]interface{}{"list": list})
			if err != nil {
				sendError(w, http.StatusInternalServerError, "Ошибка сохранения игроков")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}

	case "/projects":
		docRef := client.Collection("list_data").Doc("projects")

		if r.Method == "GET" {
			doc, err := docRef.Get(ctx)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]Project{})
				return
			}
			var data struct {
				List []Project `firestore:"list"`
			}
			doc.DataTo(&data)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(data.List)
			return
		}

		if r.Method == "POST" {
			if !checkAdminAuth(r) {
				sendError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			var list []Project
			if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
				sendError(w, http.StatusBadRequest, "Некорректный JSON")
				return
			}
			_, err = docRef.Set(ctx, map[string]interface{}{"list": list})
			if err != nil {
				sendError(w, http.StatusInternalServerError, "Ошибка сохранения проектов")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}

	case "/demons":
		if r.Method != "GET" {
			sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		iter := client.Collection("demons").Limit(100).Documents(ctx)
		docs, err := iter.GetAll()
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Ошибка получения демонов")
			return
		}
		var list []interface{}
		for _, d := range docs {
			list = append(list, d.Data())
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)

	case "/api/leaderboard":
		if r.Method != "GET" {
			sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		// Проверяем кэш в памяти бэкенда
		cacheMu.Lock()
		if time.Now().Before(cacheExpiry) && cachedData != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write(cachedData)
			cacheMu.Unlock()
			return
		}
		cacheMu.Unlock()

		// Тянем список игроков, добавленных хостом
		doc, err := client.Collection("list_data").Doc("players").Get(ctx)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		var playerData struct {
			List []Player `firestore:"list"`
		}
		doc.DataTo(&playerData)

		var result []FullPlayerData
		httpClient := &http.Client{Timeout: 3 * time.Second} // Снизили таймаут, чтобы не копить зависшие запросы

		// Если список игроков пуст, сразу отдаем пустой массив
		if len(playerData.List) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}

		var wg sync.WaitGroup
		var mu sync.Mutex // Защищает общий слайс result от состояния гонки (Race Condition)

		// Агрегируем статистику с demonlist API конкурентно
		for _, p := range playerData.List {
			wg.Add(1)
			go func(playerName string) {
				defer wg.Done()

				// ФИКС PATH TRAVERSAL: Строгое экранирование спецсимволов и путей
				escapedName := url.PathEscape(playerName)

				// Запрос 1: Основные данные игрока
				respData, err := httpClient.Get("https://api.demonlist.org/api/v1/players/by_name/" + escapedName)
				if err != nil {
					return
				}
				var pData interface{}
				json.NewDecoder(respData.Body).Decode(&pData)
				respData.Body.Close()

				// Запрос 2: Рекорды игрока
				respRecs, err := httpClient.Get("https://api.demonlist.org/api/v1/players/by_name/" + escapedName + "/records/")
				if err != nil {
					return
				}
				var pRecs interface{}
				json.NewDecoder(respRecs.Body).Decode(&pRecs)
				respRecs.Body.Close()

				// Безопасно сохраняем данные в общий слайс под блокировкой мутекса
				mu.Lock()
				result = append(result, FullPlayerData{
					Name:    playerName,
					Data:    pData,
					Records: pRecs,
				})
				mu.Unlock()
			}(p.Name)
		}

		// Ждем завершения всех горутин
		wg.Wait()

		// Если из-за сетевых ошибок ни один игрок не зафетчился, отдаем пустой массив вместо nil
		if result == nil {
			result = []FullPlayerData{}
		}

		jsonData, err := json.Marshal(result)
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Ошибка обработки данных")
			return
		}

		// Обновляем кэш на 10 минут
		cacheMu.Lock()
		cachedData = jsonData
		cacheExpiry = time.Now().Add(10 * time.Minute)
		cacheMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)

	default:
		sendError(w, http.StatusNotFound, "Маршрут не найден")
	}
}
