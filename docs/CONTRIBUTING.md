# Contributing to AgentOrbit

Thank you for your interest in contributing to AgentOrbit!

## Contributor License Agreement (CLA)

By submitting a pull request, you agree to the following:

1. You grant AgentOrbit and its maintainers a perpetual, worldwide, non-exclusive, royalty-free, irrevocable license to use, reproduce, modify, sublicense, and distribute your contribution under any license, including proprietary licenses.
2. You confirm that you have the right to grant this license — the contribution is your original work, or you have permission from the copyright holder.
3. You understand that your contribution may be used in both the open source and commercial versions of AgentOrbit.

This CLA allows us to maintain a dual-licensing model (open source + commercial cloud) while keeping the project sustainable. Your contribution will always remain available under the AGPL-3.0 license in the open source version.

**By opening a pull request, you indicate your agreement with this CLA.**

## Development Setup

### Prerequisites

- Go 1.24+
- Node.js 22+
- Docker and Docker Compose
- [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html) CLI

### Getting Started

```bash
git clone https://github.com/agentorbit-tech/agentorbit.git
cd agentorbit

# Start the database
cp .env.example .env
# Edit .env -- replace all changeme_ values
docker compose up postgres -d

# Start the processing service
cd processing
go run ./cmd/processing
# In a new terminal:

# Start the frontend dev server
cd web
npm install
npm run dev
# In a new terminal:

# Start the proxy
cd proxy
go run ./cmd/proxy
```

The dashboard is at http://localhost:5173 (Vite dev server) or http://localhost:8081 (embedded).

## Project Structure

```
agentorbit/
  proxy/        # Stateless proxy service (Go, chi)
  processing/   # Stateful API + async workers (Go, chi, sqlc, golang-migrate)
  web/          # SPA dashboard (Vite + React 19 + TypeScript)
  docs/         # Documentation
```

## Code Style

### Go

- Run `go fmt ./...` before committing
- Run `go vet ./...` to catch common issues
- Use `golangci-lint run` for comprehensive linting

### TypeScript

- Follow the existing ESLint and Prettier configuration in `web/`
- Run `npx tsc --noEmit` to check types

### SQL

- Edit `.sql` files in `processing/queries/`
- Run `sqlc generate` to regenerate Go code
- Never edit generated files in `processing/internal/db/` directly

### Migrations

- Use sequential 4-digit numbering: `0001_`, `0002_`, etc.
- Place in `processing/migrations/`
- Always create both `.up.sql` and `.down.sql` files
- Migrations run automatically at Processing startup

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes with clear, atomic commits
4. Ensure all tests pass: `go test ./...` in both `proxy/` and `processing/`
5. Open a PR against `main`
6. One approval is required for merge
7. Squash merge is preferred for feature branches

### Branch Naming

- `fix/short-description` — bug fixes
- `feat/short-description` — new features
- `docs/short-description` — documentation
- `refactor/short-description` — code changes

### Commit Messages

Use conventional commit format:

- `feat:` new features
- `fix:` bug fixes
- `docs:` documentation changes
- `refactor:` code changes that neither fix bugs nor add features
- `test:` adding or updating tests

## Reporting Issues

Use GitHub Issues. Please include:

- **Bug reports**: steps to reproduce, expected vs actual behavior, environment details
- **Feature requests**: describe the use case and proposed solution

## Security Vulnerabilities

If you discover a security vulnerability, **do not open a public issue**. Instead, email agentorbit.tech@gmail.com (or your preferred contact). We will respond within 48 hours.

## License

By contributing, you agree that your contributions will be licensed under the [AGPL-3.0 license](LICENSE), and you accept the CLA terms described above.