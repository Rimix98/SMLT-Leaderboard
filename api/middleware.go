package handler

import (
	"bytes"
	"compress/gzip"
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
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/microcosm-cc/bluemonday"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	trustProxy     bool
	primaryJWTKey  []byte
	primaryJWTID   string

	jwtSecrets     []jwtKey
	jwtSecretsMu   sync.RWMutex
	jwtSecretsOnce sync.Once

	rateLimitSalt  string
	saltOnce       sync.Once

	httpClient = &http.Client{Timeout: 15 * time.Second}

	maxRequestBody = int64(1024 * 1024)

	tokenVerCache   sync.Map
	tokenVerCacheMu sync.Mutex
)

var (
	reProjectID    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)
	reAlphanumeric = regexp.MustCompile(`^[a-zA-Z0-9 _.\-]+$`)
	reDiscord      = regexp.MustCompile(`^[a-zA-Z0-9 _.\-#]+$`)
	reVideoID      = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	reRoleName     = regexp.MustCompile(`^[a-zA-Z0-9 _.\-]+$`)
	reHexColor     = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
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
		gz.Close()
	}
}

func init() {
	trustProxy = os.Getenv("TRUST_PROXY") == "true" || os.Getenv("VERCEL") == "1"
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
		if len(primary) < 32 {
			log.Fatalf("[jwt] JWT_SECRET too short (%d bytes), need at least 32", len(primary))
		}
		primaryJWTKey = []byte(primary)
		primaryJWTID = "1"
		jwtSecrets = append(jwtSecrets, jwtKey{Secret: primaryJWTKey, ID: primaryJWTID})
		for i := 2; ; i++ {
			key := os.Getenv(fmt.Sprintf("JWT_SECRET_%d", i))
			if key == "" {
				break
			}
			if len(key) < 32 {
				log.Printf("[jwt] JWT_SECRET_%d too short (%d bytes), skipping", i, len(key))
				continue
			}
			jwtSecrets = append(jwtSecrets, jwtKey{Secret: []byte(key), ID: fmt.Sprintf("%d", i)})
		}
	})
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

func hashIP(ip string) string {
	if rateLimitSalt == "" {
		initRateLimitSalt()
	}
	hash := sha256.Sum256([]byte(ip + rateLimitSalt))
	return hex.EncodeToString(hash[:16])
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
		return nil
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

func setSecureCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
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

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

type responseCapture struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.ResponseWriter.WriteHeader(code)
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b)
	return rc.ResponseWriter.Write(b)
}
