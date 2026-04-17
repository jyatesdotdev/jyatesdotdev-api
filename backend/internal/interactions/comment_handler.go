package interactions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
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
	Token       string `json:"token"`
}

func (h *Handler) GetComments(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	ipAddress := h.extractIP(r)

	responses, err := h.Service.GetComments(r.Context(), slug, ipAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(responses)
}

func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	var req CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.Content = strings.TrimSpace(req.Content)
	req.AuthorName = strings.TrimSpace(req.AuthorName)

	if req.Slug == "" || req.Content == "" || req.AuthorName == "" {
		http.Error(w, "slug, content, and authorName are required", http.StatusBadRequest)
		return
	}

	ipAddress := h.extractIP(r)

	commentID, err := h.Service.CreateComment(r.Context(), req, ipAddress)
	if err != nil {
		if errors.Is(err, ErrInvalidRecaptcha) {
			http.Error(w, "invalid recaptcha token", http.StatusForbidden)
			return
		}
		if errors.Is(err, ErrRecaptchaFailed) {
			http.Error(w, "recaptcha verification failed", http.StatusInternalServerError)
			return
		}
		if errors.Is(err, ErrInvalidInput) {
			http.Error(w, "content or authorName is invalid after sanitization", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "comment submitted successfully and is pending approval",
		"id":      commentID,
	})
}

type ToggleCommentLikeRequest struct {
	Slug  string `json:"slug"`
	Token string `json:"token"`
}

func (h *Handler) ToggleCommentLike(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		http.Error(w, "comment ID is required", http.StatusBadRequest)
		return
	}

	var req ToggleCommentLikeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	if req.Slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	ipAddress := h.extractIP(r)

	err := h.Service.ToggleCommentLike(r.Context(), req.Slug, commentID, ipAddress, req.Token)
	if err != nil {
		if errors.Is(err, ErrInvalidRecaptcha) {
			http.Error(w, "invalid recaptcha token", http.StatusForbidden)
			return
		}
		if errors.Is(err, ErrRecaptchaFailed) {
			http.Error(w, "recaptcha verification failed", http.StatusInternalServerError)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
