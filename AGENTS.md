# jyatesdotdev-api

Go Lambda backend for jyates.dev: four functions behind API Gateway, DynamoDB
single-table storage, SES email. All Go code is in `backend/` (see `backend/AGENTS.md`).
This level holds LocalStack tooling, CI, and (stale) spec docs.

## ⚠️ Docs warning

`requirements.md`, `design.md`, `tasks.md` are **stale migration-planning docs**. The
code has diverged from them: reCAPTCHA was replaced with honeypot fields, comments are
**auto-approved by default** (`AUTO_APPROVE=false` holds them pending), and likes are
keyed by `X-Visitor-Id` header (with a per-IP daily rate limit on adds). Parts of
README.md are also stale (says Go 1.22 and documents reCAPTCHA/IP-based likes).
**Trust the code, not these docs.**

## Local dev (LocalStack, run from this directory)

- `docker-compose.yml` — LocalStack 3.0.2 on :4566 (lambda, apigateway, dynamodb, ses,
  ssm) + a `backend` container on :8080 that runs **only the interactions binary**.
  `localstack-init/01_init.sh` auto-creates the table (with TTL on `expiresAt`) and SES
  identities when LocalStack is healthy.
- `./deploy-localstack.sh` — builds and deploys interactions/contact/admin to LocalStack
  (**not the authorizer**; builds GOARCH=amd64, unlike prod's arm64).
- `./setup-apigw-localstack.sh` — builds the REST API tree, writes the id to `.api_id`.
  Admin routes get `authorization-type NONE` — **local admin is unauthenticated**.
- `./seed-localstack.sh` — seeds post `an-introduction` (likeCount 10), a pending comment
  from Alice, an approved comment from Bob, and SES identities. The
  `jyatesdotdev-integration` E2E specs assert on these exact values.
- `./run-e2e.sh` — full local cycle: LocalStack up → deploy → create table →
  `go test -tags=integration ./...` → smoke invoke → cleanup.
- Full-stack (with frontend): use `../jyatesdotdev-integration/start-dev.sh` instead.

## CI (`.github/workflows/`)

- `deploy.yml` — builds all 4 lambdas (linux/arm64, `provided.al2023`), zips to
  `s3://<ARTIFACTS_BUCKET>/lambdas/<sha>/`, then dispatches `deploy_api` to
  `jyatesdotdev-infra` and `run_e2e` to `jyatesdotdev-integration`.
- `security.yml` — gosec + `go test -short ./...`.
- `codeql.yml` — CodeQL Go analysis.
- Note: CI pins Go 1.22 while `backend/go.mod` says 1.26.2 — a known inconsistency.
