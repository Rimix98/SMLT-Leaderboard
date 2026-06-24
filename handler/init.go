package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	firebase "firebase.google.com/go"
	"github.com/mojocn/base64Captcha"
	"google.golang.org/api/option"
)

func init() {
	trustProxy = os.Getenv("TRUST_PROXY") == "true" || os.Getenv("VERCEL") == "1"
	initFirestore()
	initRateLimiter()
	initRateLimitSalt()
	initJWTSecrets()
	initAdminKnock()
	initDiscordAlerts()
	initDiscordShame()
	startTokenBlacklistCleanup()
	go shameBoardNotificationLoop()
	go cleanupCaptchaEscalation()
}

func initAdminKnock() {
	adminKnockOnce.Do(func() {
		if fsClient != nil {
			slog.Info("knock using firestore store")
			adminKnockStore = &firestoreKnockStore{}
		} else {
			slog.Info("knock using memory store")
			adminKnockStore = newAdminKnockStore()
		}
	})
}

func initRateLimitSalt() {
	saltOnce.Do(func() {
		buf := make([]byte, 32)
		for tries := 0; tries < 3; tries++ {
			if _, err := rand.Read(buf); err == nil {
				rateLimitSalt = hex.EncodeToString(buf)
				return
			}
		}
		rateLimitSalt = fmt.Sprintf("%x|%d", time.Now().UnixNano(), os.Getpid())
	})
}

func initJWTSecrets() {
	jwtSecretsOnce.Do(func() {
		primary := os.Getenv("JWT_SECRET")
		if primary == "" {
			slog.Warn("JWT_SECRET not set, auth will fail")
			return
		}
		if len(primary) < 32 {
			slog.Error("JWT_SECRET too short", "bytes", len(primary), "min", 32)
			os.Exit(1)
		}
		primaryJWTKey = []byte(primary)
		primaryJWTID = "1"
		jwtSecrets = append(jwtSecrets, jwtKey{Secret: primaryJWTKey, ID: primaryJWTID})
		for i := 2; ; i++ {
			key := os.Getenv(fmt.Sprintf("JWT_SECRET_%d", i))
			if key == "" {
				break
			}
			if len(key) < 32 {
				slog.Warn("JWT_SECRET too short, skipping", "index", i, "bytes", len(key))
				continue
			}
			jwtSecrets = append(jwtSecrets, jwtKey{Secret: []byte(key), ID: fmt.Sprintf("%d", i)})
		}
	})
}

func initFirestore() {
	fsOnce.Do(func() {
		ctx := context.Background()
		creds := os.Getenv("FIRESTORE_CREDENTIALS")
		if creds == "" {
			fsErr = errors.New("FIRESTORE_CREDENTIALS not set")
			slog.Warn("firestore not configured", "error", fsErr)
			return
		}
		app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(creds)))
		if err != nil {
			fsErr = err
			slog.Error("firestore init app failed", "error", err)
			return
		}
		fsClient, err = app.Firestore(ctx)
		if err != nil {
			fsErr = err
			slog.Error("firestore connect failed", "error", err)
		}
	})
}

func ensureCaptcha() {
	captchaOnce.Do(func() {
		if fsClient != nil {
			slog.Info("captcha using firestore store")
			captchaStore = &firestoreCaptchaStore{client: fsClient}
		} else {
			slog.Info("captcha using default memory store")
			captchaStore = base64Captcha.DefaultMemStore
		}
		captchaInst = base64Captcha.NewCaptcha(
			base64Captcha.NewDriverDigit(80, 240, 5, 0.7, 80),
			captchaStore,
		)
	})
}
