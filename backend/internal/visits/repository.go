package visits

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

// ErrRateLimited is returned when an IP exceeds its daily visit-recording cap.
var ErrRateLimited = errors.New("rate limit exceeded")

// visitRateLimitMax is the maximum visit records a single IP may add per UTC day.
const visitRateLimitMax = 20

const statsGeoPK = "STATS#GEO"

type CountryVisits struct {
	Country     string `json:"country"`
	CountryName string `json:"countryName"`
	Count       int64  `json:"count"`
}

type Repository interface {
	IncrementCountryVisit(ctx context.Context, country, countryName, ipAddress string) error
	GetCountryVisits(ctx context.Context) ([]CountryVisits, error)
}

type dynamoRepository struct {
	db *db.Client
}

func NewRepository(dbClient *db.Client) Repository {
	return &dynamoRepository{db: dbClient}
}

// IncrementCountryVisit atomically bumps the per-country hit counter alongside
// a per-IP daily rate-limit item (same pattern as the like rate limiter).
func (r *dynamoRepository) IncrementCountryVisit(ctx context.Context, country, countryName, ipAddress string) error {
	now := time.Now().UTC()

	transItems := []types.TransactWriteItem{
		{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: statsGeoPK},
					"SK": &types.AttributeValueMemberS{Value: "COUNTRY#" + country},
				},
				UpdateExpression: aws.String("ADD #c :one SET countryName = if_not_exists(countryName, :name), updatedAt = :now"),
				ExpressionAttributeNames: map[string]string{
					"#c": "count",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":one":  &types.AttributeValueMemberN{Value: "1"},
					":name": &types.AttributeValueMemberS{Value: countryName},
					":now":  &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
				},
			},
		},
		{
			Update: &types.Update{
				TableName: aws.String(r.db.TableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "RATELIMIT#IP#" + ipAddress},
					"SK": &types.AttributeValueMemberS{Value: "VISITS#" + now.Format("2006-01-02")},
				},
				UpdateExpression:    aws.String("ADD #c :one SET expiresAt = if_not_exists(expiresAt, :exp)"),
				ConditionExpression: aws.String("attribute_not_exists(#c) OR #c < :max"),
				ExpressionAttributeNames: map[string]string{
					"#c": "count",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":one": &types.AttributeValueMemberN{Value: "1"},
					":max": &types.AttributeValueMemberN{Value: strconv.Itoa(visitRateLimitMax)},
					":exp": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(48*time.Hour).Unix(), 10)},
				},
			},
		},
	}

	_, err := r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transItems,
	})
	return mapRateLimitError(err, 1)
}

func (r *dynamoRepository) GetCountryVisits(ctx context.Context) ([]CountryVisits, error) {
	queryOutput, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.db.TableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: statsGeoPK},
			":skPrefix": &types.AttributeValueMemberS{Value: "COUNTRY#"},
		},
	})
	if err != nil {
		return nil, err
	}

	type countryItem struct {
		SK          string `dynamodbav:"SK"`
		CountryName string `dynamodbav:"countryName"`
		Count       int64  `dynamodbav:"count"`
	}
	var items []countryItem
	if err := attributevalue.UnmarshalListOfMaps(queryOutput.Items, &items); err != nil {
		return nil, err
	}

	visits := make([]CountryVisits, 0, len(items))
	for _, item := range items {
		visits = append(visits, CountryVisits{
			Country:     strings.TrimPrefix(item.SK, "COUNTRY#"),
			CountryName: item.CountryName,
			Count:       item.Count,
		})
	}
	return visits, nil
}

// mapRateLimitError translates a TransactionCanceledException into ErrRateLimited
// when the cancellation reason at rateLimitIdx is the rate-limit condition failing.
func mapRateLimitError(err error, rateLimitIdx int) error {
	if err == nil {
		return nil
	}
	var tce *types.TransactionCanceledException
	if errors.As(err, &tce) && rateLimitIdx < len(tce.CancellationReasons) &&
		aws.ToString(tce.CancellationReasons[rateLimitIdx].Code) == "ConditionalCheckFailed" {
		return ErrRateLimited
	}
	return err
}
