package db

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type DynamoDBAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

type Client struct {
	DynamoDBAPI
	TableName string
}

func NewClient(ctx context.Context) (*Client, error) {
	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "jyatesdotdev-state"
	}

	endpoint := os.Getenv("DYNAMODB_ENDPOINT") // For local development

	var cfg aws.Config
	var err error

	if endpoint != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion("us-east-1"), // Default region for local
		)
		if err != nil {
			return nil, err
		}

		client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})

		return &Client{
			DynamoDBAPI: client,
			TableName:   tableName,
		}, nil
	}

	cfg, err = config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &Client{
		DynamoDBAPI: dynamodb.NewFromConfig(cfg),
		TableName:   tableName,
	}, nil
}
