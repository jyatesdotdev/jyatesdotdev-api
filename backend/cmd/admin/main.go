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

	"github.com/jyates/jyatesdotdev-api/backend/internal/admin"
	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
)

var chiLambda *chiadapter.ChiLambda

func init() {
	ctx := context.Background()

	dbClient, err := db.NewClient(ctx)
	if err != nil {
		log.Fatalf("Could not initialize DynamoDB client: %v", err)
	}

	adminRepo := admin.NewRepository(dbClient)
	adminService := admin.NewService(adminRepo)
	adminHandler := admin.NewHandler(adminService)

	chiLambda = chiadapter.New(newRouter(adminHandler))
}

func newRouter(adminHandler *admin.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// The handler's own routes are prefixed with /comments, so the effective
	// paths are /api/v1/admin/comments[/{id}].
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Mount("/", adminHandler.Routes())
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
		log.Printf("Starting admin server on port %s", port)

		ctx := context.Background()
		dbClient, _ := db.NewClient(ctx)
		adminRepo := admin.NewRepository(dbClient)
		adminService := admin.NewService(adminRepo)
		adminHandler := admin.NewHandler(adminService)

		r := newRouter(adminHandler)

		srv := &http.Server{
			Addr:              ":" + port,
			Handler:           r,
			ReadHeaderTimeout: 3 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())
	}
}
