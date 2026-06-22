package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	discordBotToken   string
	discordGuildID    string
	discordShameRole  string
	discordShameOnce  sync.Once
	shameCacheMu      sync.RWMutex
	shameCacheData    []byte
	shameCacheExpires time.Time
)

func initDiscordShame() {
	discordShameOnce.Do(func() {
		discordBotToken = os.Getenv("DISCORD_BOT_TOKEN")
		discordGuildID = os.Getenv("DISCORD_GUILD_ID")
		discordShameRole = os.Getenv("DISCORD_SHAME_ROLE_ID")
		if discordBotToken == "" || discordGuildID == "" || discordShameRole == "" {
			log.Println("[shame] Discord env vars not set, shame board disabled")
		} else {
			log.Printf("[shame] enabled (guild=%s, role=%s)", discordGuildID, discordShameRole)
		}
	})
}

type ShameBoardEntry struct {
	DiscordID string `json:"discordId" firestore:"discordId"`
	Username  string `json:"username" firestore:"username"`
	Avatar    string `json:"avatar" firestore:"avatar"`
	Reason    string `json:"reason" firestore:"reason"`
	AddedAt   string `json:"addedAt" firestore:"addedAt"`
	AddedBy   string `json:"addedBy" firestore:"addedBy"`
}

type discordGuildMember struct {
	User struct {
		ID            string `json:"id"`
		Username      string `json:"username"`
		Avatar        string `json:"avatar"`
		Discriminator string `json:"discriminator"`
	} `json:"user"`
	Roles []string `json:"roles"`
}

type discordGuildMemberResponse struct {
	Members []discordGuildMember `json:"-"`
}

func fetchDiscordRoleMembers() ([]discordGuildMember, error) {
	if discordBotToken == "" {
		return nil, fmt.Errorf("Discord bot token not configured")
	}

	var allMembers []discordGuildMember
	var after string

	for {
		url := fmt.Sprintf("https://discord.com/api/v10/guilds/%s/members?limit=1000", discordGuildID)
		if after != "" {
			url += "&after=" + after
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Authorization", "Bot "+discordBotToken)

		resp, err := httpClient.Do(req)
		cancel()
		if err != nil {
			return nil, err
		}

		var members []discordGuildMember
		if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if resp.StatusCode == 429 {
			var rateLimit struct {
				RetryAfter float64 `json:"retry_after"`
			}
			json.NewDecoder(resp.Body).Decode(&rateLimit)
			time.Sleep(time.Duration(rateLimit.RetryAfter*1000) * time.Millisecond)
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("Discord API returned %d", resp.StatusCode)
		}

		for _, m := range members {
			for _, roleID := range m.Roles {
				if roleID == discordShameRole {
					allMembers = append(allMembers, m)
					break
				}
			}
		}

		if len(members) < 1000 {
			break
		}
		after = members[len(members)-1].User.ID
	}

	return allMembers, nil
}

func getShameBoardFirestore(ctx context.Context) (map[string]ShameBoardEntry, error) {
	doc, err := fsClient.Collection("config").Doc("shame_board").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return make(map[string]ShameBoardEntry), nil
		}
		return nil, err
	}
	var raw map[string]interface{}
	if err := doc.DataTo(&raw); err != nil {
		return make(map[string]ShameBoardEntry), nil
	}
	entries := make(map[string]ShameBoardEntry)
	if membersRaw, ok := raw["members"]; ok {
		if membersArr, ok := membersRaw.([]interface{}); ok {
			for _, item := range membersArr {
				if m, ok := item.(map[string]interface{}); ok {
					id, _ := m["discordId"].(string)
					if id == "" {
						continue
					}
					entry := ShameBoardEntry{
						DiscordID: id,
						Username:  fmt.Sprintf("%v", m["username"]),
						Avatar:    fmt.Sprintf("%v", m["avatar"]),
						Reason:    fmt.Sprintf("%v", m["reason"]),
						AddedAt:   fmt.Sprintf("%v", m["addedAt"]),
						AddedBy:   fmt.Sprintf("%v", m["addedBy"]),
					}
					entries[id] = entry
				}
			}
		}
	}
	return entries, nil
}

