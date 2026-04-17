//go:build integration
// +build integration

package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jyates/jyatesdotdev-api/backend/internal/admin"
	"github.com/jyates/jyatesdotdev-api/backend/internal/contact"
	"github.com/jyates/jyatesdotdev-api/backend/internal/db"
	"github.com/jyates/jyatesdotdev-api/backend/internal/email"
	"github.com/jyates/jyatesdotdev-api/backend/internal/interactions"
)

func setupTestDB(t *testing.T) *db.Client {
	ctx := context.Background()
	os.Setenv("DYNAMODB_ENDPOINT", "http://localhost:4566")
	os.Setenv("DYNAMODB_TABLE_NAME", "integration-test-table")
	os.Setenv("AWS_REGION", "us-east-1")
	os.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	require.NoError(t, err)

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:4566")
	})

	tableName := "integration-test-table"

	// Try to delete table if it exists from previous run
	_, _ = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})

	// Create Table
	_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("GSI1"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GSI1PK"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("GSI1SK"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
	})
	require.NoError(t, err)

	dbClient := &db.Client{
		DynamoDBAPI: client,
		TableName:   tableName,
	}

	return dbClient
}

func TestIntegration_CommentsFlow(t *testing.T) {
	// 1. Setup DB and Handlers
	dbClient := setupTestDB(t)
	interactionsHandler := interactions.NewHandler(dbClient, nil)
	adminHandler := admin.NewHandler(dbClient)

	os.Setenv("SKIP_RECAPTCHA", "true")
	defer os.Unsetenv("SKIP_RECAPTCHA")

	// Router setup
	r := chi.NewRouter()
	r.Mount("/api/v1/comments", interactionsHandler.CommentRoutes())
	r.Mount("/api/v1/admin", adminHandler.Routes())

	slug := "integration-test-post"

	// 2. Submit a comment
	reqBody := `{"slug": "` + slug + `", "content": "This is a great post!", "authorName": "Test User", "token": "dummy"}`
	req := httptest.NewRequest("POST", "/api/v1/comments", strings.NewReader(reqBody))
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var createResp map[string]string
	err := json.NewDecoder(w.Body).Decode(&createResp)
	require.NoError(t, err)
	commentID := createResp["id"]
	require.NotEmpty(t, commentID)

	// 3. Verify comment is pending in admin
	req = httptest.NewRequest("GET", "/api/v1/admin/comments", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var pendingResp []admin.CommentResponse
	err = json.NewDecoder(w.Body).Decode(&pendingResp)
	require.NoError(t, err)
	require.Len(t, pendingResp, 1)
	assert.Equal(t, commentID, pendingResp[0].ID)
	assert.Equal(t, "pending", pendingResp[0].Status)

	// 4. Verify public comments are empty (since it's pending)
	req = httptest.NewRequest("GET", "/api/v1/comments?slug="+slug, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var publicResp []interactions.CommentResponse
	err = json.NewDecoder(w.Body).Decode(&publicResp)
	require.NoError(t, err)
	require.Len(t, publicResp, 0)

	// 5. Admin approves comment
	reqBody = `{"slug": "` + slug + `", "status": "approved"}`
	req = httptest.NewRequest("PUT", "/api/v1/admin/comments/"+commentID, strings.NewReader(reqBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// 6. Verify comment is now visible publicly
	req = httptest.NewRequest("GET", "/api/v1/comments?slug="+slug, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.NewDecoder(w.Body).Decode(&publicResp)
	require.NoError(t, err)
	require.Len(t, publicResp, 1)
	assert.Equal(t, commentID, publicResp[0].ID)
	assert.Equal(t, "This is a great post!", publicResp[0].Content)
	assert.Equal(t, 0, publicResp[0].LikeCount)

	// 7. Toggle Like on comment
	reqBody = `{"slug": "` + slug + `", "token": "dummy"}`
	req = httptest.NewRequest("POST", "/api/v1/comments/"+commentID+"/like", strings.NewReader(reqBody))
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// 8. Verify LikeCount increased and UserHasLiked is true
	time.Sleep(100 * time.Millisecond) // Ensure eventual consistency if needed (usually strongly consistent on same item, but query on GSI might be eventual)
	req = httptest.NewRequest("GET", "/api/v1/comments?slug="+slug, nil)
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.NewDecoder(w.Body).Decode(&publicResp)
	require.NoError(t, err)
	require.Len(t, publicResp, 1)
	assert.Equal(t, 1, publicResp[0].LikeCount)
	assert.True(t, publicResp[0].UserHasLiked)
}

func TestIntegration_ContactFlow(t *testing.T) {
	os.Setenv("SKIP_RECAPTCHA", "true")
	defer os.Unsetenv("SKIP_RECAPTCHA")

	// Router setup
	r := chi.NewRouter()
	
	// Create dummy email service (nil API internally handles prints and ignores)
	emailSvc, _ := email.NewSESClient(context.Background())
	contactHandler := contact.NewHandler(emailSvc)
	
	r.Mount("/api/v1/contact", contactHandler.Routes())

	// 1. Submit contact form successfully
	reqBody := `{"name": "Integration User", "email": "integration@example.com", "message": "Test message", "token": "dummy"}`
	req := httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	
	require.Equal(t, http.StatusOK, w.Code)
	
	var createResp map[string]string
	err := json.NewDecoder(w.Body).Decode(&createResp)
	require.NoError(t, err)
	assert.Equal(t, "message sent successfully", createResp["message"])
	
	// 2. Submit bad request
	badReqBody := `{"name": "", "email": "integration@example.com", "message": "Test message", "token": "dummy"}`
	req = httptest.NewRequest("POST", "/api/v1/contact", strings.NewReader(badReqBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	
	require.Equal(t, http.StatusBadRequest, w.Code)
}
