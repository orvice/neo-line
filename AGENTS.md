# AGENTS.md

This file provides project-specific guidance for AI coding agents working in this repository.

## Project Overview

neo-line is a Go server monitoring service built on the Butterfly application framework.

The product goal is to manage monitoring configuration for servers and check network services exposed by those servers, including:

- TCP port reachability
- URL availability for HTTP and HTTPS endpoints
- TLS Port handshake and certificate state monitoring
- Custom SNI name support for URL and TLS Port checks
- Health status aggregation for monitors and servers

All monitoring business configuration is stored in MongoDB. Agents should treat MongoDB as the source of truth for servers, monitors, enabled flags, probe parameters, thresholds, and alert policy configuration.

The project documentation is written in Chinese. Keep user-facing documentation in Chinese unless explicitly requested otherwise.

## Current Stack

- Language: Go
- Module: `github.com/orvice/neo-line`
- Application framework: `butterfly.orx.me/core/app`
- HTTP router: `github.com/gin-gonic/gin`
- Monitoring configuration store: MongoDB
- Protobuf tooling: Buf v2 configuration
- Generated protobuf output path: `pkg/proto`

## Repository Layout

- `cmd/server/main.go` — application entrypoint
- `docs/` — Chinese project documentation
- `proto/` — protobuf source definitions
- `buf.yaml` — Buf module, lint, and breaking-change configuration
- `buf.gen.yaml` — protobuf code generation configuration
- `go.mod`, `go.sum` — Go module files

## Documentation Rules

- Keep documentation under `docs/` in Chinese.
- Update `docs/features.md` when adding, changing, or removing product features.
- Update `docs/monitoring-configuration.md` when changing monitor configuration fields or behavior.
- Update `docs/README.md` when the high-level product scope or document index changes.
- Prefer concrete examples for monitoring configuration. YAML snippets in docs are field-shape examples for MongoDB documents, not local configuration files.
- Document operational defaults, including ports, intervals, timeouts, retry behavior, SNI behavior, and certificate expiration thresholds.
- Document MongoDB collection and field changes when configuration or runtime state persistence changes.

## Coding Guidelines

- Run `gofmt` on Go files before finishing changes.
- Keep the application entrypoint small; move domain logic into packages as the project grows.
- Prefer clear domain names:
  - `Server` for monitored server resources
  - `Monitor` for a configured check attached to a server
  - `CheckResult` for one execution result of a monitor
  - `HealthStatus` for computed state
- Initial monitor kinds should align with documentation:
  - `tcp`
  - `url`
  - `tls_port`
- Health states should align with documentation:
  - `Healthy`
  - `Warning`
  - `Critical`
  - `Down`
  - `Unknown`
- MongoDB is the authority for monitoring business configuration:
  - Read server and monitor configuration from MongoDB.
  - Write API-created or API-updated configuration to MongoDB.
  - Do not introduce local YAML, JSON, or static files as a source for server or monitor configuration.
  - Only minimal bootstrap settings, such as MongoDB connection information, may come from runtime environment or Butterfly application configuration.

## Monitoring Behavior Requirements

### TCP Monitor

A TCP monitor should:

- Connect to configured `host:port`.
- Respect timeout and retry settings.
- Record connection latency when successful.
- Record useful error details when failed.

### URL Monitor

A URL monitor should:

- Use `kind: url`.
- Support `http` and `https` URLs through one monitor kind.
- Support method, headers, expected status codes, timeout, and retries.
- For `https` URLs, validate TLS handshake behavior.
- For `https` URLs, support TLS verification configuration and custom `sni_name`.
- Record HTTP status code, DNS / TCP / TLS / total latency, error stage, error message, and checked timestamp.

### TLS Port Monitor

A TLS Port monitor should:

- Use `kind: tls_port`.
- Connect to configured `host:port`.
- Perform a TLS handshake without sending an HTTP request.
- Support TLS verification configuration.
- Support custom `sni_name`.
- Read peer certificate metadata.
- Record subject, issuer, DNS names, serial number, not-before, not-after, and days remaining.
- Support warning and critical expiration thresholds.
- Record DNS / TCP / TLS latency and TLS-related error details.

Default threshold proposal:

- Warning: certificate expires within 30 days
- Critical: certificate expires within 7 days
- Down: certificate is expired, not yet valid, or TLS handshake fails

## Protobuf Guidelines

- Put protobuf definitions under `proto/`.
- Follow the current package style, for example `neoline.v1`.
- Keep `go_package` aligned with the module path and generated output path:
  `github.com/orvice/neo-line/pkg/proto/...`
- After changing proto files, regenerate code using the Buf configuration when generation tooling is available.
- Do not manually edit generated files under `pkg/proto` once generation is introduced.

## Validation Commands

Use these commands when relevant:

```bash
go fmt ./...
go test ./...
go build ./...
```

For protobuf changes, use Buf commands when available:

```bash
buf lint
buf generate
```

## Git Notes

Before committing, check repository state:

```bash
git status --short
```
