# MERCH loyalty backend

HTTPS API for the MERCH store card: customers, points ledger, cashier commit/refund, admin, Apple PassKit web service, Google Wallet JWT, and the `/card/add` issuance page.

Source of truth is this server, not Wallet.

## Install Go (macOS)

Need Go **1.23+**. On this machine Homebrew installs it:

```bash
brew install go
export PATH="$(go env GOROOT)/bin:$(go env GOPATH)/bin:$PATH"
go version   # go1.23 or newer
```

Official tarball: https://go.dev/dl/

## Quick start (SQLite, no Docker)

```bash
cp .env.example .env          # optional; defaults work for local
make run                      # listens on :8080, DB ./data/merch.db
```

Default admin after first start: `admin` / `changeme` (override with `ADMIN_BOOTSTRAP_*`).

## Postgres (docker compose)

```bash
docker compose up --build
```

API: http://localhost:8080  
DB: `postgres://merch:merch@localhost:5432/merch`

## Tests

```bash
make test          # go test ./...  (SQLite in-memory, no Apple/Google keys)
make vet
```

## Environment

See `.env.example`. Secrets and certificate files stay out of git.

| Variable | Default / notes |
|---|---|
| `DATABASE_URL` | `sqlite:./data/merch.db` or `postgres://…` |
| `API_BASE_URL` / `PUBLIC_BASE_URL` | used in enroll URLs and `webServiceURL` |
| `CASHIER_JWT_SECRET` / `ADMIN_JWT_SECRET` | **change in prod**; cashier JWT TTL 12h |
| `APPLE_*` | empty → stub `.pkpass` + log `wallet update skipped` |
| `GOOGLE_ISSUER_ID` / `GOOGLE_SA_JSON` | empty → no-op adapter |
| `TERMS_URL` / `SUPPORT_CONTACT` | printed on the pass back |

§16 of the TZ (domain, legal entity, colours) is not blocking: values live in env / admin `loyalty_settings`.

## Curl examples

```bash
# enroll
curl -sS -c cookies.txt -X POST http://localhost:8080/public/enroll \
  -H 'Content-Type: application/json' \
  -d '{"name":"Анна","phone":"+79001112233"}'

# same phone / cookie does not create a second card
curl -sS -b cookies.txt -X POST http://localhost:8080/public/enroll \
  -H 'Content-Type: application/json' \
  -d '{"name":"Анна","phone":"+79001112233"}'

# cashier login (12h JWT)
TOKEN=$(curl -sS -X POST http://localhost:8080/cashier/login \
  -H 'Content-Type: application/json' \
  -d '{"login":"admin","password":"changeme"}' | jq -r .token)

# lookup
curl -sS -X POST http://localhost:8080/cashier/lookup \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"barcode":"MCH-........"}'

# quote (no write)
curl -sS -X POST http://localhost:8080/cashier/quote-redeem \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"barcode":"MCH-........","receipt_amount_rub":4500,"requested_points":500}'

# seed points as admin, then commit
ADMIN=$(curl -sS -X POST http://localhost:8080/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"login":"admin","password":"changeme"}' | jq -r .token)

curl -sS -X POST http://localhost:8080/admin/adjust \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"barcode":"MCH-........","delta":500,"reason":"seed for demo"}'

curl -sS -X POST http://localhost:8080/cashier/commit \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"receipt_id":"11111111-1111-4111-8111-111111111111","barcode":"MCH-........","receipt_amount_rub":4500,"redeem_points":500,"store_id":"00000000-0000-4000-8000-000000000001"}'
# earn = floor((4500-500)*5/100) = 200
```

Issuance page: http://localhost:8080/card/add  
Huawei / no GMS: the same page shows a web card with QR (`?platform=web`).

## Layout

```
cmd/api/main.go
internal/{config,httpapi,auth,loyalty,store,jobs,barcode,wallet/apple,wallet/google}
migrations/
web/templates/
```

OpenAPI: `openapi.yaml`.
