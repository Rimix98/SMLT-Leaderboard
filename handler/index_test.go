package handler

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
)

// ──────────────────────────────────────────────
// remoteAddrIP
// ──────────────────────────────────────────────

func TestRemoteAddrIP(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected string
	}{
		{"ipv4 with port", "192.168.1.1:8080", "192.168.1.1"},
		{"ipv6 with port", "[::1]:8080", "::1"},
		{"ipv4 no port", "10.0.0.1", "10.0.0.1"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tt.addr}
			got := remoteAddrIP(r)
			if got != tt.expected {
				t.Errorf("remoteAddrIP(%q) = %q, want %q", tt.addr, got, tt.expected)
			}
		})
	}
}

// ──────────────────────────────────────────────
// getRealIP
// ──────────────────────────────────────────────

func TestGetRealIP_NoProxy(t *testing.T) {
	trustProxy = false
	defer func() { trustProxy = false }()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	got := getRealIP(r)
	if got != "1.2.3.4" {
		t.Errorf("getRealIP = %q, want 1.2.3.4", got)
	}
}

func TestGetRealIP_XForwardedFor(t *testing.T) {
	trustProxy = true
	defer func() { trustProxy = false }()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	got := getRealIP(r)
	if got != "203.0.113.50" {
		t.Errorf("getRealIP = %q, want 203.0.113.50", got)
	}
}

func TestGetRealIP_XVercelForwardedFor(t *testing.T) {
	trustProxy = true
	defer func() { trustProxy = false }()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:80"
	r.Header.Set("X-Vercel-Forwarded-For", "198.51.100.1")
	r.Header.Set("X-Forwarded-For", "203.0.113.50")
	got := getRealIP(r)
	if got != "198.51.100.1" {
		t.Errorf("getRealIP = %q, want 198.51.100.1", got)
	}
}

func TestGetRealIP_InvalidForwardedFor(t *testing.T) {
	trustProxy = true
	defer func() { trustProxy = false }()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "not-an-ip")
	got := getRealIP(r)
	if got != "127.0.0.1" {
		t.Errorf("getRealIP = %q, want 127.0.0.1", got)
	}
}

// ──────────────────────────────────────────────
// hashIP
// ──────────────────────────────────────────────

func TestHashIP_Deterministic(t *testing.T) {
	rateLimitSalt = "test-salt-value"
	a := hashIP("1.2.3.4")
	b := hashIP("1.2.3.4")
	if a != b {
		t.Errorf("hashIP not deterministic: %q != %q", a, b)
	}
}

func TestHashIP_DifferentIPs(t *testing.T) {
	rateLimitSalt = "test-salt-value"
	a := hashIP("1.2.3.4")
	b := hashIP("5.6.7.8")
	if a == b {
		t.Errorf("hashIP produced same hash for different IPs: %q", a)
	}
}

func TestHashIP_ReturnsHex(t *testing.T) {
	rateLimitSalt = "test-salt-value"
	got := hashIP("10.0.0.1")
	if _, err := hex.DecodeString(got); err != nil {
		t.Errorf("hashIP did not return valid hex: %q", got)
	}
}

// ──────────────────────────────────────────────
// isBlockedBot
// ──────────────────────────────────────────────

func TestIsBlockedBot_Empty(t *testing.T) {
	if !isBlockedBot("") {
		t.Error("empty UA should be blocked")
	}
}

func TestIsBlockedBot_KnownBots(t *testing.T) {
	bots := []string{
		"sqlmap/1.5",
		"nikto/2.1.6",
		"Mozilla/5.0 (compatible; Nmap Scripting Engine)",
		"curl/7.68.0",
		"Wget/1.21",
		"python-requests/2.25.1",
		"Go-http-client/1.1",
		"Java/11.0.2",
		"BurpSuite/2023.1",
		"Mozilla/5.0 (hydra)",
		"Nuclei - Open-source project",
	}
	for _, ua := range bots {
		if !isBlockedBot(ua) {
			t.Errorf("should block bot UA: %q", ua)
		}
	}
}

