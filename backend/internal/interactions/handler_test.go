package interactions

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

type MockService struct {
	mock.Mock
}

func (m *MockService) GetLikes(ctx context.Context, slug, visitorID string) (LikesResponse, error) {
	args := m.Called(ctx, slug, visitorID)
	return args.Get(0).(LikesResponse), args.Error(1)
}

func (m *MockService) ToggleLike(ctx context.Context, slug, visitorID string) (LikesResponse, error) {
	args := m.Called(ctx, slug, visitorID)
	return args.Get(0).(LikesResponse), args.Error(1)
}

func (m *MockService) GetComments(ctx context.Context, slug, visitorID string) ([]CommentResponse, error) {
	args := m.Called(ctx, slug, visitorID)
	return args.Get(0).([]CommentResponse), args.Error(1)
}

func (m *MockService) CreateComment(ctx context.Context, req CreateCommentRequest, ipAddress string) (string, error) {
	args := m.Called(ctx, req, ipAddress)
	return args.String(0), args.Error(1)
}

func (m *MockService) ToggleCommentLike(ctx context.Context, slug, commentID, visitorID string) error {
	args := m.Called(ctx, slug, commentID, visitorID)
	return args.Error(0)
}

func TestGetLikes(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	slug := "test-post"
	visitorID := "visitor-abc-123"

	mockSvc.On("GetLikes", mock.Anything, slug, visitorID).Return(LikesResponse{
		Slug:         slug,
		LikeCount:    5,
		UserHasLiked: false,
	}, nil)

	req := httptest.NewRequest("GET", "/api/v1/likes?slug="+slug, nil)
	req.Header.Set("X-Visitor-Id", visitorID)
	w := httptest.NewRecorder()

	handler.GetLikes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp LikesResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, slug, resp.Slug)
	assert.Equal(t, 5, resp.LikeCount)
	assert.False(t, resp.UserHasLiked)

	mockSvc.AssertExpectations(t)
}

func TestGetLikes_NoVisitorId(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("GetLikes", mock.Anything, "test-post", "").Return(LikesResponse{
		Slug:      "test-post",
		LikeCount: 5,
	}, nil)

	req := httptest.NewRequest("GET", "/api/v1/likes?slug=test-post", nil)
	w := httptest.NewRecorder()

	handler.GetLikes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestToggleLike_InvalidSlug(t *testing.T) {
	handler := NewHandler(nil)

	reqBody := `{"slug": ""}`
	req := httptest.NewRequest("POST", "/api/v1/likes", strings.NewReader(reqBody))
	req.Header.Set("X-Visitor-Id", "visitor-123")
	w := httptest.NewRecorder()

	handler.ToggleLike(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestToggleLike_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil)

	reqBody := `{"slug": "test-post"` // Invalid JSON
	req := httptest.NewRequest("POST", "/api/v1/likes", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.ToggleLike(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestToggleLike_MissingVisitorId(t *testing.T) {
	handler := NewHandler(nil)

	reqBody := `{"slug": "test-post"}`
	req := httptest.NewRequest("POST", "/api/v1/likes", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.ToggleLike(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "X-Visitor-Id")
}

func TestToggleLike_Success(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	visitorID := "visitor-abc-123"

	mockSvc.On("ToggleLike", mock.Anything, "test-post", visitorID).Return(LikesResponse{
		Slug:         "test-post",
		LikeCount:    6,
		UserHasLiked: true,
	}, nil)

	reqBody := `{"slug": "test-post"}`
	req := httptest.NewRequest("POST", "/api/v1/likes", strings.NewReader(reqBody))
	req.Header.Set("X-Visitor-Id", visitorID)
	w := httptest.NewRecorder()

	handler.ToggleLike(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp LikesResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "test-post", resp.Slug)
	assert.Equal(t, 6, resp.LikeCount)
	assert.True(t, resp.UserHasLiked)
	mockSvc.AssertExpectations(t)
}

func TestGetLikes_NoSlug(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/likes", nil)
	w := httptest.NewRecorder()
	handler.GetLikes(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetLikes_MetadataError(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("GetLikes", mock.Anything, "test", "visitor-123").Return(LikesResponse{}, assert.AnError)

	req := httptest.NewRequest("GET", "/api/v1/likes?slug=test", nil)
	req.Header.Set("X-Visitor-Id", "visitor-123")
	w := httptest.NewRecorder()
	handler.GetLikes(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRoutes(t *testing.T) {
	handler := NewHandler(nil)
	r := handler.Routes()
	assert.NotNil(t, r)
}
