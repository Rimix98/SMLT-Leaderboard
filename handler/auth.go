package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
		if cached, ok := tokenVerCache.Load("tokenVersion"); ok {
			entry := cached.(*tokenVersionCacheEntry)
			if entry.version > requiredVer {
				return errors.New("token version too old")
			}
			return nil
		}
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

func rotateToken(w http.ResponseWriter, r *http.Request, claims *jwt.MapClaims) {
	newJTI, err := generateJTI()
	if err != nil {
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
	})
	newToken.Header["kid"] = primaryJWTID
	tokenString, err := newToken.SignedString(primaryJWTKey)
	if err != nil {
		return
	}

	if oldJTI, ok := (*claims)["jti"].(string); ok && oldJTI != "" {
		blacklistToken(r.Context(), oldJTI)
	}

	setSecureCookie(w, "auth_token", tokenString, 86400)
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

		rotateToken(w, r, claims)

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
		if err != nil || cookie.Value == "" || headerToken == "" || subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookie.Value)) != 1 {
			sendError(w, http.StatusForbidden, "Доступ запрещен")
			return
		}
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

func handleGetCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка генерации токена")
		return
	}
	tokenStr := hex.EncodeToString(token)
	setCSRFCookie(w, tokenStr, 3600)
	writeJSON(w, map[string]interface{}{"success": true, "token": tokenStr})
}

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
	})
	newToken.Header["kid"] = primaryJWTID
	tokenString, err := newToken.SignedString(primaryJWTKey)
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
