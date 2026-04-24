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

func (m *MockEmailService) SendAdminNotification(ctx context.Context, subject, body string) error {
	args := m.Called(ctx, subject, body)
	return args.Error(0)
}

func (m *MockEmailService) SendContactEmail(ctx context.Context, name, replyTo, message string) error {
	args := m.Called(ctx, name, replyTo, message)
	return args.Error(0)
}

func TestSubmitContact_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil)
	reqBody := `{"name": "test"` // invalid json
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitContact_MissingFields(t *testing.T) {
	handler := NewHandler(nil)
	reqBody := `{"name": "", "email": "", "message": ""}`
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitContact_HoneypotTriggered(t *testing.T) {
	handler := NewHandler(nil)
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

	handler := NewHandler(mockEmail)
	reqBody := `{"name": "John", "email": "john@example.com", "message": "Hello"}`
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.SubmitContact(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockEmail.AssertExpectations(t)
}

func TestRoutes(t *testing.T) {
	handler := NewHandler(nil)
	r := handler.Routes()
	assert.NotNil(t, r)
}
