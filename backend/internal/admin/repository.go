package admin

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

type Repository interface {
	GetComments(ctx context.Context, status string) ([]CommentItem, error)
	GetComment(ctx context.Context, slug, commentID string) (*CommentItem, error)
	UpdateCommentStatus(ctx context.Context, slug, commentID, status, updatedAt string) error
	DeleteComment(ctx context.Context, slug, commentID string) error
}

type dynamoRepository struct {
	db *db.Client
}

func NewRepository(dbClient *db.Client) Repository {
	return &dynamoRepository{db: dbClient}
}

func (r *dynamoRepository) GetComments(ctx context.Context, status string) ([]CommentItem, error) {
	queryOutput, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.db.TableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :status"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: "STATUS#" + status},
		},
	})
	if err != nil {
		return nil, err
	}

	var items []CommentItem
	if err := attributevalue.UnmarshalListOfMaps(queryOutput.Items, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *dynamoRepository) GetComment(ctx context.Context, slug, commentID string) (*CommentItem, error) {
	getItemOut, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
		},
	})
	if err != nil {
		return nil, err
	}

	if getItemOut.Item == nil {
		return nil, nil
	}

	var item CommentItem
	if err := attributevalue.UnmarshalMap(getItemOut.Item, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *dynamoRepository) UpdateCommentStatus(ctx context.Context, slug, commentID, status, updatedAt string) error {
	update := expression.Set(expression.Name("status"), expression.Value(status)).
		Set(expression.Name("GSI1PK"), expression.Value("STATUS#"+status)).
		Set(expression.Name("updatedAt"), expression.Value(updatedAt))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return err
	}

	_, err = r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})

	return err
}

func (r *dynamoRepository) DeleteComment(ctx context.Context, slug, commentID string) error {
	_, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
		},
	})
	return err
}
