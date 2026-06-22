package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"
)

func newAdminKnockStore() *adminKnockStoreT {
	s := &adminKnockStoreT{
		store:  make(map[string]*adminKnockEntry),
		stopCh: make(chan struct{}),
	}
	go s.cleanup()
	return s
}

func (s *adminKnockStoreT) stop() {
	close(s.stopCh)
}

func (s *adminKnockStoreT) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for ip, e := range s.store {
				if now.After(e.expiresAt) {
					delete(s.store, ip)
				}
			}
			s.mu.Unlock()
		case <-s.stopCh:
			return
		}
	}
}

func (s *adminKnockStoreT) set(ip, key string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[ip] = &adminKnockEntry{
		key:       key,
		expiresAt: time.Now().Add(ttl),
	}
}

func (s *adminKnockStoreT) get(ip string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.store[ip]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		delete(s.store, ip)
		return "", false
	}
	return e.key, true
}

func (s *adminKnockStoreT) delete(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, ip)
}

func (s *firestoreKnockStore) get(ip string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	docRef := fsClient.Collection("admin_knocks").Doc(ipToDocID(ip))
	doc, err := docRef.Get(ctx)
	if err != nil {
		return "", false
	}
	var entry struct {
		Key       string    `firestore:"key"`
		ExpiresAt time.Time `firestore:"expiresAt"`
	}
	if err := doc.DataTo(&entry); err != nil {
		return "", false
	}
	if time.Now().After(entry.ExpiresAt) {
		docRef.Delete(context.Background())
		return "", false
	}
	return entry.Key, true
}

func (s *firestoreKnockStore) set(ip, key string, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := fsClient.Collection("admin_knocks").Doc(ipToDocID(ip)).Set(ctx, map[string]interface{}{
		"key":       key,
		"expiresAt": time.Now().Add(ttl),
	})
	if err != nil {
		log.Printf("[knock] firestore set: %v", err)
	}
}

func (s *firestoreKnockStore) delete(ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := fsClient.Collection("admin_knocks").Doc(ipToDocID(ip)).Delete(ctx)
	if err != nil {
		log.Printf("[knock] firestore delete: %v", err)
	}
}

func (s *firestoreKnockStore) stop() {}

func ipToDocID(ip string) string {
	return strings.NewReplacer(".", "-", ":", "-").Replace(ip)
}

const adminKnockTTL = 15 * time.Minute

func generateAdminKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func knockMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getRealIP(r)
		headerKey := r.Header.Get("X-Admin-Path-Key")
		if headerKey == "" {
			sendError(w, http.StatusNotFound, "Роут не найден")
			return
		}
		storedKey, ok := adminKnockStore.get(ip)
		if !ok {
			sendError(w, http.StatusNotFound, "Роут не найден")
			return
		}
		if subtle.ConstantTimeCompare([]byte(headerKey), []byte(storedKey)) != 1 {
			sendError(w, http.StatusNotFound, "Роут не найден")
			return
		}
		next.ServeHTTP(w, r)
	}
}

func handleAdminKnock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	key, err := generateAdminKey()
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Ошибка генерации ключа")
		return
	}
	ip := getRealIP(r)
	adminKnockStore.set(ip, key, adminKnockTTL)
	log.Printf("[knock] admin key issued (TTL=%v)", adminKnockTTL)
	writeJSON(w, map[string]interface{}{
		"key":        key,
		"ttl":        int(adminKnockTTL.Seconds()),
		"expires_in": adminKnockTTL.String(),
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
