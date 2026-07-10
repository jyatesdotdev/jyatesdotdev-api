package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
	"github.com/jyates/jyatesdotdev-api/backend/internal/email"
	"github.com/jyates/jyatesdotdev-api/backend/internal/notifications"
	"github.com/jyates/jyatesdotdev-api/backend/internal/subscriptions"
)

var notificationService *notifications.Service

func init() {
	ctx := context.Background()
	dbClient, err := db.NewClient(ctx)
	if err != nil {
		log.Fatalf("Could not initialize DynamoDB client: %v", err)
	}
	mailer, err := email.NewSESClient(ctx)
	if err != nil {
		log.Fatalf("Could not initialize SES mailer: %v", err)
	}
	contacts, err := subscriptions.NewContactStore(ctx, dbClient)
	if err != nil {
		log.Fatalf("Could not initialize subscriber contacts: %v", err)
	}
	objects, err := notifications.NewObjectStore(ctx)
	if err != nil {
		log.Fatalf("Could not initialize S3 object store: %v", err)
	}
	contactListName := os.Getenv("SES_CONTACT_LIST_NAME")
	if contactListName == "" {
		contactListName = "jyatesdotdev-updates"
	}
	notificationService = notifications.NewService(
		objects,
		notifications.NewDeliveryRepository(dbClient),
		contacts,
		mailer,
		contactListName,
	)
}

func Handler(ctx context.Context, event events.S3Event) error {
	var deliveryErr error
	for _, record := range event.Records {
		key := record.S3.Object.URLDecodedKey
		if key == "" {
			key = record.S3.Object.Key
		}
		result, err := notificationService.Deliver(ctx, record.S3.Bucket.Name, key)
		if err != nil {
			deliveryErr = errors.Join(
				deliveryErr,
				fmt.Errorf("deliver %s/%s: %w", record.S3.Bucket.Name, key, err),
			)
			continue
		}
		log.Printf(
			"notification manifest delivered: bucket=%s key=%s duplicate=%t sent=%d failed=%d skipped=%d",
			record.S3.Bucket.Name,
			key,
			result.Duplicate,
			result.Sent,
			result.Failed,
			result.Skipped,
		)
	}
	return deliveryErr
}

func main() {
	lambda.Start(Handler)
}
