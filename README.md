# Progo

## Overview

Progo is a full-stack project management application with federation support. The current system combines a Go backend, a React frontend, PostgreSQL persistence, Redis-backed realtime/federation infrastructure, and Prometheus monitoring.

## Current State

- User registration and login with JWT authentication.
- Owner bootstrap through the backend maintenance CLI.
- Instance roles: `owner`, `admin`, and `user`.
- Project CRUD with project-local members, invitations, custom roles, and permission sets.
- Ticket workflow with kanban-style status columns, drag-and-drop status updates, priorities, ticket types, parent/subtask relationships, links, and comments.
- Project graph view backed by the ticket graph API, with bounded filters for large projects.
- ActivityPub foundation with local actor documents, WebFinger discovery, inbox/outbox routes, HTTP signatures, remote actor caching, signed delivery, retries, and delivery inspection.
- Federation administration for domain blocks, remote actors, delivery summaries, retry actions, and audit events.
- Health, readiness, structured request logging, body limits, rate limits on public surfaces, protected Prometheus metrics, and container health checks.
- Static OpenAPI contract at `backend/docs/openapi.yaml`; authenticated REST routes are under `/api/v1`.

## Tech Stack

### Backend

- Go 1.25
- Echo
- PostgreSQL with `sqlx`
- `golang-migrate` migrations
- Redis and Asynq for ActivityPub delivery jobs
- JWT authentication
- Prometheus metrics
- ActivityPub, WebFinger, and HTTP Signatures

### Frontend

- React 19 with TypeScript
- Vite
- Tailwind CSS
- Radix UI/shadcn-style components and Lucide icons
- React Router 7
- TanStack Query and Axios
- Zustand
- React Hook Form and Zod
- dnd-kit
- react-force-graph-2d

### Runtime Services

- Backend API container
- Backend worker container
- Frontend container
- PostgreSQL
- Redis
- Migration runner
- Prometheus

## Getting Started

### Fresh VM Install

For a new production-like instance, point your domain at the VM first and open ports `80` and `443`.

The installer expects these commands to already exist on the VM:

- `docker` with the `docker compose` plugin
- `caddy`
- `curl`, `tar`, `find`, `cp`, `rm`, and `sha256sum` (or `shasum`)
- `sudo` when not running as root

Run a trusted copy of the installer from an interactive SSH shell, pinning an immutable commit and its independently published archive digest:

```bash
PROGO_REF=<full-commit-sha> \
PROGO_ARCHIVE_SHA256=<published-sha256> \
sh deploy/install.sh
```

The installer refuses unverified archives by default. `PROGO_ALLOW_UNVERIFIED_DOWNLOAD=true` exists only for disposable local testing.

The installer downloads deploy assets, creates `/opt/progo/app/progo.yml` interactively when missing, exports `/opt/progo/app/.env`, pulls the configured container images, runs migrations, deploys the inactive blue/green slot, checks health, and reloads Caddy.

Rerunning the installer is update-safe for existing instances: it keeps the existing `progo.yml` and `.env`, refreshes deploy assets and migrations, creates a database backup, and switches traffic only after the new slot is healthy.

Create the first owner account after the deployment is healthy:

```bash
cd /opt/progo/app
printf 'change-this-password\n' | ./deploy/pmsctl.sh owner create --username owner --email owner@example.test --password-stdin
```

This can be run only while no owner account exists.

Optional installer overrides:

```bash
PROGO_REF=<full-commit-sha> PROGO_ARCHIVE_SHA256=<published-sha256> IMAGE_TAG=<commit-tag> APP_DIR=/opt/progo/app sh deploy/install.sh
```

### Prerequisites

- Docker and Docker Compose
- Optional for local development: Go 1.26.5+, Node.js 22+, and pnpm 11.1.2

### Local Docker

1. Clone this repository and enter the checkout.

2. Create local environment configuration:

   ```bash
   cp .env.example .env
   ```

3. Replace the sample secrets in `.env` before using the stack on a shared machine or in production. At minimum, set strong values for:

   ```text
   POSTGRES_PASSWORD
   JWT_SECRET_KEY
   METRICS_TOKEN
   ACTOR_PRIVATE_KEY_ENCRYPTION_KEY
   ACTOR_PRIVATE_KEY_PREVIOUS_ENCRYPTION_KEYS
   SMTP_HOST
   SMTP_PORT
   SMTP_USERNAME
   SMTP_PASSWORD
   SMTP_FROM_ADDRESS
   SMTP_FROM_NAME
   SMTP_IMPLICIT_TLS
   ```

4. Start the stack:

   ```bash
   docker compose up --build
   ```

   The `migrations` service waits for PostgreSQL and runs all SQL migrations before the backend starts.

5. Create the first owner account:

   ```powershell
   "change-this-password" | docker compose run --rm -T backend /app/pmsctl owner create --username owner --email owner@example.test --password-stdin
   ```

   This can be run only while no owner account exists.

6. Open the application:

   - Frontend: `http://localhost:5173`
   - Backend API: `http://localhost:8080`
   - Prometheus: `http://localhost:9090`

By default, PostgreSQL, Redis, Prometheus, and worker metrics bind to `127.0.0.1`. For production, use `APP_ENV=production`, an HTTPS `PUBLIC_BASE_URL`, a non-local `LOCAL_DOMAIN`, strong secrets, and keep insecure federation flags disabled.

## Local Development

### Backend

