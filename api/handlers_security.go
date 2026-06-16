package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	mathrand "math/rand/v2"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/iterator"
)

var blockedBotPatterns = []string{
	"sqlmap",
	"nikto",
	"nessus",
	"openvas",
	"w3af",
	"arachni",
	"skipfish",
	"whatweb",
	"dirbuster",
	"gobuster",
	"ffuf",
	"wfuzz",
	"masscan",
	"zgrab",
	"httpx",
	"nuclei",
	"jaeles",
	"xray",
	"vulmap",
	"pocsuite",
	"hydra",
	"medusa",
	"ncrack",
	"patator",
	"brutus",
	"metasploit",
	"burpsuite",
	"owasp",
	"acunetix",
	"appscan",
	"webinspect",
	"paros",
	"wparos",
	"webscarab",
	"mitmproxy",
	"charles",
	"fiddler",
	"grabber",
	"wapiti",
	"havij",
	"canari",
	"slowloris",
	"goldeneye",
	"slowhttptest",
	"rudy",
	"tor",
	"curl",
	"wget",
	"python",
	"perl",
	"ruby",
	"php/",
	"scrapy",
	"crawler",
	"spider",
	"scraper",
	"harvest",
	"extract",
	"scan",
	"exploit",
	"hack",
	"crack",
	"brute",
	"go-http-client",
	"java/",
	"libwww",
	"lwp-trivial",
	"webbandit",
	"webcopier",
	"webzip",
	"teleport",
	"sitecopy",
	"httrack",
	"clixboard",
	"cms探测",
	"nmap",
	"zmap",
	"unicornsql",
	"sqlbf",
	"sqlbrute",
	"sqlsmack",
	"sqlfury",
	"sqlninja",
	"bbqsql",
	"jsql",
	"qualys",
	"ratproxy",
	"websecurify",
	"netsparker",
}

var blockedPathPatterns = []string{
	"wp-admin",
	"wp-login",
	"xmlrpc.php",
	"wp-content",
	"wp-includes",
	"administrator",
	"config.php",
	"config.inc",
	"setup.php",
	"install.php",
	"shell.php",
	"cmd.php",
	"backdoor",
	"webshell",
	"c99",
	"r57",
	"b374k",
	" FilesMan",
	"File manager",
	".htaccess",
	"web.config",
	"crossdomain.xml",
}

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
		return nil
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
		return nil
	}
	_, err := fsClient.Collection("device_bans").Doc(fingerprint).Delete(ctx)
	return err
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
			log.Printf("[dashboard] iter error: %v", err)
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

func auditLog(ctx context.Context, entry AuditEntry) {
	if fsClient == nil {
		return
	}
	entry.CreatedAt = time.Now()
	_, err := fsClient.Collection("audit_log").NewDoc().Set(ctx, entry)
	if err != nil {
		log.Printf("[audit] failed to write log: %v", err)
	}
}

func securityEvent(ctx context.Context, eventType, ip, path string, detail interface{}) {
	log.Printf("[security] %s ip=%s path=%s", eventType, ip, path)
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
		log.Printf("[security] write failed: %v", err)
	}

	alertSecurityEvent(eventType, ip, path, detail)
}

func alertSecurityEvent(eventType, ip, path string, detail interface{}) {
	log.Printf("[security] %s ip=%s path=%s", eventType, ip, path)
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
				return nil, nil
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
