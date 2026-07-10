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

type PostMetadata struct {
	LikeCount int `dynamodbav:"likeCount"`
}

type PostLike struct {
	CreatedAt string `dynamodbav:"createdAt"`
}

type LikesResponse struct {
	Slug         string `json:"slug"`
	LikeCount    int    `json:"likeCount"`
	UserHasLiked bool   `json:"userHasLiked"`
}

type ToggleLikeRequest struct {
	Slug string `json:"slug"`
}

type Handler struct {
	Service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{
		Service: service,
	}
}

func (h *Handler) GetLikes(w http.ResponseWriter, r *http.Request) {
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

	resp, err := h.Service.GetLikes(r.Context(), slug, visitorID)
	if err != nil {
		log.Printf("get likes failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) ToggleLike(w http.ResponseWriter, r *http.Request) {
	var req ToggleLikeRequest
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

	resp, err := h.Service.ToggleLike(r.Context(), req.Slug, visitorID, requestmeta.ClientIP(r))
	if err != nil {
		switch {
		case errors.Is(err, ErrRateLimited):
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		case errors.Is(err, ErrConflict):
			http.Error(w, "like state changed; retry the request", http.StatusConflict)
			return
		}
		log.Printf("toggle like failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetLikes)
	r.Post("/", h.ToggleLike)
	return r
}
