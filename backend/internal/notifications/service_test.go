package notifications

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	validManifestID = "0123456789abcdef0123456789abcdef01234567"
	validObjectKey  = "notification-events/" + validManifestID + ".json"
	validManifest   = `{
  "version": 1,
  "id": "0123456789abcdef0123456789abcdef01234567",
  "events": [{
    "topic": "blog",
    "title": "A new post",
    "summary": "What changed and why.",
    "url": "https://jyates.dev/blog/a-new-post"
  }]
}`
)

type MockObjectStore struct {
	mock.Mock
}

func (m *MockObjectStore) Get(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, bucket, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return io.NopCloser(strings.NewReader(args.String(0))), args.Error(1)
}

type MockDeliveryRepository struct {
	mock.Mock
}

func (m *MockDeliveryRepository) ManifestComplete(
	ctx context.Context,
	manifestID string,
) (bool, error) {
	args := m.Called(ctx, manifestID)
	return args.Bool(0), args.Error(1)
}

func (m *MockDeliveryRepository) BeginRecipient(
	ctx context.Context,
	manifestID string,
	eventIndex int,
	email string,
) (RecipientClaim, error) {
	args := m.Called(ctx, manifestID, eventIndex, email)
	return args.Get(0).(RecipientClaim), args.Error(1)
}

func (m *MockDeliveryRepository) CompleteRecipient(
	ctx context.Context,
	claim RecipientClaim,
) error {
	return m.Called(ctx, claim).Error(0)
}

func (m *MockDeliveryRepository) ReleaseRecipient(
	ctx context.Context,
	claim RecipientClaim,
) error {
	return m.Called(ctx, claim).Error(0)
}

func (m *MockDeliveryRepository) CompleteManifest(
	ctx context.Context,
	manifestID string,
) error {
	return m.Called(ctx, manifestID).Error(0)
}

type MockContacts struct {
	mock.Mock
}

func (m *MockContacts) UpsertContact(ctx context.Context, email string, topics []string) error {
	args := m.Called(ctx, email, topics)
	return args.Error(0)
}

