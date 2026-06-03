# Claude Development Guide for LFX V2 Forwards Service

## Project Overview

The LFX V2 Forwards Service is a stateless Go microservice in the LFX v2 platform. It manages **email alias forwarding** via [forwardemail.net](https://forwardemail.net). It handles:
- **Current**: Receiving NATS request/reply calls to check alias availability, create/update forwarding routing, and read the current forwarding target for a caller's alias on a managed domain.

It owns alias **routing** (alias → target email). Alias **ownership** is managed separately by `lfx-v2-auth-service` (linked Auth0 identities). `lfx-self-serve` orchestrates both: it claims ownership through the auth service, then sets up routing through this service. There is no persistence layer — the forwarding state lives in forwardemail.net.

## Key Technologies

- **Language**: Go 1.25+
- **Messaging**: NATS core (request/reply, queue groups)
- **Auth**: Auth0-issued JWTs verified via JWKS (`lestrrat-go/jwx/v2`)
- **Upstream API**: forwardemail.net REST (stdlib `net/http`, Basic Auth)
- **Observability**: OpenTelemetry (traces, metrics, logs) + slog structured logging
- **Container**: Chainguard distroless images, built with Ko
- **Orchestration**: Kubernetes with Helm charts (External Secrets Operator)

## Architecture

```
cmd/forwards-api/
├── main.go                   # OTel bootstrap, signal handling, graceful shutdown
└── service/
    ├── config.go             # ALL env var reads live here — AppConfigFromEnv()
    ├── implementations.go    # InitInfrastructure() wires singletons (NATSClient, ForwardSvc)
    └── subscriptions.go      # StartSubscriptions() — slice of {subject, bind} subscribers

internal/domain/
├── model/                    # Pure data: Alias, Forward, Claims + validation; sentinel errors
└── port/                     # Interfaces: ForwardEmailProvider, AuthServiceClient, TokenVerifier

internal/service/
└── forward.go                # ForwardService — HandleCheckAlias / HandleSetTarget / HandleGetForward

internal/infrastructure/
├── forwardemail/
│   └── client.go             # ForwardEmailProvider adapter (forwardemail.net REST)
├── authservice/
│   └── client.go             # AuthServiceClient — NATS r/r to lfx-v2-auth-service
├── jwt/
│   └── parser.go             # TokenVerifier — Auth0 JWKS + RSA verification
├── nats/
│   ├── client.go             # NATS connection & queue-group wrapper
│   └── errors.go             # Infrastructure error types
└── observability/
    ├── log.go                # slog + OTel handler init
    └── otel.go               # OTel SDK bootstrap

pkg/
└── api/
    └── forwards.go           # Public contract: NATS subjects, request/reply types
```

## Build Commands

```bash
make build       # Compile binary to bin/lfx-v2-forwards-service/forwards-service
make test        # Run tests with race detector and coverage
make check       # fmt + lint + license-check + go vet
make lint        # golangci-lint (v2.2.2)
```

Other targets: `make run` (build + execute), `make docker-build`, `make helm-install-local`, `make helm-templates`.

## Conventions

### Config injection
All **service** `os.Getenv` calls belong in `cmd/forwards-api/service/config.go` → `AppConfigFromEnv()`. The rest of the codebase receives typed values via the `AppConfig` and `service.Config` structs, never calling `os.Getenv` themselves. Required env vars: `FORWARDEMAIL_API_TOKEN`, `FORWARDS_DOMAINS`, `AUTH0_DOMAIN`, `AUTH0_AUDIENCE`.

**Exception:** OpenTelemetry `OTEL_*` vars are read directly in `internal/infrastructure/observability/otel.go` (`OTelConfigFromEnv`), following OTel SDK conventions — do not move these into `config.go`.

### Dependency injection
`InitInfrastructure()` in `cmd/forwards-api/service/implementations.go` builds the infrastructure adapters and populates the package-level singletons `NATSClient` and `ForwardSvc`. `Shutdown()` tears them down. All dependencies are passed as port interfaces.

### Adding a new NATS subscriber
1. Add the subject constant and request/reply types to `pkg/api/forwards.go`
2. Add the `Handle<Name>` method to `*ForwardService` in `internal/service/forward.go`
3. Add a `subscribe<Name>` func and append it to the `subscribers` slice in `cmd/forwards-api/service/subscriptions.go`

### Error handling
- Service methods return a string `errCode` (e.g. `alias_invalid`, `alias_reserved`, `domain_required`, `domain_not_allowed`, `unauthorized`, `not_found`, `target_email_invalid`, `forwardemail_error`) which is surfaced in the reply's `error` field.
- Malformed NATS payloads reply with `malformed_request` and are discarded (they will never parse successfully on retry).
- Domain sentinel errors (`ErrAliasNotFound`, `ErrNoAliasForDomain`) live in `internal/domain/model/errors.go`.

### Logging
- Use `slog.DebugContext`, `slog.InfoContext`, `slog.WarnContext`, `slog.ErrorContext`
- Always pass `ctx` so OTel trace correlation works

### License headers
Every `.go` file must start with (enforced by `make license-check`):
```go
// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
```

## NATS Subjects

| Subject | Auth | Description |
|---|---|---|
| `lfx.forwards-service.check_alias` | None | Check whether an alias exists/is available in forwardemail.net |
| `lfx.forwards-service.set_target` | JWT | Create or update the forwarding routing for the caller's alias on a domain |
| `lfx.forwards-service.get_forward` | JWT | Read the current forwarding target for the caller's alias on a domain |
| `lfx.auth-service.user_emails.read` | — | Outbound request/reply: resolve the caller's alias on the domain (configurable via `AUTH_SERVICE_SUBJECT`) |

## Related Services

| Service | Relationship |
|---|---|
| `lfx-v2-auth-service` | Source of caller email identities; resolves alias ownership on a domain via NATS request/reply |
| `lfx-self-serve` | Orchestrator; publishes `check_alias` / `set_target` / `get_forward` requests |
| forwardemail.net | External REST API where the forwarding rules are stored |
