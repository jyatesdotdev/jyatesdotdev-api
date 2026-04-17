package interactions

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

type Repository interface {
	GetPostMetadata(ctx context.Context, slug string) (*PostMetadata, error)
	CheckUserLike(ctx context.Context, slug, ipAddress string) (bool, error)
	ToggleLike(ctx context.Context, slug, ipAddress string) error

	GetApprovedComments(ctx context.Context, slug string) ([]CommentItem, error)
	GetUserLikedComments(ctx context.Context, slug, ipAddress string) (map[string]bool, error)
	CreateComment(ctx context.Context, item CommentItem) error

	ToggleCommentLike(ctx context.Context, slug, commentID, ipAddress string) error
}

type dynamoRepository struct {
	db *db.Client
}

func NewRepository(dbClient *db.Client) Repository {
	return &dynamoRepository{db: dbClient}
}

func (r *dynamoRepository) GetPostMetadata(ctx context.Context, slug string) (*PostMetadata, error) {
	metadataOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		return nil, err
	}

	var metadata PostMetadata
	if metadataOutput.Item != nil {
		if err := attributevalue.UnmarshalMap(metadataOutput.Item, &metadata); err != nil {
			return nil, err
		}
		return &metadata, nil
	}
	return &metadata, nil
}

func (r *dynamoRepository) CheckUserLike(ctx context.Context, slug, ipAddress string) (bool, error) {
	if ipAddress == "" {
		return false, nil
	}
	likeOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "LIKE#" + ipAddress},
		},
	})
	if err != nil {
		return false, err
	}
	return likeOutput.Item != nil, nil
}

func (r *dynamoRepository) ToggleLike(ctx context.Context, slug, ipAddress string) error {
	likeOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "LIKE#" + ipAddress},
		},
	})
	if err != nil {
		return err
	}

	exists := likeOutput.Item != nil
	var transItems []types.TransactWriteItem

	if exists {
		transItems = append(transItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "LIKE#" + ipAddress},
				},
			},
		})
		update := expression.Add(expression.Name("likeCount"), expression.Value(-1))
		expr, err := expression.NewBuilder().WithUpdate(update).Build()
		if err != nil {
			return err
		}
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "METADATA"},
				},
				UpdateExpression:          expr.Update(),
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
			},
		})
	} else {
		like := map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK":        &types.AttributeValueMemberS{Value: "LIKE#" + ipAddress},
			"createdAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		}
		transItems = append(transItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.db.TableName),
				Item:      like,
			},
		})
		update := expression.Add(expression.Name("likeCount"), expression.Value(1))
		expr, err := expression.NewBuilder().WithUpdate(update).Build()
		if err != nil {
			return err
		}
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "METADATA"},
				},
				UpdateExpression:          expr.Update(),
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
			},
		})
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transItems,
	})
	return err
}

func (r *dynamoRepository) GetApprovedComments(ctx context.Context, slug string) ([]CommentItem, error) {
	queryOutput, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.db.TableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :status AND begins_with(GSI1SK, :slugPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":     &types.AttributeValueMemberS{Value: "STATUS#approved"},
			":slugPrefix": &types.AttributeValueMemberS{Value: "POST#" + slug + "#"},
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

func (r *dynamoRepository) GetUserLikedComments(ctx context.Context, slug, ipAddress string) (map[string]bool, error) {
	likedCommentIDs := make(map[string]bool)
	if ipAddress == "" {
		return likedCommentIDs, nil
	}

	userLikedQuery, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.db.TableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: "POST#" + slug + "#USER#" + ipAddress},
			":skPrefix": &types.AttributeValueMemberS{Value: "LIKE#COMMENT#"},
		},
	})
	if err != nil {
		return likedCommentIDs, err
	}

	for _, likedItem := range userLikedQuery.Items {
		if skVal, ok := likedItem["SK"].(*types.AttributeValueMemberS); ok {
			likedID := strings.TrimPrefix(skVal.Value, "LIKE#COMMENT#")
			likedCommentIDs[likedID] = true
		}
	}
	return likedCommentIDs, nil
}

func (r *dynamoRepository) CreateComment(ctx context.Context, item CommentItem) error {
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.db.TableName),
		Item:      av,
	})
	return err
}

func (r *dynamoRepository) ToggleCommentLike(ctx context.Context, slug, commentID, ipAddress string) error {
	likeOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.db.TableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
			"SK": &types.AttributeValueMemberS{Value: "LIKE#" + ipAddress},
		},
	})
	if err != nil {
		return err
	}

	exists := likeOutput.Item != nil
	var transItems []types.TransactWriteItem

	if exists {
		transItems = append(transItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
					"SK": &types.AttributeValueMemberS{Value: "LIKE#" + ipAddress},
				},
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug + "#USER#" + ipAddress},
					"SK": &types.AttributeValueMemberS{Value: "LIKE#COMMENT#" + commentID},
				},
			},
		})
		update := expression.Add(expression.Name("likeCount"), expression.Value(-1))
		expr, err := expression.NewBuilder().WithUpdate(update).Build()
		if err != nil {
			return err
		}
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
				},
				UpdateExpression:          expr.Update(),
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
			},
		})
	} else {
		now := time.Now().UTC().Format(time.RFC3339)
		transItems = append(transItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.db.TableName),
				Item: map[string]types.AttributeValue{
					"PK":        &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
					"SK":        &types.AttributeValueMemberS{Value: "LIKE#" + ipAddress},
					"createdAt": &types.AttributeValueMemberS{Value: now},
				},
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.db.TableName),
				Item: map[string]types.AttributeValue{
					"PK":        &types.AttributeValueMemberS{Value: "POST#" + slug + "#USER#" + ipAddress},
					"SK":        &types.AttributeValueMemberS{Value: "LIKE#COMMENT#" + commentID},
					"createdAt": &types.AttributeValueMemberS{Value: now},
				},
			},
		})
		update := expression.Add(expression.Name("likeCount"), expression.Value(1))
		expr, err := expression.NewBuilder().WithUpdate(update).Build()
		if err != nil {
			return err
		}
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
				},
				UpdateExpression:          expr.Update(),
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
			},
		})
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transItems,
	})
	return err
}