func (m *MockContacts) ListContacts(
	ctx context.Context,
	topic, nextToken string,
) ([]string, string, error) {
	args := m.Called(ctx, topic, nextToken)
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

type MockMailer struct {
	mock.Mock
}

func (m *MockMailer) SendUpdateNotification(
	ctx context.Context,
	to, topic, subject, body, contactListName string,
) error {
	args := m.Called(ctx, to, topic, subject, body, contactListName)
	return args.Error(0)
}

func startedClaim(email string) RecipientClaim {
	return RecipientClaim{
		ManifestID:    validManifestID,
		EventIndex:    0,
		RecipientHash: hashRecipient(email),
		AttemptID:     "attempt-1",
		State:         RecipientStarted,
	}
}

func configuredDelivery(t *testing.T) (*MockObjectStore, *MockDeliveryRepository) {
	t.Helper()
	objects := new(MockObjectStore)
	objects.On("Get", mock.Anything, "site", validObjectKey).Return(validManifest, nil)
	deliveries := new(MockDeliveryRepository)
	deliveries.On("ManifestComplete", mock.Anything, validManifestID).Return(false, nil)
	return objects, deliveries
}

func TestDeliver_CheckpointsEachRecipient(t *testing.T) {
	objects, deliveries := configuredDelivery(t)
	firstClaim := startedClaim("one@example.com")
	deliveries.On("BeginRecipient", mock.Anything, validManifestID, 0, "one@example.com").
		Return(firstClaim, nil)
	deliveries.On("CompleteRecipient", mock.Anything, firstClaim).Return(nil)
	deliveries.On("BeginRecipient", mock.Anything, validManifestID, 0, "two@example.com").
		Return(RecipientClaim{State: RecipientComplete}, nil)
	deliveries.On("CompleteManifest", mock.Anything, validManifestID).Return(nil)
	contacts := new(MockContacts)
	contacts.On("ListContacts", mock.Anything, "blog", "").
		Return([]string{"one@example.com", "two@example.com"}, "", nil)
	mailer := new(MockMailer)
	mailer.On(
		"SendUpdateNotification",
		mock.Anything,
		"one@example.com",
		"blog",
		"New blog post: A new post",
		mock.Anything,
		"updates",
	).Return(nil)
	service := NewService(objects, deliveries, contacts, mailer, "updates")

	result, err := service.Deliver(context.Background(), "site", validObjectKey)

	require.NoError(t, err)
	assert.Equal(t, DeliveryResult{Sent: 1, Skipped: 1}, result)
	deliveries.AssertExpectations(t)
	contacts.AssertExpectations(t)
	mailer.AssertExpectations(t)
}

func TestDeliver_SkipsACompletedManifest(t *testing.T) {
	objects := new(MockObjectStore)
	objects.On("Get", mock.Anything, "site", validObjectKey).Return(validManifest, nil)
	deliveries := new(MockDeliveryRepository)
	deliveries.On("ManifestComplete", mock.Anything, validManifestID).Return(true, nil)
	service := NewService(objects, deliveries, new(MockContacts), new(MockMailer), "updates")

	result, err := service.Deliver(context.Background(), "site", validObjectKey)

	require.NoError(t, err)
	assert.True(t, result.Duplicate)
}

func TestDeliver_ReleasesFailedRecipientForRetry(t *testing.T) {
	objects, deliveries := configuredDelivery(t)
	claim := startedClaim("one@example.com")
	deliveries.On("BeginRecipient", mock.Anything, validManifestID, 0, "one@example.com").
		Return(claim, nil)
	deliveries.On("ReleaseRecipient", mock.Anything, claim).Return(nil)
	secondClaim := startedClaim("two@example.com")
	deliveries.On("BeginRecipient", mock.Anything, validManifestID, 0, "two@example.com").
		Return(secondClaim, nil)
	deliveries.On("CompleteRecipient", mock.Anything, secondClaim).Return(nil)
	contacts := new(MockContacts)
	contacts.On("ListContacts", mock.Anything, "blog", "").
		Return([]string{"one@example.com", "two@example.com"}, "", nil)
	mailer := new(MockMailer)
	mailer.On("SendUpdateNotification", mock.Anything, "one@example.com", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("ses unavailable"))
	mailer.On("SendUpdateNotification", mock.Anything, "two@example.com", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	service := NewService(objects, deliveries, contacts, mailer, "updates")

	result, err := service.Deliver(context.Background(), "site", validObjectKey)

	assert.ErrorContains(t, err, "send notification")
	assert.Equal(t, 1, result.Failed)
	assert.Equal(t, 1, result.Sent)
	deliveries.AssertNotCalled(t, "CompleteManifest", mock.Anything, mock.Anything)
	deliveries.AssertExpectations(t)
}

func TestDeliver_RetriesWhileARecipientIsInProgress(t *testing.T) {
	objects, deliveries := configuredDelivery(t)
	deliveries.On("BeginRecipient", mock.Anything, validManifestID, 0, "one@example.com").
		Return(RecipientClaim{State: RecipientInProgress}, nil)
	contacts := new(MockContacts)
	contacts.On("ListContacts", mock.Anything, "blog", "").
		Return([]string{"one@example.com"}, "", nil)
	service := NewService(objects, deliveries, contacts, new(MockMailer), "updates")

	_, err := service.Deliver(context.Background(), "site", validObjectKey)

	assert.ErrorContains(t, err, "still processing")
	deliveries.AssertNotCalled(t, "CompleteManifest", mock.Anything, mock.Anything)
}

func TestDeliver_RejectsAMismatchedObjectKey(t *testing.T) {
	objects := new(MockObjectStore)
	objects.On("Get", mock.Anything, "site", "notification-events/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.json").
		Return(validManifest, nil)
	service := NewService(objects, new(MockDeliveryRepository), new(MockContacts), new(MockMailer), "updates")

	_, err := service.Deliver(
		context.Background(),
		"site",
		"notification-events/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.json",
	)

	assert.ErrorContains(t, err, "does not match")
}
