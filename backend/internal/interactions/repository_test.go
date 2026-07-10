package interactions

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

type repositoryDynamoMock struct {
	db.DynamoDBAPI
	mock.Mock
}

func (m *repositoryDynamoMock) GetItem(ctx context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	args := m.Called(ctx, input)
	output, _ := args.Get(0).(*dynamodb.GetItemOutput)
	return output, args.Error(1)
}

func (m *repositoryDynamoMock) TransactWriteItems(ctx context.Context, input *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	args := m.Called(ctx, input)
	output, _ := args.Get(0).(*dynamodb.TransactWriteItemsOutput)
	return output, args.Error(1)
}

func newTestRepository(api db.DynamoDBAPI) Repository {
	return NewRepository(&db.Client{DynamoDBAPI: api, TableName: "test-table"})
}

func TestToggleLike_AddUsesConditionalTransaction(t *testing.T) {
	api := new(repositoryDynamoMock)
	api.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil)
	api.On("TransactWriteItems", mock.Anything, mock.MatchedBy(func(input *dynamodb.TransactWriteItemsInput) bool {
		if len(input.TransactItems) != 3 {
			return false
		}
		marker := input.TransactItems[0].Put
		metadata := input.TransactItems[1].Update
		limiter := input.TransactItems[2].Update
		return aws.ToString(marker.ConditionExpression) == "attribute_not_exists(PK)" &&
			strings.Contains(aws.ToString(metadata.ConditionExpression), "attribute_exists(PK)") &&
			strings.HasPrefix(limiter.Key["SK"].(*types.AttributeValueMemberS).Value, "LIKES#")
	})).Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	err := newTestRepository(api).ToggleLike(context.Background(), "test-post", "visitor-1", "192.0.2.1")

	assert.NoError(t, err)
	api.AssertExpectations(t)
}

func TestCreateComment_UsesAtomicRateLimit(t *testing.T) {
	api := new(repositoryDynamoMock)
	api.On("TransactWriteItems", mock.Anything, mock.MatchedBy(func(input *dynamodb.TransactWriteItemsInput) bool {
		if len(input.TransactItems) != 2 {
			return false
		}
		comment := input.TransactItems[0].Put
		limiter := input.TransactItems[1].Update
		return aws.ToString(comment.ConditionExpression) == "attribute_not_exists(PK)" &&
			strings.HasPrefix(limiter.Key["SK"].(*types.AttributeValueMemberS).Value, "COMMENTS#")
	})).Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	err := newTestRepository(api).CreateComment(context.Background(), CommentItem{
		PK: "POST#test-post", SK: "COMMENT#comment-1",
	}, "192.0.2.1")

	assert.NoError(t, err)
	api.AssertExpectations(t)
}

func TestToggleCommentLike_AddRequiresApprovedComment(t *testing.T) {
	api := new(repositoryDynamoMock)
	api.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil)
	api.On("TransactWriteItems", mock.Anything, mock.MatchedBy(func(input *dynamodb.TransactWriteItemsInput) bool {
		if len(input.TransactItems) != 4 {
			return false
		}
		return strings.Contains(aws.ToString(input.TransactItems[2].Update.ConditionExpression), "#status = :approved")
	})).Return(&dynamodb.TransactWriteItemsOutput{}, nil)

	err := newTestRepository(api).ToggleCommentLike(context.Background(), "test-post", "comment-1", "visitor-1", "192.0.2.1")

	assert.NoError(t, err)
	api.AssertExpectations(t)
}

func TestMapTransactionError(t *testing.T) {
	conditional := func(index, length int) error {
		reasons := make([]types.CancellationReason, length)
		reasons[index].Code = aws.String("ConditionalCheckFailed")
		return &types.TransactionCanceledException{CancellationReasons: reasons}
	}

	assert.ErrorIs(t, mapTransactionError(conditional(2, 3), 2, 0, 1), ErrRateLimited)
	assert.ErrorIs(t, mapTransactionError(conditional(0, 3), 2, 0, 1), ErrConflict)
	assert.ErrorIs(t, mapTransactionError(conditional(1, 2), 1), ErrRateLimited)
	assert.NoError(t, mapTransactionError(nil, -1, 0))
}
