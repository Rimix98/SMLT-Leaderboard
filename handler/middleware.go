package handler

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

func hashIP(ip string) string {
	if rateLimitSalt == "" {
		initRateLimitSalt()
	}
	hash := sha256.Sum256([]byte(ip + rateLimitSalt))
	return hex.EncodeToString(hash[:16])
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
		for i := 0; i < len(parts); i++ {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip != nil {
				return ip.String()
			}
		}
	}
	return remoteAddrIP(r)
}

func sendError(w http.ResponseWriter, status int, msg string) {
	log.Printf("[error] %d %s", status, msg)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	sendError(w, http.StatusMethodNotAllowed, "Метод не разрешен")
}

func decodeRequestJSON(w http.ResponseWriter, r *http.Request, dest interface{}) error {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
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

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func setSecureCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-" + name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	})
}

func setCSRFCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-csrf_token",
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-" + name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func requestPath(r *http.Request) string {
	p := r.URL.Path
	if p == "/api" || p == "/api/" {
		return r.RequestURI
	}
	return p
}

func requireFirestore(w http.ResponseWriter) bool {
	if fsErr != nil || fsClient == nil {
		sendError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return false
	}
	return true
}

func isBlockedBot(ua string) bool {
	if ua == "" {
		return true
	}
	lower := strings.ToLower(ua)
	for _, pattern := range blockedBotPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	browserPrefixes := []string{
		"mozilla/", "chrome/", "safari/", "firefox/", "edge/", "opera/",
		"webkit/", "blink/",
	}
	isBrowser := false
	for _, prefix := range browserPrefixes {
		if strings.HasPrefix(lower, prefix) {
			isBrowser = true
			break
		}
	}
	if !isBrowser {
		return true
	}
	return false
}

func isBlockedPath(path string) bool {
	lower := strings.ToLower(path)
	for _, pattern := range blockedPathPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
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
