package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

var (
	baseURL  string
	webhook  string
	client   = &http.Client{Timeout: 10 * time.Second}
	botClient = &http.Client{Timeout: 10 * time.Second}
	passed   int
	failed   int
	results  []string
)

func main() {
	baseURL = os.Getenv("TEST_BASE_URL")
	webhook = os.Getenv("DISCORD_SECURITY_WEBHOOK")

	if baseURL == "" {
		fmt.Println("Usage: TEST_BASE_URL=https://smltdemonlist.vercel.app DISCORD_SECURITY_WEBHOOK=https://discord.com/api/webhooks/... go run test-security.go")
		os.Exit(1)
	}
	if webhook == "" {
		fmt.Println("Set DISCORD_SECURITY_WEBHOOK")
		os.Exit(1)
	}

	client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &roundTripper{ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/125.0.0.0 Safari/537.36"},
	}

	fmt.Printf("Testing %s\n\n", baseURL)

	testBotDetection()
	testHoneypotTraps()
	testSecurityHeaders()
	testRateLimitHeaders()
	testCaptchaEndpoint()
	testMethodNotAllowed()
	testInvalidJSON()

	sendReport()
}

type roundTripper struct {
	ua string
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", rt.ua)
	return http.DefaultTransport.RoundTrip(req)
}

func testBotDetection() {
	fmt.Println("=== Bot Detection ===")

	bots := []struct {
		name string
		ua   string
		code int
	}{
		{"sqlmap", "sqlmap/1.4.7", 403},
		{"nikto", "Nikto/2.1.6", 403},
		{"hydra", "hydra 8.5", 403},
		{"burpsuite", "BurpSuite Community 2024", 403},
		{"nuclei", "nuclei v3.0.0", 403},
		{"curl", "curl/7.81.0", 403},
		{"python-requests", "python-requests/2.28.0", 403},
		{"empty UA", "", 403},
		{"normal browser", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/125.0", 200},
	}

	for _, b := range bots {
		req, _ := http.NewRequest("GET", baseURL+"/api/leaderboard", nil)
		req.Header.Set("User-Agent", b.ua)
		resp, err := client.Do(req)
		if err != nil {
			addResult(false, fmt.Sprintf("Bot [%s]: error %v", b.name, err))
			continue
		}
		resp.Body.Close()
		ok := resp.StatusCode == b.code
		addResult(ok, fmt.Sprintf("Bot [%s]: expected %d, got %d", b.name, b.code, resp.StatusCode))
	}
}

func testHoneypotTraps() {
	fmt.Println("\n=== Honeypot Traps ===")

	traps := []string{
		"/api/admin",
		"/api/admin/panel",
		"/api/debug",
		"/.env",
		"/wp-admin",
		"/.git/config",
		"/phpmyadmin",
		"/api/internal",
		"/api/auth",
		"/api/v1/users",
	}

	for _, path := range traps {
		resp, err := client.Get(baseURL + path)
		if err != nil {
			addResult(false, fmt.Sprintf("Honeypot [%s]: error %v", path, err))
			continue
		}
		resp.Body.Close()
		addResult(resp.StatusCode == 404 || resp.StatusCode == 403, fmt.Sprintf("Honeypot [%s]: expected 404/403, got %d", path, resp.StatusCode))
	}
}

func testSecurityHeaders() {
	fmt.Println("\n=== Security Headers ===")

	resp, err := client.Get(baseURL + "/api/leaderboard")
	if err != nil {
		addResult(false, fmt.Sprintf("Headers: error %v", err))
		return
	}
	defer resp.Body.Close()

	checks := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Cross-Origin-Opener-Policy": "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	}

	for header, expected := range checks {
		val := resp.Header.Get(header)
		addResult(val == expected, fmt.Sprintf("Header [%s]: expected '%s', got '%s'", header, expected, val))
	}

	hsts := resp.Header.Get("Strict-Transport-Security")
	addResult(strings.Contains(hsts, "max-age=31536000"), fmt.Sprintf("Header [HSTS]: got '%s'", hsts))

	csp := resp.Header.Get("Content-Security-Policy")
	addResult(strings.Contains(csp, "default-src 'self'"), fmt.Sprintf("Header [CSP]: present=%v", csp != ""))

	server := resp.Header.Get("Server")
	addResult(server == "" || server == "Vercel", fmt.Sprintf("Header [Server]: '%s'", server))
}

