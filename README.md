## Overview

This repository contains an unnamed full-stack project management application with federation support. The current system combines a Go backend, a React frontend, PostgreSQL persistence, Redis-backed background delivery, and Prometheus monitoring.

## Current State

- User registration and login with JWT authentication.
- Owner bootstrap through the backend maintenance CLI.
- Instance roles: `owner`, `admin`, and `user`.
- Project CRUD with project-local members, invitations, custom roles, and permission sets.
- Ticket workflow with kanban-style status columns, drag-and-drop status updates, priorities, ticket types, parent/subtask relationships, links, and comments.
- Project graph view backed by the ticket graph API.
- ActivityPub foundation with local actor documents, WebFinger discovery, inbox/outbox routes, HTTP signatures, remote actor caching, signed delivery, retries, and delivery inspection.
- Federation administration for domain blocks, remote actors, delivery summaries, retry actions, and audit events.
- Health, readiness, structured request logging, body limits, rate limits on public surfaces, protected Prometheus metrics, and container health checks.
- Static OpenAPI contract at `backend/docs/openapi.yaml`.

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
- Frontend Vite container
- PostgreSQL
- Redis
- Migration runner
- Prometheus

## Getting Started

### Prerequisites

- Docker and Docker Compose
- Optional for local development: Go 1.25+, Node.js 22+, and pnpm

### Run with Docker

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
   ```

4. Start the stack:

   ```bash
   docker compose up --build
   ```

   The `migrations` service waits for PostgreSQL and runs all SQL migrations before the backend starts.

5. Create the first owner account:

   ```powershell
   "change-this-password" | docker compose run --rm -T backend ./pmsctl owner create --username owner --email owner@example.test --password-stdin
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

The backend image also builds `./pmsctl` for maintenance tasks:

```bash
docker compose run --rm backend ./pmsctl owner create --help
docker compose run --rm backend ./pmsctl federation discover --help
docker compose run --rm backend ./pmsctl federation follow --help
docker compose run --rm backend ./pmsctl federation accept-follow --help
```

## Configuration Notes

- `APP_ROLE` can be `api`, `worker`, or `all`. Docker Compose runs separate API and worker services.
- `APP_ENV=production` enables stricter runtime validation.
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
migrations/                PostgreSQL migration files
monitoring/prometheus/     Prometheus scrape configuration
docker-compose.yml         Local multi-service runtime
docker-compose.instance.yml Example instance-oriented compose file
```
