package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func handleGetStaff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}

	if cached, ok := cacheGet("staff"); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=120")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	ctx := r.Context()
	doc, err := fsClient.Collection("config").Doc("staff").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			writeJSON(w, []StaffRole{})
			return
		}
		log.Printf("[staff] Get staff doc: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	var data struct {
		Roles []StaffRole `json:"roles" firestore:"roles"`
	}
	if err := doc.DataTo(&data); err != nil {
		log.Printf("[staff] DataTo error: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	body, _ := json.Marshal(data.Roles)
	cacheSet("staff", body, 2*time.Minute)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=120")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

func handleSaveStaff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var roles []StaffRole
	if err := decodeRequestJSON(w, r, &roles); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	if len(roles) > 50 {
		sendError(w, http.StatusBadRequest, "Слишком много ролей")
		return
	}
	for i := range roles {
		roles[i].Name = sanitizeString(roles[i].Name)
		roles[i].Color = sanitizeString(roles[i].Color)
		if err := validateRoleName(roles[i].Name); err != nil {
			sendError(w, http.StatusBadRequest, "Некорректные данные")
			return
		}
		c, err := normalizeColor(roles[i].Color)
		if err != nil {
			sendError(w, http.StatusBadRequest, "Некорректный цвет")
			return
		}
		roles[i].Color = c
		for j := range roles[i].Players {
			roles[i].Players[j].Nickname = sanitizeString(roles[i].Players[j].Nickname)
			roles[i].Players[j].Discord = sanitizeString(roles[i].Players[j].Discord)
		}
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		return tx.Set(docRef, map[string]interface{}{"roles": roles}, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] save roles: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action:  "staff.save",
		Details: map[string]int{"count": len(roles)},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]bool{"success": true})
}

func handleStaffAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int    `json:"roleIndex"`
		Nickname  string `json:"nickname"`
		Discord   string `json:"discord"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Nickname = sanitizeString(req.Nickname)
	req.Discord = sanitizeString(req.Discord)
	if err := validateNickname(req.Nickname); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}
	if err := validateDiscord(req.Discord); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errValidation{"invalid role index"}
		}
		player := StaffPlayer{Nickname: req.Nickname, Discord: req.Discord}
		data.Roles[req.RoleIndex].Players = append(data.Roles[req.RoleIndex].Players, player)
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})

	if err != nil {
		if _, ok := err.(errValidation); ok {
			sendError(w, http.StatusBadRequest, "Неверный индекс роли")
		} else {
			log.Printf("[staff] add player: %v", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.add",

		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"nickname":  req.Nickname,
		},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]interface{}{
		"success":  true,
		"nickname": req.Nickname,
		"discord":  req.Discord,
	})
}

func handleCreateStaffRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Name = sanitizeString(req.Name)
	req.Color = sanitizeString(req.Color)
	if err := validateRoleName(req.Name); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}
	c, err := normalizeColor(req.Color)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Некорректный цвет")
		return
	}
	req.Color = c

	ctx := r.Context()
	newRole := StaffRole{Name: req.Name, Color: req.Color, Players: []StaffPlayer{}}
	err = fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err != nil {
			if status.Code(err) == codes.NotFound {
				data.Roles = []StaffRole{}
			} else {
				return err
			}
		} else {
			if err := doc.DataTo(&data); err != nil {
				data.Roles = []StaffRole{}
			}
		}
		data.Roles = append(data.Roles, newRole)
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] create role: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.createRole",

		Details: map[string]string{"name": req.Name, "color": req.Color},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]interface{}{
		"success": true,
		"name":    req.Name,
		"color":   req.Color,
		"players": []StaffPlayer{},
	})
}

func handleDeleteStaffRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int `json:"roleIndex"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errValidation{"invalid role index"}
		}
		data.Roles = append(data.Roles[:req.RoleIndex], data.Roles[req.RoleIndex+1:]...)
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		if _, ok := err.(errValidation); ok {
			sendError(w, http.StatusBadRequest, "Неверный индекс роли")
		} else {
			log.Printf("[staff] delete role: %v", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.deleteRole",

		Details: map[string]int{"roleIndex": req.RoleIndex},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]bool{"success": true})
}

func handleUpdateStaffRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex    int           `json:"roleIndex"`
		Name         string        `json:"name"`
		Color        string        `json:"color"`
		Players      []StaffPlayer `json:"players"`
		TiersEnabled *bool         `json:"tiersEnabled"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Name = sanitizeString(req.Name)
	req.Color = sanitizeString(req.Color)
	if err := validateRoleName(req.Name); err != nil {
		sendError(w, http.StatusBadRequest, "Некорректные данные")
		return
	}
	c, err := normalizeColor(req.Color)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Некорректный цвет")
		return
	}
	req.Color = c

	ctx := r.Context()
	err = fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errValidation{"invalid role index"}
		}
		data.Roles[req.RoleIndex].Name = req.Name
		data.Roles[req.RoleIndex].Color = req.Color
		if req.Players != nil {
			for j := range req.Players {
				req.Players[j].Nickname = sanitizeString(req.Players[j].Nickname)
				req.Players[j].Discord = sanitizeString(req.Players[j].Discord)
			}
			data.Roles[req.RoleIndex].Players = req.Players
		}
		if req.TiersEnabled != nil {
			data.Roles[req.RoleIndex].TiersEnabled = *req.TiersEnabled
		}
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		if _, ok := err.(errValidation); ok {
			sendError(w, http.StatusBadRequest, "Неверный индекс роли")
		} else {
			log.Printf("[staff] update role: %v", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.updateRole",

		Details: map[string]interface{}{"roleIndex": req.RoleIndex, "name": req.Name, "color": req.Color},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]bool{"success": true})
}

func handleStaffRole(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleCreateStaffRole(w, r)
	case http.MethodPut:
		handleUpdateStaffRole(w, r)
	case http.MethodDelete:
		handleDeleteStaffRole(w, r)
	default:
		methodNotAllowed(w, "POST, PUT, DELETE")
	}
}

func handleStaffRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int    `json:"roleIndex"`
		Nickname  string `json:"nickname"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Nickname = sanitizeString(req.Nickname)

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		if req.RoleIndex < 0 || req.RoleIndex >= len(data.Roles) {
			return errValidation{"invalid role index"}
		}
		players := data.Roles[req.RoleIndex].Players
		found := false
		for i, p := range players {
			if p.Nickname == req.Nickname {
				data.Roles[req.RoleIndex].Players = append(players[:i], players[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return errValidation{"player not found"}
		}
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		if ve, ok := err.(errValidation); ok {
			if ve.msg == "invalid role index" {
				sendError(w, http.StatusBadRequest, "Неверный индекс роли")
			} else {
				sendError(w, http.StatusNotFound, "Игрок не найден")
			}
		} else {
			log.Printf("[staff] remove player: %v", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.removePlayer",

		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"nickname":  req.Nickname,
		},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]bool{"success": true})
}

func handleReorderStaffRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		RoleIndex int    `json:"roleIndex"`
		Direction string `json:"direction"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}
		var data struct {
			Roles []StaffRole `json:"roles" firestore:"roles"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}
		idx := req.RoleIndex
		target := idx - 1
		if req.Direction == "down" {
			target = idx + 1
		}
		if target < 0 || target >= len(data.Roles) {
			return errors.New("invalid move")
		}
		data.Roles[idx], data.Roles[target] = data.Roles[target], data.Roles[idx]
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
	if err != nil {
		log.Printf("[staff] reorder: %v", err)
		if err.Error() == "invalid move" {
			sendError(w, http.StatusBadRequest, "Некорректное перемещение")
		} else {
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		}
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.reorder",

		Details: map[string]interface{}{
			"roleIndex": req.RoleIndex,
			"direction": req.Direction,
		},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]bool{"success": true})
}

func handleGetStaffTiers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}
	ctx := r.Context()
	doc, err := fsClient.Collection("config").Doc("staff").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			writeJSON(w, map[string]interface{}{"gp": []StaffTierEntry{}, "deco": []StaffTierEntry{}})
			return
		}
		log.Printf("[staff] Get tiers doc: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	var raw map[string]interface{}
	if err := doc.DataTo(&raw); err != nil {
		log.Printf("[staff] tiers DataTo error: %v", err)
		writeJSON(w, map[string]interface{}{"gp": []StaffTierEntry{}, "deco": []StaffTierEntry{}})
		return
	}
	gpTiers := []StaffTierEntry{}
	decoTiers := []StaffTierEntry{}
	if gpRaw, ok := raw["gp_tiers"]; ok {
		if gpArr, ok := gpRaw.([]interface{}); ok {
			for _, item := range gpArr {
				if m, ok := item.(map[string]interface{}); ok {
					nickname, _ := m["nickname"].(string)
					tier, _ := m["tier"].(string)
					gpTiers = append(gpTiers, StaffTierEntry{Nickname: nickname, Tier: tier})
				}
			}
		}
	}
	if decoRaw, ok := raw["deco_tiers"]; ok {
		if decoArr, ok := decoRaw.([]interface{}); ok {
			for _, item := range decoArr {
				if m, ok := item.(map[string]interface{}); ok {
					nickname, _ := m["nickname"].(string)
					tier, _ := m["tier"].(string)
					decoTiers = append(decoTiers, StaffTierEntry{Nickname: nickname, Tier: tier})
				}
			}
		}
	}
	writeJSON(w, map[string]interface{}{"gp": gpTiers, "deco": decoTiers})
}

