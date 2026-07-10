package subscriptions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockRequestRepository struct {
	mock.Mock
}

func (m *MockRequestRepository) Save(
	ctx context.Context,
	tokenHash, email string,
	topics []string,
	expiresAt time.Time,
) error {
	args := m.Called(ctx, tokenHash, email, topics, expiresAt)
	return args.Error(0)
}

func (m *MockRequestRepository) Consume(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (PendingRequest, error) {
	args := m.Called(ctx, tokenHash, now)
	return args.Get(0).(PendingRequest), args.Error(1)
}

func (m *MockRequestRepository) Delete(ctx context.Context, tokenHash string) error {
	args := m.Called(ctx, tokenHash)
	return args.Error(0)
}

type MockContactStore struct {
	mock.Mock
}

func (m *MockContactStore) UpsertContact(ctx context.Context, email string, topics []string) error {
	args := m.Called(ctx, email, topics)
	return args.Error(0)
}

func (m *MockContactStore) ListContacts(
	ctx context.Context,
	topic, nextToken string,
) ([]string, string, error) {
	args := m.Called(ctx, topic, nextToken)
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

type MockMailer struct {
	mock.Mock
}

func (m *MockMailer) SendSubscriptionConfirmation(
	ctx context.Context,
	to, confirmationURL string,
) error {
	args := m.Called(ctx, to, confirmationURL)
	return args.Error(0)
}

func TestRequestSubscription_SavesHashedTokenAndSendsConfirmation(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	rawToken := bytes.Repeat([]byte{0x42}, 32)
	sum := sha256.Sum256(rawToken)
	tokenHash := hex.EncodeToString(sum[:])
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	repository := new(MockRequestRepository)
	repository.On(
		"Save",
		mock.Anything,
		tokenHash,
		"reader@example.com",
		[]string{TopicBlog},
		now.Add(confirmationLifetime),
	).Return(nil)
	mailer := new(MockMailer)
	mailer.On(
		"SendSubscriptionConfirmation",
		mock.Anything,
		"reader@example.com",
		"https://jyates.dev/subscribe/confirm?token="+token,
	).Return(nil)
	contacts := new(MockContactStore)
	configured, err := NewService(repository, contacts, mailer, "https://jyates.dev/")
	require.NoError(t, err)
	svc := configured.(*service)
	svc.tokenReader = bytes.NewReader(rawToken)
	svc.now = func() time.Time { return now }

	err = svc.RequestSubscription(context.Background(), "reader@example.com", []string{TopicBlog})

	require.NoError(t, err)
	repository.AssertExpectations(t)
	mailer.AssertExpectations(t)
}

func TestRequestSubscription_RemovesTokenWhenEmailFails(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x24}, 32)
	sum := sha256.Sum256(rawToken)
	tokenHash := hex.EncodeToString(sum[:])
	repository := new(MockRequestRepository)
	repository.On("Save", mock.Anything, tokenHash, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	repository.On("Delete", mock.Anything, tokenHash).Return(nil)
	mailer := new(MockMailer)
	mailer.On("SendSubscriptionConfirmation", mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("ses unavailable"))
	configured, err := NewService(repository, new(MockContactStore), mailer, "https://jyates.dev")
	require.NoError(t, err)
	svc := configured.(*service)
	svc.tokenReader = bytes.NewReader(rawToken)

	err = svc.RequestSubscription(context.Background(), "reader@example.com", []string{TopicBlog})

	assert.Error(t, err)
	repository.AssertCalled(t, "Delete", mock.Anything, tokenHash)
}

func TestConfirmSubscription_UpsertsPreferencesAndConsumesToken(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x11}, 32)
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := hashToken(rawToken)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repository := new(MockRequestRepository)
	repository.On("Consume", mock.Anything, tokenHash, now).Return(PendingRequest{
		Email:     "reader@example.com",
		Topics:    []string{TopicBlog, TopicProjects},
		ExpiresAt: now.Add(time.Hour).Unix(),
	}, nil)
	contacts := new(MockContactStore)
	contacts.On(
		"UpsertContact",
		mock.Anything,
		"reader@example.com",
		[]string{TopicBlog, TopicProjects},
	).Return(nil)
	configured, err := NewService(repository, contacts, new(MockMailer), "https://jyates.dev")
	require.NoError(t, err)
	svc := configured.(*service)
	svc.now = func() time.Time { return now }

	err = svc.ConfirmSubscription(context.Background(), token)

	require.NoError(t, err)
	contacts.AssertExpectations(t)
	repository.AssertExpectations(t)
}

func TestConfirmSubscription_RejectsExpiredToken(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x33}, 32)
	tokenHash := hashToken(rawToken)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repository := new(MockRequestRepository)
	repository.On("Consume", mock.Anything, tokenHash, now).
		Return(PendingRequest{}, ErrRequestNotFound)
	configured, err := NewService(repository, new(MockContactStore), new(MockMailer), "https://jyates.dev")
	require.NoError(t, err)
	svc := configured.(*service)
	svc.now = func() time.Time { return now }

	err = svc.ConfirmSubscription(context.Background(), base64.RawURLEncoding.EncodeToString(rawToken))

	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestConfirmSubscription_RestoresRequestWhenContactUpdateFails(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x44}, 32)
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := hashToken(rawToken)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	repository := new(MockRequestRepository)
	repository.On("Consume", mock.Anything, tokenHash, now).Return(PendingRequest{
		Email:     "reader@example.com",
		Topics:    []string{TopicBlog},
		ExpiresAt: expiresAt.Unix(),
	}, nil)
	repository.On(
		"Save",
		mock.Anything,
		tokenHash,
		"reader@example.com",
		[]string{TopicBlog},
		expiresAt,
	).Return(nil)
	contacts := new(MockContactStore)
	contacts.On("UpsertContact", mock.Anything, "reader@example.com", []string{TopicBlog}).
		Return(errors.New("ses unavailable"))
	configured, err := NewService(repository, contacts, new(MockMailer), "https://jyates.dev")
	require.NoError(t, err)
	svc := configured.(*service)
	svc.now = func() time.Time { return now }

	err = svc.ConfirmSubscription(context.Background(), token)

	assert.ErrorContains(t, err, "save subscriber preferences")
	repository.AssertExpectations(t)
}