func TestIsBlockedBot_BrowserAllowed(t *testing.T) {
	browsers := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 OPR/106.0.0.0",
	}
	for _, ua := range browsers {
		if isBlockedBot(ua) {
			t.Errorf("should NOT block browser UA: %q", ua)
		}
	}
}

func TestIsBlockedBot_BrowserWithBlockedWord(t *testing.T) {
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 curl/7.68.0"
	if !isBlockedBot(ua) {
		t.Error("browser UA containing 'curl' should be blocked")
	}
}

// ──────────────────────────────────────────────
// isBlockedPath
// ──────────────────────────────────────────────

func TestIsBlockedPath(t *testing.T) {
	blocked := []string{
		"/wp-admin",
		"/wp-login.php",
		"/xmlrpc.php",
		"/administrator",
		"/config.php",
		"/shell.php",
		"/.htaccess",
		"/web.config",
		"/c99",
		"/r57",
	}
	for _, p := range blocked {
		if !isBlockedPath(p) {
			t.Errorf("should block path: %q", p)
		}
	}
}

func TestIsBlockedPath_Allowed(t *testing.T) {
	allowed := []string{
		"/api/leaderboard",
		"/api/projects",
		"/api/login",
		"/index.html",
		"/leaderboard.html",
	}
	for _, p := range allowed {
		if isBlockedPath(p) {
			t.Errorf("should NOT block path: %q", p)
		}
	}
}

