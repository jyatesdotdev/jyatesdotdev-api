package interactions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"

	"github.com/jyates/jyatesdotdev-api/backend/internal/email"
)

var (
	ErrInvalidInput = errors.New("invalid input after sanitization")
	ErrHoneypot     = errors.New("honeypot field was filled")
)

type Service interface {
	GetLikes(ctx context.Context, slug, visitorID string) (LikesResponse, error)
	ToggleLike(ctx context.Context, slug, visitorID string) (LikesResponse, error)

	GetComments(ctx context.Context, slug, visitorID string) ([]CommentResponse, error)
	CreateComment(ctx context.Context, req CreateCommentRequest, ipAddress string) (string, error)
	ToggleCommentLike(ctx context.Context, slug, commentID, visitorID string) error
}

type service struct {
	repo         Repository
	emailService email.Service
}

func NewService(repo Repository, emailService email.Service) Service {
	return &service{
		repo:         repo,
		emailService: emailService,
	}
}

func (s *service) GetLikes(ctx context.Context, slug, visitorID string) (LikesResponse, error) {
	metadata, err := s.repo.GetPostMetadata(ctx, slug)
	if err != nil {
		return LikesResponse{}, err
	}

	userHasLiked, err := s.repo.CheckUserLike(ctx, slug, visitorID)
	if err != nil {
		return LikesResponse{}, err
	}

	return LikesResponse{
		Slug:         slug,
		LikeCount:    metadata.LikeCount,
		UserHasLiked: userHasLiked,
	}, nil
}

func (s *service) ToggleLike(ctx context.Context, slug, visitorID string) (LikesResponse, error) {
	if err := s.repo.ToggleLike(ctx, slug, visitorID); err != nil {
		return LikesResponse{}, err
	}

	return s.GetLikes(ctx, slug, visitorID)
}

func (s *service) GetComments(ctx context.Context, slug, visitorID string) ([]CommentResponse, error) {
	items, err := s.repo.GetApprovedComments(ctx, slug)
	if err != nil {
		return nil, err
	}

	responses := make([]CommentResponse, 0, len(items))
	for _, item := range items {
		id := strings.TrimPrefix(item.SK, "COMMENT#")
		responses = append(responses, CommentResponse{
			ID:         id,
			Content:    item.Content,
			AuthorName: item.AuthorName,
			CreatedAt:  item.CreatedAt,
			LikeCount:  item.LikeCount,
		})
	}

	if len(responses) > 0 {
		likedCommentIDs, err := s.repo.GetUserLikedComments(ctx, slug, visitorID)
		if err == nil {
			for i := range responses {
				if likedCommentIDs[responses[i].ID] {
					responses[i].UserHasLiked = true
				}
			}
		}
	}

	return responses, nil
}

func (s *service) CreateComment(ctx context.Context, req CreateCommentRequest, ipAddress string) (string, error) {
	// Honeypot: reject if the hidden field was filled (bot behavior)
	if req.Website != "" {
		return "", ErrHoneypot
	}

	p := bluemonday.StrictPolicy()
	sanitizedContent := p.Sanitize(req.Content)
	sanitizedAuthorName := p.Sanitize(req.AuthorName)

	if sanitizedContent == "" || sanitizedAuthorName == "" {
		return "", ErrInvalidInput
	}

	now := time.Now().UTC().Format(time.RFC3339)
	commentID := uuid.New().String()
	status := "approved"

	item := CommentItem{
		PK:          "POST#" + req.Slug,
		SK:          "COMMENT#" + commentID,
		GSI1PK:      "STATUS#" + status,
		GSI1SK:      "POST#" + req.Slug + "#" + now,
		Content:     sanitizedContent,
		AuthorName:  sanitizedAuthorName,
		AuthorEmail: req.AuthorEmail,
		IPAddress:   ipAddress,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
		LikeCount:   0,
	}

	if err := s.repo.CreateComment(ctx, item); err != nil {
		return "", err
	}

	if s.emailService != nil {
		// #nosec G118
		go func() {
			emailCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			subject := "New Comment on " + req.Slug
			body := "A new comment was submitted by " + sanitizedAuthorName + ".\n\nContent:\n" + sanitizedContent + "\n\nIt has been auto-approved. Use the admin dashboard to moderate if needed."
			_ = s.emailService.SendAdminNotification(emailCtx, subject, body)
		}()
	}

	return commentID, nil
}

func (s *service) ToggleCommentLike(ctx context.Context, slug, commentID, visitorID string) error {
	return s.repo.ToggleCommentLike(ctx, slug, commentID, visitorID)
}
