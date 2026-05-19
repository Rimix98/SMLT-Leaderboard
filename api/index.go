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
	"google.golang.org/api/option"
)

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
	Participants []string `json:"participants" firestore:"participants"` // теперь это массив строк!
}

type LoginRequest struct {
	Password     string `json:"password"`
	CaptchaToken string `json:"captchaToken"`
}

type ipLimit struct {
	requests  int
	resetTime time.Time
}

var (
	limiterMu sync.Mutex
	ipMap     = make(map[string]*ipLimit)
)

func isRateLimited(ip string) bool {
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
	return lim.requests > 60 // Лимит: 60 запросов в минуту
}

// ==========================================
// ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ И АУТЕНТИФИКАЦИЯ
// ==========================================

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
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

	config := &firebase.Config{ProjectID: "smlt-97ce4"}
	opt := option.WithCredentialsFile("serviceAccountKey.json")
	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, err
	}
	return app.Firestore(ctx)
}

func checkAdminAuth(r *http.Request) bool {
	// Строгое чтение JWT-токена из HttpOnly Куки
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return false
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// Если секрета нет, логируем ошибку и не пускаем никого
		println("КРИТИЧЕСКАЯ ОШИБКА: JWT_SECRET не задан в переменных окружения!")
		return false
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	return ok && claims["role"] == "admin"
}

func verifyTurnstile(token, ip string) bool {
	secret := os.Getenv("TURNSTILE_SECRET")
	if secret == "" {
		println("КРИТИЧЕСКАЯ ОШИБКА: TURNSTILE_SECRET не задан в конфигурации!")
		return false
	}

	resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify",
		url.Values{
			"secret":   {secret},
			"response": {token},
			"remoteip": {ip},
		},
	)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Success
}

// ==========================================
// ОСНОВНОЙ ОБРАБОТЧИК
// ==========================================

func Handler(w http.ResponseWriter, r *http.Request) {
	// --- 1. НАСТРОЙКА КОРРЕКТНОГО CORS ---
	origin := r.Header.Get("Origin")
	allowedOrigin := "https://smltdemonlist.vercel.app/"

	// Разрешаем локалку для разработки
	if origin == "http://localhost:3000" || origin == "http://127.0.0.1:5500" || strings.HasPrefix(origin, "http://localhost:") {
		allowedOrigin = origin
	}

	if origin != "" && (origin == allowedOrigin) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	}
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Если это предзапрос (OPTIONS) от браузера — сразу возвращаем 200
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Всегда возвращаем JSON в случае ошибок
	sendError := func(code int, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
	}

	// Применяем Rate Limiter по IP
	if isRateLimited(getClientIP(r)) {
		sendError(http.StatusTooManyRequests, "Too many requests. Slow down!")
		return
	}

	ctx := context.Background()
	client, err := getFirestore(ctx)
	if err != nil {
		sendError(http.StatusInternalServerError, "Ошибка подключения к базе данных (Firestore)")
		return
	}
	defer client.Close()

	// --- 2. ДОБАВЛЕННЫЙ РОУТ ДЛЯ ПРОВЕРКИ АВТОРИЗАЦИИ ИЗ ЛОКАЛСТОРИДЖА ---
	if r.URL.Path == "/auth/verify" && r.Method == "GET" {
		if !checkAdminAuth(r) {
			sendError(http.StatusUnauthorized, "Unauthorized")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// АВТОРИЗАЦИЯ ХОСТА
	if r.URL.Path == "/auth/login" && r.Method == "POST" {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(http.StatusBadRequest, "Некорректный запрос")
			return
		}

		if !verifyTurnstile(req.CaptchaToken, getClientIP(r)) {
			sendError(http.StatusBadRequest, "Капча не пройдена или токен устарел")
			return
		}

		adminDoc, err := client.Collection("config").Doc("admin").Get(ctx)
		if err != nil {
			sendError(http.StatusInternalServerError, "Ошибка БД: не найден конфиг админа")
			return
		}
		var adminData struct {
			PasswordHash string `firestore:"password_hash"`
		}
		adminDoc.DataTo(&adminData)

		if adminData.PasswordHash == "" {
			sendError(http.StatusInternalServerError, "Ошибка конфигурации: пароль админа не задан в БД")
			return
		}

		// Bcrypt проверка
		err = bcrypt.CompareHashAndPassword([]byte(adminData.PasswordHash), []byte(req.Password))
		if err != nil {
			sendError(http.StatusUnauthorized, "Неверный пароль")
			return
		}

		// Генерация JWT токена
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			sendError(http.StatusInternalServerError, "Ошибка сервера: JWT_SECRET не настроен")
			return
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"role": "admin",
			"exp":  time.Now().Add(24 * time.Hour).Unix(),
		})

		tokenString, err := token.SignedString([]byte(jwtSecret))
		if err != nil {
			sendError(http.StatusInternalServerError, "Ошибка создания токена")
			return
		}

		// Установка защищенной HttpOnly куки
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    tokenString,
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Path:     "/",
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		return
	}

	// ВЫХОД ИЗ СИСТЕМЫ (LOGOUT)
	if r.URL.Path == "/auth/logout" && r.Method == "POST" {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    "",
			Expires:  time.Now().Add(-1 * time.Hour),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Path:     "/",
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		return
	}

	// ИГРОКИ (PLAYERS)
	if r.URL.Path == "/players" {
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
				sendError(http.StatusUnauthorized, "Unauthorized")
				return
			}
			var list []Player
			if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
				sendError(http.StatusBadRequest, "Некорректный JSON")
				return
			}
			_, err = docRef.Set(ctx, map[string]interface{}{"list": list})
			if err != nil {
				sendError(http.StatusInternalServerError, "Ошибка сохранения игроков")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
	}

	// ПРОЕКТЫ (PROJECTS)
	if r.URL.Path == "/projects" {
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
				sendError(http.StatusUnauthorized, "Unauthorized")
				return
			}
			var list []Project
			if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
				sendError(http.StatusBadRequest, "Некорректный JSON")
				return
			}
			_, err = docRef.Set(ctx, map[string]interface{}{"list": list})
			if err != nil {
				sendError(http.StatusInternalServerError, "Ошибка сохранения проектов")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
	}

	// ДЕМОНЫ (DEMONS)
	if r.URL.Path == "/demons" && r.Method == "GET" {
		iter := client.Collection("demons").Documents(ctx)
		docs, err := iter.GetAll()
		if err != nil {
			sendError(http.StatusInternalServerError, "Ошибка получения демонов")
			return
		}
		var list []interface{}
		for _, d := range docs {
			list = append(list, d.Data())
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
		return
	}

	sendError(http.StatusNotFound, "Маршрут не найден")
}