func handleGetShameBoard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	initDiscordShame()
	if discordBotToken == "" {
		sendError(w, http.StatusServiceUnavailable, "Discord интеграция не настроена")
		return
	}

	shameCacheMu.RLock()
	if shameCacheData != nil && time.Now().Before(shameCacheExpires) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
		w.Header().Set("X-Cache", "HIT")
		w.Write(shameCacheData)
		shameCacheMu.RUnlock()
		return
	}
	shameCacheMu.RUnlock()

	discordMembers, err := fetchDiscordRoleMembers()
	if err != nil {
		log.Printf("[shame] fetch discord members: %v", err)
		sendError(w, http.StatusBadGateway, "Ошибка получения данных из Discord")
		return
	}

	ctx := r.Context()
	fsEntries, err := getShameBoardFirestore(ctx)
	if err != nil {
		log.Printf("[shame] firestore read: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	var result []ShameBoardEntry
	for _, dm := range discordMembers {
		entry := ShameBoardEntry{
			DiscordID: dm.User.ID,
			Username:  dm.User.Username,
			Avatar:    dm.User.Avatar,
		}
		if fsEntry, ok := fsEntries[dm.User.ID]; ok {
			entry.Reason = fsEntry.Reason
			entry.AddedAt = fsEntry.AddedAt
			entry.AddedBy = fsEntry.AddedBy
		}
		result = append(result, entry)
	}

	body, _ := json.Marshal(result)
	shameCacheMu.Lock()
	shameCacheData = body
	shameCacheExpires = time.Now().Add(1 * time.Minute)
	shameCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

func handleSaveShameReason(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	initDiscordShame()
	if discordBotToken == "" {
		sendError(w, http.StatusServiceUnavailable, "Discord интеграция не настроена")
		return
	}

	var req struct {
		DiscordID string `json:"discordId"`
		Reason    string `json:"reason"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.DiscordID = sanitizeString(req.DiscordID)
	req.Reason = sanitizeString(req.Reason)
	if req.DiscordID == "" || len(req.DiscordID) > 64 {
		sendError(w, http.StatusBadRequest, "Некорректный Discord ID")
		return
	}
	if len(req.Reason) > 500 {
		sendError(w, http.StatusBadRequest, "Причина слишком длинная")
		return
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("shame_board")
		doc, err := tx.Get(docRef)
		var data struct {
			Members []ShameBoardEntry `firestore:"members"`
		}
		if err != nil {
			if status.Code(err) == codes.NotFound {
				data.Members = []ShameBoardEntry{}
			} else {
				return err
			}
		} else {
			if err := doc.DataTo(&data); err != nil {
				data.Members = []ShameBoardEntry{}
			}
		}

		found := false
		for i, entry := range data.Members {
			if entry.DiscordID == req.DiscordID {
				data.Members[i].Reason = req.Reason
				found = true
				break
			}
		}
		if !found {
			data.Members = append(data.Members, ShameBoardEntry{
				DiscordID: req.DiscordID,
				Reason:    req.Reason,
				AddedAt:   time.Now().UTC().Format(time.RFC3339),
			})
		}

		return tx.Set(docRef, map[string]interface{}{"members": data.Members}, firestore.Merge(firestore.FieldPath{"members"}))
	})

	if err != nil {
		log.Printf("[shame] save reason: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	cacheInvalidate("shame")
	shameCacheMu.Lock()
	shameCacheData = nil
	shameCacheMu.Unlock()

	writeJSON(w, map[string]bool{"success": true})
}

func handleShameBoardCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}
	initDiscordShame()
	if discordBotToken == "" {
		sendError(w, http.StatusServiceUnavailable, "Discord интеграция не настроена")
		return
	}

	discordMembers, err := fetchDiscordRoleMembers()
	if err != nil {
		log.Printf("[shame] check fetch: %v", err)
		sendError(w, http.StatusBadGateway, "Ошибка получения данных из Discord")
		return
	}

	ctx := r.Context()
	fsEntries, err := getShameBoardFirestore(ctx)
	if err != nil {
		log.Printf("[shame] check firestore: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	type Notification struct {
		DiscordID string `json:"discordId"`
		Username  string `json:"username"`
	}

	var notifications []Notification
	for _, dm := range discordMembers {
		if _, exists := fsEntries[dm.User.ID]; !exists {
			notifications = append(notifications, Notification{
				DiscordID: dm.User.ID,
				Username:  dm.User.Username,
			})
		}
	}

	writeJSON(w, map[string]interface{}{
		"newMembers":   notifications,
		"totalOnBoard": len(discordMembers),
	})
}

func handleDeleteShameEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}

	var req struct {
		DiscordID string `json:"discordId"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.DiscordID = sanitizeString(req.DiscordID)
	if req.DiscordID == "" {
		sendError(w, http.StatusBadRequest, "Некорректный Discord ID")
		return
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("shame_board")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Members []ShameBoardEntry `firestore:"members"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}

		found := false
		for i, entry := range data.Members {
			if entry.DiscordID == req.DiscordID {
				data.Members = append(data.Members[:i], data.Members[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return errValidation{"entry not found"}
		}

		return tx.Set(docRef, map[string]interface{}{"members": data.Members}, firestore.Merge(firestore.FieldPath{"members"}))
	})

	if err != nil {
		if _, ok := err.(errValidation); ok {
			sendError(w, http.StatusNotFound, "Запись не найдена")
		} else {
			log.Printf("[shame] delete entry: %v", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}

	cacheInvalidate("shame")
	shameCacheMu.Lock()
	shameCacheData = nil
	shameCacheMu.Unlock()

	writeJSON(w, map[string]bool{"success": true})
}

func handleAddShameManual(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}

	var req struct {
		Username  string `json:"username"`
		DiscordID string `json:"discordId"`
		Reason    string `json:"reason"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Username = sanitizeString(req.Username)
	req.DiscordID = sanitizeString(req.DiscordID)
	req.Reason = sanitizeString(req.Reason)
	if req.Username == "" || len(req.Username) > 64 {
		sendError(w, http.StatusBadRequest, "Некорректное имя")
		return
	}
	if req.DiscordID == "" {
		req.DiscordID = "manual_" + req.Username
	}
	if len(req.Reason) > 500 {
		sendError(w, http.StatusBadRequest, "Причина слишком длинная")
		return
	}

	ctx := r.Context()
	entry := ShameBoardEntry{
		DiscordID: req.DiscordID,
		Username:  req.Username,
		Reason:    req.Reason,
		AddedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("shame_board")
		doc, err := tx.Get(docRef)
		var data struct {
			Members []ShameBoardEntry `firestore:"members"`
		}
		if err != nil {
			if status.Code(err) == codes.NotFound {
				data.Members = []ShameBoardEntry{}
			} else {
				return err
			}
		} else {
			if err := doc.DataTo(&data); err != nil {
				data.Members = []ShameBoardEntry{}
			}
		}

		existingIDs := make(map[string]bool)
		for _, e := range data.Members {
			existingIDs[e.DiscordID] = true
		}
		if existingIDs[entry.DiscordID] {
			return errValidation{"duplicate"}
		}

		data.Members = append(data.Members, entry)
		return tx.Set(docRef, map[string]interface{}{"members": data.Members}, firestore.Merge(firestore.FieldPath{"members"}))
	})

	if err != nil {
		if ve, ok := err.(errValidation); ok && ve.msg == "duplicate" {
			sendError(w, http.StatusConflict, "Участник уже есть на Доске позора")
		} else {
			log.Printf("[shame] manual add: %v", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}

	cacheInvalidate("shame")
	shameCacheMu.Lock()
	shameCacheData = nil
	shameCacheMu.Unlock()

	auditLog(ctx, AuditEntry{
		Action:  "shame.manualAdd",
		Details: map[string]interface{}{"username": req.Username, "discordId": req.DiscordID},
	})

	writeJSON(w, map[string]interface{}{
		"success": true,
		"entry":   entry,
	})
}

func handleSyncShameBoard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	initDiscordShame()
	if discordBotToken == "" {
		sendError(w, http.StatusServiceUnavailable, "Discord интеграция не настроена")
		return
	}
	_ = r.Body

	discordMembers, err := fetchDiscordRoleMembers()
	if err != nil {
		log.Printf("[shame] sync fetch: %v", err)
		sendError(w, http.StatusBadGateway, "Ошибка получения данных из Discord")
		return
	}

	ctx := r.Context()
	fsEntries, err := getShameBoardFirestore(ctx)
	if err != nil {
		log.Printf("[shame] sync firestore: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	var newMembers []ShameBoardEntry
	for _, dm := range discordMembers {
		if _, exists := fsEntries[dm.User.ID]; !exists {
			newMembers = append(newMembers, ShameBoardEntry{
				DiscordID: dm.User.ID,
				Username:  dm.User.Username,
				Avatar:    dm.User.Avatar,
				AddedAt:   time.Now().UTC().Format(time.RFC3339),
			})
		}
	}

	if len(newMembers) == 0 {
		writeJSON(w, map[string]interface{}{
			"success":   true,
			"newCount":  0,
			"added":     []ShameBoardEntry{},
		})
		return
	}

	err = fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("shame_board")
		doc, err := tx.Get(docRef)
		var data struct {
			Members []ShameBoardEntry `firestore:"members"`
		}
		if err != nil {
			if status.Code(err) == codes.NotFound {
				data.Members = []ShameBoardEntry{}
			} else {
				return err
			}
		} else {
			if err := doc.DataTo(&data); err != nil {
				data.Members = []ShameBoardEntry{}
			}
		}

		existingIDs := make(map[string]bool)
		for _, e := range data.Members {
			existingIDs[e.DiscordID] = true
		}

		for _, nm := range newMembers {
			if !existingIDs[nm.DiscordID] {
				data.Members = append(data.Members, nm)
			}
		}

		return tx.Set(docRef, map[string]interface{}{"members": data.Members}, firestore.Merge(firestore.FieldPath{"members"}))
	})

	if err != nil {
		log.Printf("[shame] sync save: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}

	cacheInvalidate("shame")
	shameCacheMu.Lock()
	shameCacheData = nil
	shameCacheMu.Unlock()

	var added []ShameBoardEntry
	for _, dm := range discordMembers {
		if _, exists := fsEntries[dm.User.ID]; !exists {
			entry := ShameBoardEntry{
				DiscordID: dm.User.ID,
				Username:  dm.User.Username,
				Avatar:    dm.User.Avatar,
				AddedAt:   time.Now().UTC().Format(time.RFC3339),
			}
			added = append(added, entry)
		}
	}

	writeJSON(w, map[string]interface{}{
		"success":  true,
		"newCount": len(newMembers),
		"added":    added,
	})
}

func shameBoardNotificationLoop() {
	if discordBotToken == "" || discordWebhookURL == "" {
		return
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		knownIDs := make(map[string]bool)
		loaded := false

		for range ticker.C {
			if fsClient == nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			members, err := fetchDiscordRoleMembers()
			if err != nil {
				cancel()
				log.Printf("[shame] notification check failed: %v", err)
				continue
			}

			fsEntries, err := getShameBoardFirestore(ctx)
			cancel()
			if err != nil {
				log.Printf("[shame] notification fs read failed: %v", err)
				continue
			}

			if !loaded {
				for _, m := range members {
					knownIDs[m.User.ID] = true
				}
				loaded = true
				continue
			}

			for _, m := range members {
				if !knownIDs[m.User.ID] {
					knownIDs[m.User.ID] = true

					hasReason := false
					if entry, ok := fsEntries[m.User.ID]; ok && entry.Reason != "" {
						hasReason = true
					}

					desc := fmt.Sprintf("**%s** добавлен на Доску позора.", m.User.Username)
					if hasReason {
						desc += fmt.Sprintf("\nПричина: %s", fsEntries[m.User.ID].Reason)
					} else {
						desc += "\nПричина не указана."
					}

					embed := discordEmbed{
						Title:       "Доска позора - новый участник",
						Description: desc,
						Color:       0xFF0000,
						Fields: []discordEmbedField{
							{Name: "Discord ID", Value: m.User.ID, Inline: true},
							{Name: "Username", Value: m.User.Username, Inline: true},
						},
						Footer:    &discordEmbedFooter{Text: "SMLT Leaderboard"},
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					}
					payload := discordWebhookPayload{Embeds: []discordEmbed{embed}}
					body, _ := json.Marshal(payload)

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordWebhookURL, strings.NewReader(string(body)))
					if err != nil {
						cancel()
						continue
					}
					req.Header.Set("Content-Type", "application/json")
					resp, err := httpClient.Do(req)
					cancel()
					if err != nil {
						log.Printf("[shame] webhook send: %v", err)
						continue
					}
					resp.Body.Close()
					if resp.StatusCode == 429 {
						time.Sleep(2 * time.Second)
					}
				}
			}

			for id := range knownIDs {
				found := false
				for _, m := range members {
					if m.User.ID == id {
						found = true
						break
					}
				}
				if !found {
					delete(knownIDs, id)
				}
			}
		}
	}()
}
