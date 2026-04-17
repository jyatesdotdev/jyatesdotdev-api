package admin

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrCommentNotFound = errors.New("comment not found")
	ErrInvalidStatus   = errors.New("invalid status")
)

type Service interface {
	GetComments(ctx context.Context, status string) ([]CommentResponse, error)
	UpdateCommentStatus(ctx context.Context, slug, commentID, status string) error
	DeleteComment(ctx context.Context, slug, commentID string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) GetComments(ctx context.Context, status string) ([]CommentResponse, error) {
	items, err := s.repo.GetComments(ctx, status)
	if err != nil {
		return nil, err
	}

	responses := make([]CommentResponse, 0, len(items))
	for _, item := range items {
		slug := strings.TrimPrefix(item.PK, "POST#")
		id := strings.TrimPrefix(item.SK, "COMMENT#")
		responses = append(responses, CommentResponse{
			ID:          id,
			Slug:        slug,
			Content:     item.Content,
			AuthorName:  item.AuthorName,
			AuthorEmail: item.AuthorEmail,
			IPAddress:   item.IPAddress,
			Status:      item.Status,
			CreatedAt:   item.CreatedAt,
		})
	}
	return responses, nil
}

func (s *service) UpdateCommentStatus(ctx context.Context, slug, commentID, status string) error {
	if status != "approved" && status != "pending" && status != "rejected" {
		return ErrInvalidStatus
	}

	comment, err := s.repo.GetComment(ctx, slug, commentID)
	if err != nil {
		return err
	}
	if comment == nil {
		return ErrCommentNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return s.repo.UpdateCommentStatus(ctx, slug, commentID, status, now)
}

func (s *service) DeleteComment(ctx context.Context, slug, commentID string) error {
	return s.repo.DeleteComment(ctx, slug, commentID)
}
