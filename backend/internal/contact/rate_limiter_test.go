package contact

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

type updateDynamoMock struct {
	db.DynamoDBAPI
	input *dynamodb.UpdateItemInput
	err   error
}

func (m *updateDynamoMock) UpdateItem(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	m.input = input
	return &dynamodb.UpdateItemOutput{}, m.err
}

func TestRateLimiter_UsesDailyTTLItem(t *testing.T) {
	api := new(updateDynamoMock)
	limiter := NewRateLimiter(&db.Client{DynamoDBAPI: api, TableName: "test-table"})

	err := limiter.Allow(context.Background(), "192.0.2.1")

	assert.NoError(t, err)
	assert.Equal(t, "RATELIMIT#IP#192.0.2.1", api.input.Key["PK"].(*types.AttributeValueMemberS).Value)
	assert.True(t, strings.HasPrefix(api.input.Key["SK"].(*types.AttributeValueMemberS).Value, "CONTACT#"))
	assert.Equal(t, "5", api.input.ExpressionAttributeValues[":max"].(*types.AttributeValueMemberN).Value)
	assert.Contains(t, aws.ToString(api.input.ConditionExpression), "#count < :max")
}

func TestRateLimiter_MapsConditionalFailure(t *testing.T) {
	api := &updateDynamoMock{err: &types.ConditionalCheckFailedException{}}
	limiter := NewRateLimiter(&db.Client{DynamoDBAPI: api, TableName: "test-table"})

	err := limiter.Allow(context.Background(), "192.0.2.1")

	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestRateLimiter_PreservesUnexpectedError(t *testing.T) {
	want := errors.New("dynamodb unavailable")
	api := &updateDynamoMock{err: want}
	limiter := NewRateLimiter(&db.Client{DynamoDBAPI: api, TableName: "test-table"})

	err := limiter.Allow(context.Background(), "192.0.2.1")

	assert.ErrorIs(t, err, want)
}
