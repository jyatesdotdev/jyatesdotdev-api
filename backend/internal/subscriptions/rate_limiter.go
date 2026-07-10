package subscriptions

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

var ErrRateLimited = errors.New("rate limit exceeded")

const subscriptionRateLimitMax = 5

type RateLimiter interface {
	Allow(ctx context.Context, ipAddress string) error
}

type dynamoRateLimiter struct {
	db *db.Client
}

func NewRateLimiter(dbClient *db.Client) RateLimiter {
	return &dynamoRateLimiter{db: dbClient}
}

func (l *dynamoRateLimiter) Allow(ctx context.Context, ipAddress string) error {
	now := time.Now().UTC()
	_, err := l.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(l.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "RATELIMIT#IP#" + ipAddress},
			"SK": &types.AttributeValueMemberS{Value: "SUBSCRIPTIONS#" + now.Format("2006-01-02")},
		},
		UpdateExpression:    aws.String("ADD #count :one SET expiresAt = if_not_exists(expiresAt, :expires)"),
		ConditionExpression: aws.String("attribute_not_exists(#count) OR #count < :max"),
		ExpressionAttributeNames: map[string]string{
			"#count": "count",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one":     &types.AttributeValueMemberN{Value: "1"},
			":max":     &types.AttributeValueMemberN{Value: strconv.Itoa(subscriptionRateLimitMax)},
			":expires": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(48*time.Hour).Unix(), 10)},
		},
	})
	if err == nil {
		return nil
	}

	var conditionalErr *types.ConditionalCheckFailedException
	if errors.As(err, &conditionalErr) {
		return ErrRateLimited
	}
	return err
}
