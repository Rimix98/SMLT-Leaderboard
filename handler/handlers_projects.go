package handler

import (
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/api/iterator"
)

func handleGetProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !requireFirestore(w) {
		return
	}

	if cached, ok := cacheGet("projects"); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=120")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	ctx := r.Context()
	iter := fsClient.Collection("projects").Documents(ctx)
	projects := make([]Project, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			slog.Error("projects iter error", "error", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
			return
		}
		var p Project
		if err := doc.DataTo(&p); err != nil {
			continue
		}
		projects = append(projects, p)
	}
	body, _ := json.Marshal(projects)
	cacheSet("projects", body, 2*time.Minute)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=120")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

func handleSaveProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireFirestore(w) {
		return
	}
	var projectList []Project
	if err := decodeRequestJSON(w, r, &projectList); err != nil {
		slog.Error("projects decode error", "error", err)
		sendError(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}
	ctx := r.Context()
	for i, p := range projectList {
		projectList[i].Name = sanitizeString(p.Name)
		projectList[i].VideoID = sanitizeString(p.VideoID)
		projectList[i].Comment = sanitizeString(p.Comment)
		projectList[i].Verifier = sanitizeString(p.Verifier)
		for j, part := range projectList[i].Participants {
			projectList[i].Participants[j] = sanitizeString(part)
		}
		if len(projectList[i].Name) > 100 ||
			len(projectList[i].VideoID) > 200 ||
			len(projectList[i].Comment) > 500 ||
			len(projectList[i].Verifier) > 50 {
			sendError(w, http.StatusBadRequest, "Слишком длинное поле в проекте")
			return
		}
		for _, part := range projectList[i].Participants {
			if len(part) > 5000 {
				sendError(w, http.StatusBadRequest, "Слишком длинное имя участника")
				return
			}
		}
	}

	seen := make(map[string]bool)
	for _, p := range projectList {
		if p.ID == "" {
			continue
		}
		var docID string
		if p.ID == "-" {
			b := make([]byte, 8)
			if _, err := crypto_rand.Read(b); err != nil {
				slog.Error("projects rand error", "error", err)
				sendError(w, http.StatusInternalServerError, "Ошибка генерации ID")
				return
			}
			docID = fmt.Sprintf("-%x", b)
		} else {
			if err := validateProjectID(p.ID); err != nil {
				slog.Error("invalid project id", "id", p.ID, "error", err)
				sendError(w, http.StatusBadRequest, "Некорректный ID проекта")
				return
			}
			if seen[p.ID] {
				slog.Error("duplicate project id", "id", p.ID)
				sendError(w, http.StatusBadRequest, "ID проекта уже существует")
				return
			}
			docID = p.ID
		}
		seen[docID] = true
		ref := fsClient.Collection("projects").Doc(docID)
		if _, err := ref.Set(ctx, p); err != nil {
			slog.Error("project set failed", "docID", docID, "error", err)
			sendError(w, http.StatusInternalServerError, "Ошибка базы данных")
			return
		}
	}

	iter := fsClient.Collection("projects").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			slog.Error("projects delete iter error", "error", err)
			break
		}
		if !seen[doc.Ref.ID] {
			if _, err := doc.Ref.Delete(ctx); err != nil {
				slog.Error("project delete failed", "docID", doc.Ref.ID, "error", err)
			}
		}
	}

	auditLog(r.Context(), AuditEntry{
		Action:  "projects.save",
		Details: map[string]int{"count": len(projectList)},
	})
	cacheInvalidate("projects")
	writeJSON(w, map[string]bool{"success": true})
}
