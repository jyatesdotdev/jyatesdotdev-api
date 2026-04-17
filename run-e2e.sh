#!/bin/bash
set -e

# Configuration
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1
ENDPOINT="http://localhost:4566"

echo "🚀 Starting LocalStack..."
docker-compose up -d localstack

echo "⏳ Waiting for LocalStack to be ready..."
until curl -s "$ENDPOINT/_localstack/health" | grep -q '"dynamodb": "running"'; do
  sleep 2
done
until curl -s "$ENDPOINT/_localstack/health" | grep -q '"lambda": "running"'; do
  sleep 2
done

echo "📦 Deploying Lambdas to LocalStack..."
./deploy-localstack.sh

echo "🛠️ Creating DynamoDB table..."
aws --endpoint-url=$ENDPOINT dynamodb create-table \
    --table-name jyatesdotdev-state \
    --attribute-definitions AttributeName=PK,AttributeType=S AttributeName=SK,AttributeType=S AttributeName=GSI1PK,AttributeType=S AttributeName=GSI1SK,AttributeType=S \
    --key-schema AttributeName=PK,KeyType=HASH AttributeName=SK,KeyType=RANGE \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 \
    --global-secondary-indexes \
        "[{\"IndexName\": \"GSI1\",\"KeySchema\":[{\"AttributeName\":\"GSI1PK\",\"KeyType\":\"HASH\"},{\"AttributeName\":\"GSI1SK\",\"KeyType\":\"RANGE\"}],\"Projection\":{\"ProjectionType\":\"ALL\"},\"ProvisionedThroughput\":{\"ReadCapacityUnits\":5,\"WriteCapacityUnits\":5}}]" || true

echo "🧪 Running Go Integration Tests..."
cd backend
go test -v -tags=integration ./...
cd ..

echo "💨 Running Lambda Smoke Test..."
aws --endpoint-url=$ENDPOINT lambda invoke \
    --function-name interactions-api \
    --cli-binary-format raw-in-base64-out \
    --payload '{"path": "/api/v1/likes", "httpMethod": "GET", "queryStringParameters": {"slug": "smoke-test"}}' \
    response.json > /dev/null

STATUS_CODE=$(grep -o '"statusCode":[0-9]*' response.json | cut -d: -f2)
rm response.json

if [ "$STATUS_CODE" == "200" ]; then
    echo "✅ Smoke test passed! (Status: 200)"
else
    echo "❌ Smoke test failed! (Status: $STATUS_CODE)"
    exit 1
fi

echo "🧹 Cleaning up..."
docker-compose stop localstack > /dev/null
rm -f backend/*.zip backend/bootstrap
echo "🎉 All tests completed successfully and environment cleaned up!"
