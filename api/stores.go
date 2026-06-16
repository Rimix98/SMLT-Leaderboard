package handler

import (
	"context"
	"crypto/subtle"
	"log"
	"strings"
	"time"
)

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

func newAdminKnockStore() *adminKnockStoreT {
	s := &adminKnockStoreT{
		store:  make(map[string]*adminKnockEntry),
		stopCh: make(chan struct{}),
	}
	go s.cleanup()
	return s
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

func (s *firestoreCaptchaStore) Set(id string, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.client.Collection("captcha").Doc(id).Set(ctx, map[string]interface{}{
		"value":     value,
		"expiresAt": time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		log.Printf("[captcha] firestore set: %v", err)
	}
	return err
}

func (s *firestoreCaptchaStore) Get(id string, clear bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	doc, err := s.client.Collection("captcha").Doc(id).Get(ctx)
	if err != nil {
		return ""
	}
	var data struct {
		Value     string    `firestore:"value"`
		ExpiresAt time.Time `firestore:"expiresAt"`
	}
	if err := doc.DataTo(&data); err != nil {
		return ""
	}
	if time.Now().After(data.ExpiresAt) {
		if clear {
			if _, delErr := s.client.Collection("captcha").Doc(id).Delete(ctx); delErr != nil {
				log.Printf("[captcha] delete expired: %v", delErr)
			}
		}
		return ""
	}
	if clear {
		if _, delErr := s.client.Collection("captcha").Doc(id).Delete(ctx); delErr != nil {
			log.Printf("[captcha] delete after get: %v", delErr)
		}
	}
	return data.Value
}

func (s *firestoreCaptchaStore) Verify(id, answer string, clear bool) bool {
	stored := s.Get(id, clear)
	if stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(answer)) == 1
}
