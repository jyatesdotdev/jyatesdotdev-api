package interactions

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

type Repository interface {
	GetPostMetadata(ctx context.Context, slug string) (*PostMetadata, error)
	CheckUserLike(ctx context.Context, slug, visitorID string) (bool, error)
	ToggleLike(ctx context.Context, slug, visitorID, ipAddress string) error

	GetApprovedComments(ctx context.Context, slug string) ([]CommentItem, error)
	GetUserLikedComments(ctx context.Context, slug, visitorID string) (map[string]bool, error)
	CreateComment(ctx context.Context, item CommentItem, ipAddress string) error

	ToggleCommentLike(ctx context.Context, slug, commentID, visitorID, ipAddress string) error
}

// likeRateLimitMax is the maximum number of like ADDs a single IP may perform per UTC day.
const (
	likeRateLimitMax    = 100
	commentRateLimitMax = 10
)

// rateLimitTransactItem returns a transact item that atomically increments the
// per-IP daily like counter, failing the whole transaction once the cap is hit.
// The item expires via DynamoDB TTL (expiresAt) 48h after first write.
func rateLimitTransactItem(tableName, ipAddress, namespace string, max int) types.TransactWriteItem {
	now := time.Now().UTC()
	return types.TransactWriteItem{
		Update: &types.Update{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "RATELIMIT#IP#" + ipAddress},
				"SK": &types.AttributeValueMemberS{Value: namespace + "#" + now.Format("2006-01-02")},
			},
			UpdateExpression:    aws.String("ADD #c :one SET expiresAt = if_not_exists(expiresAt, :exp)"),
			ConditionExpression: aws.String("attribute_not_exists(#c) OR #c < :max"),
			ExpressionAttributeNames: map[string]string{
				"#c": "count",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":one": &types.AttributeValueMemberN{Value: "1"},
				":max": &types.AttributeValueMemberN{Value: strconv.Itoa(max)},
				":exp": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(48*time.Hour).Unix(), 10)},
			},
		},
	}
}

// mapRateLimitError translates a TransactionCanceledException into ErrRateLimited
// when the cancellation reason at rateLimitIdx is the rate-limit condition failing.
// Any other error (including condition failures on other items) is returned as-is.
func mapTransactionError(err error, rateLimitIdx int, conflictIndexes ...int) error {
	if err == nil || rateLimitIdx < 0 {
		if err == nil {
			return nil
		}
	}
	var tce *types.TransactionCanceledException
	if !errors.As(err, &tce) {
		return err
	}
	if isConditionalFailure(tce, rateLimitIdx) {
		return ErrRateLimited
	}
	for _, index := range conflictIndexes {
		if isConditionalFailure(tce, index) {
			return ErrConflict
		}
	}
	return err
}

func isConditionalFailure(tce *types.TransactionCanceledException, index int) bool {
	return index >= 0 && index < len(tce.CancellationReasons) &&
		aws.ToString(tce.CancellationReasons[index].Code) == "ConditionalCheckFailed"
}

type dynamoRepository struct {
	db *db.Client
}

func NewRepository(dbClient *db.Client) Repository {
	return &dynamoRepository{db: dbClient}
}

func (r *dynamoRepository) GetPostMetadata(ctx context.Context, slug string) (*PostMetadata, error) {
	metadataOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.db.TableName),
		ConsistentRead: aws.Bool(true),
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

func (r *dynamoRepository) CheckUserLike(ctx context.Context, slug, visitorID string) (bool, error) {
	if visitorID == "" {
		return false, nil
	}
	likeOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.db.TableName),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "LIKE#" + visitorID},
		},
	})
	if err != nil {
		return false, err
	}
	return likeOutput.Item != nil, nil
}

