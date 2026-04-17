#!/bin/bash
set -e

echo "Building Go Lambda functions..."
cd backend
GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap cmd/interactions/main.go
zip -j interactions.zip bootstrap

GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap cmd/contact/main.go
zip -j contact.zip bootstrap

GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap cmd/admin/main.go
zip -j admin.zip bootstrap
cd ..

echo "Deploying to LocalStack..."

# Create Role (dummy) - suppress error if IAM is not enabled or role exists
aws --endpoint-url=http://localhost:4566 iam create-role --role-name lambda-ex --assume-role-policy-document '{"Version": "2012-10-17","Statement": [{ "Action": "sts:AssumeRole", "Principal": {"Service": "lambda.amazonaws.com"}, "Effect": "Allow"}]}' 2>/dev/null || true

deploy_lambda() {
    NAME=$1
    ZIP=$2
    ENV=$3
    
    echo "Processing Lambda: $NAME"
    if aws --endpoint-url=http://localhost:4566 lambda get-function --function-name "$NAME" >/dev/null 2>&1; then
        echo "Updating existing function $NAME..."
        aws --endpoint-url=http://localhost:4566 lambda update-function-code --function-name "$NAME" --zip-file "fileb://$ZIP" >/dev/null
        aws --endpoint-url=http://localhost:4566 lambda update-function-configuration --function-name "$NAME" --environment "Variables=$ENV" >/dev/null
    else
        echo "Creating new function $NAME..."
        aws --endpoint-url=http://localhost:4566 lambda create-function \
            --function-name "$NAME" \
            --runtime provided.al2023 \
            --handler bootstrap \
            --role arn:aws:iam::000000000000:role/lambda-ex \
            --zip-file "fileb://$ZIP" \
            --environment "Variables=$ENV" >/dev/null
            
        aws --endpoint-url=http://localhost:4566 lambda create-function-url-config \
            --function-name "$NAME" \
            --auth-type NONE >/dev/null
    fi
}

deploy_lambda "interactions-api" "backend/interactions.zip" "{DYNAMODB_ENDPOINT=http://localstack:4566,DYNAMODB_TABLE_NAME=jyatesdotdev-state,SKIP_RECAPTCHA=true}"
deploy_lambda "contact-api" "backend/contact.zip" "{SES_FROM_EMAIL=test@jyates.dev,SES_ADMIN_EMAIL=admin@jyates.dev,SKIP_RECAPTCHA=true}"
deploy_lambda "admin-api" "backend/admin.zip" "{DYNAMODB_ENDPOINT=http://localstack:4566,DYNAMODB_TABLE_NAME=jyatesdotdev-state}"

echo "Functions deployed successfully to LocalStack."
