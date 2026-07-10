package interactions

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jyates/jyatesdotdev-api/backend/internal/requestmeta"
)

type CommentItem struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	GSI1PK      string `dynamodbav:"GSI1PK"`
	GSI1SK      string `dynamodbav:"GSI1SK"`
	Content     string `dynamodbav:"content"`
	AuthorName  string `dynamodbav:"authorName"`
	AuthorEmail string `dynamodbav:"authorEmail"`
	IPAddress   string `dynamodbav:"ipAddress"`
	Status      string `dynamodbav:"status"`
	CreatedAt   string `dynamodbav:"createdAt"`
	UpdatedAt   string `dynamodbav:"updatedAt"`
	LikeCount   int    `dynamodbav:"likeCount"`
}

type CommentResponse struct {
	ID           string `json:"id"`
	Content      string `json:"content"`
	AuthorName   string `json:"authorName"`
	CreatedAt    string `json:"createdAt"`
	LikeCount    int    `json:"likeCount"`
	UserHasLiked bool   `json:"userHasLiked"`
}

type CreateCommentRequest struct {
	Slug        string `json:"slug"`
	Content     string `json:"content"`
	AuthorName  string `json:"authorName"`
	AuthorEmail string `json:"authorEmail"`
	Website     string `json:"website"` // honeypot field
}

func (h *Handler) GetComments(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if !validIdentifier(slug) {
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}

	visitorID := strings.TrimSpace(r.Header.Get("X-Visitor-Id"))
	if visitorID != "" && !validIdentifier(visitorID) {
		http.Error(w, "invalid X-Visitor-Id header", http.StatusBadRequest)
		return
	}

	responses, err := h.Service.GetComments(r.Context(), slug, visitorID)
	if err != nil {
		log.Printf("get comments failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(responses)
}

func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	var req CreateCommentRequest
	if err := requestmeta.DecodeJSON(w, r, &req, commentBodyLimit); err != nil {
		if errors.Is(err, requestmeta.ErrBodyTooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.Content = strings.TrimSpace(req.Content)
	req.AuthorName = strings.TrimSpace(req.AuthorName)
	req.AuthorEmail = strings.TrimSpace(req.AuthorEmail)

	if !validIdentifier(req.Slug) || req.Content == "" || !validLength(req.Content, maxCommentLength) ||
		req.AuthorName == "" || !validLength(req.AuthorName, maxNameLength) || !validEmail(req.AuthorEmail) {
		http.Error(w, "invalid slug, content, authorName, or authorEmail", http.StatusBadRequest)
		return
	}

	ipAddress := requestmeta.ClientIP(r)

	commentID, err := h.Service.CreateComment(r.Context(), req, ipAddress)
	if err != nil {
		switch {
		case errors.Is(err, ErrHoneypot):
			writeCommentCreated(w, "")
			return
		case errors.Is(err, ErrInvalidInput):
			http.Error(w, "content or authorName is invalid after sanitization", http.StatusBadRequest)
			return
		case errors.Is(err, ErrRateLimited):
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		log.Printf("create comment failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeCommentCreated(w, commentID)
}

func writeCommentCreated(w http.ResponseWriter, commentID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "comment posted successfully",
		"id":      commentID,
	})
}

type ToggleCommentLikeRequest struct {
	Slug string `json:"slug"`
}

func (h *Handler) ToggleCommentLike(w http.ResponseWriter, r *http.Request) {
	commentID := strings.TrimSpace(chi.URLParam(r, "id"))
	if !validIdentifier(commentID) {
		http.Error(w, "invalid comment ID", http.StatusBadRequest)
		return
	}

	var req ToggleCommentLikeRequest
	if err := requestmeta.DecodeJSON(w, r, &req, toggleBodyLimit); err != nil {
		if errors.Is(err, requestmeta.ErrBodyTooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	if !validIdentifier(req.Slug) {
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}

	visitorID := strings.TrimSpace(r.Header.Get("X-Visitor-Id"))
	if !validIdentifier(visitorID) {
		http.Error(w, "valid X-Visitor-Id header is required", http.StatusBadRequest)
		return
	}

	err := h.Service.ToggleCommentLike(r.Context(), req.Slug, commentID, visitorID, requestmeta.ClientIP(r))
	if err != nil {
		switch {
		case errors.Is(err, ErrRateLimited):
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		case errors.Is(err, ErrConflict):
			http.Error(w, "comment like state changed; retry the request", http.StatusConflict)
			return
		}
		log.Printf("toggle comment like failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "comment like toggled successfully",
	})
}

func (h *Handler) CommentRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetComments)
	r.Post("/", h.CreateComment)
	r.Post("/{id}/like", h.ToggleCommentLike)
	return r
}
