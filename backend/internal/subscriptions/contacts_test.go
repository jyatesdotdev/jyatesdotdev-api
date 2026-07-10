package subscriptions

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockSESv2API struct {
	mock.Mock
}

func (m *MockSESv2API) CreateContact(
	ctx context.Context,
	params *sesv2.CreateContactInput,
	_ ...func(*sesv2.Options),
) (*sesv2.CreateContactOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*sesv2.CreateContactOutput), args.Error(1)
}

func (m *MockSESv2API) GetContact(
	ctx context.Context,
	params *sesv2.GetContactInput,
	_ ...func(*sesv2.Options),
) (*sesv2.GetContactOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*sesv2.GetContactOutput), args.Error(1)
}

func (m *MockSESv2API) UpdateContact(
	ctx context.Context,
	params *sesv2.UpdateContactInput,
	_ ...func(*sesv2.Options),
) (*sesv2.UpdateContactOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*sesv2.UpdateContactOutput), args.Error(1)
}

func (m *MockSESv2API) ListContacts(
	ctx context.Context,
	params *sesv2.ListContactsInput,
	_ ...func(*sesv2.Options),
) (*sesv2.ListContactsOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*sesv2.ListContactsOutput), args.Error(1)
}

func TestUpsertContact_CreatesExplicitTopicPreferences(t *testing.T) {
	api := new(MockSESv2API)
	api.On("CreateContact", mock.Anything, mock.MatchedBy(func(input *sesv2.CreateContactInput) bool {
		return aws.ToString(input.ContactListName) == "updates" &&
			aws.ToString(input.EmailAddress) == "reader@example.com" &&
			len(input.TopicPreferences) == 2 &&
			input.TopicPreferences[0].SubscriptionStatus == sestypes.SubscriptionStatusOptIn &&
			input.TopicPreferences[1].SubscriptionStatus == sestypes.SubscriptionStatusOptOut
	})).Return(&sesv2.CreateContactOutput{}, nil)
	store := &sesContactStore{api: api, contactListName: "updates"}

	err := store.UpsertContact(context.Background(), "reader@example.com", []string{TopicBlog})

	require.NoError(t, err)
	api.AssertExpectations(t)
}

func TestUpsertContact_AddsTopicsWithoutRemovingExistingOptIns(t *testing.T) {
	api := new(MockSESv2API)
	api.On("CreateContact", mock.Anything, mock.Anything).
		Return(&sesv2.CreateContactOutput{}, &sestypes.AlreadyExistsException{})
	api.On("GetContact", mock.Anything, mock.Anything).Return(&sesv2.GetContactOutput{
		TopicPreferences: []sestypes.TopicPreference{
			{TopicName: aws.String(TopicBlog), SubscriptionStatus: sestypes.SubscriptionStatusOptIn},
			{TopicName: aws.String(TopicProjects), SubscriptionStatus: sestypes.SubscriptionStatusOptOut},
		},
	}, nil)
	api.On("UpdateContact", mock.Anything, mock.MatchedBy(func(input *sesv2.UpdateContactInput) bool {
		return !input.UnsubscribeAll &&
			input.TopicPreferences[0].SubscriptionStatus == sestypes.SubscriptionStatusOptIn &&
			input.TopicPreferences[1].SubscriptionStatus == sestypes.SubscriptionStatusOptIn
	})).Return(&sesv2.UpdateContactOutput{}, nil)
	store := &sesContactStore{api: api, contactListName: "updates"}

	err := store.UpsertContact(context.Background(), "reader@example.com", []string{TopicProjects})

	require.NoError(t, err)
	api.AssertExpectations(t)
}

func TestListContacts_FiltersExplicitOptIns(t *testing.T) {
	api := new(MockSESv2API)
	api.On("ListContacts", mock.Anything, mock.MatchedBy(func(input *sesv2.ListContactsInput) bool {
		return input.Filter.FilteredStatus == sestypes.SubscriptionStatusOptIn &&
			aws.ToString(input.Filter.TopicFilter.TopicName) == TopicBlog &&
			!input.Filter.TopicFilter.UseDefaultIfPreferenceUnavailable
	})).Return(&sesv2.ListContactsOutput{
		Contacts:  []sestypes.Contact{{EmailAddress: aws.String("reader@example.com")}},
		NextToken: aws.String("next"),
	}, nil)
	store := &sesContactStore{api: api, contactListName: "updates"}

	emails, nextToken, err := store.ListContacts(context.Background(), TopicBlog, "")

	require.NoError(t, err)
	assert.Equal(t, []string{"reader@example.com"}, emails)
	assert.Equal(t, "next", nextToken)
}
