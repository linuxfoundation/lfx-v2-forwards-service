# lfx-v2-forwards-service

Stateless LFX v2 Go microservice that manages email alias forwarding via [forwardemail.net](https://forwardemail.net).

Alias **ownership** (`<alias>@<domain>` → LFID) lives in `lfx-v2-auth-service` as a system-managed Auth0 linked identity. This service only manages the **routing** (`<alias>@<domain>` → `<target_email>`) in forwardemail.net.

`lfx-self-serve` orchestrates both services.

---

## NATS subjects

All subjects are under `lfx.forwards-service.`.

| Subject | Auth | Description |
|---|---|---|
| `lfx.forwards-service.check_alias` | None | Check whether an alias exists in forwardemail.net. |
| `lfx.forwards-service.set_target` | JWT | Create or update forwarding routing for the caller's alias. |
| `lfx.forwards-service.get_forward` | JWT | Read the current forwarding target for the caller's alias. |

All request types require a `domain` field. Requests missing or sending an empty `domain` receive a `domain_required` error.

### check_alias

```json
// request
{"alias": "johndoe", "domain": "linux.com"}

// reply — alias available
{"exists": false, "alias": "johndoe"}

// reply — alias taken
{"exists": true, "alias": "johndoe"}

// reply — invalid alias
{"error": "alias_invalid"}

// reply — domain not in allowed list
{"error": "domain_not_allowed"}
```

### set_target

```json
// request
{
  "user": {"auth_token": "<jwt>"},
  "domain": "linux.com",
  "target_email": "me@example.com"
}

// reply — success
{"alias": "johndoe", "target_email": "me@example.com", "updated_at": "2026-05-28T00:00:00Z"}

// reply — caller has no identity on this domain
{"error": "not_found"}

// reply — domain not in allowed list
{"error": "domain_not_allowed"}
```

### get_forward

```json
// request
{"user": {"auth_token": "<jwt>"}, "domain": "linux.com"}

// reply — found
{"found": true, "alias": "johndoe", "target_email": "me@example.com"}

// reply — no identity on this domain
{"found": false}

// reply — domain not in allowed list
{"error": "domain_not_allowed"}
```

### Error codes

| Code | Meaning |
|---|---|
| `alias_invalid` | Alias fails format/length/character validation |
| `alias_reserved` | Alias is on the reserved-name list |
| `domain_required` | The `domain` field was missing or empty |
| `domain_not_allowed` | Requested domain is not in the service's configured domain list |
| `unauthorized` | JWT missing, invalid, or auth-service call failed |
| `not_found` | Caller has no alias identity on the requested domain |
| `target_email_invalid` | `target_email` is missing or not a valid email address |
| `forwardemail_error` | forwardemail.net API call failed |

Full message contracts: [`pkg/api/forwards.go`](pkg/api/forwards.go).

---

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `NATS_URL` | Yes | — | NATS server URL |
| `NATS_CREDENTIALS_FILE` | No | `""` | NKey credentials file path |
| `FORWARDEMAIL_API_TOKEN` | Yes | — | forwardemail.net basic-auth API token |
| `FORWARDEMAIL_BASE_URL` | No | `https://api.forwardemail.net` | Override for testing |
| `FORWARDS_DOMAINS` | Yes | — | Comma-separated allow-list of managed domains. Callers must always send `domain` explicitly. |
| `FORWARDS_RESERVED_NAMES` | No | `""` | Comma-separated extra reserved alias names |
| `AUTH0_DOMAIN` | Yes | — | Auth0 tenant hostname for JWKS fetch (e.g. `linuxfoundation-dev.auth0.com`) |
| `AUTH0_AUDIENCE` | Yes | — | Expected JWT audience |
| `AUTH_SERVICE_SUBJECT` | No | `lfx.auth-service.user_emails.read` | NATS subject for auth-service |
| `AUTH_SERVICE_REQUEST_TIMEOUT` | No | `5s` | Timeout for auth-service NATS calls |
| `LOG_LEVEL` | No | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `OTEL_*` | No | — | Standard OpenTelemetry environment variables |

---

## Local development

### Prerequisites

- Go 1.25+
- NATS server (or dev cluster via `kubectl port-forward`)
- A forwardemail.net API token
- Auth0 domain + audience (dev tenant)

### Build and run

```bash
make build

NATS_URL=nats://localhost:4222 \
FORWARDEMAIL_API_TOKEN=<your-token> \
AUTH0_DOMAIN=linuxfoundation-dev.auth0.com \
AUTH0_AUDIENCE=https://linuxfoundation-dev.auth0.com/api/v2/ \
FORWARDS_DOMAINS=linux.com,linuxfoundation.org \
./bin/lfx-v2-forwards-service/forwards-service
```

### Testing with nats CLI

```bash
# check_alias — no auth required
nats req lfx.forwards-service.check_alias '{"alias":"johndoe","domain":"linux.com"}'

# set_target — JWT required
nats req lfx.forwards-service.set_target \
  '{"user":{"auth_token":"<jwt>"},"domain":"linux.com","target_email":"me@example.com"}'

# get_forward — JWT required
nats req lfx.forwards-service.get_forward \
  '{"user":{"auth_token":"<jwt>"},"domain":"linux.com"}'

# missing domain — returns domain_required
nats req lfx.forwards-service.check_alias '{"alias":"johndoe"}'
# reply: {"error":"domain_required"}
```

### Tests

```bash
make test
```

### Lint

```bash
make lint
```

### Helm template rendering

```bash
make helm-templates
```

---

## Architecture

```
lfx-self-serve
     │
     ├── NATS r/r ──► lfx-v2-auth-service   (alias ownership in Auth0)
     │
     └── NATS r/r ──► lfx-v2-forwards-service
                           │
                           ├── NATS r/r ──► lfx-v2-auth-service (user_emails.read)
                           │
                           └── HTTPS ──────► api.forwardemail.net
```

The service is **stateless** — no database, no KV store, no JetStream.
