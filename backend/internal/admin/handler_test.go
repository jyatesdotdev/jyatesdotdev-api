package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockService struct {
	mock.Mock
}

func (m *MockService) GetComments(ctx context.Context, status string) ([]CommentResponse, error) {
	args := m.Called(ctx, status)
	return args.Get(0).([]CommentResponse), args.Error(1)
}

func (m *MockService) UpdateCommentStatus(ctx context.Context, slug, commentID, status string) error {
	args := m.Called(ctx, slug, commentID, status)
	return args.Error(0)
}

func (m *MockService) DeleteComment(ctx context.Context, slug, commentID string) error {
	args := m.Called(ctx, slug, commentID)
	return args.Error(0)
}

func TestGetPendingComments(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("GetComments", mock.Anything, "pending").Return([]CommentResponse{
		{
			ID:      "123",
			Slug:    "test",
			Content: "Pending comment",
			Status:  "pending",
		},
	}, nil)

	req := httptest.NewRequest("GET", "/api/v1/admin/comments", nil)
	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []CommentResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Len(t, resp, 1)
	assert.Equal(t, "123", resp[0].ID)
	assert.Equal(t, "test", resp[0].Slug)

	mockSvc.AssertExpectations(t)
}

func TestUpdateCommentStatus(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("UpdateCommentStatus", mock.Anything, "test", "123", "approved").Return(nil)

	reqBody := `{"slug": "test", "status": "approved"}`
	req := httptest.NewRequest("PUT", "/comments/123", strings.NewReader(reqBody))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	handler.UpdateCommentStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDeleteComment(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("DeleteComment", mock.Anything, "test", "123").Return(nil)

	reqBody := `{"slug": "test"}`
	req := httptest.NewRequest("DELETE", "/comments/123", strings.NewReader(reqBody))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	handler.DeleteComment(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestUpdateCommentStatus_MissingCommentId(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("PUT", "/comments/", strings.NewReader(`{"slug":"test","status":"approved"}`))
	w := httptest.NewRecorder()
	handler.UpdateCommentStatus(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateCommentStatus_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("PUT", "/comments/123", strings.NewReader(`{"slug":"test"`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	handler.UpdateCommentStatus(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateCommentStatus_MissingFields(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("PUT", "/comments/123", strings.NewReader(`{"slug":"","status":""}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	handler.UpdateCommentStatus(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateCommentStatus_InvalidStatus(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("UpdateCommentStatus", mock.Anything, "test", "123", "weird").Return(ErrInvalidStatus)

	req := httptest.NewRequest("PUT", "/comments/123", strings.NewReader(`{"slug":"test","status":"weird"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	handler.UpdateCommentStatus(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateCommentStatus_NotFound(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("UpdateCommentStatus", mock.Anything, "test", "123", "approved").Return(ErrCommentNotFound)

	reqBody := `{"slug": "test", "status": "approved"}`
	req := httptest.NewRequest("PUT", "/comments/123", strings.NewReader(reqBody))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.UpdateCommentStatus(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteComment_MissingCommentId(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("DELETE", "/comments/", strings.NewReader(`{"slug":"test"}`))
	w := httptest.NewRecorder()
	handler.DeleteComment(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteComment_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("DELETE", "/comments/123", strings.NewReader(`{"slug":"test"`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	handler.DeleteComment(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteComment_MissingSlug(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("DELETE", "/comments/123", strings.NewReader(`{"slug":""}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("commentId", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	handler.DeleteComment(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRoutes(t *testing.T) {
	handler := NewHandler(nil)
	r := handler.Routes()
	assert.NotNil(t, r)
}
