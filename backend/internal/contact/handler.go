package contact

import (
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/jyates/jyatesdotdev-api/backend/internal/email"
	"github.com/jyates/jyatesdotdev-api/backend/internal/requestmeta"
)

const (
	contactBodyLimit = 16 * 1024
	maxNameLength    = 100
	maxEmailLength   = 254
	maxMessageLength = 5000
)

type Request struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
	Website string `json:"website"` // honeypot field — should always be empty
}

type Handler struct {
	EmailService email.Service
	RateLimiter  RateLimiter
}

func NewHandler(emailService email.Service, rateLimiter RateLimiter) *Handler {
	return &Handler{
		EmailService: emailService,
		RateLimiter:  rateLimiter,
	}
}

func (h *Handler) SubmitContact(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := requestmeta.DecodeJSON(w, r, &req, contactBodyLimit); err != nil {
		if errors.Is(err, requestmeta.ErrBodyTooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Message = strings.TrimSpace(req.Message)

	if req.Name == "" || !validLength(req.Name, maxNameLength) || !validEmail(req.Email) ||
		req.Message == "" || !validLength(req.Message, maxMessageLength) {
		http.Error(w, "invalid name, email, or message", http.StatusBadRequest)
		return
	}

	// Honeypot: reject if the hidden field was filled (bot behavior)
	if req.Website != "" {
		writeSuccess(w)
		return
	}

	if h.RateLimiter != nil {
		if err := h.RateLimiter.Allow(r.Context(), requestmeta.ClientIP(r)); err != nil {
			if errors.Is(err, ErrRateLimited) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			log.Printf("contact rate limit failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	if h.EmailService != nil {
		err := h.EmailService.SendContactEmail(r.Context(), req.Name, req.Email, req.Message)
		if err != nil {
			log.Printf("SES send error: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	writeSuccess(w)
}

func writeSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	w.Write([]byte(`{"message":"message sent successfully"}`))
}

func validLength(value string, maxLength int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxLength
}

func validEmail(value string) bool {
	if value == "" || len(value) > maxEmailLength {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.SubmitContact)
	return r
}
