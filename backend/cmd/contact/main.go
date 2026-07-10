package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	chiadapter "github.com/awslabs/aws-lambda-go-api-proxy/chi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jyates/jyatesdotdev-api/backend/internal/contact"
	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
	"github.com/jyates/jyatesdotdev-api/backend/internal/email"
	"github.com/jyates/jyatesdotdev-api/backend/internal/subscriptions"
)

var chiLambda *chiadapter.ChiLambda

func init() {
	ctx := context.Background()

	emailClient, err := email.NewSESClient(ctx)
	if err != nil {
		log.Fatalf("Could not initialize SES client: %v", err)
	}

	dbClient, err := db.NewClient(ctx)
	if err != nil {
		log.Fatalf("Could not initialize DynamoDB client: %v", err)
	}

	contactHandler := contact.NewHandler(emailClient, contact.NewRateLimiter(dbClient))
	subscriptionHandler := newSubscriptionHandler(ctx, dbClient, emailClient)

	chiLambda = chiadapter.New(newRouter(contactHandler, subscriptionHandler))
}

func newSubscriptionHandler(
	ctx context.Context,
	dbClient *db.Client,
	emailClient *email.SESClient,
) *subscriptions.Handler {
	contacts, err := subscriptions.NewContactStore(ctx, dbClient)
	if err != nil {
		log.Fatalf("Could not initialize subscriber contacts: %v", err)
	}
	siteURL := os.Getenv("SITE_URL")
	if siteURL == "" {
		siteURL = "https://jyates.dev"
	}
	service, err := subscriptions.NewService(
		subscriptions.NewRequestRepository(dbClient),
		contacts,
		emailClient,
		siteURL,
	)
	if err != nil {
		log.Fatalf("Could not initialize subscription service: %v", err)
	}
	return subscriptions.NewHandler(service, subscriptions.NewRateLimiter(dbClient))
}

func newRouter(
	contactHandler *contact.Handler,
	subscriptionHandler *subscriptions.Handler,
) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Mount("/contact", contactHandler.Routes())
		r.Mount("/subscriptions", subscriptionHandler.Routes())
	})

	return r
}

func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return chiLambda.ProxyWithContext(ctx, req)
}

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(Handler)
	} else {
		// Run as a normal HTTP server for local development
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		// #nosec G706 -- The port value is sourced from our own environment variables at startup, not from untrusted user input.
		log.Printf("Starting contact server on port %s", port)

		ctx := context.Background()
		emailClient, _ := email.NewSESClient(ctx)
		dbClient, _ := db.NewClient(ctx)
		contactHandler := contact.NewHandler(emailClient, contact.NewRateLimiter(dbClient))
		subscriptionHandler := newSubscriptionHandler(ctx, dbClient, emailClient)

		r := newRouter(contactHandler, subscriptionHandler)

		srv := &http.Server{
			Addr:              ":" + port,
			Handler:           r,
			ReadHeaderTimeout: 3 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())
	}
}
