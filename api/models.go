package handler

import "time"

type StaffPlayer struct {
	Nickname string `json:"nickname" firestore:"nickname"`
	Discord  string `json:"discord" firestore:"discord"`
}

type StaffRole struct {
	Name    string        `json:"name" firestore:"name"`
	Color   string        `json:"color" firestore:"color"`
	Players []StaffPlayer `json:"players" firestore:"players"`
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
	AdminIP   string      `json:"adminIp" firestore:"adminIp"`
	Details   interface{} `json:"details" firestore:"details"`
	CreatedAt time.Time   `json:"createdAt" firestore:"createdAt"`
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
