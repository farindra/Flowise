# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is **Flowise** — a platform for building AI agents visually. It is a pnpm monorepo managed with Turborepo, running Node ≥ 20 and pnpm ≥ 10.26.

## Commands

### Development

```bash
pnpm install          # Install all dependencies across all packages
pnpm build            # Build all packages (turbo, respects dependency order)
pnpm dev              # Start all packages in dev/watch mode (hot reload on ui + server)
pnpm start            # Start production server from packages/server/bin
```

If the build runs out of memory:
```bash
export NODE_OPTIONS="--max-old-space-size=4096"
pnpm build
```

> **Important**: Changes in `packages/components` do **not** hot-reload. You must run `pnpm build` to pick up component changes even in dev mode.

### Testing

```bash
pnpm test                                        # Run all tests
cd packages/server && pnpm test                  # Server tests only
cd packages/components && pnpm test              # Components tests only
cd packages/agentflow && pnpm test               # Agentflow tests only
pnpm --filter flowise-components test            # Equivalent filter syntax
pnpm --filter @flowiseai/agentflow test:coverage # With coverage
```

Tests are co-located with source files: `Foo.ts` → `Foo.test.ts` in the same directory.

### Linting & Formatting

```bash
pnpm lint         # ESLint across all packages
pnpm lint-fix     # ESLint with auto-fix
pnpm format       # Prettier on all TS/TSX/MD files
```

Prettier config (from root `package.json`): `printWidth: 140`, `singleQuote: true`, `jsxSingleQuote: true`, no trailing commas, `tabWidth: 4`, no semicolons.

### Database Migrations

All migration commands run from `packages/server`:

```bash
pnpm typeorm:migration-generate   # Generate a migration from entity changes
pnpm typeorm:migration-run        # Apply pending migrations
pnpm typeorm:migration-revert     # Revert the last migration
```

Migration files live in `packages/server/src/database/migrations/{sqlite,mysql,mariadb,postgres}/`.

## Architecture

### Package Structure

| Package | Name | Purpose |
|---|---|---|
| `packages/server` | `flowise` | Express API backend, serves the built UI |
| `packages/ui` | `flowise-ui` | React frontend (Vite) |
| `packages/components` | `flowise-components` | All third-party node integrations (LangChain, AI providers) |
| `packages/agentflow` | `@flowiseai/agentflow` | Embeddable React component for the visual agent builder |
| `packages/observe` | `@flowiseai/observe` | Embeddable React components for observability |
| `packages/api-documentation` | — | Auto-generated Swagger UI from the Express router |

### Server (`packages/server`)

**Entry point**: `src/index.ts` creates an Express app, wires middleware, mounts `src/routes/`, and initializes singletons.

Key singletons initialized at startup:
- **`NodesPool`** (`src/NodesPool.ts`): Dynamically loads all node classes from the compiled `flowise-components` package. This is how the server discovers available nodes and credentials.
- **`CachePool`** (`src/CachePool.ts`): In-memory and Redis-backed cache for LLM instances.
- **`QueueManager`** (`src/queue/QueueManager.ts`): BullMQ queues for predictions, vector upserts, and scheduled flows. Only active when `REDIS_URL` is set; otherwise falls back to synchronous execution.
- **`ScheduleBeat`** (`src/schedule/ScheduleBeat.ts`): Cron-like scheduler for chatflows.

**Database**: TypeORM, configured in `src/DataSource.ts`. Supports `sqlite` (default, file at `~/.flowise/database.sqlite`), `mysql`, `mariadb`, and `postgres` via `DATABASE_TYPE` env var. Schema changes always require a migration — `synchronize: false` is enforced.

**Request flow for predictions**: Route → Controller (`src/controllers/`) → Service (`src/services/predictions/`) → `utilBuildChatflow` (`src/utils/buildChatflow.ts`), which resolves nodes, builds the LangChain graph, and runs inference.

**Horizontal scaling** (`MODE=queue`): The main process enqueues prediction jobs into Redis (BullMQ). Separate worker processes (`pnpm start-worker`) consume the queue. The `QUEUE_NAME` env var identifies the shared queue.

### Components (`packages/components`)

Each node implements the `INode` interface from `src/Interface.ts`:
- **`init()`** — instantiates the LangChain object (chain, retriever, etc.)
- **`run()`** — executes the node (used by tool-type nodes)
- **`vectorStoreMethods`** — `upsert`, `search`, `delete` for vector store nodes

Nodes live in `nodes/` organized by category (e.g., `nodes/chatmodels/`, `nodes/tools/`). Credentials live in `credentials/`.

**Credential security rule**: Secret fields (`api keys`, `passwords`, connection strings with embedded credentials) must use `type: 'password'` or `type: 'url'`. Fields typed `string` are returned in plaintext via the API. When in doubt, use `type: 'password'`.

### UI (`packages/ui`)

React 18 + Vite + MUI v5 + Redux Toolkit + React Router v6 + ReactFlow. State is split between Redux (global app state) and React context.

Dev server runs on port 8080 (set `VITE_PORT` in `packages/ui/.env`). In dev mode it proxies API calls to the server running on the port set in `packages/server/.env` (default 3000).

### Agentflow (`packages/agentflow`)

Embeddable visual agent-flow builder. Follows **Domain-Driven Modular Architecture**:

- **`atoms/`** — Dumb UI primitives (no business logic, no API calls). May only import from `core/primitives` and `core/theme`.
- **`features/`** — Smart domain modules (`canvas`, `node-palette`, `generator`, `node-editor`). Each has its own `index.ts` gatekeeper. Features must **not** import from each other.
- **`core/`** — Framework-agnostic TypeScript: types, validation, node config, theme tokens. No React components. `core/primitives/` is safe for atoms; `core/utils/` is not.
- **`infrastructure/`** — API client (axios) and state contexts (React context + reducers).

Dependency direction: `root files → features → core / infrastructure`. Never upward.

### Environment Variables

All env vars go in `packages/server/.env` (see `.env.example` for full reference). Key variables:

| Variable | Purpose |
|---|---|
| `PORT` | Server port (default 3000) |
| `DATABASE_TYPE` | `sqlite` \| `mysql` \| `mariadb` \| `postgres` |
| `MODE` | `main` (default) or `queue` (Redis-backed horizontal scaling) |
| `REDIS_URL` | Redis connection; enables queue mode automatically |
| `SECRETKEY_PATH` | Where the credential encryption key is stored |
| `FLOWISE_SECRETKEY_OVERWRITE` | Override encryption key directly |
| `STORAGE_TYPE` | `local` \| `s3` \| `gcs` \| `azure` |
| `DISABLED_NODES` | Comma-separated node names to hide from the UI |
