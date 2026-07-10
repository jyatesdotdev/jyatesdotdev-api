package contact

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockEmailService struct {
	mock.Mock
}

type MockRateLimiter struct {
	mock.Mock
}

func (m *MockRateLimiter) Allow(ctx context.Context, ipAddress string) error {
	args := m.Called(ctx, ipAddress)
	return args.Error(0)
}

func (m *MockEmailService) SendAdminNotification(ctx context.Context, subject, body string) error {
	args := m.Called(ctx, subject, body)
	return args.Error(0)
}

func (m *MockEmailService) SendContactEmail(ctx context.Context, name, replyTo, message string) error {
	args := m.Called(ctx, name, replyTo, message)
	return args.Error(0)
}

func TestSubmitContact_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil, nil)
	reqBody := `{"name": "test"` // invalid json
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitContact_MissingFields(t *testing.T) {
	handler := NewHandler(nil, nil)
	reqBody := `{"name": "", "email": "", "message": ""}`
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitContact_HoneypotTriggered(t *testing.T) {
	handler := NewHandler(nil, nil)
	reqBody := `{"name": "Bot", "email": "bot@spam.com", "message": "Buy stuff", "website": "http://spam.com"}`
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	// Returns 200 to not tip off the bot, but no email is sent
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "message sent successfully", resp["message"])
}

func TestSubmitContact_Success(t *testing.T) {
	mockEmail := new(MockEmailService)
	mockEmail.On("SendContactEmail", mock.Anything, "John", "john@example.com", "Hello").Return(nil)

	limiter := new(MockRateLimiter)
	limiter.On("Allow", mock.Anything, "198.51.100.2").Return(nil)
	handler := NewHandler(mockEmail, limiter)
	reqBody := `{"name": "John", "email": "john@example.com", "message": "Hello"}`
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	req.Header.Set("CloudFront-Viewer-Address", "198.51.100.2:12345")
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockEmail.AssertExpectations(t)
	limiter.AssertExpectations(t)
}

func TestRoutes(t *testing.T) {
	handler := NewHandler(nil, nil)
	r := handler.Routes()
	assert.NotNil(t, r)
}

func TestSubmitContact_InvalidEmail(t *testing.T) {
	handler := NewHandler(nil, nil)
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(`{"name":"John","email":"bad","message":"Hello"}`))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitContact_RateLimited(t *testing.T) {
	limiter := new(MockRateLimiter)
	limiter.On("Allow", mock.Anything, mock.Anything).Return(ErrRateLimited)
	handler := NewHandler(nil, limiter)
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(`{"name":"John","email":"john@example.com","message":"Hello"}`))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	limiter.AssertExpectations(t)
}
