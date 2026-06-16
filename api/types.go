package handler

import (
	"context"
	"regexp"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mojocn/base64Captcha"
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

type jwtKey struct {
	Secret []byte
	ID     string
}

type memoryBucket struct {
	count   int
	resetAt time.Time
}

type tokenVersionCacheEntry struct {
	version   int64
	expiresAt time.Time
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
}

type firestoreCaptchaStore struct {
	client *firestore.Client
}

var (
	fsClient *firestore.Client
	fsOnce   sync.Once
	fsErr    error

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
)

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

const adminKnockTTL = 15 * time.Minute
