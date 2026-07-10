package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockService struct {
	mock.Mock
}

func (m *MockService) RequestSubscription(ctx context.Context, email string, topics []string) error {
	args := m.Called(ctx, email, topics)
	return args.Error(0)
}

func (m *MockService) ConfirmSubscription(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

type MockRateLimiter struct {
	mock.Mock
}

func (m *MockRateLimiter) Allow(ctx context.Context, ipAddress string) error {
	args := m.Called(ctx, ipAddress)
	return args.Error(0)
}

func TestSubscribe_Success(t *testing.T) {
	service := new(MockService)
	service.On(
		"RequestSubscription",
		mock.Anything,
		"reader@example.com",
		[]string{TopicBlog, TopicProjects},
	).Return(nil)
	limiter := new(MockRateLimiter)
	limiter.On("Allow", mock.Anything, "198.51.100.8").Return(nil)
	handler := NewHandler(service, limiter)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscriptions",
		strings.NewReader(`{"email":" Reader@Example.com ","topics":["projects","blog","blog"]}`),
	)
	req.Header.Set("CloudFront-Viewer-Address", "198.51.100.8:443")
	w := httptest.NewRecorder()

	handler.Subscribe(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	service.AssertExpectations(t)
	limiter.AssertExpectations(t)
}

func TestSubscribe_Honeypot(t *testing.T) {
	service := new(MockService)
	limiter := new(MockRateLimiter)
	handler := NewHandler(service, limiter)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscriptions",
		strings.NewReader(`{"email":"bot@example.com","topics":["blog"],"website":"spam"}`),
	)
	w := httptest.NewRecorder()

	handler.Subscribe(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	service.AssertNotCalled(t, "RequestSubscription")
	limiter.AssertNotCalled(t, "Allow")
}

func TestSubscribe_InvalidTopics(t *testing.T) {
	handler := NewHandler(nil, nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscriptions",
		strings.NewReader(`{"email":"reader@example.com","topics":["security"]}`),
	)
	w := httptest.NewRecorder()

	handler.Subscribe(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubscribe_RateLimited(t *testing.T) {
	limiter := new(MockRateLimiter)
	limiter.On("Allow", mock.Anything, mock.Anything).Return(ErrRateLimited)
	handler := NewHandler(nil, limiter)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscriptions",
		strings.NewReader(`{"email":"reader@example.com","topics":["blog"]}`),
	)
	w := httptest.NewRecorder()

	handler.Subscribe(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestConfirm_Success(t *testing.T) {
	service := new(MockService)
	service.On("ConfirmSubscription", mock.Anything, "token").Return(nil)
	handler := NewHandler(service, nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscriptions/confirm",
		strings.NewReader(`{"token":"token"}`),
	)
	w := httptest.NewRecorder()

	handler.Confirm(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]string
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, "subscription confirmed", response["message"])
}

func TestConfirm_Expired(t *testing.T) {
	service := new(MockService)
	service.On("ConfirmSubscription", mock.Anything, "expired").Return(ErrInvalidToken)
	handler := NewHandler(service, nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscriptions/confirm",
		strings.NewReader(`{"token":"expired"}`),
	)
	w := httptest.NewRecorder()

	handler.Confirm(w, req)

	assert.Equal(t, http.StatusGone, w.Code)
}

func TestSubscribe_ServiceFailure(t *testing.T) {
	service := new(MockService)
	service.On("RequestSubscription", mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("email unavailable"))
	handler := NewHandler(service, nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscriptions",
		strings.NewReader(`{"email":"reader@example.com","topics":["blog"]}`),
	)
	w := httptest.NewRecorder()

	handler.Subscribe(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
