# jyatesdotdev-api

Go backend for [jyates.dev](https://jyates.dev) — four Lambda functions behind API Gateway.

## Architecture

- **Language**: Go 1.26 (CI reads the version from `backend/go.mod`)
- **Runtime**: AWS Lambda (ARM64, `provided.al2023`)
- **Database**: DynamoDB (KMS-encrypted, on-demand)
- **Email**: SES v2 (sends from `blog@jyates.dev`)
- **Router**: chi (via `aws-lambda-go-api-proxy`)
- **Structure**: Handlers → Services → Repositories

### Functions

| Function | Purpose |
|---|---|
| `interactions` | Likes and comments (GET/POST) |
| `contact` | Contact form → SES email |
| `admin` | Comment moderation (approve/reject/delete) |
| `authorizer` | Basic Auth for admin endpoints |

### API Routes (via CloudFront `/api/*`)

```
GET  /api/v1/likes?slug=...
POST /api/v1/likes                    {slug}
GET  /api/v1/comments?slug=...
POST /api/v1/comments                 {slug, content, authorName, authorEmail, website}
POST /api/v1/comments/:id/like        {slug}
POST /api/v1/contact                  {name, email, message, website}
GET  /api/v1/admin/comments?status=...
PUT  /api/v1/admin/comments/:id       {slug, status}
DELETE /api/v1/admin/comments/:id     {slug}
```

### Spam Protection & Identity

- **Honeypot**: `website` is a hidden form field that must be empty; submissions that fill it are rejected (contact returns a fake 200 so bots aren't tipped off). There is no reCAPTCHA.
- **Likes** are deduplicated by the `X-Visitor-Id` request header (required — requests without it get a 400), not by IP.
- **Comments** are created **auto-approved**; the admin dashboard can flip them to `pending`/`rejected` after the fact. The client IP (first entry of `X-Forwarded-For`) is stored on comment records for moderation context only.

### DynamoDB Schema

Single table `jyatesdotdev-state` with single-table design:

| PK | SK | Purpose |
|---|---|---|
| `POST#<slug>` | `METADATA` | Post metadata (likeCount) |
| `POST#<slug>` | `LIKE#<visitorID>` | Like record (existence = liked) |
| `POST#<slug>` | `COMMENT#<uuid>` | Comment (content, author, status, likeCount) |
| `COMMENT#<uuid>` | `LIKE#<visitorID>` | Comment like tracking |
| `POST#<slug>#USER#<visitorID>` | `LIKE#COMMENT#<uuid>` | User-comment like cross-reference |

GSI1 (`GSI1PK`/`GSI1SK`) indexes comments by status for admin queries (e.g., `STATUS#approved`, `STATUS#pending`).

## Testing

```bash
cd backend
go test -short ./...
```

## Deployment

Pushes to `main` (under `backend/**`) or manual `workflow_dispatch` trigger the pipeline:

1. Build four Lambda binaries (cross-compiled for `linux/arm64`)
2. Zip and upload to the artifacts S3 bucket under `lambdas/<git-sha>/`
3. Dispatch `deploy_api` to `jyatesdotdev-infra` with the artifact locations, and `run_e2e` to `jyatesdotdev-integration`

### Manual Trigger

```bash
gh workflow run deploy.yml --repo <owner>/jyatesdotdev-api --ref main
```

This builds, uploads, and dispatches to infra automatically.

### Required Secrets & Variables

| Type | Name | Description |
|---|---|---|
| Secret | `AWS_ROLE_ARN` | GitHub OIDC deploy role ARN |
| Secret | `INFRA_REPO_PAT` | PAT to trigger `jyatesdotdev-infra` / `jyatesdotdev-integration` dispatches |
| Variable | `ARTIFACTS_BUCKET` | S3 bucket for Lambda zips |
| Variable | `AWS_REGION` | `us-west-2` |