func handleSetStaffTier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var req struct {
		Category string `json:"category"`
		Nickname string `json:"nickname"`
		Tier     string `json:"tier"`
	}
	if err := decodeRequestJSON(w, r, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Кривой JSON")
		return
	}
	req.Category = sanitizeString(req.Category)
	req.Nickname = sanitizeString(req.Nickname)
	req.Tier = sanitizeString(req.Tier)

	if req.Category != "gp" && req.Category != "deco" {
		sendError(w, http.StatusBadRequest, "Некорректная категория")
		return
	}
	if req.Nickname == "" || len(req.Nickname) > 32 {
		sendError(w, http.StatusBadRequest, "Некорректный ник")
		return
	}
	validTiers := map[string]bool{"priority": true, "base": true, "reserve": true, "na": true}
	if !validTiers[req.Tier] {
		sendError(w, http.StatusBadRequest, "Некорректный тир")
		return
	}

	ctx := r.Context()
	err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return tx.Set(docRef, map[string]interface{}{
					req.Category + "_tiers": []StaffTierEntry{{Nickname: req.Nickname, Tier: req.Tier}},
				})
			}
			return err
		}
		var data struct {
			Roles     []StaffRole      `json:"roles" firestore:"roles"`
			GPTiers   []StaffTierEntry `json:"gp_tiers" firestore:"gp_tiers"`
			DecoTiers []StaffTierEntry `json:"deco_tiers" firestore:"deco_tiers"`
		}
		if err := doc.DataTo(&data); err != nil {
			return err
		}

		var tiers *[]StaffTierEntry
		if req.Category == "gp" {
			tiers = &data.GPTiers
		} else {
			tiers = &data.DecoTiers
		}

		found := false
		for i, entry := range *tiers {
			if entry.Nickname == req.Nickname {
				(*tiers)[i].Tier = req.Tier
				found = true
				break
			}
		}
		if !found {
			*tiers = append(*tiers, StaffTierEntry{Nickname: req.Nickname, Tier: req.Tier})
		}

		return tx.Set(docRef, map[string]interface{}{
			req.Category + "_tiers": *tiers,
		}, firestore.Merge(firestore.FieldPath{req.Category + "_tiers"}))
	})

	if err != nil {
		log.Printf("[staff] set tier: %v", err)
		sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
		return
	}
	auditLog(r.Context(), AuditEntry{
		Action: "staff.setTier",

		Details: map[string]interface{}{
			"category": req.Category,
			"nickname": req.Nickname,
			"tier":     req.Tier,
		},
	})
	cacheInvalidate("staff")
	writeJSON(w, map[string]bool{"success": true})
}

func validateProjectID(id string) error {
	if id == "" || !reProjectID.MatchString(id) {
		return errors.New("invalid project id")
	}
	return nil
}

func validateNickname(n string) error {
	if len(n) < 1 || len(n) > 32 {
		return errors.New("invalid nickname length")
	}
	return nil
}

func validateDiscord(d string) error {
	if d == "" {
		return nil
	}
	if len(d) > 64 {
		return errors.New("discord too long")
	}
	if !reDiscord.MatchString(d) {
		return errors.New("invalid discord characters")
	}
	return nil
}

func validateRoleName(n string) error {
	if len(n) < 2 || len(n) > 32 {
		return errors.New("invalid role name length")
	}
	if !reRoleName.MatchString(n) {
		return errors.New("invalid role name characters")
	}
	return nil
}

func normalizeColor(color string) (string, error) {
	if color == "" {
		return "#3b82f6", nil
	}
	if !reHexColor.MatchString(color) {
		return "", errors.New("некорректный цвет")
	}
	if !strings.HasPrefix(color, "#") {
		return "#" + color, nil
	}
	return color, nil
}

func readStaffConfig(ctx context.Context) (*staffData, error) {
	docRef := fsClient.Collection("config").Doc("staff")
	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return &staffData{Roles: []StaffRole{}}, nil
		}
		return nil, err
	}
	var data staffData
	if err := doc.DataTo(&data); err != nil {
		return &staffData{Roles: []StaffRole{}}, nil
	}
	return &data, nil
}

func writeStaffConfig(ctx context.Context, data *staffData) error {
	docRef := fsClient.Collection("config").Doc("staff")
	_, err := docRef.Set(ctx, data, firestore.Merge(firestore.FieldPath{"roles"}))
	return err
}

func updateStaffConfig(ctx context.Context, fn func(data *staffData) error) error {
	return fsClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := fsClient.Collection("config").Doc("staff")
		doc, err := tx.Get(docRef)
		var data staffData
		if err != nil {
			if status.Code(err) == codes.NotFound {
				data.Roles = []StaffRole{}
			} else {
				return err
			}
		} else {
			if err := doc.DataTo(&data); err != nil {
				data.Roles = []StaffRole{}
			}
		}
		if err := fn(&data); err != nil {
			return err
		}
		return tx.Set(docRef, data, firestore.Merge(firestore.FieldPath{"roles"}))
	})
}
