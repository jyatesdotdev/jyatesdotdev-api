package interactions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) GetPostMetadata(ctx context.Context, slug string) (*PostMetadata, error) {
	args := m.Called(ctx, slug)
	return args.Get(0).(*PostMetadata), args.Error(1)
}

func (m *MockRepository) CheckUserLike(ctx context.Context, slug, visitorID string) (bool, error) {
	args := m.Called(ctx, slug, visitorID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ToggleLike(ctx context.Context, slug, visitorID, ipAddress string) error {
	args := m.Called(ctx, slug, visitorID, ipAddress)
	return args.Error(0)
}

func (m *MockRepository) GetApprovedComments(ctx context.Context, slug string) ([]CommentItem, error) {
	args := m.Called(ctx, slug)
	return args.Get(0).([]CommentItem), args.Error(1)
}

func (m *MockRepository) GetUserLikedComments(ctx context.Context, slug, visitorID string) (map[string]bool, error) {
	args := m.Called(ctx, slug, visitorID)
	return args.Get(0).(map[string]bool), args.Error(1)
}

func (m *MockRepository) CreateComment(ctx context.Context, item CommentItem) error {
	args := m.Called(ctx, item)
	return args.Error(0)
}

func (m *MockRepository) ToggleCommentLike(ctx context.Context, slug, commentID, visitorID, ipAddress string) error {
	args := m.Called(ctx, slug, commentID, visitorID, ipAddress)
	return args.Error(0)
}

func TestCreateComment_AutoApproveDisabled(t *testing.T) {
	t.Setenv("AUTO_APPROVE", "false")

	mockRepo := new(MockRepository)
	svc := NewService(mockRepo, nil)

	var created CommentItem
	mockRepo.On("CreateComment", mock.Anything, mock.MatchedBy(func(item CommentItem) bool {
		created = item
		return true
	})).Return(nil)

	req := CreateCommentRequest{
		Slug:       "test-post",
		Content:    "Hello world",
		AuthorName: "John Doe",
	}

	commentID, err := svc.CreateComment(context.Background(), req, "192.0.2.1")

	assert.NoError(t, err)
	assert.NotEmpty(t, commentID)
	assert.Equal(t, "pending", created.Status)
	assert.Equal(t, "STATUS#pending", created.GSI1PK)
	mockRepo.AssertExpectations(t)
}
