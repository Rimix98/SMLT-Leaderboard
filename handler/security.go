package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/iterator"
)

var honeypotPaths = map[string]bool{
	"/api/admin":       true,
	"/api/admin/":      true,
	"/api/admin/login": true,
	"/api/admin/panel": true,
	"/api/debug":       true,
	"/api/debug/":      true,
	"/api/internal":    true,
	"/api/internal/":   true,
	"/api/health":      true,
	"/api/config":      true,
	"/api/users":       true,
	"/api/users/":      true,
	"/api/user":        true,
	"/api/auth":        true,
	"/api/auth/":       true,
	"/api/v1":          true,
	"/api/v1/":         true,
	"/_debug":          true,
	"/wp-admin":        true,
	"/wp-login.php":    true,
	"/.env":            true,
	"/.git/config":     true,
	"/.git/HEAD":       true,
	"/phpmyadmin":      true,
	"/admin":           true,
	"/admin/":          true,
	"/debug":           true,
	"/server-status":   true,
	"/.well-known":     true,
	"/api/swagger":     true,
	"/api/docs":        true,
	"/api/graphql":     true,
}

func initDiscordAlerts() {
	alertOnce.Do(func() {
		discordWebhookURL = os.Getenv("DISCORD_SECURITY_WEBHOOK")
		if discordWebhookURL == "" {
			slog.Warn("DISCORD_SECURITY_WEBHOOK not set, alerts disabled")
			return
		}
		alertQueue = make(chan discordAlert, 300)
		for i := 0; i < 2; i++ {
			go alertWorker(i)
		}
		slog.Info("discord webhook alerts enabled", "workers", 2, "buffer", 300)
	})
}

