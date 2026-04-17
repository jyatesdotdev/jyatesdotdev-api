package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	Service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{
		Service: service,
	}
}

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
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Content     string `json:"content"`
	AuthorName  string `json:"authorName"`
	AuthorEmail string `json:"authorEmail"`
	IPAddress   string `json:"ipAddress"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

type UpdateStatusRequest struct {
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

type DeleteRequest struct {
	Slug string `json:"slug"`
}

func (h *Handler) GetComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		statusFilter = "pending"
	}

	responses, err := h.Service.GetComments(ctx, statusFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(responses)
}

func (h *Handler) UpdateCommentStatus(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "commentId")
	if commentID == "" {
		http.Error(w, "commentId is required", http.StatusBadRequest)
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.Status = strings.TrimSpace(req.Status)

	if req.Slug == "" || req.Status == "" {
		http.Error(w, "slug and status are required", http.StatusBadRequest)
		return
	}

	err := h.Service.UpdateCommentStatus(r.Context(), req.Slug, commentID, req.Status)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrCommentNotFound) {
			http.Error(w, "comment not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "status updated successfully",
	})
}

func (h *Handler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "commentId")
	if commentID == "" {
		http.Error(w, "commentId is required", http.StatusBadRequest)
		return
	}

	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	if req.Slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	err := h.Service.DeleteComment(r.Context(), req.Slug, commentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "comment deleted successfully",
	})
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/comments", h.GetComments)
	r.Put("/comments/{commentId}", h.UpdateCommentStatus)
	r.Delete("/comments/{commentId}", h.DeleteComment)
	return r
}
