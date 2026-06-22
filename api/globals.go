package handler

import (
	"net/http"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mojocn/base64Captcha"
)

var (
	fsClient *firestore.Client
	fsOnce   sync.Once
	fsErr    error

	httpClient = &http.Client{Timeout: 15 * time.Second}

	trustProxy     bool
	maxRequestBody = int64(1024 * 1024)
	primaryJWTKey  []byte
	primaryJWTID   string

	globalRateLimiter rateLimiter
	rlOnce            sync.Once
	rlStop            func()

	rateLimitSalt string
	saltOnce      sync.Once

	jwtSecrets     []jwtKey
	jwtSecretsMu   sync.RWMutex
	jwtSecretsOnce sync.Once

	captchaStore base64Captcha.Store
	captchaInst  *base64Captcha.Captcha
	captchaOnce  sync.Once

	tokenVerCache sync.Map

	adminKnockStore knockStore
	adminKnockOnce  sync.Once

	captchaEscalation sync.Map

	discordWebhookURL string
	alertQueue        chan discordAlert
	alertOnce         sync.Once

	apiCache = &responseCache{entries: make(map[string]*cacheEntry)}

	alertColors = map[string]int{
		"honeypot_triggered":           0xFF0000,
		"honeypot_token_blacklisted":   0xFF4500,
		"bot_blocked":                  0xFF6600,
		"blocked_path":                 0xFF6600,
		"login_failed":                 0xFFAA00,
		"captcha_failed":               0xFFAA00,
		"rate_limit_exceeded":          0xFFCC00,
		"oversized_request":            0xFF8800,
		"device_banned":                0xFF0000,
		"token_refreshed":              0x5865F2,
	}
	alertTitles = map[string]string{
		"honeypot_triggered":           "Honeypot triggered",
		"honeypot_token_blacklisted":   "Token blacklisted via honeypot",
		"bot_blocked":                  "Bot/scanner blocked",
		"blocked_path":                 "Blocked path accessed",
		"login_failed":                 "Login failed",
		"captcha_failed":               "CAPTCHA failed",
		"rate_limit_exceeded":          "Rate limit exceeded",
		"oversized_request":            "Oversized request",
		"device_banned":                "Banned device accessed",
		"token_refreshed":              "Token refreshed",
	}

	blockedBotPatterns = []string{
		"sqlmap", "nikto", "nessus", "openvas", "w3af", "arachni",
		"skipfish", "whatweb", "dirbuster", "gobuster", "ffuf", "wfuzz",
		"masscan", "zgrab", "httpx", "nuclei", "jaeles", "xray", "vulmap",
		"pocsuite", "hydra", "medusa", "ncrack", "patator", "brutus",
		"metasploit", "burpsuite", "owasp", "acunetix", "appscan",
		"webinspect", "paros", "wparos", "webscarab", "mitmproxy",
		"charles", "fiddler", "grabber", "wapiti", "havij", "canari",
		"slowloris", "goldeneye", "slowhttptest", "rudy", "tor", "curl",
		"wget", "python", "perl", "ruby", "php/", "scrapy", "crawler",
		"spider", "scraper", "harvest", "extract", "scan", "exploit",
		"hack", "crack", "brute", "go-http-client", "java/", "libwww",
		"lwp-trivial", "webbandit", "webcopier", "webzip", "teleport",
		"sitecopy", "httrack", "clixboard", "cms", "nmap", "zmap",
		"unicornsql", "sqlbf", "sqlbrute", "sqlsmack", "sqlfury",
		"sqlninja", "bbqsql", "jsql", "arachni", "skipfish", "nikto",
		"openvas", "nessus", "qualys", "ratproxy", "w3af",
		"websecurify", "netsparker",
	}

	blockedPathPatterns = []string{
		"wp-admin", "wp-login", "xmlrpc.php", "wp-content", "wp-includes",
		"administrator", "config.php", "config.inc", "setup.php",
		"install.php", "shell.php", "cmd.php", "backdoor", "webshell",
		"c99", "r57", "b374k", " FilesMan", "File manager", ".htaccess",
		"web.config", "crossdomain.xml",
	}
)
