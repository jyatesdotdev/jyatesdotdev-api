package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/jyates/jyatesdotdev-api/backend/internal/auth"
)

func main() {
	lambda.Start(auth.HandleRequest)
}
