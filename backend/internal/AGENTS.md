# internal/ — packages

| Package | Responsibility |
|---|---|
| `db` | `DynamoDBAPI` interface + `Client{api, TableName}`; the interface exists so repos are mockable. `NewClient` reads `DYNAMODB_TABLE_NAME` / `DYNAMODB_ENDPOINT` (local endpoint forces region us-east-1). |
| `interactions` | Likes + comments: handler (`Routes()` for likes, `CommentRoutes()` for comments), service, DynamoDB repo. Atomic like counts via `TransactWriteItems` + `ADD likeCount`. |
| `visits` | Geo/visitor map. `GeoRoutes()` serves `GET /geo` (whereami — reflects the `CloudFront-Viewer-*` request headers straight back). `Routes()` serves `GET /visits` (aggregate per-country counts, sorted desc, plus caller's own country as `you`) and `POST /visits` (atomically bumps `PK=STATS#GEO`, `SK=COUNTRY#<alpha2>` via `TransactWriteItems` + `ADD count`, gated by a per-IP daily rate limit of **20/day**). Requests without a valid 2-letter `CloudFront-Viewer-Country` (e.g. local dev) are a silent 204 no-op. |
| `admin` | Comment moderation: list by status (defaults to `pending`), update status (`approved`/`pending`/`rejected`), delete. |
| `auth` | Basic-Auth TOKEN authorizer. Reads `ADMIN_USERNAME`/`ADMIN_PASSWORD` **env vars** — denies if unset. |
| `email` | `Service` interface. SES **v1** when `SES_ENDPOINT` set (LocalStack — v2 is pro-only), SES **v2** in prod. No-op client when `SES_FROM_EMAIL`/`SES_ADMIN_EMAIL` unset. |
| `contact` | Contact-form handler: honeypot + validation → email. |
| `recaptcha` | **DEAD CODE** — not imported anywhere since the honeypot migration. Do not wire it back in without being asked. |

## Behavioral gotchas (things the docs get wrong)

- **Likes are keyed by the `X-Visitor-Id` request header, not IP.** Requests without the
  header get a 400. But like ADDs are also rate-limited per client IP — 100/day (UTC),
  enforced atomically in the like transaction via a `RATELIMIT#IP#<ip>` counter item;
  exceeding it returns `ErrRateLimited` → HTTP 429. `extractIP()` supplies the IP and
  also stamps `ipAddress` on comment records for moderation context.
- **Comments are auto-approved by default** — status `"approved"` on create unless the
  env var `AUTO_APPROVE` is exactly `"false"`, in which case new comments are `pending`
  (the admin UI can still flip statuses either way).
- The admin-notification email on new comments is sent **synchronously and best-effort**
  (5s timeout from the request context; a failed send is logged, never fails the create).
- **Spam protection is a honeypot**: a hidden `Website` field. Contact returns a fake
  200 when it's filled (don't tip off bots); interactions returns `ErrHoneypot`.
- Comment content is sanitized with bluemonday `StrictPolicy`.
- Admin credentials: the authorizer reads **env vars only** (Terraform injects them in
  prod). Prod SSM params `/jyatesdotdev/admin/*` exist purely as the operator's record of
  the generated password — nothing reads SSM.
- The credential comparison in `auth` uses `crypto/subtle.ConstantTimeCompare` for both
  username and password (both evaluated, no short-circuit).

## Tests

Unit tests are handler-level with testify/mock (hand-written mocks) and `httptest`.
The repo-root `integration_test.go` (build tag `integration`) asserts the default
auto-approve flow (new comments immediately `approved`) — don't run it with
`AUTO_APPROVE=false`.
