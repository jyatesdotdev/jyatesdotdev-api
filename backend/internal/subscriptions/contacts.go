package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

const (
	TopicBlog     = "blog"
	TopicProjects = "projects"
)

var supportedTopics = []string{TopicBlog, TopicProjects}

type SESv2API interface {
	CreateContact(ctx context.Context, params *sesv2.CreateContactInput, optFns ...func(*sesv2.Options)) (*sesv2.CreateContactOutput, error)
	GetContact(ctx context.Context, params *sesv2.GetContactInput, optFns ...func(*sesv2.Options)) (*sesv2.GetContactOutput, error)
	UpdateContact(ctx context.Context, params *sesv2.UpdateContactInput, optFns ...func(*sesv2.Options)) (*sesv2.UpdateContactOutput, error)
	ListContacts(ctx context.Context, params *sesv2.ListContactsInput, optFns ...func(*sesv2.Options)) (*sesv2.ListContactsOutput, error)
}

type sesContactStore struct {
	api             SESv2API
	contactListName string
}

// NewContactStore uses DynamoDB with LocalStack because SESv2 contact lists are
// not available in the community image. Production always uses SESv2.
func NewContactStore(ctx context.Context, dbClient *db.Client) (ContactStore, error) {
	contactListName := os.Getenv("SES_CONTACT_LIST_NAME")
	if contactListName == "" {
		contactListName = "jyatesdotdev-updates"
	}
	if os.Getenv("SES_ENDPOINT") != "" {
		return &dynamoContactStore{db: dbClient}, nil
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &sesContactStore{
		api:             sesv2.NewFromConfig(cfg),
		contactListName: contactListName,
	}, nil
}

func topicPreferences(
	selected []string,
	existing []sestypes.TopicPreference,
) []sestypes.TopicPreference {
	selectedSet := make(map[string]bool, len(selected))
	for _, topic := range selected {
		selectedSet[topic] = true
	}
	existingStatuses := make(map[string]sestypes.SubscriptionStatus, len(existing))
	for _, preference := range existing {
		topic := aws.ToString(preference.TopicName)
		if topic == TopicBlog || topic == TopicProjects {
			existingStatuses[topic] = preference.SubscriptionStatus
		}
	}

	preferences := make([]sestypes.TopicPreference, 0, len(supportedTopics))
	for _, topic := range supportedTopics {
		status := sestypes.SubscriptionStatusOptOut
		if existingStatus, ok := existingStatuses[topic]; ok {
			status = existingStatus
		}
		if selectedSet[topic] {
			status = sestypes.SubscriptionStatusOptIn
		}
		preferences = append(preferences, sestypes.TopicPreference{
			TopicName:          aws.String(topic),
			SubscriptionStatus: status,
		})
	}
	return preferences
}

func (s *sesContactStore) UpsertContact(ctx context.Context, email string, topics []string) error {
	preferences := topicPreferences(topics, nil)
	_, err := s.api.CreateContact(ctx, &sesv2.CreateContactInput{
		ContactListName:  aws.String(s.contactListName),
		EmailAddress:     aws.String(email),
		TopicPreferences: preferences,
		UnsubscribeAll:   false,
	})
	if err == nil {
		return nil
	}

	var exists *sestypes.AlreadyExistsException
	if !errors.As(err, &exists) {
		return err
	}
	existing, err := s.api.GetContact(ctx, &sesv2.GetContactInput{
		ContactListName: aws.String(s.contactListName),
		EmailAddress:    aws.String(email),
	})
	if err != nil {
		return err
	}
	_, err = s.api.UpdateContact(ctx, &sesv2.UpdateContactInput{
		ContactListName:  aws.String(s.contactListName),
		EmailAddress:     aws.String(email),
		TopicPreferences: topicPreferences(topics, existing.TopicPreferences),
		UnsubscribeAll:   false,
	})
	return err
}

func (s *sesContactStore) ListContacts(
	ctx context.Context,
	topic, nextToken string,
) ([]string, string, error) {
	if topic != TopicBlog && topic != TopicProjects {
		return nil, "", fmt.Errorf("unsupported subscription topic: %s", topic)
	}
	input := &sesv2.ListContactsInput{
		ContactListName: aws.String(s.contactListName),
		Filter: &sestypes.ListContactsFilter{
			FilteredStatus: sestypes.SubscriptionStatusOptIn,
			TopicFilter: &sestypes.TopicFilter{
				TopicName:                         aws.String(topic),
				UseDefaultIfPreferenceUnavailable: false,
			},
		},
		PageSize: aws.Int32(100),
	}
	if nextToken != "" {
		input.NextToken = aws.String(nextToken)
	}

	output, err := s.api.ListContacts(ctx, input)
	if err != nil {
		return nil, "", err
	}
	emails := make([]string, 0, len(output.Contacts))
	for _, contact := range output.Contacts {
		if email := aws.ToString(contact.EmailAddress); email != "" {
			emails = append(emails, email)
		}
	}
	return emails, aws.ToString(output.NextToken), nil
}

type localContact struct {
	PK    string `dynamodbav:"PK"`
	SK    string `dynamodbav:"SK"`
	Email string `dynamodbav:"email"`
}

type dynamoContactStore struct {
	db *db.Client
}

func (s *dynamoContactStore) UpsertContact(
	ctx context.Context,
	email string,
	topics []string,
) error {
	for _, topic := range topics {
		item, err := attributevalue.MarshalMap(localContact{
			PK:    "LOCAL_SUBSCRIPTIONS#" + topic,
			SK:    "EMAIL#" + email,
			Email: email,
		})
		if err != nil {
			return err
		}
		if _, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(s.db.TableName),
			Item:      item,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *dynamoContactStore) ListContacts(
	ctx context.Context,
	topic, nextToken string,
) ([]string, string, error) {
	if topic != TopicBlog && topic != TopicProjects {
		return nil, "", fmt.Errorf("unsupported subscription topic: %s", topic)
	}
	if nextToken != "" {
		return nil, "", errors.New("local contact pagination token is invalid")
	}
	output, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.db.TableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":pk": &dynamodbtypes.AttributeValueMemberS{Value: "LOCAL_SUBSCRIPTIONS#" + topic},
		},
	})
	if err != nil {
		return nil, "", err
	}

	contacts := make([]localContact, 0, len(output.Items))
	if err := attributevalue.UnmarshalListOfMaps(output.Items, &contacts); err != nil {
		return nil, "", err
	}
	emails := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		emails = append(emails, contact.Email)
	}
	return emails, "", nil
}