// ──────────────────────────────────────────────
// sanitizeString
// ──────────────────────────────────────────────

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello world", "hello world"},
		{"strips html", "<script>alert(1)</script>", ""},
		{"keeps safe tags", "<b>bold</b>", "<b>bold</b>"},
		{"trims whitespace", "  hello  ", "hello"},
		{"empty", "", ""},
		{"unicode", "Привет мир", "Привет мир"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeString(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ──────────────────────────────────────────────
// ipToDocID
// ──────────────────────────────────────────────

func TestIpToDocID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ipv4", "192.168.1.1", "192-168-1-1"},
		{"ipv6", "::1", "--1"},
		{"ipv6 full", "2001:db8::1", "2001-db8--1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ipToDocID(tt.input)
			if got != tt.expected {
				t.Errorf("ipToDocID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ──────────────────────────────────────────────
// Validation functions
// ──────────────────────────────────────────────

func TestValidateProjectID(t *testing.T) {
	valid := []string{"my-project", "test_123", "a", "A-B_C-d-50"}
	for _, id := range valid {
		if err := validateProjectID(id); err != nil {
			t.Errorf("validateProjectID(%q) should be valid: %v", id, err)
		}
	}
	invalid := []string{"", "has space", "special!@#", "too$long", "../traversal"}
	for _, id := range invalid {
		if err := validateProjectID(id); err == nil {
			t.Errorf("validateProjectID(%q) should be invalid", id)
		}
	}
}

func TestValidateProjectID_MaxLength(t *testing.T) {
	if err := validateProjectID("a"); err != nil {
		t.Errorf("single char should be valid: %v", err)
	}
	long := ""
	for i := 0; i < 51; i++ {
		long += "a"
	}
	if err := validateProjectID(long); err == nil {
		t.Error("51 chars should be invalid")
	}
}

func TestValidateNickname(t *testing.T) {
	if err := validateNickname("Player1"); err != nil {
		t.Errorf("valid nickname should pass: %v", err)
	}
	if err := validateNickname(""); err == nil {
		t.Error("empty nickname should fail")
	}
	if err := validateNickname("a"); err != nil {
		t.Errorf("single char nickname should pass: %v", err)
	}
	long := ""
	for i := 0; i < 33; i++ {
		long += "a"
	}
	if err := validateNickname(long); err == nil {
		t.Error("33 char nickname should fail")
	}
}

func TestValidateDiscord(t *testing.T) {
	if err := validateDiscord(""); err != nil {
		t.Errorf("empty discord should be valid: %v", err)
	}
	if err := validateDiscord("user#1234"); err != nil {
		t.Errorf("valid discord should pass: %v", err)
	}
	if err := validateDiscord("User Name#5678"); err != nil {
		t.Errorf("valid discord with spaces should pass: %v", err)
	}
	long := ""
	for i := 0; i < 65; i++ {
		long += "a"
	}
	if err := validateDiscord(long); err == nil {
		t.Error("65 char discord should fail")
	}
	if err := validateDiscord("user<script>"); err == nil {
		t.Error("discord with angle brackets should fail")
	}
}

func TestValidateRoleName(t *testing.T) {
	if err := validateRoleName("Admin"); err != nil {
		t.Errorf("valid role name should pass: %v", err)
	}
	if err := validateRoleName("A"); err == nil {
		t.Error("single char role name should fail")
	}
	long := ""
	for i := 0; i < 33; i++ {
		long += "a"
	}
	if err := validateRoleName(long); err == nil {
		t.Error("33 char role name should fail")
	}
	if err := validateRoleName("admin<script>"); err == nil {
		t.Error("role name with angle brackets should fail")
	}
}

// ──────────────────────────────────────────────
// generateJTI / generateAdminKey
// ──────────────────────────────────────────────

func TestGenerateJTI(t *testing.T) {
	jti, err := generateJTI()
	if err != nil {
		t.Fatalf("generateJTI error: %v", err)
	}
	if len(jti) != 32 {
		t.Errorf("JTI length = %d, want 32", len(jti))
	}
	if _, err := hex.DecodeString(jti); err != nil {
		t.Errorf("JTI is not valid hex: %q", jti)
	}
}

func TestGenerateJTI_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		jti, err := generateJTI()
		if err != nil {
			t.Fatalf("generateJTI error: %v", err)
		}
		if seen[jti] {
			t.Fatalf("duplicate JTI: %q", jti)
		}
		seen[jti] = true
	}
}

func TestGenerateAdminKey(t *testing.T) {
	key, err := generateAdminKey()
	if err != nil {
		t.Fatalf("generateAdminKey error: %v", err)
	}
	if len(key) != 64 {
		t.Errorf("admin key length = %d, want 64", len(key))
	}
	if _, err := hex.DecodeString(key); err != nil {
		t.Errorf("admin key is not valid hex: %q", key)
	}
}

func TestGenerateAdminKey_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key, err := generateAdminKey()
		if err != nil {
			t.Fatalf("generateAdminKey error: %v", err)
		}
		if seen[key] {
			t.Fatalf("duplicate admin key: %q", key)
		}
		seen[key] = true
	}
}

// ──────────────────────────────────────────────
// requestPath
// ──────────────────────────────────────────────

func TestRequestPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		rawURI   string
		expected string
	}{
		{"normal path", "/api/leaderboard", "/api/leaderboard", "/api/leaderboard"},
		{"api root", "/api", "/api?foo=bar", "/api?foo=bar"},
		{"api trailing slash", "/api/", "/api/?foo=bar", "/api/?foo=bar"},
		{"no trailing slash", "/api/projects", "/api/projects", "/api/projects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.rawURI, nil)
			r.URL.Path = tt.path
			got := requestPath(r)
			if got != tt.expected {
				t.Errorf("requestPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ──────────────────────────────────────────────
// isHoneypot
// ──────────────────────────────────────────────

func TestIsHoneypot(t *testing.T) {
	blocked := []string{
		"/wp-admin",
		"/.env",
		"/debug",
		"/api/admin",
		"/phpmyadmin",
		"/admin",
		"/.git/config",
	}
	for _, p := range blocked {
		if !isHoneypot(p) {
			t.Errorf("should detect honeypot path: %q", p)
		}
	}
}

func TestIsHoneypot_Allowed(t *testing.T) {
	allowed := []string{
		"/api/leaderboard",
		"/api/projects",
		"/api/login",
		"/index.html",
		"/",
	}
	for _, p := range allowed {
		if isHoneypot(p) {
			t.Errorf("should NOT be honeypot: %q", p)
		}
	}
}

// ──────────────────────────────────────────────
// defaultPlayersList
// ──────────────────────────────────────────────

func TestDefaultPlayersList(t *testing.T) {
	players := defaultPlayersList()
	if len(players) == 0 {
		t.Error("defaultPlayersList returned empty")
	}
	for i, p := range players {
		if p.Name == "" {
			t.Errorf("player %d has empty name", i)
		}
	}
}

// ──────────────────────────────────────────────
// Regex patterns
// ──────────────────────────────────────────────

func TestRegexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		re      *regexp.Regexp
		valid   []string
		invalid []string
	}{
		{
			name:    "reProjectID",
			re:      reProjectID,
			valid:   []string{"a", "my-project", "test_123", "ABC-DEF-1234567890"},
			invalid: []string{"", "has space", "special!@#", "a]"},
		},
		{
			name:    "reVideoID",
			re:      reVideoID,
			valid:   []string{"dQw4w9WgXcQ", "abc123DEFG_"},
			invalid: []string{"", "short", "too-long-video-id-123456", "special!"},
		},
		{
			name:    "reHexColor",
			re:      reHexColor,
			valid:   []string{"#ff0000", "ff0000", "00ff00", "#00FF00"},
			invalid: []string{"", "red", "#fff", "#gggggg", "ff00"},
		},
		{
			name:    "reCaptchaID",
			re:      reCaptchaID,
			valid:   []string{"abc12345", "a1b2c3d4e5f6g7h8"},
			invalid: []string{"", "short", "special!@#"},
		},
		{
			name:    "reAlphanumeric",
			re:      reAlphanumeric,
			valid:   []string{"Player1", "user name", "test.key", "a-b"},
			invalid: []string{"", "test<script>", "user@name"},
		},
		{
			name:    "reDiscord",
			re:      reDiscord,
			valid:   []string{"user#1234", "User Name", "test.discrim"},
			invalid: []string{"user<script>", "user@name"},
		},
		{
			name:    "reRoleName",
			re:      reRoleName,
			valid:   []string{"Admin", "Team Leader", "test-role", "A"},
			invalid: []string{"admin<script>", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"_valid", func(t *testing.T) {
			for _, s := range tt.valid {
				if !tt.re.MatchString(s) {
					t.Errorf("%s should match %q", tt.name, s)
				}
			}
		})
		t.Run(tt.name+"_invalid", func(t *testing.T) {
			for _, s := range tt.invalid {
				if tt.re.MatchString(s) {
					t.Errorf("%s should NOT match %q", tt.name, s)
				}
			}
		})
	}
}

// ──────────────────────────────────────────────
// Token version cache
// ──────────────────────────────────────────────

func TestTokenVersionCache(t *testing.T) {
	entry := &tokenVersionCacheEntry{
		version:   5,
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	tokenVerCache.Store("tokenVersion", entry)

	loaded, ok := tokenVerCache.Load("tokenVersion")
	if !ok {
		t.Fatal("token version cache miss")
	}
	e := loaded.(*tokenVersionCacheEntry)
	if e.version != 5 {
		t.Errorf("cached version = %d, want 5", e.version)
	}
	tokenVerCache.Delete("tokenVersion")
}

// ──────────────────────────────────────────────
// CORS headers in Handler
// ──────────────────────────────────────────────

func TestHandler_CORSHeaders(t *testing.T) {
	allowedOrigins := []string{
		"https://smltdemonlist.vercel.app",
		"https://smlt-demonlist.ru",
		"https://www.smlt-demonlist.ru",
	}
	for _, origin := range allowedOrigins {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
		Handler(w, r)
		got := w.Header().Get("Access-Control-Allow-Origin")
		if got != origin {
			t.Errorf("CORS for %s: got %q, want %q", origin, got, origin)
		}
	}
}

func TestHandler_CORSBlocked(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	r.Header.Set("Origin", "https://evil.com")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	Handler(w, r)
	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Errorf("CORS should not set header for evil origin, got %q", got)
	}
}

func TestHandler_SecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	Handler(w, r)
	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"X-XSS-Protection":      "1; mode=block",
		"Referrer-Policy":       "strict-origin-when-cross-origin",
	}
	for k, want := range headers {
		got := w.Header().Get(k)
		if got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
}

func TestHandler_RequestID(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	Handler(w, r)
	reqID := w.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Error("X-Request-ID header not set")
	}
	if _, err := hex.DecodeString(reqID); err != nil {
		t.Errorf("X-Request-ID is not valid hex: %q", reqID)
	}
}

func TestHandler_OPTIONS(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/api/leaderboard", nil)
	Handler(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS should return 204, got %d", w.Code)
	}
}

func TestHandler_NotFound(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	Handler(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown route should return 404, got %d", w.Code)
	}
}

// ──────────────────────────────────────────────
// normalizeColor
// ──────────────────────────────────────────────

func TestNormalizeColor(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantColor string
		wantErr   bool
	}{
		{"empty defaults to blue", "", "#3b82f6", false},
		{"valid with hash", "#ff0000", "#ff0000", false},
		{"valid without hash", "ff0000", "#ff0000", false},
		{"valid uppercase", "#00FF00", "#00FF00", false},
		{"valid mixed case", "Aa1b2C", "#Aa1b2C", false},
		{"invalid hex", "red", "", true},
		{"too short", "#fff", "", true},
		{"too long", "#ffffff00", "", true},
		{"special chars", "#gggggg", "", true},
		{"spaces", "#ff 00 00", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeColor(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeColor(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.wantColor {
				t.Errorf("normalizeColor(%q) = %q, want %q", tt.input, got, tt.wantColor)
			}
		})
	}
}

// ──────────────────────────────────────────────
// Discord webhook alerts
// ──────────────────────────────────────────────

func TestBuildEmbed(t *testing.T) {
	tests := []struct {
		name      string
		alert     discordAlert
		wantTitle string
		wantColor int
	}{
		{
			name:      "honeypot",
			alert:     discordAlert{eventType: "honeypot_triggered", ip: "1.2.3.4", path: "/wp-admin", detail: map[string]string{"method": "GET", "ua": "sqlmap"}},
			wantTitle: "🛡️ Honeypot triggered",
			wantColor: 0xFF0000,
		},
		{
			name:      "login_failed",
			alert:     discordAlert{eventType: "login_failed", ip: "5.6.7.8", path: "/api/login", detail: map[string]string{"reason": "wrong_password"}},
			wantTitle: "🛡️ Login failed",
			wantColor: 0xFFAA00,
		},
		{
			name:      "unknown event",
			alert:     discordAlert{eventType: "something_new", ip: "1.2.3.4", path: "/api/test"},
			wantTitle: "🛡️ something_new",
			wantColor: 0x808080,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embed := buildEmbed(tt.alert)
			if embed.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", embed.Title, tt.wantTitle)
			}
			if embed.Color != tt.wantColor {
				t.Errorf("color = %d, want %d", embed.Color, tt.wantColor)
			}
			if embed.Timestamp == "" {
				t.Error("timestamp should not be empty")
			}
			if embed.Footer == nil || embed.Footer.Text != "SMLT Security" {
				t.Error("footer should be 'SMLT Security'")
			}
		})
	}
}

