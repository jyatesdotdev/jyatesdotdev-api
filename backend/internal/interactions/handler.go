package interactions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
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
	Slug  string `json:"slug"`
	Token string `json:"token"`
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
	slug := r.URL.Query().Get("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	ipAddress := h.extractIP(r)

	resp, err := h.Service.GetLikes(r.Context(), slug, ipAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) ToggleLike(w http.ResponseWriter, r *http.Request) {
	var req ToggleLikeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	ipAddress := h.extractIP(r)

	resp, err := h.Service.ToggleLike(r.Context(), req.Slug, ipAddress, req.Token)
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

func (h *Handler) extractIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		if ip, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(ip)
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}
