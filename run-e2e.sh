#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_PROJECT_NAME="jyatesdotdev-api-e2e"
ENDPOINT="http://127.0.0.1:4566"
STACK_STARTED=false

compose() {
  docker compose --project-name "$COMPOSE_PROJECT_NAME" --file "$SCRIPT_DIR/docker-compose.yml" "$@"
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ "$STACK_STARTED" == "true" ]]; then
    echo "Cleaning up ${COMPOSE_PROJECT_NAME}..."
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
    rm -f \
      "$SCRIPT_DIR/response.json" \
      "$SCRIPT_DIR/backend/bootstrap" \
      "$SCRIPT_DIR/backend/admin.zip" \
      "$SCRIPT_DIR/backend/contact.zip" \
      "$SCRIPT_DIR/backend/interactions.zip"
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

wait_for() {
  local description="$1"
  local timeout_seconds="$2"
  shift 2
  local started_at=$SECONDS

  until "$@"; do
    if (( SECONDS - started_at >= timeout_seconds )); then
      echo "Timed out after ${timeout_seconds}s waiting for ${description}." >&2
      return 1
    fi
    sleep 2
  done
}

services_ready() {
  local health
  health="$(curl --fail --silent --show-error "$ENDPOINT/_localstack/health" 2>/dev/null)" || return 1
  grep -qE '"dynamodb": "(running|available)"' <<<"$health" &&
    grep -qE '"lambda": "(running|available)"' <<<"$health"
}

table_ready() {
  aws --endpoint-url="$ENDPOINT" dynamodb describe-table \
    --table-name jyatesdotdev-state >/dev/null 2>&1
}

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

docker info >/dev/null 2>&1 || {
  echo "Docker is not running." >&2
  exit 1
}

running_project="$(docker ps --filter "label=com.docker.compose.project=${COMPOSE_PROJECT_NAME}" --format '{{.Names}}' | paste -sd, -)"
port_owner="$(docker ps --filter publish=4566 --format '{{.Names}}' | paste -sd, -)"
if [[ -n "$running_project" ]]; then
  echo "Compose project ${COMPOSE_PROJECT_NAME} is already running: ${running_project}" >&2
  exit 1
fi
if [[ -n "$port_owner" ]]; then
  echo "Port 4566 is already published by Docker container(s): ${port_owner}" >&2
  exit 1
fi
if command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:4566 -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Port 4566 is already in use by another process." >&2
  lsof -nP -iTCP:4566 -sTCP:LISTEN >&2
  exit 1
fi

compose down --volumes --remove-orphans >/dev/null 2>&1 || true
echo "Starting isolated LocalStack project ${COMPOSE_PROJECT_NAME}..."
STACK_STARTED=true
compose up --detach localstack

if ! wait_for "LocalStack services" 120 services_ready || ! wait_for "DynamoDB initialization" 60 table_ready; then
  compose logs localstack >&2 || true
  exit 1
fi

echo "Deploying Lambdas to LocalStack..."
cd "$SCRIPT_DIR"
./deploy-localstack.sh

echo "Running Go integration tests..."
(
  cd backend
  go test -v -tags=integration ./...
)

echo "Running Lambda smoke test..."
aws --endpoint-url="$ENDPOINT" lambda invoke \
  --function-name interactions-api \
  --cli-binary-format raw-in-base64-out \
  --payload '{"path":"/api/v1/likes","httpMethod":"GET","queryStringParameters":{"slug":"smoke-test"}}' \
  response.json >/dev/null

status_code="$(jq -r '.statusCode // empty' response.json)"
if [[ "$status_code" != "200" ]]; then
  echo "Smoke test failed with status ${status_code:-unknown}." >&2
  cat response.json >&2
  exit 1
fi

echo "All API integration tests passed."
