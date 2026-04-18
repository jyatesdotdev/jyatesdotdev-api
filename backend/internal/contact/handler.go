package contact

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jyates/jyatesdotdev-api/backend/internal/email"
	"github.com/jyates/jyatesdotdev-api/backend/internal/recaptcha"
)

type Request struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
	Token   string `json:"recaptchaToken"`
}

type Handler struct {
	EmailService email.Service
}

func NewHandler(emailService email.Service) *Handler {
	return &Handler{
		EmailService: emailService,
	}
}

func (h *Handler) SubmitContact(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Message = strings.TrimSpace(req.Message)

	if req.Name == "" || req.Email == "" || req.Message == "" {
		http.Error(w, "name, email, and message are required", http.StatusBadRequest)
		return
	}

	// Verify ReCAPTCHA
	valid, err := recaptcha.Verify(req.Token, "contact_form")
	if err != nil {
		http.Error(w, "recaptcha verification failed", http.StatusInternalServerError)
		return
	}
	if !valid {
		http.Error(w, "invalid recaptcha token", http.StatusForbidden)
		return
	}

	if h.EmailService != nil {
		err := h.EmailService.SendContactEmail(r.Context(), req.Name, req.Email, req.Message)
		if err != nil {
			log.Printf("SES send error: %v", err)
			http.Error(w, "failed to send email", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(map[string]string{
		"message": "message sent successfully",
	})
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.SubmitContact)
	return r
}
