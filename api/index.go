package handler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
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

var (
	limiterMu sync.Mutex
	ipMap     = make(map[string]*ipLimit)
)

// Бессмертный рейтлимитер: если боты забивают мапу, она сбрасывается без утечки памяти и нагрузки на CPU
func isRateLimited(ip string) bool {
	limiterMu.Lock()
	defer limiterMu.Unlock()

	now := time.Now()

	// Умная очистка памяти в Serverless: чистим только протухшие сессии
	if len(ipMap) > 1000 {
		for k, v := range ipMap {
			if now.After(v.resetTime) {
				delete(ipMap, k)
			}
		}
		if len(ipMap) > 1500 {
			return true
		}
	}

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

// Умная защита от CSRF (пропускает localhost для тестов) + жесткая проверка подписи JWT
func checkAdminAuth(r *http.Request) bool {
	if r.Method != "GET" && r.Method != "OPTIONS" {
		origin := r.Header.Get("Origin")
		isLocal := strings.HasPrefix(origin, "http://localhost:") || origin == "http://127.0.0.1:5500"

		if origin != "https://smltdemonlist.vercel.app" && !isLocal {
			println("Блокировка CSRF: подозрительный запрос с ориджина:", origin)
			return false
		}
	}

	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return false
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		println("КРИТИЧЕСКАЯ ОШИБКА: JWT_SECRET не задан!")
		return false
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			println("Внимание: попытка подменить алгоритм подписи токена!")
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

	sendError := func(code int, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
	}

	if isRateLimited(getClientIP(r)) {
		sendError(http.StatusTooManyRequests, "Too many requests. Slow down!")
		return
	}

	ctx := context.Background()
	client, err := getFirestore(ctx)
	if err != nil {
		sendError(http.StatusInternalServerError, "Ошибка подключения к Firestore")
		return
	}
	defer client.Close()

	// РОУТЫ
	if strings.HasSuffix(r.URL.Path, "/auth/verify") && r.Method == "GET" {
		if !checkAdminAuth(r) {
			sendError(http.StatusUnauthorized, "Unauthorized")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	if strings.HasSuffix(r.URL.Path, "/auth/login") && r.Method == "POST" {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(http.StatusBadRequest, "Некорректный запрос")
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
			sendError(http.StatusInternalServerError, "Ошибка конфигурации: пароль админа пуст")
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(adminData.PasswordHash), []byte(req.Password))
		if err != nil {
			sendError(http.StatusUnauthorized, "Неверный пароль")
			return
		}

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

		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    tokenString,
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode, // Меняем Lax на Strict для максимальной защиты
			Path:     "/",
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		return
	}

	if strings.HasSuffix(r.URL.Path, "/auth/logout") {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    "",
			Expires:  time.Now().Add(-1 * time.Hour), // или time.Unix(0, 0)
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode, // Меняем на Strict здесь
			Path:     "/",
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	if strings.HasSuffix(r.URL.Path, "/stats/countries") && r.Method == "GET" {
		// В идеале этот кусок оптимизировать, но для начала сойдет чтение коллекции
		iter := client.Collection("players").Documents(ctx)
		stats := make(map[string]int)

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				sendError(http.StatusInternalServerError, "Ошибка базы данных")
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
		return
	}

	if strings.HasSuffix(r.URL.Path, "/players") {
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

			// ВАЛИДАЦИЯ: Защита от дурака и хакера
			if len(list) == 0 {
				sendError(http.StatusBadRequest, "Список игроков не может быть пустым")
				return
			}
			for _, p := range list {
				trimmed := strings.TrimSpace(p.Name)
				if trimmed == "" || len(trimmed) > 50 {
					sendError(http.StatusBadRequest, "Некорректное или слишком длинное имя игрока")
					return
				}
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

	if strings.HasSuffix(r.URL.Path, "/projects") {
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

	if strings.HasSuffix(r.URL.Path, "/demons") && r.Method == "GET" {
		// Добавляем жесткий лимит .Limit(100), чтобы сервер не лег при росте базы
		iter := client.Collection("demons").Limit(100).Documents(ctx)
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