func alertWorker(id int) {
	cooldown := make(map[string]time.Time)
	for alert := range alertQueue {
		key := fmt.Sprintf("%s:%s", alert.eventType, alert.ip)
		if last, ok := cooldown[key]; ok && time.Since(last) < 10*time.Second {
			continue
		}
		cooldown[key] = time.Now()

		embed := buildEmbed(alert)
		payload := discordWebhookPayload{Embeds: []discordEmbed{embed}}
		body, _ := json.Marshal(payload)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordWebhookURL, strings.NewReader(string(body)))
		if err != nil {
			cancel()
			slog.Error("discord worker build request failed", "worker", id, "error", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		cancel()
		if err != nil {
			slog.Error("discord worker send failed", "worker", id, "error", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 429 {
			slog.Warn("discord worker rate limited", "worker", id)
			time.Sleep(2 * time.Second)
		}
	}
}

func buildEmbed(alert discordAlert) discordEmbed {
	title := alertTitles[alert.eventType]
	if title == "" {
		title = alert.eventType
	}
	color := alertColors[alert.eventType]
	if color == 0 {
		color = 0x808080
	}

	desc := fmt.Sprintf("**IP:** `%s`\n**Path:** `%s`", alert.ip, alert.path)

	var fields []discordEmbedField
	if detailMap, ok := alert.detail.(map[string]string); ok {
		for k, v := range detailMap {
			if k == "ua" && len(v) > 100 {
				v = v[:100] + "..."
			}
			fields = append(fields, discordEmbedField{
				Name:   k,
				Value:  "`" + v + "`",
				Inline: true,
			})
		}
	}

	return discordEmbed{
		Title:       "🛡️ " + title,
		Description: desc,
		Color:       color,
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "SMLT Security"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func securityEvent(ctx context.Context, eventType, ip, path string, detail interface{}) {
	slog.Info("security event", "type", eventType, "ip", ip, "path", path)
	if fsClient == nil {
		return
	}
	_, err := fsClient.Collection("security_events").NewDoc().Set(ctx, SecurityEvent{
		Type:      eventType,
		IP:        ip,
		Path:      path,
		Detail:    detail,
		CreatedAt: time.Now(),
	})
	if err != nil {
		slog.Error("security event write failed", "error", err)
	}

	alertSecurityEvent(eventType, ip, path, detail)
}

func alertSecurityEvent(eventType, ip, path string, detail interface{}) {
	slog.Info("security alert", "type", eventType, "ip", ip, "path", path)
	if alertQueue == nil {
		return
	}
	select {
	case alertQueue <- discordAlert{eventType: eventType, ip: ip, path: path, detail: detail}:
	default:
		slog.Warn("discord alert queue full, dropping event", "type", eventType)
	}
}

func alertLoginFailure(ip string, reason string) {
	alertSecurityEvent("login_failed", ip, "/api/login", map[string]string{"reason": reason})
}

func alertHoneypot(ip, path, method, ua string) {
	alertSecurityEvent("honeypot_triggered", ip, path, map[string]string{
		"method": method,
		"ua":     ua,
	})
}

func isHoneypot(path string) bool {
	cleaned := strings.TrimSuffix(path, "/")
	if honeypotPaths[cleaned] || honeypotPaths[cleaned+"/"] {
		return true
	}
	for p := range honeypotPaths {
		if strings.HasPrefix(cleaned, p) {
			return true
		}
	}
	return false
}

func handleHoneypot(w http.ResponseWriter, r *http.Request) {
	ip := getRealIP(r)

	securityEvent(r.Context(), "honeypot_triggered", ip, r.URL.Path, map[string]string{
		"method": r.Method,
		"ua":     r.UserAgent(),
	})
	alertHoneypot(ip, r.URL.Path, r.Method, r.UserAgent())

	if cookie, err := r.Cookie("__Host-auth_token"); err == nil && cookie.Value != "" {
		claims := &jwt.MapClaims{}
		if _, parseErr := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			kid, _ := t.Header["kid"].(string)
			return lookupJWTSecret(kid)
		}); parseErr == nil {
			if jti, ok := (*claims)["jti"].(string); ok && jti != "" {
				blacklistToken(r.Context(), jti)
				securityEvent(r.Context(), "honeypot_token_blacklisted", ip, r.URL.Path, map[string]string{
					"jti": jti,
				})
				alertSecurityEvent("honeypot_token_blacklisted", ip, r.URL.Path, map[string]string{
					"jti": jti,
				})
			}
		}
	}

	time.Sleep(time.Duration(50+mathrand.IntN(200)) * time.Millisecond)
	sendError(w, http.StatusNotFound, "Роут не найден")
}

func generateFingerprint(r *http.Request) string {
	ua := r.UserAgent()
	al := r.Header.Get("Accept-Language")
	ae := r.Header.Get("Accept-Encoding")
	accept := r.Header.Get("Accept")

	raw := strings.ToLower(ua + "|" + al + "|" + ae + "|" + accept)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:12])
}

func isDeviceBanned(ctx context.Context, fingerprint string) bool {
	if fsClient == nil || fingerprint == "" {
		return false
	}
	doc, err := fsClient.Collection("device_bans").Doc(fingerprint).Get(ctx)
	if err != nil {
		return false
	}
	var ban DeviceBan
	if err := doc.DataTo(&ban); err != nil {
		return false
	}
	if time.Now().Before(ban.ExpiresAt) {
		return true
	}
	doc.Ref.Delete(ctx)
	return false
}

func banDevice(ctx context.Context, fingerprint, ip, ua, reason, bannedBy string, duration time.Duration) error {
	if fsClient == nil {
		return errors.New("firestore not available")
	}
	ban := DeviceBan{
		Fingerprint: fingerprint,
		IP:          ip,
		UA:          ua,
		Reason:      reason,
		BannedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(duration),
		BannedBy:    bannedBy,
	}
	_, err := fsClient.Collection("device_bans").Doc(fingerprint).Set(ctx, ban)
	return err
}

func unbanDevice(ctx context.Context, fingerprint string) error {
	if fsClient == nil {
		return errors.New("firestore not available")
	}
	_, err := fsClient.Collection("device_bans").Doc(fingerprint).Delete(ctx)
	return err
}

func isIPBanned(ctx context.Context, ip string) bool {
	if fsClient == nil || ip == "" {
		return false
	}
	doc, err := fsClient.Collection("ip_bans").Doc(ip).Get(ctx)
	if err != nil {
		return false
	}
	var ban IPBan
	if err := doc.DataTo(&ban); err != nil {
		return false
	}
	if time.Now().Before(ban.ExpiresAt) {
		return true
	}
	doc.Ref.Delete(ctx)
	return false
}

func banIP(ctx context.Context, ip, reason, bannedBy string, duration time.Duration) error {
	if fsClient == nil {
		return errors.New("firestore not available")
	}
	ban := IPBan{
		IP:        ip,
		Reason:    reason,
		BannedAt:  time.Now(),
		ExpiresAt: time.Now().Add(duration),
		BannedBy:  bannedBy,
	}
	_, err := fsClient.Collection("ip_bans").Doc(ip).Set(ctx, ban)
	return err
}

func unbanIP(ctx context.Context, ip string) error {
	if fsClient == nil {
		return errors.New("firestore not available")
	}
	_, err := fsClient.Collection("ip_bans").Doc(ip).Delete(ctx)
	return err
}

func handleIPBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}

	var req struct {
		IP       string `json:"ip"`
		Reason   string `json:"reason"`
		Duration int    `json:"duration"` // seconds, 0 = 24h
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный запрос")
		return
	}
	req.IP = strings.TrimSpace(req.IP)
	if net.ParseIP(req.IP) == nil {
		sendError(w, http.StatusBadRequest, "Неверный IP")
		return
	}
	if req.Reason == "" {
		req.Reason = "Без причины"
	}
	dur := 24 * time.Hour
	if req.Duration > 0 {
		dur = time.Duration(req.Duration) * time.Second
	}

	adminIP := getRealIP(r)
	if err := banIP(r.Context(), req.IP, req.Reason, adminIP, dur); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка бана")
		return
	}

	securityEvent(r.Context(), "ip_banned_manual", req.IP, "/api/security/ip-ban", map[string]string{
		"reason":   req.Reason,
		"duration": dur.String(),
		"admin":    adminIP,
	})
	alertSecurityEvent("ip_banned_manual", req.IP, "/api/security/ip-ban", map[string]string{
		"reason": req.Reason,
		"admin":  adminIP,
	})

	writeJSON(w, map[string]string{"status": "banned", "ip": req.IP})
}

