package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

const (
	recipientLease    = 90 * time.Second
	deliveryRetention = 90 * 24 * time.Hour
)

type RecipientClaimState string

const (
	RecipientStarted    RecipientClaimState = "started"
	RecipientComplete   RecipientClaimState = "complete"
	RecipientInProgress RecipientClaimState = "in_progress"
)

type RecipientClaim struct {
	ManifestID    string
	EventIndex    int
	RecipientHash string
	AttemptID     string
	State         RecipientClaimState
}

type DeliveryRepository interface {
	ManifestComplete(ctx context.Context, manifestID string) (bool, error)
	BeginRecipient(ctx context.Context, manifestID string, eventIndex int, email string) (RecipientClaim, error)
	CompleteRecipient(ctx context.Context, claim RecipientClaim) error
	ReleaseRecipient(ctx context.Context, claim RecipientClaim) error
	CompleteManifest(ctx context.Context, manifestID string) error
}

type dynamoDeliveryRepository struct {
	db  *db.Client
	now func() time.Time
}

func NewDeliveryRepository(dbClient *db.Client) DeliveryRepository {
	return &dynamoDeliveryRepository{db: dbClient, now: time.Now}
}

func manifestKey(manifestID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "NOTIFICATION#" + manifestID},
		"SK": &types.AttributeValueMemberS{Value: "MANIFEST"},
	}
}

func recipientKey(manifestID string, eventIndex int, hash string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "NOTIFICATION#" + manifestID},
		"SK": &types.AttributeValueMemberS{
			Value: fmt.Sprintf("RECIPIENT#%02d#%s", eventIndex, hash),
		},
	}
}

func hashRecipient(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

func (r *dynamoDeliveryRepository) ManifestComplete(
	ctx context.Context,
	manifestID string,
) (bool, error) {
	output, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.db.TableName),
		Key:            manifestKey(manifestID),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return false, err
	}
	status, ok := output.Item["status"].(*types.AttributeValueMemberS)
	return ok && status.Value == "complete", nil
}

func (r *dynamoDeliveryRepository) BeginRecipient(
	ctx context.Context,
	manifestID string,
	eventIndex int,
	email string,
) (RecipientClaim, error) {
	recipientHash := hashRecipient(email)
	for attempt := 0; attempt < 2; attempt++ {
		now := r.now().UTC()
		claim := RecipientClaim{
			ManifestID:    manifestID,
			EventIndex:    eventIndex,
			RecipientHash: recipientHash,
			AttemptID:     uuid.NewString(),
			State:         RecipientStarted,
		}
		item := recipientKey(manifestID, eventIndex, recipientHash)
		item["status"] = &types.AttributeValueMemberS{Value: "processing"}
		item["attemptID"] = &types.AttributeValueMemberS{Value: claim.AttemptID}
		item["leaseUntil"] = &types.AttributeValueMemberN{
			Value: strconv.FormatInt(now.Add(recipientLease).Unix(), 10),
		}
		item["updatedAt"] = &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)}
		item["expiresAt"] = &types.AttributeValueMemberN{
			Value: strconv.FormatInt(now.Add(deliveryRetention).Unix(), 10),
		}

		_, err := r.db.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(r.db.TableName),
			Item:      item,
			ConditionExpression: aws.String(
				"attribute_not_exists(PK) OR (#status = :processing AND leaseUntil < :now)",
			),
			ExpressionAttributeNames: map[string]string{"#status": "status"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":processing": &types.AttributeValueMemberS{Value: "processing"},
				":now":        &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Unix(), 10)},
			},
		})
		if err == nil {
			return claim, nil
		}
		var conditionalErr *types.ConditionalCheckFailedException
		if !errors.As(err, &conditionalErr) {
			return RecipientClaim{}, err
		}

		output, getErr := r.db.GetItem(ctx, &dynamodb.GetItemInput{
			TableName:      aws.String(r.db.TableName),
			Key:            recipientKey(manifestID, eventIndex, recipientHash),
			ConsistentRead: aws.Bool(true),
		})
		if getErr != nil {
			return RecipientClaim{}, getErr
		}
		status, ok := output.Item["status"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}
		claim.AttemptID = ""
		if status.Value == "complete" {
			claim.State = RecipientComplete
		} else {
			claim.State = RecipientInProgress
		}
		return claim, nil
	}
	return RecipientClaim{}, errors.New("recipient delivery state changed repeatedly")
}

func (r *dynamoDeliveryRepository) CompleteRecipient(
	ctx context.Context,
	claim RecipientClaim,
) error {
	now := r.now().UTC()
	_, err := r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.db.TableName),
		Key:       recipientKey(claim.ManifestID, claim.EventIndex, claim.RecipientHash),
		UpdateExpression: aws.String(
			"SET #status = :complete, completedAt = :now REMOVE leaseUntil, attemptID",
		),
		ConditionExpression:      aws.String("#status = :processing AND attemptID = :attempt"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":complete":   &types.AttributeValueMemberS{Value: "complete"},
			":processing": &types.AttributeValueMemberS{Value: "processing"},
			":attempt":    &types.AttributeValueMemberS{Value: claim.AttemptID},
			":now":        &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		},
	})
	return err
}

func (r *dynamoDeliveryRepository) ReleaseRecipient(
	ctx context.Context,
	claim RecipientClaim,
) error {
	_, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:                aws.String(r.db.TableName),
		Key:                      recipientKey(claim.ManifestID, claim.EventIndex, claim.RecipientHash),
		ConditionExpression:      aws.String("#status = :processing AND attemptID = :attempt"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":processing": &types.AttributeValueMemberS{Value: "processing"},
			":attempt":    &types.AttributeValueMemberS{Value: claim.AttemptID},
		},
	})
	return err
}

func (r *dynamoDeliveryRepository) CompleteManifest(
	ctx context.Context,
	manifestID string,
) error {
	now := r.now().UTC()
	item := manifestKey(manifestID)
	item["status"] = &types.AttributeValueMemberS{Value: "complete"}
	item["completedAt"] = &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)}
	item["expiresAt"] = &types.AttributeValueMemberN{
		Value: strconv.FormatInt(now.Add(deliveryRetention).Unix(), 10),
	}
	_, err := r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.db.TableName),
		Item:      item,
	})
	return err
}