func TestBuildEmbed_FieldsFromDetail(t *testing.T) {
	alert := discordAlert{
		eventType: "bot_blocked",
		ip:        "10.0.0.1",
		path:      "/api/test",
		detail:    map[string]string{"ua": "sqlmap/1.5"},
	}
	embed := buildEmbed(alert)
	if len(embed.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(embed.Fields))
	}
	if embed.Fields[0].Name != "ua" {
		t.Errorf("field name = %q, want ua", embed.Fields[0].Name)
	}
}

func TestBuildEmbed_TruncatesLongUA(t *testing.T) {
	longUA := ""
	for i := 0; i < 200; i++ {
		longUA += "a"
	}
	alert := discordAlert{
		eventType: "bot_blocked",
		ip:        "10.0.0.1",
		path:      "/",
		detail:    map[string]string{"ua": longUA},
	}
	embed := buildEmbed(alert)
	val := embed.Fields[0].Value
	if len(val) > 120 {
		t.Errorf("UA field too long: %d chars", len(val))
	}
}

func TestAlertSecurityEvent_QueueNil(t *testing.T) {
	alertQueue = nil
	alertSecurityEvent("test", "1.2.3.4", "/test", nil)
}

func TestAlertSecurityEvent_QueueFull(t *testing.T) {
	q := make(chan discordAlert, 1)
	q <- discordAlert{eventType: "existing"}
	alertQueue = q
	defer func() { alertQueue = nil }()
	alertSecurityEvent("new_event", "1.2.3.4", "/test", nil)
	if len(q) != 1 {
		t.Errorf("queue should still have 1 item, got %d", len(q))
	}
}

