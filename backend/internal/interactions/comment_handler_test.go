package interactions

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

func TestGetComments(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	slug := "test-post"
	visitorID := "visitor-abc-123"

	mockSvc.On("GetComments", mock.Anything, slug, visitorID).Return([]CommentResponse{
		{
			ID:           "123",
			Content:      "Hello world",
			AuthorName:   "John Doe",
			CreatedAt:    "2023-01-01T00:00:00Z",
			LikeCount:    2,
			UserHasLiked: true,
		},
	}, nil)

	req := httptest.NewRequest("GET", "/api/v1/comments?slug="+slug, nil)
	req.Header.Set("X-Visitor-Id", visitorID)
	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []CommentResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Len(t, resp, 1)
	assert.Equal(t, "123", resp[0].ID)
	assert.Equal(t, "Hello world", resp[0].Content)
	assert.Equal(t, "John Doe", resp[0].AuthorName)
	assert.Equal(t, 2, resp[0].LikeCount)
	assert.True(t, resp[0].UserHasLiked)

	mockSvc.AssertExpectations(t)
}

func TestGetComments_NoSlug(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/comments", nil)
	w := httptest.NewRecorder()

	handler.GetComments(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateComment_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil)
	reqBody := `{"slug": "test-post", "content": "hello"`
	req := httptest.NewRequest("POST", "/api/v1/comments", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.CreateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateComment_MissingFields(t *testing.T) {
	handler := NewHandler(nil)
	reqBody := `{"slug": "", "content": "", "authorName": ""}`
	req := httptest.NewRequest("POST", "/api/v1/comments", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.CreateComment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateComment_RecaptchaFailure(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("CreateComment", mock.Anything, mock.Anything, "192.0.2.1").Return("", ErrRecaptchaFailed)

	reqBody := `{"slug": "test", "content": "hello", "authorName": "John"}`
	req := httptest.NewRequest("POST", "/api/v1/comments", strings.NewReader(reqBody))
	req.RemoteAddr = "192.0.2.1"
	w := httptest.NewRecorder()

	handler.CreateComment(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestToggleCommentLike_MissingID(t *testing.T) {
	handler := NewHandler(nil)
	reqBody := `{"slug": "test-post"}`
	req := httptest.NewRequest("POST", "/like", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.ToggleCommentLike(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestToggleCommentLike_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil)
	reqBody := `{"slug": "test-post"`
	req := httptest.NewRequest("POST", "/123/like", strings.NewReader(reqBody))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	handler.ToggleCommentLike(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestToggleCommentLike_MissingVisitorId(t *testing.T) {
	handler := NewHandler(nil)
	reqBody := `{"slug": "test-post"}`
	req := httptest.NewRequest("POST", "/123/like", strings.NewReader(reqBody))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	handler.ToggleCommentLike(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "X-Visitor-Id")
}

func TestToggleCommentLike_Success(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	visitorID := "visitor-abc-123"

	mockSvc.On("ToggleCommentLike", mock.Anything, "test-post", "123", visitorID).Return(nil)

	reqBody := `{"slug": "test-post"}`
	req := httptest.NewRequest("POST", "/123/like", strings.NewReader(reqBody))
	req.Header.Set("X-Visitor-Id", visitorID)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()

	handler.ToggleCommentLike(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestCreateComment_Success(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("CreateComment", mock.Anything, mock.Anything, "127.0.0.1").Return("12345", nil)

	reqBody := `{"slug": "test-post", "content": "<b>Bold</b> comment <script>alert(1)</script>", "authorName": "John"}`
	req := httptest.NewRequest("POST", "/api/v1/comments", strings.NewReader(reqBody))
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	w := httptest.NewRecorder()

	handler.CreateComment(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestCommentRoutes(t *testing.T) {
	handler := NewHandler(nil)
	r := handler.CommentRoutes()
	assert.NotNil(t, r)
}
