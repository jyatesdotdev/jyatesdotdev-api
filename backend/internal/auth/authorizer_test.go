package auth

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
)

func TestHandleRequest_Success(t *testing.T) {
	os.Setenv("ADMIN_USERNAME", "admin")
	os.Setenv("ADMIN_PASSWORD", "secret")
	defer os.Unsetenv("ADMIN_USERNAME")
	defer os.Unsetenv("ADMIN_PASSWORD")

	token := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	req := events.APIGatewayCustomAuthorizerRequest{
		AuthorizationToken: token,
		MethodArn:          "arn:aws:execute-api:us-east-1:123456789012:api-id/stage/GET/resource",
	}

	resp, err := HandleRequest(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "admin", resp.PrincipalID)
	assert.Equal(t, "Allow", resp.PolicyDocument.Statement[0].Effect)
}

func TestHandleRequest_InvalidToken(t *testing.T) {
	os.Setenv("ADMIN_USERNAME", "admin")
	os.Setenv("ADMIN_PASSWORD", "secret")
	defer os.Unsetenv("ADMIN_USERNAME")
	defer os.Unsetenv("ADMIN_PASSWORD")

	token := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:wrong"))
	req := events.APIGatewayCustomAuthorizerRequest{
		AuthorizationToken: token,
		MethodArn:          "arn:aws:execute-api:us-east-1:123456789012:api-id/stage/GET/resource",
	}

	resp, err := HandleRequest(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "user", resp.PrincipalID)
	assert.Equal(t, "Deny", resp.PolicyDocument.Statement[0].Effect)
}

func TestHandleRequest_MissingToken(t *testing.T) {
	req := events.APIGatewayCustomAuthorizerRequest{
		AuthorizationToken: "",
	}

	_, err := HandleRequest(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, "Unauthorized", err.Error())
}

func TestHandleRequest_MissingEnv(t *testing.T) {
	os.Unsetenv("ADMIN_USERNAME")
	os.Unsetenv("ADMIN_PASSWORD")

	token := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	req := events.APIGatewayCustomAuthorizerRequest{
		AuthorizationToken: token,
		MethodArn:          "arn:aws:execute-api:us-east-1:123456789012:api-id/stage/GET/resource",
	}

	resp, err := HandleRequest(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "user", resp.PrincipalID)
	assert.Equal(t, "Deny", resp.PolicyDocument.Statement[0].Effect)
}
