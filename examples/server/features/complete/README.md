# Complete TCPGuard feature walkthrough

## Purpose

This walkthrough exercises the complete `fh` + TCPGuard example as one application flow: route-level middleware, datasource lookups, intel feeds, abuse detection, business rules, HMAC/replay protection, audit envelopes, incidents, metrics, and management APIs.

Start the application first:

```bash
cd examples/server
go run .
```

Application server: `http://127.0.0.1:18184`

Management server: `http://127.0.0.1:18185`

## 1. Clean request passes through FH

```bash
curl -i http://127.0.0.1:18184/public
```

Expected: `200 OK`, `X-TCPGuard-Decision: allow`, and:

```json
{"ok":true,"message":"clean request allowed"}
```

## 2. Datasource and intel-backed denials

Memory datasource ban:

```bash
curl -i -H 'X-User-ID: banned-user' http://127.0.0.1:18184/public
```

Tenant JSON datasource lockdown:

```bash
curl -i -H 'X-Tenant-ID: locked-tenant' http://127.0.0.1:18184/public
```

File intel feed:

```bash
curl -i -H 'X-Forwarded-For: 203.0.113.10' http://127.0.0.1:18184/public
```

Expected: enforced TCPGuard responses with compact production-safe bodies. Development mode also shows matched rules such as `cache-banned-user`, `tenant-lockdown`, and `block-bad-ip`.

## 3. Behavioral and business-rule enforcement

Account takeover signals:

```bash
curl -i -X POST \
  -H 'X-User-ID: user-1' \
  -H 'X-New-Device: true' \
  -H 'X-Previous-Country: US' \
  -H 'X-Country: NP' \
  http://127.0.0.1:18184/api/v1/account/login
```

After-hours high-value payment:

```bash
curl -i -X POST \
  -H 'X-User-ID: manager-1' \
  -H 'X-User-Role: finance_approver' \
  -H 'X-Business-Amount: 1500000' \
  -H 'X-Outside-Hours: true' \
  http://127.0.0.1:18184/api/v1/payments/approve
```

Dynamic route ownership:

```bash
curl -i -X PUT \
  -H 'X-User-ID: user-1' \
  http://127.0.0.1:18184/api/users/user-2/order/order-9
```

Expected: challenge or block responses, plus structured decision logs and audit records.

## 4. Signed transfer allow and replay block

Create a valid signature for the exact request body:

```bash
BODY='{"to":"acct-2","amount":100}'
SIGN_RESPONSE="$(curl -s -X POST \
  -H 'X-Sign-Method: POST' \
  -H 'X-Sign-Path: /api/v1/transfers' \
  --data "$BODY" \
  http://127.0.0.1:18184/_demo/sign)"
printf '%s\n' "$SIGN_RESPONSE"
```

Use `signature`, `nonce`, and `timestamp` from that response:

```bash
curl -i -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: manager-1' \
  -H 'X-TCPGuard-Signature: <signature>' \
  -H 'X-TCPGuard-Nonce: <nonce>' \
  -H 'X-TCPGuard-Timestamp: <timestamp>' \
  --data "$BODY" \
  http://127.0.0.1:18184/api/v1/transfers
```

Expected: first use is `200 OK`. Reuse the same nonce with the same signed request and TCPGuard blocks the replay.

## 5. Observe the result

```bash
curl -s http://127.0.0.1:18184/_demo/metrics
curl -s http://127.0.0.1:18184/_demo/incidents
curl -s http://127.0.0.1:18184/_demo/audit
curl -i -H 'X-API-Key: dev-management-key' http://127.0.0.1:18185/audit/verify
```

Expected: metrics show decision/action counts, incidents include high-risk outcomes, and audit verification returns a valid chain.

## Automated smoke verification

With the servers running:

```bash
cd examples/server
./scripts/verify_examples.sh
```

Set `VERIFY_DETAILS=true` while running the server with `TCPGUARD_ENV=development` to assert rule/detail strings too.
