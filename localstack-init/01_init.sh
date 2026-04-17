#!/bin/bash

# Initialize DynamoDB
awslocal dynamodb create-table \
    --table-name jyatesdotdev-state \
    --attribute-definitions \
        AttributeName=PK,AttributeType=S \
        AttributeName=SK,AttributeType=S \
        AttributeName=GSI1PK,AttributeType=S \
        AttributeName=GSI1SK,AttributeType=S \
    --key-schema \
        AttributeName=PK,KeyType=HASH \
        AttributeName=SK,KeyType=RANGE \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 \
    --global-secondary-indexes '[
        {
            "IndexName": "GSI1",
            "KeySchema": [
                {"AttributeName": "GSI1PK", "KeyType": "HASH"},
                {"AttributeName": "GSI1SK", "KeyType": "RANGE"}
            ],
            "Projection": {
                "ProjectionType": "ALL"
            },
            "ProvisionedThroughput": {
                "ReadCapacityUnits": 5,
                "WriteCapacityUnits": 5
            }
        }
    ]'

# Initialize SES
awslocal ses verify-email-identity --email-address test@jyates.dev
awslocal ses verify-email-identity --email-address admin@jyates.dev

# Initialize SSM Parameters for Admin Credentials
awslocal ssm put-parameter --name "/jyatesdotdev/admin/username" --type "String" --value "admin"
awslocal ssm put-parameter --name "/jyatesdotdev/admin/password" --type "SecureString" --value "password"
