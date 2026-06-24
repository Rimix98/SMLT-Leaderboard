package handler

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"

	"github.com/mojocn/base64Captcha"
)

func (s *firestoreCaptchaStore) Set(id string, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.client.Collection("captcha").Doc(id).Set(ctx, map[string]interface{}{
		"value":     value,
		"expiresAt": time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		slog.Error("captcha firestore set failed", "error", err)
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
				slog.Error("captcha delete expired failed", "error", delErr)
			}
		}
		return ""
	}
	if clear {
		if _, delErr := s.client.Collection("captcha").Doc(id).Delete(ctx); delErr != nil {
			slog.Error("captcha delete after get failed", "error", delErr)
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

func getCaptchaDifficulty(ip string) (int, int, float64, int) {
	val, _ := captchaEscalation.Load(ip)
	failures := 0
	if v, ok := val.(*int); ok {
		failures = *v
	}

	switch {
	case failures >= 10:
		return 80, 240, 0.5, 8
	case failures >= 5:
		return 80, 240, 0.6, 7
	case failures >= 3:
		return 80, 240, 0.65, 6
	default:
		return 80, 240, 0.7, 5
	}
}

func recordCaptchaFailure(ip string) {
	val, _ := captchaEscalation.LoadOrStore(ip, new(int))
	p := val.(*int)
	*p++
}

func cleanupCaptchaEscalation() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		captchaEscalation.Range(func(key, value interface{}) bool {
			p := value.(*int)
			if *p > 20 {
				captchaEscalation.Delete(key)
			}
			return true
		})
	}
}

func clearCaptchaEscalation(ip string) {
	captchaEscalation.Delete(ip)
}

func handleCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ensureCaptcha()

	ip := getRealIP(r)
	h, cw, noise, len := getCaptchaDifficulty(ip)

	drv := base64Captcha.NewDriverDigit(h, cw, len, noise, 80)
	c := base64Captcha.NewCaptcha(drv, captchaStore)
	id, b64s, _, err := c.Generate()
	if err != nil {
		slog.Error("captcha generation failed", "error", err)
		sendError(w, http.StatusInternalServerError, "Ошибка генерации капчи")
		return
	}
	writeJSON(w, map[string]interface{}{
		"captchaId":    id,
		"captchaImage": b64s,
		"difficulty":   len,
	})
}
