# jyatesdotdev-api

This repository contains the backend services for `jyates.dev`. It is built using **Go**, engineered to run as AWS Lambda functions behind an API Gateway, and leverages DynamoDB for serverless, on-demand performance.

## Architecture

* **Language**: Go 1.22
* **Computing**: AWS Lambda (ARM64)
* **Database**: Amazon DynamoDB
* **Orchestration**: AWS API Gateway + AWS SES
* **Structure**: Clean Architecture (Handlers -> Services -> Repositories)

The backend consists of highly isolated serverless functions:
- `interactions`: Handles public interaction endpoints (fetching/submitting comments and likes).
- `contact`: Private endpoint for user emails via AWS SES.
- `admin`: Requires Bearer Auth; used for managing comments and dashboard state.
- `authorizer`: A custom API Gateway Authorizer function that validates admin credentials.

## Local Development

You generally don't run the API by itself. It is designed to run in a mocked AWS ecosystem (LocalStack) alongside the frontend. 

1. Navigate to the Integration repository: `cd ../jyatesdotdev-integration`
2. Run the boot script: `./start-dev.sh`

This script will compile the Go binaries, start LocalStack, deploy the Lambdas, create DynamoDB tables, map mock routes in API Gateway, and boot the frontend.

## Testing

To run the unit tests natively:
```bash
cd backend
go test -short ./...
```

To run full End-to-End integration tests (hitting the database via API Gateway), use the playwright suite in the `jyatesdotdev-integration` repository.

## Deployment pipeline

Deployments are handled by GitHub Actions.
1. When code is pushed to `main`, Go code formatting and tests are enforced via the `Security Scans` workflow.
2. On pass, the `Frontend Deployment` step compiles the Go binaries, zips them, and ships them to a deployment bucket in AWS S3.
3. Finally, it uses `repository_dispatch` to fire a webhook to the `jyatesdotdev-infra` repository. The infra repository then runs Terraform to point the live Lambda functions at these newly uploaded zip files.

### Required Secrets
To enable the deployment pipeline, provide the following GitHub Action secrets:
* `AWS_ACCESS_KEY_ID`
* `AWS_SECRET_ACCESS_KEY`
* `ARTIFACT_BUCKET` (The name of the S3 bucket where Lambda zip files are stored)
* `INFRA_REPO_PAT` (A GitHub Personal Access Token to trigger `jyatesdotdev-infra`)
