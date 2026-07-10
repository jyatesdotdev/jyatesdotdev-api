package subscriptions

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jyates/jyatesdotdev-api/backend/internal/requestmeta"
)

const subscriptionBodyLimit = 4 * 1024

type SubscribeRequest struct {
	Email   string   `json:"email"`
	Topics  []string `json:"topics"`
	Website string   `json:"website"` // honeypot field -- should always be empty
}

type ConfirmRequest struct {
	Token string `json:"token"`
}

type Handler struct {
	Service     Service
	RateLimiter RateLimiter
}

func NewHandler(service Service, rateLimiter RateLimiter) *Handler {
	return &Handler{Service: service, RateLimiter: rateLimiter}
}

func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	var req SubscribeRequest
	if err := requestmeta.DecodeJSON(w, r, &req, subscriptionBodyLimit); err != nil {
		if errors.Is(err, requestmeta.ErrBodyTooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	topics, ok := normalizeTopics(req.Topics)
	if !validEmail(email) || !ok {
		http.Error(w, "invalid email or topics", http.StatusBadRequest)
		return
	}

	// Return the same accepted response for honeypot submissions.
	if req.Website != "" {
		writeAccepted(w)
		return
	}

	if h.RateLimiter != nil {
		if err := h.RateLimiter.Allow(r.Context(), requestmeta.ClientIP(r)); err != nil {
			if errors.Is(err, ErrRateLimited) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			log.Printf("subscription rate limit failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	if err := h.Service.RequestSubscription(r.Context(), email, topics); err != nil {
		log.Printf("subscription request failed: %v", err)
		http.Error(w, "could not request subscription", http.StatusInternalServerError)
		return
	}
	writeAccepted(w)
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	var req ConfirmRequest
	if err := requestmeta.DecodeJSON(w, r, &req, subscriptionBodyLimit); err != nil {
		if errors.Is(err, requestmeta.ErrBodyTooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	token := strings.TrimSpace(req.Token)
	if token == "" || len(token) > 128 {
		http.Error(w, "invalid confirmation token", http.StatusBadRequest)
		return
	}
	if err := h.Service.ConfirmSubscription(r.Context(), token); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			http.Error(w, "confirmation link is invalid or expired", http.StatusGone)
			return
		}
		log.Printf("subscription confirmation failed: %v", err)
		http.Error(w, "could not confirm subscription", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	// #nosec G104 -- Response write errors cannot be recovered here.
	w.Write([]byte(`{"message":"subscription confirmed"}`))
}

func normalizeTopics(values []string) ([]string, bool) {
	requested := make(map[string]bool, len(values))
	for _, value := range values {
		topic := strings.ToLower(strings.TrimSpace(value))
		if topic != TopicBlog && topic != TopicProjects {
			return nil, false
		}
		requested[topic] = true
	}
	if len(requested) == 0 {
		return nil, false
	}

	topics := make([]string, 0, len(requested))
	for _, topic := range supportedTopics {
		if requested[topic] {
			topics = append(topics, topic)
		}
	}
	return topics, true
}

func validEmail(value string) bool {
	if value == "" || len(value) > 254 {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

func writeAccepted(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	// #nosec G104 -- Response write errors cannot be recovered here.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "check your email to confirm your subscription",
	})
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Subscribe)
	r.Post("/confirm", h.Confirm)
	return r
}
