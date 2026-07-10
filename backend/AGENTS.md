# backend/ — Go module

Module `github.com/jyates/jyatesdotdev-api/backend`, Go 1.26.2. Key deps: chi v5 routing
via `aws-lambda-go-api-proxy` (chi adapter), aws-sdk-go-v2 (dynamodb + expression),
testify, bluemonday (sanitization), google/uuid.

## Architecture

Handlers → Services → Repositories, wired per-lambda in `cmd/*/main.go`. Each HTTP lambda
runs in two modes: `lambda.Start(chiLambda.ProxyWithContext)` when
`AWS_LAMBDA_FUNCTION_NAME` is set, else a plain `http.Server` on `PORT` (default 8080)
for local/container use.

Each HTTP lambda builds its chi router in a single `newRouter(...)` function, called from
both `init()` (Lambda path) and `main()`'s local-server branch — route changes go there.

## Commands

- Unit tests: `go test -short ./...`
- Integration tests: `go test -tags=integration ./...` — needs LocalStack on :4566
  (use `../run-e2e.sh` which handles setup). Creates its own `integration-test-table`.
- Prod build (matches CI): `GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bin/<fn>/bootstrap cmd/<fn>/main.go`
- Local LocalStack build uses amd64 (see `../deploy-localstack.sh`).

## Testing patterns

Handler unit tests use testify/mock with hand-written `MockService`/`MockRepository`
types and `httptest` — one `TestXxx` func per case, not table-driven. Follow that style.

## Env vars

`DYNAMODB_TABLE_NAME` (default `jyatesdotdev-state`), `DYNAMODB_ENDPOINT` (local
override — also forces region us-east-1), `SES_FROM_EMAIL`, `SES_ADMIN_EMAIL`,
`SES_ENDPOINT` (local → SES v1 client; unset → SES v2), `ADMIN_USERNAME`,
`ADMIN_PASSWORD` (read by the authorizer — NOT from SSM, despite prod SSM params
existing), `AUTO_APPROVE` (exactly `"true"` auto-approves new comments; unset or any
other value leaves them pending), `PORT`.

## DynamoDB single-table design (`jyatesdotdev-state`, PK/SK + GSI1)

| PK | SK | Meaning |
|---|---|---|
| `POST#<slug>` | `METADATA` | post like count (atomic `ADD likeCount`) |
| `POST#<slug>` | `LIKE#<visitorID>` | post like (existence = liked) |
| `POST#<slug>` | `COMMENT#<uuid>` | comment; GSI1PK=`STATUS#<status>`, GSI1SK=`POST#<slug>#<ts>` |
| `COMMENT#<uuid>` | `LIKE#<visitorID>` | comment like |
| `POST#<slug>#USER#<visitorID>` | `LIKE#COMMENT#<uuid>` | reverse index: visitor's comment likes |
| `RATELIMIT#IP#<ip>` | `LIKES#<yyyy-mm-dd UTC>` | per-IP daily like-add counter (cap 100 → 429; TTL via `expiresAt`, +48h) |
| `RATELIMIT#IP#<ip>` | `VISITS#<yyyy-mm-dd UTC>` | per-IP daily visit-record counter (cap 20 → 429; TTL via `expiresAt`, +48h) |
| `STATS#GEO` | `COUNTRY#<alpha2>` | per-country visit counter (atomic `ADD count`; stores `countryName`) |

Approved comments are read via GSI1 (`STATUS#approved` + `begins_with(POST#<slug>#)`).

See `cmd/AGENTS.md` for the four entry points and `internal/AGENTS.md` for package
responsibilities and behavioral gotchas.