func handleIPUnban(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}

	var req struct {
		IP string `json:"ip"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный запрос")
		return
	}
	req.IP = strings.TrimSpace(req.IP)
	if req.IP == "" {
		sendError(w, http.StatusBadRequest, "IP обязателен")
		return
	}

	if err := unbanIP(r.Context(), req.IP); err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка разбана")
		return
	}

	writeJSON(w, map[string]string{"status": "unbanned", "ip": req.IP})
}

func handleSecurityDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}

	type eventCount struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}
	type recentEvent struct {
		Type      string    `json:"type"`
		IP        string    `json:"ip"`
		Path      string    `json:"path"`
		CreatedAt time.Time `json:"createdAt"`
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	since := time.Now().Add(-24 * time.Hour)

	iter := fsClient.Collection("security_events").
		Where("createdAt", ">=", since).
		Documents(ctx)
	defer iter.Stop()

	total := 0
	byType := make(map[string]int)
	topIPs := make(map[string]int)
	var recent []recentEvent

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			slog.Error("security dashboard iter error", "error", err)
			break
		}
		var ev SecurityEvent
		if err := doc.DataTo(&ev); err != nil {
			continue
		}
		total++
		byType[ev.Type]++
		ipHash := hashIP(ev.IP)
		topIPs[ipHash]++
		if len(recent) < 20 {
			recent = append(recent, recentEvent{
				Type:      ev.Type,
				IP:        ipHash,
				Path:      ev.Path,
				CreatedAt: ev.CreatedAt,
			})
		}
	}

	typeCounts := make([]eventCount, 0, len(byType))
	for t, c := range byType {
		typeCounts = append(typeCounts, eventCount{Type: t, Count: c})
	}
	sort.Slice(typeCounts, func(i, j int) bool {
		return typeCounts[i].Count > typeCounts[j].Count
	})

	type ipCount struct {
		IP    string `json:"ip"`
		Count int    `json:"count"`
	}
	ipCounts := make([]ipCount, 0, len(topIPs))
	for ip, c := range topIPs {
		ipCounts = append(ipCounts, ipCount{IP: ip, Count: c})
	}
	sort.Slice(ipCounts, func(i, j int) bool {
		return ipCounts[i].Count > ipCounts[j].Count
	})
	if len(ipCounts) > 10 {
		ipCounts = ipCounts[:10]
	}

	writeJSON(w, map[string]interface{}{
		"period": "24h",
		"total":  total,
		"byType": typeCounts,
		"topIPs": ipCounts,
		"recent": recent,
	})
}
