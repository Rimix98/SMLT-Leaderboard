package handler

import (
	"compress/gzip"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

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

func botDetectionMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ua := r.UserAgent()
		ip := getRealIP(r)

		if isDeviceBanned(r.Context(), generateFingerprint(r)) {
			securityEvent(r.Context(), "device_banned", ip, r.URL.Path, map[string]string{
				"ua":          ua,
				"fingerprint": generateFingerprint(r),
			})
			sendError(w, http.StatusForbidden, "Устройство заблокировано")
			return
		}

		if isBlockedBot(ua) {
			fp := generateFingerprint(r)
			securityEvent(r.Context(), "bot_blocked", ip, r.URL.Path, map[string]string{
				"ua":          ua,
				"fingerprint": fp,
			})
			alertSecurityEvent("bot_blocked", ip, r.URL.Path, map[string]string{"ua": ua})
			time.Sleep(time.Duration(mathrand.IntN(500)+200) * time.Millisecond)
			sendError(w, http.StatusForbidden, "Доступ запрещен")
			return
		}

		if isBlockedPath(r.URL.Path) {
			fp := generateFingerprint(r)
			securityEvent(r.Context(), "blocked_path", ip, r.URL.Path, map[string]string{
				"ua":          ua,
				"fingerprint": fp,
			})
			alertSecurityEvent("blocked_path", ip, r.URL.Path, map[string]string{"ua": ua})
			time.Sleep(time.Duration(mathrand.IntN(500)+200) * time.Millisecond)
			sendError(w, http.StatusForbidden, "Доступ запрещен")
			return
		}

		if r.ContentLength > 5*1024*1024 {
			securityEvent(r.Context(), "oversized_request", ip, r.URL.Path, map[string]int{
				"size": int(r.ContentLength),
			})
			sendError(w, http.StatusRequestEntityTooLarge, "Слишком большой запрос")
			return
		}

		next(w, r)
	}
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
	return nil, fmt.Errorf("jwt secret not found")
}

func generateAdminKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func generateJTI() (string, error) {
	buf := make([]byte, 16)
	for tries := 0; tries < 3; tries++ {
		if _, err := rand.Read(buf); err == nil {
			return hex.EncodeToString(buf), nil
		}
	}
	return "", fmt.Errorf("crypto/rand unavailable")
}