// ──────────────────────────────────────────────
// Path traversal protection
// ──────────────────────────────────────────────

func TestPlayerHistory_TraversalPath(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/history/../../../etc/passwd", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	Handler(w, r)
	// Should return 400 (invalid playerId) or 404, NOT 200
	if w.Code == http.StatusOK {
		t.Error("path traversal should not return 200")
	}
}

func TestPlayerHistory_ValidPlayerID(t *testing.T) {
	// Valid alphanumeric ID should pass validation (may fail on Firestore, but shouldn't be 400 for validation)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/history/TestPlayer123", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	Handler(w, r)
	// Should NOT be 400 for validation (may be 503 if no Firestore)
	if w.Code == http.StatusBadRequest {
		t.Error("valid player ID should pass validation")
	}
}

func TestPlayerHistory_TooLong(t *testing.T) {
	longID := ""
	for i := 0; i < 65; i++ {
		longID += "a"
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/history/"+longID, nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	Handler(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("oversized playerId should return 400, got %d", w.Code)
	}
}

func TestPlayerHistory_SpecialChars(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/history/<script>alert(1)</script>", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	Handler(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("XSS in playerId should return 400, got %d", w.Code)
	}
}

// ──────────────────────────────────────────────
// Context timeout (smoke test: ensure handler doesn't hang)
// ──────────────────────────────────────────────

