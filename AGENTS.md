# jyatesdotdev-api

Go Lambda backend for jyates.dev: four HTTP functions behind API Gateway plus an
S3-triggered notification function, DynamoDB single-table storage, and SES email/contact
lists. All Go code is in `backend/` (see `backend/AGENTS.md`).
This level holds LocalStack tooling, CI, and (stale) spec docs.

## ⚠️ Docs warning

`requirements.md`, `design.md`, `tasks.md` are **stale migration-planning docs**. The
code has diverged from them: reCAPTCHA was replaced with honeypot fields, comments are
pending unless `AUTO_APPROVE=true`, and likes are
keyed by `X-Visitor-Id` header (with a per-IP daily rate limit on adds).
**Trust the code, not these docs.**

## Local dev (LocalStack, run from this directory)

- `docker-compose.yml` — LocalStack 3.0.2 on loopback :4566 (lambda, apigateway,
  dynamodb, ses, s3, ssm, iam) + a `backend` container on loopback :8080 that runs
  **only the interactions binary**. It deliberately has no global container names;
  orchestration scripts use isolated Compose project names.
  `localstack-init/01_init.sh` auto-creates the table (with TTL on `expiresAt`) and SES
  identities when LocalStack is healthy.
- `./deploy-localstack.sh` — builds and deploys interactions/contact/admin/notifications
  to LocalStack (**not the authorizer**; builds GOARCH=amd64, unlike prod's arm64).
  Subscription confirmation and delivery use SES v1 plus DynamoDB-backed local topic
  contacts because SESv2 contact lists require LocalStack Pro.
- `./setup-apigw-localstack.sh` — builds the REST API tree, writes the id to `.api_id`.
  Admin routes get `authorization-type NONE` — **local admin is unauthenticated**.
- `./seed-localstack.sh` — seeds post `an-introduction` (likeCount 10), a pending comment
  from Alice, an approved comment from Bob, and SES identities. The
  `jyatesdotdev-integration` E2E specs assert on these exact values.
- `./run-e2e.sh` — full local cycle: LocalStack up → deploy → create table →
  `go test -tags=integration ./...` → smoke invoke → cleanup.
- Full-stack (with frontend): use `../jyatesdotdev-integration/start-dev.sh` instead.

## CI (`.github/workflows/`)

- `deploy.yml` — builds all 5 lambdas (linux/arm64, `provided.al2023`), zips to
  `s3://<ARTIFACTS_BUCKET>/lambdas/<sha>/`, then dispatches `deploy_api` to
  `jyatesdotdev-infra` and `run_e2e` to `jyatesdotdev-integration`.
- `security.yml` — gosec + `go test -short ./...`.
- `codeql.yml` — CodeQL Go analysis.
- CI reads the Go version from `backend/go.mod` (currently 1.26.2) via `go-version-file`, so there is no version pin to drift.
