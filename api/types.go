package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
)

type StaffPlayer struct {
	Nickname string `json:"nickname" firestore:"nickname"`
	Discord  string `json:"discord" firestore:"discord"`
}

type StaffRole struct {
	Name         string        `json:"name" firestore:"name"`
	Color        string        `json:"color" firestore:"color"`
	Players      []StaffPlayer `json:"players" firestore:"players"`
	TiersEnabled bool          `json:"tiersEnabled" firestore:"tiersEnabled"`
}

type StaffTierEntry struct {
	Nickname string `json:"nickname" firestore:"nickname"`
	Tier     string `json:"tier" firestore:"tier"`
}

type Project struct {
	Name         string   `json:"name" firestore:"name"`
	VideoID      string   `json:"videoId" firestore:"videoId"`
	ID           string   `json:"id" firestore:"id"`
	Comment      string   `json:"comment" firestore:"comment"`
	Status       string   `json:"status" firestore:"status"`
	Verifier     string   `json:"verifier" firestore:"verifier"`
	Participants []string `json:"participants" firestore:"participants"`
}

type Player struct {
	Name string `json:"name" firestore:"name"`
}

type FullPlayerData struct {
	Name    string      `json:"name"`
	Data    interface{} `json:"data"`
	Records interface{} `json:"records"`
}

type AuditEntry struct {
	Action    string      `json:"action" firestore:"action"`
	Details   interface{} `json:"details" firestore:"details"`
	CreatedAt time.Time   `json:"createdAt" firestore:"createdAt"`
}

type errValidation struct{ msg string }

func (e errValidation) Error() string { return e.msg }

type jwtKey struct {
	Secret []byte
	ID     string
}

type memoryBucket struct {
	count   int
	resetAt time.Time
}

type PlayerHistoryEntry struct {
	PlayerID     string    `json:"playerId" firestore:"playerId"`
	PlayerName   string    `json:"playerName" firestore:"playerName"`
	Date         string    `json:"date" firestore:"date"`
	Rank         int       `json:"rank" firestore:"rank"`
	Score        float64   `json:"score" firestore:"score"`
	RecordsCount int       `json:"recordsCount" firestore:"recordsCount"`
	HardestLevel string    `json:"hardestLevel" firestore:"hardestLevel"`
	SnapshotAt   time.Time `json:"snapshotAt" firestore:"snapshotAt"`
}

type LeaderboardCheckResponse struct {
	Hash        string `json:"hash"`
	LastUpdated string `json:"lastUpdated"`
	PlayerCount int    `json:"playerCount"`
}

type tokenVersionCacheEntry struct {
	version   int64
	expiresAt time.Time
}

type discordAlert struct {
	eventType string
	ip        string
	path      string
	detail    interface{}
}

type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Color       int                 `json:"color"`
	Fields      []discordEmbedField `json:"fields,omitempty"`
	Footer      *discordEmbedFooter `json:"footer,omitempty"`
	Timestamp   string              `json:"timestamp"`
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbedFooter struct {
	Text string `json:"text"`
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type cacheEntry struct {
	data      []byte
	hash      string
	expiresAt time.Time
	lastSet   time.Time
}

type responseCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type rateLimiter interface {
	allow(ctx context.Context, key string, max int, window time.Duration) (bool, error)
}

type memoryLimiter struct {
	mu     sync.Mutex
	keys   map[string]*memoryBucket
	stopCh chan struct{}
}

type upstashLimiter struct {
	url   string
	token string
	http  *http.Client
}

type firestoreCaptchaStore struct {
	client *firestore.Client
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	statusCode int
}

type knockStore interface {
	get(ip string) (string, bool)
	set(ip, key string, ttl time.Duration)
	delete(ip string)
	stop()
}

type adminKnockEntry struct {
	key       string
	expiresAt time.Time
}

type adminKnockStoreT struct {
	mu     sync.Mutex
	store  map[string]*adminKnockEntry
	stopCh chan struct{}
}

type firestoreKnockStore struct{}

type SecurityEvent struct {
	Type      string      `firestore:"type" json:"type"`
	IP        string      `firestore:"ip" json:"ip"`
	Path      string      `firestore:"path" json:"path"`
	Detail    interface{} `firestore:"detail,omitempty" json:"detail,omitempty"`
	CreatedAt time.Time   `firestore:"createdAt" json:"createdAt"`
}

type DeviceBan struct {
	Fingerprint string    `firestore:"fingerprint" json:"fingerprint"`
	IP          string    `firestore:"ip" json:"ip"`
	UA          string    `firestore:"ua" json:"ua"`
	Reason      string    `firestore:"reason" json:"reason"`
	BannedAt    time.Time `firestore:"bannedAt" json:"bannedAt"`
	ExpiresAt   time.Time `firestore:"expiresAt" json:"expiresAt"`
	BannedBy    string    `firestore:"bannedBy" json:"bannedBy"`
}

type staffData struct {
	Roles []StaffRole `json:"roles" firestore:"roles"`
}

var (
	reProjectID    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)
	reAlphanumeric = regexp.MustCompile(`^[\p{L}0-9 _.\-]+$`)
	reDiscord      = regexp.MustCompile(`^[a-zA-Z0-9 _.\-#]+$`)
	reVideoID      = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	reRoleName     = regexp.MustCompile(`^[\p{L}0-9 _.\-]+$`)
	reHexColor     = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$`)
	reCaptchaID    = regexp.MustCompile(`^[a-zA-Z0-9]{8,64}$`)
)

var defaultPlayerNames = []string{
	"samoletik", "paradoxiz", "clokman", "itzslxnq", "H30n41k_GmD",
	"Filkoty", "DarBeast", "Florned", "Marzyiiik", "euphoriak8",
	"npoctou_gamer", "NopanicGD", "CandyCloud22", "Vakum", "Daggit",
	"Loran", "tapxyhh", "SerGio", "Fanim59", "prostoymofficial",
	"toxik blaze", "NatrixGMD", "toxatort", "SpaceRS", "yeahme",
	"Спини", "Linqwq", "RossceorpGD", "69liqu69",
}

var errRateLimitExceeded = errors.New("rate limit exceeded")
