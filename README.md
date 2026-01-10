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
   Copy `.env.example` to `.env` (if needed) or rely on the defaults in `docker-compose.yml`.

3. **Run the application**:
   ```bash
   docker-compose up --build
   ```

4. **Access the application**:
   - Frontend: [http://localhost:5173](http://localhost:5173)
   - Backend API: [http://localhost:8080](http://localhost:8080)

### Manual Setup (Development)

#### Backend
```bash
cd backend
# Create .env file with DB_SOURCE and JWT_SECRET_KEY
go run cmd/api/main.go
```

#### Frontend
```bash
cd frontend
pnpm install
pnpm dev
```

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
