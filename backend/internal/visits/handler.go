package visits

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/jyates/jyatesdotdev-api/backend/internal/requestmeta"
)

// GeoResponse reflects the CloudFront-Viewer-* headers CloudFront adds at the
// edge (forwarded to the origin by the API cache policy in the infra repo).
type GeoResponse struct {
	Country     string `json:"country"`
	CountryName string `json:"countryName,omitempty"`
	City        string `json:"city,omitempty"`
	TimeZone    string `json:"timeZone,omitempty"`
	Latitude    string `json:"latitude,omitempty"`
	Longitude   string `json:"longitude,omitempty"`
}

type StatsResponse struct {
	Total     int64           `json:"total"`
	Countries []CountryVisits `json:"countries"`
	// You is the caller's own country code so the map can highlight it.
	You string `json:"you,omitempty"`
}

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

type Handler struct {
	Service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{Service: service}
}

func geoFromRequest(r *http.Request) GeoResponse {
	return GeoResponse{
		Country:     r.Header.Get("CloudFront-Viewer-Country"),
		CountryName: r.Header.Get("CloudFront-Viewer-Country-Name"),
		City:        r.Header.Get("CloudFront-Viewer-City"),
		TimeZone:    r.Header.Get("CloudFront-Viewer-Time-Zone"),
		Latitude:    r.Header.Get("CloudFront-Viewer-Latitude"),
		Longitude:   r.Header.Get("CloudFront-Viewer-Longitude"),
	}
}

// WhereAmI reflects the caller's own edge-resolved location back to them.
func (h *Handler) WhereAmI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(geoFromRequest(r))
}

// RecordVisit bumps the caller's country counter. Requests without a valid
// country header (e.g. local dev without CloudFront) are a silent no-op.
func (h *Handler) RecordVisit(w http.ResponseWriter, r *http.Request) {
	geo := geoFromRequest(r)
	if !countryCodePattern.MatchString(geo.Country) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err := h.Service.RecordVisit(r.Context(), geo.Country, geo.CountryName, requestmeta.ClientIP(r))
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		log.Printf("record visit failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Service.GetStats(r.Context())
	if err != nil {
		log.Printf("get visit stats failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	resp := StatsResponse{
		Total:     stats.Total,
		Countries: stats.Countries,
	}
	if geo := geoFromRequest(r); countryCodePattern.MatchString(geo.Country) {
		resp.You = geo.Country
	}

	w.Header().Set("Content-Type", "application/json")
	// #nosec G104 -- We are writing directly to the HTTP response writer; handling write errors here is generally unnecessary.
	json.NewEncoder(w).Encode(resp)
}

// GeoRoutes serves /geo.
func (h *Handler) GeoRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.WhereAmI)
	return r
}

// Routes serves /visits.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetStats)
	r.Post("/", h.RecordVisit)
	return r
}
