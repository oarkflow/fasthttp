#!/usr/bin/env bash
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKDIR="$(mktemp -d)"
PORT="${PORT:-3000}"
ADDR="127.0.0.1:${PORT}"
BASE_URL="${FH_BASE_URL:-http://${ADDR}}"
SERVER_PID=""
COOKIE_JAR="${WORKDIR}/cookies.txt"
FAILURES=0
PASSES=0

cleanup() {
  if [[ -n "${SERVER_PID}" ]]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    wait "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "${WORKDIR}"
}
trap cleanup EXIT

log() {
  printf '%s\n' "$*"
}

record_pass() {
  PASSES=$((PASSES + 1))
  log "ok   $1"
}

record_fail() {
  FAILURES=$((FAILURES + 1))
  log "FAIL $1"
  if [[ $# -gt 1 ]]; then
    log "     $2"
  fi
}

request() {
  local method="$1"
  local path="$2"
  shift 2
  local body=""
  if [[ $# -gt 0 && "$1" != -* ]]; then
    body="$1"
    shift
  fi
  local out="${WORKDIR}/response.txt"
  local args=(-sS -o "${out}" -D "${WORKDIR}/headers.txt" -w "%{http_code}" -X "${method}")
  if [[ -n "${body}" ]]; then
    args+=(-d "${body}")
  fi
  while [[ $# -gt 0 ]]; do
    args+=("$1")
    shift
  done
  local code
  code="$(curl "${args[@]}" "${BASE_URL}${path}" 2>"${WORKDIR}/curl.err")"
  local curl_status=$?
  cat "${WORKDIR}/headers.txt" "${out}" >"${WORKDIR}/last.txt"
  if [[ ${curl_status} -ne 0 ]]; then
    code="curl-error"
    cat "${WORKDIR}/curl.err" >>"${WORKDIR}/last.txt"
  fi
  printf '%s' "${code}"
}

expect_status() {
  local name="$1"
  local expected="$2"
  shift 2
  local code
  code="$(request "$@")"
  if [[ "${code}" == "${expected}" ]]; then
    record_pass "${name}"
  else
    record_fail "${name}" "expected HTTP ${expected}, got ${code}: $(tr '\n' ' ' <"${WORKDIR}/last.txt" | cut -c1-220)"
  fi
}

expect_contains() {
  local name="$1"
  local expected_status="$2"
  local needle="$3"
  shift 3
  local code
  code="$(request "$@")"
  if [[ "${code}" == "${expected_status}" ]] && grep -Fq "${needle}" "${WORKDIR}/last.txt"; then
    record_pass "${name}"
  else
    record_fail "${name}" "expected HTTP ${expected_status} and '${needle}', got ${code}: $(tr '\n' ' ' <"${WORKDIR}/last.txt" | cut -c1-220)"
  fi
}

expect_header() {
  local name="$1"
  local expected_status="$2"
  local header="$3"
  local value="$4"
  shift 4
  local code
  code="$(request "$@")"
  if [[ "${code}" == "${expected_status}" ]] && grep -Eiq "^${header}: ${value}" "${WORKDIR}/headers.txt"; then
    record_pass "${name}"
  else
    record_fail "${name}" "expected HTTP ${expected_status} and ${header}: ${value}, got ${code}: $(tr '\n' ' ' <"${WORKDIR}/last.txt" | cut -c1-220)"
  fi
}

start_server() {
  if [[ -n "${FH_BASE_URL:-}" ]]; then
    log "Using existing server at ${FH_BASE_URL}"
    return
  fi
  log "Starting all-middlewares example at ${BASE_URL}"
  (
    cd "${ROOT}" || exit 1
    FH_DATA_DIR="${WORKDIR}/fh-data" go run ./examples/all-middlewares -addr ":${PORT}" -upstream "http://127.0.0.1:1" >"${WORKDIR}/server.log" 2>&1
  ) &
  SERVER_PID=$!

  for _ in {1..80}; do
    if curl -fsS "${BASE_URL}/health" >/dev/null 2>&1; then
      return
    fi
    if ! kill -0 "${SERVER_PID}" >/dev/null 2>&1; then
      record_fail "server start" "$(cat "${WORKDIR}/server.log")"
      exit 1
    fi
    sleep 0.25
  done
  record_fail "server start" "timed out waiting for ${BASE_URL}; log: $(cat "${WORKDIR}/server.log")"
  exit 1
}

start_server

expect_contains "health" 200 '"status":"ok"' GET /health
expect_contains "request id" 200 '"request_id"' GET /demo/request-id
expect_header "security headers" 200 "X-Content-Type-Options" "nosniff" GET /demo/security-headers -I
expect_status "basic auth denied" 401 GET /demo/basic-auth
expect_contains "basic auth accepted" 200 '"authenticated"' GET /demo/basic-auth -u admin:password
expect_contains "api key accepted" 200 '"api key accepted"' GET /demo/api-key -H "X-API-Key: demo-key-123"
expect_status "api key denied" 401 GET /demo/api-key -H "X-API-Key: wrong-key"
expect_contains "api version accepted" 200 '"api_version":"2026-01"' GET /demo/api-version -H "Accept-Version: 2026-01"
expect_status "api version denied" 400 GET /demo/api-version -H "Accept-Version: 2099-99"
expect_contains "policy" 200 '"policy applied"' POST /demo/policy ""
expect_contains "body limit under" 200 '"size":14' POST /demo/body-limit '{"short":"ok"}' -H "Content-Type: application/json"
expect_status "body limit over" 413 POST /demo/body-limit '{"data":"this exceeds the 100 byte ceiling for this route and will be rejected with enough extra bytes"}' -H "Content-Type: application/json"
expect_status "timeout" 503 GET /demo/timeout

expect_status "rate limit 1" 200 GET /demo/rate-limit
expect_status "rate limit 2" 200 GET /demo/rate-limit
expect_status "rate limit 3" 200 GET /demo/rate-limit
expect_status "rate limit 4" 429 GET /demo/rate-limit

expect_header "cache miss" 200 "X-Cache" "MISS" GET /demo/cache
expect_header "cache hit" 200 "X-Cache" "HIT" GET /demo/cache
expect_contains "compress plain" 200 "hello world!" GET /demo/compress
expect_header "compress gzip" 200 "Content-Encoding" "gzip" GET /demo/compress -H "Accept-Encoding: gzip"

csrf_code="$(curl -sS -c "${COOKIE_JAR}" "${BASE_URL}/demo/csrf-token" -o "${WORKDIR}/csrf.json" -w "%{http_code}")"
CSRF="$(sed -n 's/.*"csrf_token":"\([^"]*\)".*/\1/p' "${WORKDIR}/csrf.json")"
if [[ "${csrf_code}" == "200" && -n "${CSRF}" ]]; then
  record_pass "csrf token"
else
  record_fail "csrf token" "expected token response, got ${csrf_code}: $(cat "${WORKDIR}/csrf.json")"
fi
expect_contains "csrf submit accepted" 200 '"CSRF token accepted"' POST /demo/csrf-submit "" -b "${COOKIE_JAR}" -H "X-CSRF-Token: ${CSRF}"
expect_status "csrf submit denied" 403 POST /demo/csrf-submit ""

expect_contains "contract accepted" 200 '"contract satisfied"' POST /demo/contract '{}' -H "Content-Type: application/json" -H "X-Client-ID: abc"
expect_status "contract missing header" 400 POST /demo/contract '{}' -H "Content-Type: application/json"
expect_status "contract wrong type" 415 POST /demo/contract "" -H "Content-Type: text/plain"

SIG_BODY='{"hello":"world"}'
TIMESTAMP="$(date +%s)"
SIG="$(printf '%s' "${TIMESTAMP}.${SIG_BODY}" | openssl dgst -sha256 -hmac "hmac-demo-secret" -binary | xxd -p -c 256)"
expect_contains "signature accepted" 200 '"signature verified"' POST /demo/signature "${SIG_BODY}" -H "Content-Type: application/json" -H "X-Signature: t=${TIMESTAMP},sig=${SIG}"
expect_status "signature denied" 401 POST /demo/signature "${SIG_BODY}" -H "Content-Type: application/json" -H "X-Signature: t=${TIMESTAMP},sig=bad"

NONCE="nonce-${RANDOM}-${RANDOM}"
expect_contains "replay accepted" 200 '"nonce accepted"' POST /demo/replay "" -H "X-Nonce: ${NONCE}"
expect_status "replay denied" 409 POST /demo/replay "" -H "X-Nonce: ${NONCE}"
expect_contains "ip whitelist" 200 '"ip allowed"' GET /demo/ip-whitelist
expect_contains "actor" 200 '"user_id":"user-42"' POST /demo/actor "" -H "X-User-ID: user-42"
expect_contains "rewrite" 200 "this route was reached" GET /old-api/hello

IDEMP="order-${RANDOM}-${RANDOM}"
expect_status "idempotency first" 201 POST /demo/idempotency '{}' -H "Content-Type: application/json" -H "Idempotency-Key: ${IDEMP}"
expect_status "idempotency replay" 201 POST /demo/idempotency '{}' -H "Content-Type: application/json" -H "Idempotency-Key: ${IDEMP}"
expect_status "idempotency conflict" 409 POST /demo/idempotency '{"different":"payload"}' -H "Content-Type: application/json" -H "Idempotency-Key: ${IDEMP}"
expect_contains "lifecycle" 200 '"lifecycle hooks executed"' POST /demo/lifecycle ""

expect_contains "workflow basic" 200 '"type":"basic"' POST /demo/workflow/basic ""
expect_contains "workflow conditional vip" 200 '"discount":"10%"' POST /demo/workflow/conditional "" -H "X-Plan: vip"
expect_contains "workflow conditional standard" 200 '"discount":"0%"' POST /demo/workflow/conditional "" -H "X-Plan: standard"
expect_contains "workflow branch vip" 200 '"route":"vip"' POST /demo/workflow/branch "" -H "X-Plan: vip"
expect_contains "workflow branch standard" 200 '"route":"standard"' POST /demo/workflow/branch "" -H "X-Plan: standard"
expect_contains "workflow branch default" 200 '"route":"default"' POST /demo/workflow/branch "" -H "X-Plan: other"
expect_contains "workflow parallel" 200 '"type":"parallel"' POST /demo/workflow/parallel ""

expect_contains "circuit closed" 200 '"circuit closed' GET /demo/circuit-breaker
expect_status "circuit fail 1" 500 GET "/demo/circuit-breaker?fail=true"
expect_status "circuit fail 2" 500 GET "/demo/circuit-breaker?fail=true"
expect_status "circuit open" 503 GET /demo/circuit-breaker
expect_status "proxy unreachable" 502 GET /demo/proxy/test
expect_contains "metrics" 200 "fh_requests_total" GET "/metrics?format=prometheus"
expect_status "static" 200 GET /static/test.json
expect_contains "named route target" 200 "hello world" GET /hello/world
expect_contains "named route generated" 200 '"/hello/world"' GET /named-route-example
expect_status "redirect old home" 302 GET /old-home
expect_status "redirect named route" 302 GET /go-hello
expect_contains "params" 200 '"id":"42"' GET "/demo/params/42?filter=active"

expect_contains "codec json" 200 '"name":"Alice"' POST /demo/codecs '{"name":"Alice","age":30}' -H "Content-Type: application/json"
expect_contains "codec form" 200 '"name":"Bob"' POST /demo/codecs 'name=Bob&role=admin' -H "Content-Type: application/x-www-form-urlencoded"
expect_contains "codec xml" 200 '"name":"Carol"' POST /demo/codecs '<root><name>Carol</name></root>' -H "Content-Type: application/xml"
expect_contains "codec multipart" 200 '"name":"Dave"' POST /demo/codecs "" -F "name=Dave" -F "age=28"
expect_status "error handling" 422 GET /demo/error
expect_status "panic recovery" 500 GET /demo/panic
expect_status "queue enqueue" 202 POST /demo/queue ""
expect_status "queue stats" 200 GET /demo/queue/stats
expect_contains "journal" 200 '"every request' GET /demo/journal
expect_status "cors preflight" 204 OPTIONS /demo/cors "" -H "Origin: http://localhost:5173" -H "Access-Control-Request-Method: GET"
expect_header "cors actual" 200 "Access-Control-Allow-Origin" "http://localhost:5173" GET /demo/cors -H "Origin: http://localhost:5173"
expect_status "outbox" 202 POST /demo/outbox ""
WEBHOOK="evt-${RANDOM}-${RANDOM}"
expect_status "inbox accepted" 202 POST /demo/inbox '{}' -H "Content-Type: application/json" -H "X-Webhook-ID: ${WEBHOOK}"
expect_status "inbox duplicate" 202 POST /demo/inbox '{}' -H "Content-Type: application/json" -H "X-Webhook-ID: ${WEBHOOK}"

log ""
log "${PASSES} passed, ${FAILURES} failed"
if [[ ${FAILURES} -ne 0 ]]; then
  exit 1
fi
