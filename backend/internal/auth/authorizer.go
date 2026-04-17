package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

func HandleRequest(ctx context.Context, event events.APIGatewayCustomAuthorizerRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	token := event.AuthorizationToken
	if token == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	expectedUser := os.Getenv("ADMIN_USERNAME")
	expectedPass := os.Getenv("ADMIN_PASSWORD")

	if expectedUser == "" || expectedPass == "" {
		// If credentials are not configured, deny access securely
		return generatePolicy("user", "Deny", event.MethodArn), nil
	}

	parts := strings.Split(token, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "basic" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	payload, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	if pair[0] == expectedUser && pair[1] == expectedPass {
		return generatePolicy("admin", "Allow", event.MethodArn), nil
	}

	return generatePolicy("user", "Deny", event.MethodArn), nil
}

func generatePolicy(principalID, effect, resource string) events.APIGatewayCustomAuthorizerResponse {
	authResponse := events.APIGatewayCustomAuthorizerResponse{PrincipalID: principalID}

	if effect != "" && resource != "" {
		authResponse.PolicyDocument = events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   effect,
					Resource: []string{resource},
				},
			},
		}
	}

	return authResponse
}
