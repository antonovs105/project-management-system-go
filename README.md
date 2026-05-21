## 🛠 Tech Stack

### Backend
- **Language**: [Go](https://go.dev/) (v1.25+)
- **Framework**: [Echo](https://echo.labstack.com/) (High performance, extensible web framework)
- **Database**: [PostgreSQL](https://www.postgresql.org/)
- **Database Toolkit**: [sqlx](https://github.com/jmoiron/sqlx) (General purpose extensions to database/sql)
- **Migrations**: [golang-migrate](https://github.com/golang-migrate/migrate)
- **Authentication**: JWT (JSON Web Tokens)

### Frontend
- **Framework**: [React](https://reactjs.org/) (v19) with [TypeScript](https://www.typescriptlang.org/)
- **Build Tool**: [Vite](https://vitejs.dev/)
- **Styling**: [Tailwind CSS](https://tailwindcss.com/)
- **UI Components**: [shadcn/ui](https://ui.shadcn.com/) (Radix UI & Lucide React)
- **Routing**: [React Router](https://reactrouter.com/) (v7)
- **Form Handling**: [React Hook Form](https://react-hook-form.com/) with [Zod](https://zod.dev/) validation
- **State Management**: [Zustand](https://github.com/pmndrs/zustand)
- **Data Fetching**: [Axios](https://axios-http.com/) & [TanStack Query](https://tanstack.com/query/latest)
- **Visualization**: [react-force-graph-2d](https://github.com/vasturiano/react-force-graph)

## 📦 Getting Started

### Prerequisites

- [Docker](https://www.docker.com/) and [Docker Compose](https://docs.docker.com/compose/)
- (Optional for local development) [Go](https://go.dev/dl/) and [Node.js](https://nodejs.org/) (pnpm recommended)

### Quick Start with Docker

1. **Clone the repository**:
   ```bash
   git clone https://github.com/antonovs105/project-management-system-go.git
   cd project-management-system-go
   ```

2. **Set up environment variables**:
   Copy `.env.example` to `.env` before starting Compose. Replace the sample secrets before using the stack on a shared machine or in production.
   ```bash
   cp .env.example .env
   ```

3. **Run the application**:
   ```bash
   docker compose up --build
   ```

   The `migrations` service runs after Postgres is healthy and must complete before the backend starts.

4. **Access the application**:
   - Frontend: [http://localhost:5173](http://localhost:5173)
   - Backend API: [http://localhost:8080](http://localhost:8080)

   Compose binds PostgreSQL, Redis, Prometheus, and worker metrics to `127.0.0.1` by default. Production deployments must set `APP_ENV=production`, HTTPS `PUBLIC_BASE_URL`, strong secrets, and leave `FEDERATION_ALLOW_INSECURE_HTTP` unset or `false`.

### Local Development Without Containers

This path is only for development. The server runtime target is the Docker/Alpine container setup above.

#### Backend
```bash
cd backend
# Create .env file with DB_SOURCE pointing to localhost, for example:
# DB_SOURCE=postgres://postgres:postgres@localhost:5432/pms?sslmode=disable
go run cmd/api/main.go
```

#### Frontend
```bash
cd frontend
pnpm install
pnpm dev
```

### Backend Integration Tests

Integration tests are behind the `integration` build tag and expect a migrated PostgreSQL database.

With Docker running:
```bash
docker compose up -d db migrations
cd backend
TEST_DB_SOURCE=postgres://postgres:change-me-postgres-password-32-bytes@localhost:5432/pms?sslmode=disable go test -tags=integration ./internal/integration
```

If you changed `POSTGRES_PASSWORD` or `POSTGRES_PORT`, use those values in `TEST_DB_SOURCE`.

## 📂 Project Structure

```text
├── backend/            # Go source code
│   ├── cmd/api/        # Application entry point
│   ├── internal/       # Internal packages (user, project, ticket, etc.)
├── frontend/           # React source code
│   ├── src/
│   │   ├── components/ # Reusable UI components
│   │   ├── pages/      # Page components
│   │   ├── store/      # Zustand stores
│   │   └── hooks/      # Custom React hooks
├── migrations/         # Global migration files
└── docker-compose.yml  # Docker orchestration
```
