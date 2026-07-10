package subscriptions

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

var ErrRequestNotFound = errors.New("subscription request not found")

type PendingRequest struct {
	PK        string   `dynamodbav:"PK"`
	SK        string   `dynamodbav:"SK"`
	Email     string   `dynamodbav:"email"`
	Topics    []string `dynamodbav:"topics"`
	CreatedAt string   `dynamodbav:"createdAt"`
	ExpiresAt int64    `dynamodbav:"expiresAt"`
}

type RequestRepository interface {
	Save(ctx context.Context, tokenHash, email string, topics []string, expiresAt time.Time) error
	Consume(ctx context.Context, tokenHash string, now time.Time) (PendingRequest, error)
	Delete(ctx context.Context, tokenHash string) error
}

type dynamoRequestRepository struct {
	db *db.Client
}

func NewRequestRepository(dbClient *db.Client) RequestRepository {
	return &dynamoRequestRepository{db: dbClient}
}

func requestKey(tokenHash string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "SUBSCRIPTION_REQUEST#" + tokenHash},
		"SK": &types.AttributeValueMemberS{Value: "METADATA"},
	}
}

func (r *dynamoRequestRepository) Save(
	ctx context.Context,
	tokenHash, email string,
	topics []string,
	expiresAt time.Time,
) error {
	item, err := attributevalue.MarshalMap(PendingRequest{
		PK:        "SUBSCRIPTION_REQUEST#" + tokenHash,
		SK:        "METADATA",
		Email:     email,
		Topics:    topics,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		return err
	}

	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.db.TableName),
		Item:      item,
	})
	return err
}

func (r *dynamoRequestRepository) Consume(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (PendingRequest, error) {
	output, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(r.db.TableName),
		Key:                 requestKey(tokenHash),
		ConditionExpression: aws.String("attribute_exists(PK) AND expiresAt > :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Unix(), 10)},
		},
		ReturnValues: types.ReturnValueAllOld,
	})
	if err != nil {
		var conditionalErr *types.ConditionalCheckFailedException
		if errors.As(err, &conditionalErr) {
			return PendingRequest{}, ErrRequestNotFound
		}
		return PendingRequest{}, err
	}
	if len(output.Attributes) == 0 {
		return PendingRequest{}, ErrRequestNotFound
	}

	var request PendingRequest
	if err := attributevalue.UnmarshalMap(output.Attributes, &request); err != nil {
		return PendingRequest{}, err
	}
	return request, nil
}

func (r *dynamoRequestRepository) Delete(ctx context.Context, tokenHash string) error {
	_, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.db.TableName),
		Key:       requestKey(tokenHash),
	})
	return err
}