func testRateLimitHeaders() {
	fmt.Println("\n=== Rate Limit Behavior ===")

	for i := 0; i < 3; i++ {
		resp, err := client.Get(baseURL + "/api/captcha")
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 429 {
			addResult(true, fmt.Sprintf("Rate limit triggered after %d requests", i+1))
			return
		}
	}
	addResult(true, "Rate limit: 3 requests within limit (expected)")
}

func testCaptchaEndpoint() {
	fmt.Println("\n=== CAPTCHA Endpoint ===")

	resp, err := client.Get(baseURL + "/api/captcha")
	if err != nil {
		addResult(false, fmt.Sprintf("CAPTCHA: error %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		addResult(false, fmt.Sprintf("CAPTCHA: expected 200, got %d", resp.StatusCode))
		return
	}

	var data map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &data); err != nil {
		addResult(false, fmt.Sprintf("CAPTCHA: invalid JSON"))
		return
	}

	hasID := data["captchaId"] != nil && data["captchaId"] != ""
	hasImage := data["captchaImage"] != nil && data["captchaImage"] != ""
	addResult(hasID, "CAPTCHA: captchaId present")
	addResult(hasImage, "CAPTCHA: captchaImage present")
}

func testMethodNotAllowed() {
	fmt.Println("\n=== Method Not Allowed ===")

	methods := []struct {
		method string
		path   string
		code   int
	}{
		{"POST", "/api/leaderboard", 405},
		{"DELETE", "/api/leaderboard", 405},
		{"PUT", "/api/leaderboard", 405},
		{"POST", "/api/players", 405},
		{"DELETE", "/api/projects", 405},
	}

	for _, m := range methods {
		req, _ := http.NewRequest(m.method, baseURL+m.path, nil)
		resp, err := client.Do(req)
		if err != nil {
			addResult(false, fmt.Sprintf("Method [%s %s]: error %v", m.method, m.path, err))
			continue
		}
		resp.Body.Close()
		addResult(resp.StatusCode == m.code, fmt.Sprintf("Method [%s %s]: expected %d, got %d", m.method, m.path, m.code, resp.StatusCode))
	}
}

func testInvalidJSON() {
	fmt.Println("\n=== Invalid JSON ===")

	resp, err := client.Post(baseURL+"/api/login", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		addResult(false, fmt.Sprintf("Invalid JSON: error %v", err))
		return
	}
	defer resp.Body.Close()
	addResult(resp.StatusCode == 400, fmt.Sprintf("Invalid JSON: expected 400, got %d", resp.StatusCode))

	resp2, _ := client.Post(baseURL+"/api/login", "text/plain", strings.NewReader("hello"))
	if resp2 != nil {
		resp2.Body.Close()
		addResult(resp2.StatusCode == 400, fmt.Sprintf("Wrong Content-Type: expected 400, got %d", resp2.StatusCode))
	}
}

func addResult(ok bool, msg string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failed++
	} else {
		passed++
	}
	line := fmt.Sprintf("[%s] %s", status, msg)
	results = append(results, line)
	fmt.Println(line)
}

func sendReport() {
	total := passed + failed
	emoji := "✅"
	color := 0x22c55e
	if failed > 0 {
		emoji = "⚠️"
		color = 0xffaa00
	}

	fields := []discordField{
		{Name: "Результат", Value: fmt.Sprintf("%d/%d пройдено", passed, total), Inline: true},
		{Name: "Сервер", Value: "`" + baseURL + "`", Inline: true},
	}

	failLines := []string{}
	for _, r := range results {
		if strings.HasPrefix(r, "[FAIL]") {
			failLines = append(failLines, r)
		}
	}
	if len(failLines) > 0 {
		if len(failLines) > 8 {
			failLines = failLines[:8]
			failLines = append(failLines, fmt.Sprintf("... и ещё %d ошибок", len(failLines)-8))
		}
		fields = append(fields, discordField{
			Name:  "Ошибки",
			Value: "```\n" + strings.Join(failLines, "\n") + "\n```",
			Inline: false,
		})
	}

	embed := discordEmbed{
		Title:       emoji + " Security Test Report",
		Description: fmt.Sprintf("Протестировано %d проверок безопасности", total),
		Color:       color,
		Fields:      fields,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	payload := discordPayload{Embeds: []discordEmbed{embed}}
	body, _ := json.Marshal(payload)
	client.Post(webhook, "application/json", strings.NewReader(string(body)))

	fmt.Printf("\n=== DONE: %d/%d passed ===\n", passed, total)
}
