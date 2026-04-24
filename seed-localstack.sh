#!/bin/bash
set -e

ENDPOINT="http://localhost:4566"
TABLE="jyatesdotdev-state"

echo "🌱 Seeding LocalStack DynamoDB..."

# Verify SES email identities
aws --endpoint-url=$ENDPOINT ses verify-email-identity --email-address test@jyates.dev 2>/dev/null || true
aws --endpoint-url=$ENDPOINT ses verify-email-identity --email-address admin@jyates.dev 2>/dev/null || true

# Add a post metadata
aws --endpoint-url=$ENDPOINT dynamodb put-item \
    --table-name $TABLE \
    --item '{
        "PK": {"S": "POST#an-introduction"},
        "SK": {"S": "METADATA"},
        "likeCount": {"N": "10"}
    }'

# Add a pending comment
aws --endpoint-url=$ENDPOINT dynamodb put-item \
    --table-name $TABLE \
    --item '{
        "PK": {"S": "POST#an-introduction"},
        "SK": {"S": "COMMENT#1"},
        "GSI1PK": {"S": "STATUS#pending"},
        "GSI1SK": {"S": "POST#an-introduction#2026-04-16T10:00:00Z"},
        "id": {"S": "1"},
        "slug": {"S": "an-introduction"},
        "authorName": {"S": "Alice"},
        "authorEmail": {"S": "alice@example.com"},
        "content": {"S": "This is a pending comment"},
        "status": {"S": "pending"},
        "createdAt": {"S": "2026-04-16T10:00:00Z"},
        "ipAddress": {"S": "127.0.0.1"}
    }'

# Add an approved comment
aws --endpoint-url=$ENDPOINT dynamodb put-item \
    --table-name $TABLE \
    --item '{
        "PK": {"S": "POST#an-introduction"},
        "SK": {"S": "COMMENT#2"},
        "GSI1PK": {"S": "STATUS#approved"},
        "GSI1SK": {"S": "POST#an-introduction#2026-04-16T11:00:00Z"},
        "id": {"S": "2"},
        "slug": {"S": "an-introduction"},
        "authorName": {"S": "Bob"},
        "authorEmail": {"S": "bob@example.com"},
        "content": {"S": "This is an approved comment"},
        "status": {"S": "approved"},
        "createdAt": {"S": "2026-04-16T11:00:00Z"},
        "ipAddress": {"S": "127.0.0.1"}
    }'

echo "✅ Seeding complete!"
