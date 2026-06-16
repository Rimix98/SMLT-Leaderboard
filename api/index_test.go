package handler

import (
	"crypto/subtle"
	"net/http"
	"net/http/httptest"
	"testing"

	"net/url"
)

func TestHashIP_Deterministic(t *testing.T) {
	rateLimitSalt = "test-salt-12345"
	a := hashIP("192.168.1.1")
	b := hashIP("192.168.1.1")
	if a != b {
		t.Errorf("hashIP not deterministic: %s != %s", a, b)
	}
}

func TestHashIP_DifferentInputs(t *testing.T) {
	rateLimitSalt = "test-salt-12345"
	a := hashIP("192.168.1.1")
	b := hashIP("192.168.1.2")
	if a == b {
		t.Error("hashIP produced same hash for different IPs")
	}
}

func TestConstantTimeCompare(t *testing.T) {
	a := []byte("secret123")
	b := []byte("secret123")
	c := []byte("wrong456")
	if subtle.ConstantTimeCompare(a, b) != 1 {
		t.Error("ConstantTimeCompare should match equal slices")
	}
	if subtle.ConstantTimeCompare(a, c) != 0 {
		t.Error("ConstantTimeCompare should not match different slices")
	}
}

func TestIsBlockedBot(t *testing.T) {
	if !isBlockedBot("") {
		t.Error("empty UA should be blocked")
	}
	if !isBlockedBot("sqlmap/1.0") {
		t.Error("sqlmap UA should be blocked")
	}
	if isBlockedBot("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36") {
		t.Error("normal Chrome UA should not be blocked")
	}
}

func TestIsBlockedPath(t *testing.T) {
	if !isBlockedPath("/wp-admin") {
		t.Error("/wp-admin should be blocked")
	}
	if !isBlockedPath("/.htaccess") {
		t.Error("/.htaccess should be blocked")
	}
	if isBlockedPath("/api/leaderboard") {
		t.Error("/api/leaderboard should not be blocked")
	}
}

func TestIsHoneypot(t *testing.T) {
	if !isHoneypot("/api/admin") {
		t.Error("/api/admin should be honeypot")
	}
	if !isHoneypot("/api/debug") {
		t.Error("/api/debug should be honeypot")
	}
	if isHoneypot("/api/leaderboard") {
		t.Error("/api/leaderboard should not be honeypot")
	}
}

func TestValidateProjectID(t *testing.T) {
	if err := validateProjectID("proj_abc_123"); err != nil {
		t.Errorf("valid project ID rejected: %v", err)
	}
	if err := validateProjectID(""); err == nil {
		t.Error("empty project ID should be rejected")
	}
	if err := validateProjectID("../../etc/passwd"); err == nil {
		t.Error("path traversal ID should be rejected")
	}
}

func TestValidateNickname(t *testing.T) {
	if err := validateNickname("TestPlayer"); err != nil {
		t.Errorf("valid nickname rejected: %v", err)
	}
	if err := validateNickname(""); err == nil {
		t.Error("empty nickname should be rejected")
	}
	if err := validateNickname("ThisIsAVeryLongNicknameThatExceeds32Chars"); err == nil {
		t.Error("too long nickname should be rejected")
	}
}

func TestValidateRoleName(t *testing.T) {
	if err := validateRoleName("GP"); err != nil {
		t.Errorf("valid role name rejected: %v", err)
	}
	if err := validateRoleName("A"); err == nil {
		t.Error("1-char role name should be rejected")
	}
	if err := validateRoleName("<script>alert(1)</script>"); err == nil {
		t.Error("XSS role name should be rejected")
	}
}

func TestValidateDiscord(t *testing.T) {
	if err := validateDiscord(""); err != nil {
		t.Error("empty discord should be allowed")
	}
	if err := validateDiscord("user#1234"); err != nil {
		t.Errorf("valid discord rejected: %v", err)
	}
	if err := validateDiscord("user name#1234"); err != nil {
		t.Errorf("discord with space rejected: %v", err)
	}
}

func TestReHexColor(t *testing.T) {
	tests := []struct{ input string; valid bool }{
		{"#ff0000", true},
		{"ff0000", true},
		{"#FFF", false},
		{"red", false},
		{"#gggggg", false},
	}
	for _, tt := range tests {
		matched := reHexColor.MatchString(tt.input)
		if matched != tt.valid {
			t.Errorf("reHexColor(%q) = %v, want %v", tt.input, matched, tt.valid)
		}
	}
}

func TestReVideoID(t *testing.T) {
	tests := []struct{ input string; valid bool }{
		{"dQw4w9WgXcQ", true},
		{"short", false},
		{"dQw4w9WgXc", false},
	}
	for _, tt := range tests {
		matched := reVideoID.MatchString(tt.input)
		if matched != tt.valid {
			t.Errorf("reVideoID(%q) = %v, want %v", tt.input, matched, tt.valid)
		}
	}
}

func TestRequestPath(t *testing.T) {
	tests := []struct{ path, requestURI, expected string }{
		{"/api/leaderboard", "/api/leaderboard", "/api/leaderboard"},
		{"/api", "/api", "/api"},
		{"/api/", "/api/", "/api/"},
		{"/", "/", "/"},
	}
	for _, tt := range tests {
		u, _ := url.Parse(tt.path)
		r := &http.Request{URL: u, RequestURI: tt.requestURI}
		got := requestPath(r)
		if got != tt.expected {
			t.Errorf("requestPath(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}

func TestSendError(t *testing.T) {
	w := httptest.NewRecorder()
	sendError(w, http.StatusBadRequest, "test error")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"hello", "hello"},
		{"<script>alert(1)</script>", ""},
		{"  spaces  ", "spaces"},
		{"<b>bold</b>", "<b>bold</b>"},
	}
	for _, tt := range tests {
		got := sanitizeString(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeString(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
	if got := sanitizeString("<img src=x onerror=alert(1)>"); got != "<img src=\"x\">" {
		t.Errorf("sanitizeString onerror should be stripped, got %q", got)
	}
	if got := sanitizeString("safe < notags"); got != "safe &lt; notags" {
		t.Errorf("sanitizeString bare < should be escaped, got %q", got)
	}
}