Run PostgreSQL and Redis through Docker, then start the API from the backend directory:

```bash
cd backend
go run cmd/api/main.go
```

The backend reads `.env` from the current working directory when present. For native runs, make sure `DB_SOURCE`, `REDIS_ADDR`, `PUBLIC_BASE_URL`, `LOCAL_DOMAIN`, `JWT_SECRET_KEY`, `METRICS_TOKEN`, and `ACTOR_PRIVATE_KEY_ENCRYPTION_KEY` are set.

To rotate ActivityPub private-key encryption without downtime, replace `ACTOR_PRIVATE_KEY_ENCRYPTION_KEY` with the new primary secret and place the old secret in the comma-separated `ACTOR_PRIVATE_KEY_PREVIOUS_ENCRYPTION_KEYS` value. New actor keys use a key-ID-aware ciphertext format; existing rows remain decryptable while the previous key is configured. Keep previous keys available until all legacy rows have been rewritten and a verified database backup has passed a restore test.

Public local registration in production requires transactional SMTP configuration. Verification and password-reset messages are written to a PostgreSQL outbox in the same transaction as their single-use, hashed challenge; API processes never send mail inline. Worker processes lease pending messages, require TLS, and retry failures with bounded exponential backoff. Development logs the generated account link, while production never writes recovery tokens to logs.

Account security endpoints support email verification, enumeration-resistant password recovery, per-device session inventory/revocation, and a durable user-visible authentication event history. Resetting a password revokes all active sessions and each challenge can be consumed only once.

Users can enroll RFC 6238 TOTP MFA from the account page. Authenticator secrets are AES-GCM encrypted at rest, ten recovery codes are returned once and stored only as hashes, and each recovery code is atomically consumed. Password and OAuth sessions enforce MFA when enabled; users cannot be promoted to `admin` or `owner` until enrollment is active.

Projects and tickets use monotonic entity versions. Metadata edits, board moves, archive, and restore requests require a strong numeric `If-Match` value and return `412` on stale writes. The UI supplies the version returned with each entity. Archive is reversible and hidden from active lists; permanent deletion remains available only after a 30-day archive retention window. Project activity records actor-attributed before/after snapshots for project and ticket creation, edits, archive, and restore transitions.

Ticket attachments are immutable, permission-checked objects stored outside PostgreSQL in a persistent volume. The API derives MIME type from file content, rejects unsafe formats and files larger than 10 MiB, records a SHA-256 checksum, and forces downloads with `nosniff`. Production deployments that set `ATTACHMENTS_ENABLED=true` must also configure `CLAMAV_ADDR`; startup validation fails closed when malware scanning is unavailable from configuration. Back up the `attachment_data` volume together with PostgreSQL so attachment metadata and objects remain consistent.

### Frontend

```bash
cd frontend
pnpm install
pnpm dev
```

The frontend reads `VITE_API_URL`; it defaults to `http://localhost:8080`.

## Useful Commands

### Backend

```bash
cd backend
go test ./...
go vet ./...
```

Integration tests require a migrated PostgreSQL database:

```bash
docker compose up -d db migrations
cd backend
TEST_DB_SOURCE=postgres://postgres:change-me-postgres-password-32-bytes@localhost:5432/pms?sslmode=disable go test -tags=integration ./internal/integration
```

### Frontend

```bash
cd frontend
pnpm lint
pnpm build
pnpm test
```

### Maintenance CLI

The backend image also builds `/app/pmsctl` for maintenance tasks:

```bash
docker compose run --rm backend /app/pmsctl owner create --help
docker compose run --rm backend /app/pmsctl federation discover --help
docker compose run --rm backend /app/pmsctl federation follow --help
docker compose run --rm backend /app/pmsctl federation accept-follow --help
```

On a blue-green VM install, run the CLI inside the active backend container:

```bash
cd /opt/progo/app
./deploy/pmsctl.sh owner create --help
./deploy/pmsctl.sh federation discover --help
```

## Configuration Notes

- `APP_ROLE` can be `api`, `worker`, or `all`. Docker Compose runs separate API and worker services.
- `APP_ENV=production` enables stricter runtime validation.
- Authenticated application REST routes are served under `/api/v1`; public login, registration, OAuth, WebFinger, and ActivityPub routes stay at their protocol-specific paths.
- `FEDERATION_ALLOW_INSECURE_HTTP=true` is intended only for local development.
- `FEDERATION_ALLOW_PRIVATE_NETWORKS=true` is rejected in production.
- `CORS_ALLOWED_ORIGINS` controls browser origins allowed to call the API.
- `TRUSTED_PROXY_CIDRS` should be set when running behind trusted reverse proxies.
- `/metrics` requires a bearer token when `METRICS_TOKEN` is set.

## Project Structure

```text
backend/
  cmd/api/                 HTTP API and worker entrypoint
  cmd/pmsctl/              Maintenance CLI entrypoint
  docs/openapi.yaml        Static API contract
  internal/                Backend vertical slices and infrastructure
frontend/
  src/components/          Shared UI and layout components
  src/features/            Project, ticket, graph, and delivery views
  src/pages/               Route-level pages
  src/store/               Client-side auth state
  tests/                   Frontend contract tests
deploy/                    Fresh-VM installer and blue-green deployment assets
migrations/                PostgreSQL migration files
monitoring/prometheus/     Prometheus scrape configuration
docker-compose.yml         Local multi-service runtime
docker-compose.instance.yml Example instance-oriented compose file
```
