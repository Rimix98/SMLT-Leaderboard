package blacklist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

type JWTBlacklist interface {
	Add(ctx context.Context, token string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, token string) (bool, error)
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

type UpstashBlacklist struct {
	url   string
	token string
}

func NewUpstashBlacklist(url, token string) *UpstashBlacklist {
	return &UpstashBlacklist{url: url, token: token}
}

func (u *UpstashBlacklist) Add(ctx context.Context, token string, expiresAt time.Time) error {
	if token == "" {
		return nil
	}

	ttl := int64(time.Until(expiresAt).Seconds())
	if ttl <= 0 {
		return nil
	}

	tokenHash := hashToken(token)
	key := "blacklist:" + tokenHash
	reqURL := fmt.Sprintf("%s/set/%s/true/EX/%d", u.url, key, ttl)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstash error: status %d", resp.StatusCode)
	}

	return nil
}

func (u *UpstashBlacklist) IsBlacklisted(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}

	tokenHash := hashToken(token)
	key := "blacklist:" + tokenHash
	reqURL := fmt.Sprintf("%s/get/%s", u.url, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("upstash error: status %d", resp.StatusCode)
	}

	var upstashResp struct {
		Result *string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&upstashResp); err != nil {
		return false, err
	}

	return upstashResp.Result != nil && *upstashResp.Result == "true", nil
}

type MemoryBlacklist struct {
	mu     sync.Mutex
	tokens map[string]time.Time
}

func NewMemoryBlacklist() *MemoryBlacklist {
	return &MemoryBlacklist{tokens: make(map[string]time.Time)}
}

func (b *MemoryBlacklist) Add(_ context.Context, token string, expiresAt time.Time) error {
	if token == "" {
		return nil
	}
	b.mu.Lock()
	b.tokens[hashToken(token)] = expiresAt
	b.mu.Unlock()
	return nil
}

func (b *MemoryBlacklist) IsBlacklisted(_ context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	tokenHash := hashToken(token)

	b.mu.Lock()
	defer b.mu.Unlock()

	expiresAt, exists := b.tokens[tokenHash]
	if !exists {
		return false, nil
	}
	if time.Now().After(expiresAt) {
		delete(b.tokens, tokenHash)
		return false, nil
	}
	return true, nil
}
