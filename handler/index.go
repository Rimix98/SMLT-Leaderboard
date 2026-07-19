package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("panic recovered", "error", rec, "stack", string(debug.Stack()))
			sendError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		}
	}()

	var reqID string
	idBytes := make([]byte, 6)
	if _, err := rand.Read(idBytes); err == nil {
		reqID = hex.EncodeToString(idBytes)
	}
	w.Header().Set("X-Request-ID", reqID)

	ctx := context.WithValue(r.Context(), ctxKeyReqID, reqID)
	ctx = context.WithValue(ctx, ctxKeyIP, getRealIP(r))
	r = r.WithContext(ctx)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), interest-cohort=(), browsing-topics=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-src https://www.youtube.com; object-src 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Del("Server")

	origin := r.Header.Get("Origin")
	if origin != "" {
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Requested-With, X-Admin-Path-Key")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := requestPath(r)

	mux := map[string]http.HandlerFunc{
		"/api/captcha":            rateLimitMiddleware(30)(handleCaptcha),
		"/api/login":              rateLimitLoginMiddleware(handleLogin),
		"/api/logout":             rateLimitLoginMiddleware(handleLogout),
		"/api/verify":             rateLimitMiddleware(60)(handleVerify),
		"/api/csrf-token":         rateLimitMiddleware(30)(handleGetCSRFToken),
		"/api/auth/refresh":       rateLimitMiddleware(10)(handleRefreshToken),
		"/api/leaderboard":        rateLimitMiddleware(30)(handleLeaderboard),
		"/api/leaderboard/check":  rateLimitMiddleware(30)(handleLeaderboardCheck),
		"/api/history/snapshot":   rateLimitMiddleware(5)(knockMiddleware(authMiddleware(csrfMiddleware(handleSaveHistorySnapshot)))),
		"/api/staff":              rateLimitMiddleware(60)(handleGetStaff),
		"/api/security/dashboard": rateLimitMiddleware(10)(authMiddleware(handleSecurityDashboard)),
		"/api/knock-knock-admin":  rateLimitMiddleware(10)(authMiddleware(csrfMiddleware(handleAdminKnock))),
		"/api/staff/add":          rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffAdd)))),
		"/api/staff/role":         rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffRole)))),
		"/api/staff/remove":       rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleStaffRemove)))),
		"/api/staff/reorder":      rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleReorderStaffRoles)))),
		"/api/staff/tiers":        rateLimitMiddleware(60)(handleGetStaffTiers),
		"/api/staff/tier":         rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSetStaffTier)))),
		"/api/staff/save":         rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSaveStaff)))),
		"/api/projects":           rateLimitMiddleware(60)(handleGetProjects),
		"/api/projects/save":      rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSaveProjects)))),
		"/api/players":            rateLimitMiddleware(60)(handleGetPlayers),
		"/api/players/save":       rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleSavePlayers)))),
		"/api/players/delete":     rateLimitMiddleware(30)(knockMiddleware(authMiddleware(csrfMiddleware(handleDeletePlayer)))),
		"/api/security/ip-ban":    rateLimitMiddleware(10)(authMiddleware(csrfMiddleware(handleIPBan))),
		"/api/security/ip-unban":  rateLimitMiddleware(10)(authMiddleware(csrfMiddleware(handleIPUnban))),
		"/api/smp/status":         rateLimitMiddleware(15)(handleSMPStatus),

	}

	h, ok := mux[path]
	if !ok {
		path = strings.TrimSuffix(path, "/")
		h, ok = mux[path]
	}
	if ok {
		gzipMiddleware(botDetectionMiddleware(h))(w, r)
		return
	}

	if isHoneypot(path) {
		handleHoneypot(w, r)
		return
	}

	if strings.HasPrefix(path, "/api/history/") && path != "/api/history/snapshot" {
		gzipMiddleware(botDetectionMiddleware(rateLimitMiddleware(60)(handlePlayerHistory)))(w, r)
		return
	}

	sendError(w, http.StatusNotFound, "Роут не найден")
}
