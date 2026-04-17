#!/bin/bash
set -e

ENDPOINT="http://localhost:4566"
REGION="us-east-1"
ACCOUNT_ID="000000000000"

echo "🚀 Setting up API Gateway in LocalStack..."

# 1. Create REST API
API_ID=$(aws --endpoint-url=$ENDPOINT apigateway create-rest-api --name 'jyatesdotdev-api' --region $REGION | jq -r .id)
ROOT_ID=$(aws --endpoint-url=$ENDPOINT apigateway get-resources --rest-api-id $API_ID --region $REGION | jq -r '.items[] | select(.path=="/") | .id')

echo "API ID: $API_ID"

# Helper function to create resource
create_resource() {
    local PARENT_ID=$1
    local PATH_PART=$2
    aws --endpoint-url=$ENDPOINT apigateway create-resource --rest-api-id $API_ID --parent-id $PARENT_ID --path-part "$PATH_PART" --region $REGION | jq -r .id
}

# Helper function to create ANY method with Lambda Proxy integration
setup_lambda_proxy() {
    local RES_ID=$1
    local LAMBDA_NAME=$2
    local HTTP_METHOD=$3
    
    # Create method
    aws --endpoint-url=$ENDPOINT apigateway put-method \
        --rest-api-id $API_ID \
        --resource-id $RES_ID \
        --http-method "$HTTP_METHOD" \
        --authorization-type "NONE" \
        --region $REGION > /dev/null
    
    # Set up integration
    aws --endpoint-url=$ENDPOINT apigateway put-integration \
        --rest-api-id $API_ID \
        --resource-id $RES_ID \
        --http-method "$HTTP_METHOD" \
        --type AWS_PROXY \
        --integration-http-method POST \
        --uri "arn:aws:apigateway:$REGION:lambda:path/2015-03-31/functions/arn:aws:lambda:$REGION:$ACCOUNT_ID:function:$LAMBDA_NAME/invocations" \
        --region $REGION > /dev/null
}

# 2. Create /api/v1 structure
API_RES_ID=$(create_resource "$ROOT_ID" "api")
V1_RES_ID=$(create_resource "$API_RES_ID" "v1")

# 3. Create /likes
LIKES_RES_ID=$(create_resource "$V1_RES_ID" "likes")
setup_lambda_proxy "$LIKES_RES_ID" "interactions-api" "GET"
setup_lambda_proxy "$LIKES_RES_ID" "interactions-api" "POST"

# 4. Create /comments
COMMENTS_RES_ID=$(create_resource "$V1_RES_ID" "comments")
setup_lambda_proxy "$COMMENTS_RES_ID" "interactions-api" "GET"
setup_lambda_proxy "$COMMENTS_RES_ID" "interactions-api" "POST"

# 5. Create /comments/{commentId}/like
COMMENT_ID_RES_ID=$(create_resource "$COMMENTS_RES_ID" "{commentId}")
COMMENT_LIKE_RES_ID=$(create_resource "$COMMENT_ID_RES_ID" "like")
setup_lambda_proxy "$COMMENT_LIKE_RES_ID" "interactions-api" "POST"

# 6. Create /contact
CONTACT_RES_ID=$(create_resource "$V1_RES_ID" "contact")
setup_lambda_proxy "$CONTACT_RES_ID" "contact-api" "POST"

# 7. Create /admin/comments
ADMIN_RES_ID=$(create_resource "$V1_RES_ID" "admin")
ADMIN_COMMENTS_RES_ID=$(create_resource "$ADMIN_RES_ID" "comments")
setup_lambda_proxy "$ADMIN_COMMENTS_RES_ID" "admin-api" "GET"

# 8. Create /admin/comments/{commentId}
ADMIN_COMMENT_ID_RES_ID=$(create_resource "$ADMIN_COMMENTS_RES_ID" "{commentId}")
setup_lambda_proxy "$ADMIN_COMMENT_ID_RES_ID" "admin-api" "PUT"
setup_lambda_proxy "$ADMIN_COMMENT_ID_RES_ID" "admin-api" "DELETE"

# 9. Create Deployment
aws --endpoint-url=$ENDPOINT apigateway create-deployment --rest-api-id $API_ID --stage-name v1 --region $REGION > /dev/null

echo "✅ API Gateway set up successfully!"
echo "Endpoint: $ENDPOINT/restapis/$API_ID/v1/_user_request_"
echo "$API_ID" > .api_id
