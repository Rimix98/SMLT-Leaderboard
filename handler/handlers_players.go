package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	players := playersForLeaderboard(r.Context())
	writeJSON(w, players)
}

func handleSavePlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var players []Player
	if err := decodeRequestJSON(w, r, &players); err != nil {
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}
	if len(players) > 200 {
		sendError(w, http.StatusBadRequest, "Слишком много игроков")
		return
	}
	for i, p := range players {
		players[i].Name = sanitizeString(p.Name)
		if len(p.Name) == 0 || len(p.Name) > 32 {
			sendError(w, http.StatusBadRequest, "Некорректные данные игрока")
			return
		}
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("players")
		return tx.Set(docRef, map[string]interface{}{"players": players})
	})
	if err != nil {
		slog.Error("players save failed", "error", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action:  "players.save",
		Details: map[string]int{"count": len(players)},
	})
	cacheInvalidate("leaderboard")
	writeJSON(w, map[string]bool{"success": true})
}

func handleDeletePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Name = sanitizeString(req.Name)
	if req.Name == "" {
		sendError(w, http.StatusBadRequest, "Имя игрока обязательно")
		return
	}
	if len(req.Name) > 32 {
		sendError(w, http.StatusBadRequest, "Слишком длинное имя игрока")
		return
	}
	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("players")
		doc, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return errors.New("player not found")
			}
			return err
		}
		var data struct {
			Players []Player `json:"players" firestore:"players"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		found := false
		for i, p := range data.Players {
			if p.Name == req.Name {
				data.Players = append(data.Players[:i], data.Players[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return errors.New("player not found")
		}
		return tx.Set(docRef, map[string]interface{}{"players": data.Players})
	})
	if err != nil {
		slog.Error("player delete failed", "name", req.Name, "error", err)
		if err.Error() == "player not found" {
			sendError(w, http.StatusNotFound, "Игрок не найден")
		} else {
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action:  "players.delete",
		Details: map[string]string{"name": req.Name},
	})
	cacheInvalidate("leaderboard")
	writeJSON(w, map[string]bool{"success": true})
}

func validateDemonlistURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return errors.New("only https allowed")
	}
	if parsed.Host != "api.demonlist.org" {
		return errors.New("only api.demonlist.org allowed")
	}
	return nil
}

func fetchAPIWithRetry(ctx context.Context, apiURL string, maxRetries int) ([]byte, error) {
	if err := validateDemonlistURL(apiURL); err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
			continue
		}
		return body, nil
	}
	return nil, lastErr
}

func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if cached, ok := cacheGet("leaderboard"); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=120")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	ctx := r.Context()
	players := playersForLeaderboard(ctx)

	type job struct {
		name string
	}
	jobs := make(chan job, len(players))
	var mu sync.Mutex
	result := make([]FullPlayerData, 0, len(players))

	var wg sync.WaitGroup
	workerCount := 5
	if len(players) < workerCount {
		workerCount = len(players)
	}

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-jobs:
					if !ok {
						return
					}
					entry := FullPlayerData{Name: j.name}
					u1 := fmt.Sprintf("https://api.demonlist.org/leaderboard/user/list?search=%s&limit=1", url.QueryEscape(j.name))
					if body, err := fetchAPIWithRetry(ctx, u1, 2); err == nil {
						json.Unmarshal(body, &entry.Data)
					}
					userID := extractUserID(entry.Data, j.name)
					if userID != "" {
						u2 := fmt.Sprintf("https://api.demonlist.org/user/record/list?user_id=%s&limit=50", userID)
						if body, err := fetchAPIWithRetry(ctx, u2, 2); err == nil {
							json.Unmarshal(body, &entry.Records)
						}
					}
					mu.Lock()
					result = append(result, entry)
					mu.Unlock()
				}
			}
		}()
	}
	for _, p := range players {
		jobs <- job{name: p.Name}
	}
	close(jobs)
	wg.Wait()

	body, _ := json.Marshal(result)
	cacheSet("leaderboard", body, 2*time.Minute)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=120")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

func handleLeaderboardCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	_, hash, lastSet, ok := cacheGetWithMeta("leaderboard")
	if !ok {
		writeJSON(w, LeaderboardCheckResponse{Hash: "", LastUpdated: "", PlayerCount: 0})
		return
	}
	writeJSON(w, LeaderboardCheckResponse{
		Hash:        hash,
		LastUpdated: lastSet.UTC().Format(time.RFC3339),
		PlayerCount: 0,
	})
}

func loadPlayersFromFirestore(ctx context.Context) ([]Player, error) {
	doc, err := fsClient.Collection("config").Doc("players").Get(ctx)
	if err != nil {
		return nil, err
	}
	var players []Player
	if err := doc.DataTo(&players); err == nil && len(players) > 0 {
		return players, nil
	}
	if raw, ok := doc.Data()["players"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &players); err != nil {
			return nil, err
		}
		if len(players) > 0 {
			return players, nil
		}
	}
	return players, nil
}

func defaultPlayersList() []Player {
	out := make([]Player, len(defaultPlayerNames))
	for i, name := range defaultPlayerNames {
		out[i] = Player{Name: name}
	}
	return out
}

func playersForLeaderboard(ctx context.Context) []Player {
	if fsClient != nil {
		players, err := loadPlayersFromFirestore(ctx)
		if err == nil && len(players) > 0 {
			return players
		}
	}
	return defaultPlayersList()
}

func extractUserID(data interface{}, playerName string) string {
	m, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	d, ok := m["data"].(map[string]interface{})
	if !ok {
		if users, ok := m["users"].([]interface{}); ok {
			return findUserID(users, playerName)
		}
		return ""
	}
	users, ok := d["users"].([]interface{})
	if !ok || len(users) == 0 {
		return ""
	}
	return findUserID(users, playerName)
}

func findUserID(users []interface{}, playerName string) string {
	nl := strings.ToLower(strings.TrimSpace(playerName))
	for _, u := range users {
		user, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		username, _ := user["username"].(string)
		if strings.ToLower(strings.TrimSpace(username)) == nl {
			id, ok := user["id"]
			if !ok {
				continue
			}
			switch v := id.(type) {
			case float64:
				return strconv.FormatInt(int64(v), 10)
			case string:
				return v
			case json.Number:
				return v.String()
			}
		}
	}
	return ""
}

func handlePlayerHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}

	playerID := strings.TrimPrefix(r.URL.Path, "/api/history/")
	playerID = strings.TrimSuffix(playerID, "/")
	if playerID == "" {
		sendError(w, http.StatusBadRequest, "playerId обязателен")
		return
	}

	ctx := r.Context()
	var entries []PlayerHistoryEntry

	if docs, err := fsClient.Collection("player_history").
		Where("playerId", "==", playerID).
		Limit(90).
		Documents(ctx).GetAll(); err != nil {
		slog.Error("history query error", "playerID", playerID, "error", err)
	} else {
		for _, doc := range docs {
			var entry PlayerHistoryEntry
			if err := doc.DataTo(&entry); err != nil {
				continue
			}
			entries = append(entries, entry)
		}
	}

	if entries == nil {
		entries = []PlayerHistoryEntry{}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date > entries[j].Date
	})

	writeJSON(w, entries)
}

func handleSaveHistorySnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}

	ctx := r.Context()
	players := playersForLeaderboard(ctx)

	type job struct {
		name string
	}
	jobs := make(chan job, len(players))
	var mu sync.Mutex
	type snapshotResult struct {
		name    string
		rank    int
		score   float64
		recs    int
		hardest string
	}
	var results []snapshotResult

	var wg sync.WaitGroup
	workerCount := 5
	if len(players) < workerCount {
		workerCount = len(players)
	}

	for wn := 0; wn < workerCount; wn++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-jobs:
					if !ok {
						return
					}
					entry := FullPlayerData{Name: j.name}
					u1 := fmt.Sprintf("https://api.demonlist.org/leaderboard/user/list?search=%s&limit=1", url.QueryEscape(j.name))
					if body, err := fetchAPIWithRetry(ctx, u1, 2); err == nil {
						json.Unmarshal(body, &entry.Data)
					}
					userID := extractUserID(entry.Data, j.name)
					rank := 0
					score := 0.0
					if m, ok := entry.Data.(map[string]interface{}); ok {
						if d, ok := m["data"].(map[string]interface{}); ok {
							if users, ok := d["users"].([]interface{}); ok && len(users) > 0 {
								if u, ok := users[0].(map[string]interface{}); ok {
									if p, ok := u["placement"].(float64); ok {
										rank = int(p)
									}
									if s, ok := u["points"].(string); ok {
										score, _ = strconv.ParseFloat(s, 64)
									}
								}
							}
						}
					}
					recs := 0
					hardest := ""
					if userID != "" {
						u2 := fmt.Sprintf("https://api.demonlist.org/user/record/list?user_id=%s&limit=50", userID)
						if body, err := fetchAPIWithRetry(ctx, u2, 2); err == nil {
							var recData interface{}
							json.Unmarshal(body, &recData)
							if rm, ok := recData.(map[string]interface{}); ok {
								if d, ok := rm["data"].(map[string]interface{}); ok {
									if records, ok := d["records"].([]interface{}); ok {
										recs = len(records)
										for _, r := range records {
											if rec, ok := r.(map[string]interface{}); ok {
												if status, _ := rec["status"].(string); status == "accepted" {
													if lvl, ok := rec["level"].(map[string]interface{}); ok {
														if placement, ok := lvl["placement"].(float64); ok {
															if name, ok := lvl["name"].(string); ok {
																if hardest == "" || placement < 100 {
																	hardest = name
																}
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
					mu.Lock()
					results = append(results, snapshotResult{name: j.name, rank: rank, score: score, recs: recs, hardest: hardest})
					mu.Unlock()
				}
			}
		}()
	}
	for _, p := range players {
		jobs <- job{name: p.Name}
	}
	close(jobs)
	wg.Wait()

	today := time.Now().UTC().Format("2006-01-02")
	batch := fsClient.Batch()
	for _, res := range results {
		docID := fmt.Sprintf("%s_%s", res.name, today)
		entry := PlayerHistoryEntry{
			PlayerID:     res.name,
			PlayerName:   res.name,
			Date:         today,
			Rank:         res.rank,
			Score:        res.score,
			RecordsCount: res.recs,
			HardestLevel: res.hardest,
			SnapshotAt:   time.Now().UTC(),
		}
		batch.Set(fsClient.Collection("player_history").Doc(docID), entry)
	}
	if _, err := batch.Commit(ctx); err != nil {
		slog.Error("history snapshot commit failed", "error", err)
		sendError(w, http.StatusInternalServerError, "Ошибка сохранения снимка")
		return
	}

	securityEvent(ctx, "history_snapshot", getRealIP(r), "/api/history/snapshot", map[string]string{
		"player_count": strconv.Itoa(len(results)),
	})
	writeJSON(w, map[string]interface{}{
		"success":      true,
		"snapshotDate": today,
		"playersSaved": len(results),
	})
}