func TestLeaderboard_TimeoutDoesNotHang(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	done := make(chan struct{})
	go func() {
		Handler(w, r)
		close(done)
	}()
	select {
	case <-done:
		// ok
	case <-time.After(45 * time.Second):
		t.Error("leaderboard handler timed out (should complete within 35s)")
	}
}

// ──────────────────────────────────────────────
// Validate helpers
// ──────────────────────────────────────────────

func TestValidateProjectID_SpecialChars(t *testing.T) {
	invalid := []string{
		"../traversal",
		"../../etc/passwd",
		"project/../../../secret",
		"project\x00null",
	}
	for _, id := range invalid {
		if err := validateProjectID(id); err == nil {
			t.Errorf("validateProjectID(%q) should be invalid", id)
		}
	}
}

func TestNormalizeColor_Empty(t *testing.T) {
	c, err := normalizeColor("")
	if err != nil {
		t.Errorf("empty color should default to blue: %v", err)
	}
	if c != "#3b82f6" {
		t.Errorf("empty color default = %q, want #3b82f6", c)
	}
}

func TestValidateRoleName_SpecialChars(t *testing.T) {
	invalid := []string{
		"<script>",
		"role<script>",
		"admin\"injection",
		"a",  // too short
		"",   // empty
	}
	for _, name := range invalid {
		if err := validateRoleName(name); err == nil {
			t.Errorf("validateRoleName(%q) should be invalid", name)
		}
	}
}

func TestValidateNickname_Boundary(t *testing.T) {
	// Exactly 32 chars should be valid
	valid32 := "abcdefghijklmnopqrstuvwxyz123456"
	if err := validateNickname(valid32); err != nil {
		t.Errorf("32-char nickname should be valid: %v", err)
	}
	// 33 chars should be invalid
	invalid33 := "abcdefghijklmnopqrstuvwxyz1234567"
	if err := validateNickname(invalid33); err == nil {
		t.Error("33-char nickname should be invalid")
	}
}
