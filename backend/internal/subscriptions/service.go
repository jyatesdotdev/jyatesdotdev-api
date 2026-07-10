package subscriptions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const confirmationLifetime = 48 * time.Hour

var ErrInvalidToken = errors.New("invalid or expired confirmation token")

type ContactStore interface {
	UpsertContact(ctx context.Context, email string, topics []string) error
	ListContacts(ctx context.Context, topic, nextToken string) ([]string, string, error)
}

type Mailer interface {
	SendSubscriptionConfirmation(ctx context.Context, to, confirmationURL string) error
}

type Service interface {
	RequestSubscription(ctx context.Context, email string, topics []string) error
	ConfirmSubscription(ctx context.Context, token string) error
}

type service struct {
	repository  RequestRepository
	contacts    ContactStore
	mailer      Mailer
	siteURL     string
	tokenReader io.Reader
	now         func() time.Time
}

func NewService(
	repository RequestRepository,
	contacts ContactStore,
	mailer Mailer,
	siteURL string,
) (Service, error) {
	base, err := url.Parse(strings.TrimRight(siteURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid subscription site URL: %q", siteURL)
	}
	return &service{
		repository:  repository,
		contacts:    contacts,
		mailer:      mailer,
		siteURL:     base.String(),
		tokenReader: rand.Reader,
		now:         time.Now,
	}, nil
}

func (s *service) RequestSubscription(ctx context.Context, email string, topics []string) error {
	rawToken := make([]byte, 32)
	if _, err := io.ReadFull(s.tokenReader, rawToken); err != nil {
		return fmt.Errorf("generate confirmation token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := hashToken(rawToken)

	if err := s.repository.Save(ctx, tokenHash, email, topics, s.now().UTC().Add(confirmationLifetime)); err != nil {
		return fmt.Errorf("save subscription request: %w", err)
	}

	confirmationURL := s.siteURL + "/subscribe/confirm?token=" + url.QueryEscape(token)
	if err := s.mailer.SendSubscriptionConfirmation(ctx, email, confirmationURL); err != nil {
		_ = s.repository.Delete(ctx, tokenHash)
		return fmt.Errorf("send subscription confirmation: %w", err)
	}
	return nil
}

func (s *service) ConfirmSubscription(ctx context.Context, token string) error {
	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(rawToken) != 32 {
		return ErrInvalidToken
	}
	tokenHash := hashToken(rawToken)

	now := s.now().UTC()
	request, err := s.repository.Consume(ctx, tokenHash, now)
	if err != nil {
		if errors.Is(err, ErrRequestNotFound) {
			return ErrInvalidToken
		}
		return fmt.Errorf("consume subscription request: %w", err)
	}

	if err := s.contacts.UpsertContact(ctx, request.Email, request.Topics); err != nil {
		// Restore a still-valid request after a downstream failure so the user can retry.
		if restoreErr := s.repository.Save(
			ctx,
			tokenHash,
			request.Email,
			request.Topics,
			time.Unix(request.ExpiresAt, 0).UTC(),
		); restoreErr != nil {
			return fmt.Errorf(
				"save subscriber preferences: %v; restore confirmation request: %w",
				err,
				restoreErr,
			)
		}
		return fmt.Errorf("save subscriber preferences: %w", err)
	}
	return nil
}

func hashToken(rawToken []byte) string {
	sum := sha256.Sum256(rawToken)
	return hex.EncodeToString(sum[:])
}
