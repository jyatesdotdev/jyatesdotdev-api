# jyatesdotdev-api

Go backend for [jyates.dev](https://jyates.dev) — four Lambda functions behind API Gateway.

## Architecture

- **Language**: Go 1.22
- **Runtime**: AWS Lambda (ARM64, `provided.al2023`)
- **Database**: DynamoDB (KMS-encrypted, on-demand)
- **Email**: SES v2 (sends from `blog@jyates.dev`)
- **Router**: chi (via `aws-lambda-go-api-proxy`)
- **Structure**: Handlers → Services → Repositories

### Functions

| Function | Purpose | reCAPTCHA Action |
|---|---|---|
| `interactions` | Likes and comments (GET/POST) | `like`, `comment`, `comment_like` |
| `contact` | Contact form → SES email | `contact_form` |
| `admin` | Comment moderation (approve/reject/delete) | — |
| `authorizer` | Basic Auth for admin endpoints | — |

### API Routes (via CloudFront `/api/*`)

```
GET  /api/v1/likes?slug=...
POST /api/v1/likes                    {slug, token}
GET  /api/v1/comments?slug=...
POST /api/v1/comments                 {slug, content, authorName, authorEmail, token}
POST /api/v1/comments/:id/like        {slug, token}
POST /api/v1/contact                  {name, email, message, recaptchaToken}
GET  /api/v1/admin/comments?status=...
PUT  /api/v1/admin/comments/:id       {slug, status}
DELETE /api/v1/admin/comments/:id     {slug}
```

### IP Handling

The `X-Forwarded-For` header passes through CloudFront → API Gateway with multiple IPs appended. The handler extracts only the first (client) IP for like deduplication.

## Testing

```bash
cd backend
go test -short ./...
```

## Deployment

Pushes to `main` (under `backend/**`) or manual `workflow_dispatch` trigger the pipeline:

1. Build four Lambda binaries (cross-compiled for `linux/arm64`)
2. Zip and upload to the artifacts S3 bucket under `lambdas/<git-sha>/`
3. Dispatch to `jyatesdotdev-infra` with the artifact locations

### Manual Trigger

```bash
gh workflow run deploy.yml --repo jyatesdotdev/jyatesdotdev-api --ref main
```

This builds, uploads, and dispatches to infra automatically.

### Required Secrets & Variables

| Type | Name | Description |
|---|---|---|
| Secret | `AWS_ROLE_ARN` | GitHub OIDC deploy role ARN |
| Secret | `INFRA_REPO_PAT` | PAT to trigger `jyatesdotdev-infra` dispatch |
| Variable | `ARTIFACTS_BUCKET` | S3 bucket for Lambda zips |
| Variable | `AWS_REGION` | `us-west-2` |
