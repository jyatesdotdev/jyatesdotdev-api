package admin

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jyates/jyatesdotdev-api/backend/internal/requestmeta"
)

const adminBodyLimit = 2 * 1024

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

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
	if statusFilter != "pending" && statusFilter != "approved" && statusFilter != "rejected" {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	responses, err := h.Service.GetComments(ctx, statusFilter)
	if err != nil {
		log.Printf("get admin comments failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(responses)
}

func (h *Handler) UpdateCommentStatus(w http.ResponseWriter, r *http.Request) {
	commentID := strings.TrimSpace(chi.URLParam(r, "commentId"))
	if !validIdentifier(commentID) {
		http.Error(w, "invalid commentId", http.StatusBadRequest)
		return
	}

	var req UpdateStatusRequest
	if err := requestmeta.DecodeJSON(w, r, &req, adminBodyLimit); err != nil {
		if errors.Is(err, requestmeta.ErrBodyTooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.Status = strings.TrimSpace(req.Status)

	if !validIdentifier(req.Slug) || (req.Status != "approved" && req.Status != "pending" && req.Status != "rejected") {
		http.Error(w, "invalid slug or status", http.StatusBadRequest)
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
		log.Printf("update comment status failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "status updated successfully",
	})
}

func (h *Handler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID := strings.TrimSpace(chi.URLParam(r, "commentId"))
	if !validIdentifier(commentID) {
		http.Error(w, "invalid commentId", http.StatusBadRequest)
		return
	}

	var req DeleteRequest
	if err := requestmeta.DecodeJSON(w, r, &req, adminBodyLimit); err != nil {
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

	err := h.Service.DeleteComment(r.Context(), req.Slug, commentID)
	if err != nil {
		if errors.Is(err, ErrCommentNotFound) {
			http.Error(w, "comment not found", http.StatusNotFound)
			return
		}
		log.Printf("delete comment failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "comment deleted successfully",
	})
}

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/comments", h.GetComments)
	r.Put("/comments/{commentId}", h.UpdateCommentStatus)
	r.Delete("/comments/{commentId}", h.DeleteComment)
	return r
}
