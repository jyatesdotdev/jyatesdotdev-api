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

	chiLambda = chiadapter.New(newRouter(contactHandler))
}

func newRouter(contactHandler *contact.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Mount("/contact", contactHandler.Routes())
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

		r := newRouter(contactHandler)

		srv := &http.Server{
			Addr:              ":" + port,
			Handler:           r,
			ReadHeaderTimeout: 3 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())
	}
}
