package interactions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"

	"github.com/jyates/jyatesdotdev-api/backend/internal/email"
	"github.com/jyates/jyatesdotdev-api/backend/internal/recaptcha"
)

var (
	ErrInvalidRecaptcha = errors.New("invalid recaptcha token")
	ErrRecaptchaFailed  = errors.New("recaptcha verification failed")
	ErrInvalidInput     = errors.New("invalid input after sanitization")
)

type Service interface {
	GetLikes(ctx context.Context, slug, ipAddress string) (LikesResponse, error)
	ToggleLike(ctx context.Context, slug, ipAddress, token string) (LikesResponse, error)

	GetComments(ctx context.Context, slug, ipAddress string) ([]CommentResponse, error)
	CreateComment(ctx context.Context, req CreateCommentRequest, ipAddress string) (string, error)
	ToggleCommentLike(ctx context.Context, slug, commentID, ipAddress, token string) error
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

func (s *service) GetLikes(ctx context.Context, slug, ipAddress string) (LikesResponse, error) {
	metadata, err := s.repo.GetPostMetadata(ctx, slug)
	if err != nil {
		return LikesResponse{}, err
	}

	userHasLiked, err := s.repo.CheckUserLike(ctx, slug, ipAddress)
	if err != nil {
		return LikesResponse{}, err
	}

	return LikesResponse{
		Slug:         slug,
		LikeCount:    metadata.LikeCount,
		UserHasLiked: userHasLiked,
	}, nil
}

func (s *service) ToggleLike(ctx context.Context, slug, ipAddress, token string) (LikesResponse, error) {
	valid, err := recaptcha.Verify(token, "like")
	if err != nil {
		return LikesResponse{}, ErrRecaptchaFailed
	}
	if !valid {
		return LikesResponse{}, ErrInvalidRecaptcha
	}

	if err := s.repo.ToggleLike(ctx, slug, ipAddress); err != nil {
		return LikesResponse{}, err
	}

	// Fetch new count
	return s.GetLikes(ctx, slug, ipAddress)
}

func (s *service) GetComments(ctx context.Context, slug, ipAddress string) ([]CommentResponse, error) {
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
		likedCommentIDs, err := s.repo.GetUserLikedComments(ctx, slug, ipAddress)
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
	valid, err := recaptcha.Verify(req.Token, "comment")
	if err != nil {
		return "", ErrRecaptchaFailed
	}
	if !valid {
		return "", ErrInvalidRecaptcha
	}

	p := bluemonday.StrictPolicy()
	sanitizedContent := p.Sanitize(req.Content)
	sanitizedAuthorName := p.Sanitize(req.AuthorName)

	if sanitizedContent == "" || sanitizedAuthorName == "" {
		return "", ErrInvalidInput
	}

	now := time.Now().UTC().Format(time.RFC3339)
	commentID := uuid.New().String()
	status := "pending"

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
			body := "A new comment was submitted by " + sanitizedAuthorName + ".\n\nContent:\n" + sanitizedContent + "\n\nPlease review it in the admin dashboard."
			_ = s.emailService.SendAdminNotification(emailCtx, subject, body)
		}()
	}

	return commentID, nil
}

func (s *service) ToggleCommentLike(ctx context.Context, slug, commentID, ipAddress, token string) error {
	valid, err := recaptcha.Verify(token, "like_comment")
	if err != nil {
		return ErrRecaptchaFailed
	}
	if !valid {
		return ErrInvalidRecaptcha
	}

	return s.repo.ToggleCommentLike(ctx, slug, commentID, ipAddress)
}
