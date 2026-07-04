package visits

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

type MockDynamo struct {
	mock.Mock
	db.DynamoDBAPI
}

func (m *MockDynamo) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	args := m.Called(ctx, params)
	out, _ := args.Get(0).(*dynamodb.TransactWriteItemsOutput)
	return out, args.Error(1)
}

func (m *MockDynamo) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	args := m.Called(ctx, params)
	out, _ := args.Get(0).(*dynamodb.QueryOutput)
	return out, args.Error(1)
}

func newTestRepo(mockDynamo *MockDynamo) Repository {
	return NewRepository(&db.Client{DynamoDBAPI: mockDynamo, TableName: "test-table"})
}

func TestIncrementCountryVisit_WritesCounterAndRateLimit(t *testing.T) {
	mockDynamo := new(MockDynamo)
	repo := newTestRepo(mockDynamo)

	mockDynamo.On("TransactWriteItems", mock.Anything, mock.MatchedBy(func(input *dynamodb.TransactWriteItemsInput) bool {
		if len(input.TransactItems) != 2 {
			return false
		}
		counter := input.TransactItems[0].Update
		limiter := input.TransactItems[1].Update
		return counter.Key["PK"].(*types.AttributeValueMemberS).Value == "STATS#GEO" &&
			counter.Key["SK"].(*types.AttributeValueMemberS).Value == "COUNTRY#US" &&
			limiter.Key["PK"].(*types.AttributeValueMemberS).Value == "RATELIMIT#IP#1.2.3.4"
	})).Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	err := repo.IncrementCountryVisit(context.Background(), "US", "United States", "1.2.3.4")

	assert.NoError(t, err)
	mockDynamo.AssertExpectations(t)
}

func TestIncrementCountryVisit_MapsRateLimit(t *testing.T) {
	mockDynamo := new(MockDynamo)
	repo := newTestRepo(mockDynamo)

	tce := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("None")},
			{Code: aws.String("ConditionalCheckFailed")},
		},
	}
	mockDynamo.On("TransactWriteItems", mock.Anything, mock.Anything).Return(nil, tce)

	err := repo.IncrementCountryVisit(context.Background(), "US", "United States", "1.2.3.4")

	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestIncrementCountryVisit_PassesThroughOtherErrors(t *testing.T) {
	mockDynamo := new(MockDynamo)
	repo := newTestRepo(mockDynamo)

	boom := errors.New("boom")
	mockDynamo.On("TransactWriteItems", mock.Anything, mock.Anything).Return(nil, boom)

	err := repo.IncrementCountryVisit(context.Background(), "US", "United States", "1.2.3.4")

	assert.ErrorIs(t, err, boom)
}

func TestGetCountryVisits(t *testing.T) {
	mockDynamo := new(MockDynamo)
	repo := newTestRepo(mockDynamo)

	mockDynamo.On("Query", mock.Anything, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
		return input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value == "STATS#GEO"
	})).Return(&dynamodb.QueryOutput{
		Items: []map[string]types.AttributeValue{
			{
				"SK":          &types.AttributeValueMemberS{Value: "COUNTRY#US"},
				"countryName": &types.AttributeValueMemberS{Value: "United States"},
				"count":       &types.AttributeValueMemberN{Value: "7"},
			},
		},
	}, nil)

	visits, err := repo.GetCountryVisits(context.Background())

	assert.NoError(t, err)
	assert.Len(t, visits, 1)
	assert.Equal(t, CountryVisits{Country: "US", CountryName: "United States", Count: 7}, visits[0])
}