func (r *dynamoRepository) ToggleLike(ctx context.Context, slug, visitorID, ipAddress string) error {
	likeOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.db.TableName),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK": &types.AttributeValueMemberS{Value: "LIKE#" + visitorID},
		},
	})
	if err != nil {
		return err
	}

	exists := likeOutput.Item != nil
	var transItems []types.TransactWriteItem
	rateLimitIdx := -1

	if exists {
		transItems = append(transItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "LIKE#" + visitorID},
				},
				ConditionExpression: aws.String("attribute_exists(PK)"),
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "METADATA"},
				},
				UpdateExpression:    aws.String("ADD #count :delta"),
				ConditionExpression: aws.String("attribute_exists(PK) AND #count > :zero"),
				ExpressionAttributeNames: map[string]string{
					"#count": "likeCount",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":delta": &types.AttributeValueMemberN{Value: "-1"},
					":zero":  &types.AttributeValueMemberN{Value: "0"},
				},
			},
		})
	} else {
		like := map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: "POST#" + slug},
			"SK":        &types.AttributeValueMemberS{Value: "LIKE#" + visitorID},
			"createdAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		}
		transItems = append(transItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName:           aws.String(r.db.TableName),
				Item:                like,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "METADATA"},
				},
				UpdateExpression:    aws.String("ADD #count :delta"),
				ConditionExpression: aws.String("attribute_exists(PK)"),
				ExpressionAttributeNames: map[string]string{
					"#count": "likeCount",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":delta": &types.AttributeValueMemberN{Value: "1"},
				},
			},
		})
		transItems = append(transItems, rateLimitTransactItem(r.db.TableName, ipAddress, "LIKES", likeRateLimitMax))
		rateLimitIdx = len(transItems) - 1
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transItems,
	})
	return mapTransactionError(err, rateLimitIdx, 0, 1)
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

func (r *dynamoRepository) GetUserLikedComments(ctx context.Context, slug, visitorID string) (map[string]bool, error) {
	likedCommentIDs := make(map[string]bool)
	if visitorID == "" {
		return likedCommentIDs, nil
	}

	userLikedQuery, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.db.TableName),
		ConsistentRead:         aws.Bool(true),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: "POST#" + slug + "#USER#" + visitorID},
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

func (r *dynamoRepository) CreateComment(ctx context.Context, item CommentItem, ipAddress string) error {
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}
	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           aws.String(r.db.TableName),
					Item:                av,
					ConditionExpression: aws.String("attribute_not_exists(PK)"),
				},
			},
			rateLimitTransactItem(r.db.TableName, ipAddress, "COMMENTS", commentRateLimitMax),
		},
	})
	return mapTransactionError(err, 1)
}

func (r *dynamoRepository) ToggleCommentLike(ctx context.Context, slug, commentID, visitorID, ipAddress string) error {
	likeOutput, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.db.TableName),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
			"SK": &types.AttributeValueMemberS{Value: "LIKE#" + visitorID},
		},
	})
	if err != nil {
		return err
	}

	exists := likeOutput.Item != nil
	var transItems []types.TransactWriteItem
	rateLimitIdx := -1

	if exists {
		transItems = append(transItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
					"SK": &types.AttributeValueMemberS{Value: "LIKE#" + visitorID},
				},
				ConditionExpression: aws.String("attribute_exists(PK)"),
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug + "#USER#" + visitorID},
					"SK": &types.AttributeValueMemberS{Value: "LIKE#COMMENT#" + commentID},
				},
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
				},
				UpdateExpression:    aws.String("ADD #count :delta"),
				ConditionExpression: aws.String("attribute_exists(PK) AND #count > :zero"),
				ExpressionAttributeNames: map[string]string{
					"#count": "likeCount",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":delta": &types.AttributeValueMemberN{Value: "-1"},
					":zero":  &types.AttributeValueMemberN{Value: "0"},
				},
			},
		})
	} else {
		now := time.Now().UTC().Format(time.RFC3339)
		transItems = append(transItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.db.TableName),
				Item: map[string]types.AttributeValue{
					"PK":        &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
					"SK":        &types.AttributeValueMemberS{Value: "LIKE#" + visitorID},
					"createdAt": &types.AttributeValueMemberS{Value: now},
				},
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.db.TableName),
				Item: map[string]types.AttributeValue{
					"PK":        &types.AttributeValueMemberS{Value: "POST#" + slug + "#USER#" + visitorID},
					"SK":        &types.AttributeValueMemberS{Value: "LIKE#COMMENT#" + commentID},
					"createdAt": &types.AttributeValueMemberS{Value: now},
				},
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			},
		})
		transItems = append(transItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "POST#" + slug},
					"SK": &types.AttributeValueMemberS{Value: "COMMENT#" + commentID},
				},
				UpdateExpression:    aws.String("ADD #count :delta"),
				ConditionExpression: aws.String("attribute_exists(PK) AND #status = :approved"),
				ExpressionAttributeNames: map[string]string{
					"#count":  "likeCount",
					"#status": "status",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":delta":    &types.AttributeValueMemberN{Value: "1"},
					":approved": &types.AttributeValueMemberS{Value: "approved"},
				},
			},
		})
		transItems = append(transItems, rateLimitTransactItem(r.db.TableName, ipAddress, "LIKES", likeRateLimitMax))
		rateLimitIdx = len(transItems) - 1
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transItems,
	})
	return mapTransactionError(err, rateLimitIdx, 0, 1, 2)
}
