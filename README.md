# SubFlow

SubFlow is a subscription management and node-processing platform inspired by the public capabilities of Sub-Store. It is being developed as a Go service with a Vue 3 frontend, a SQLite persistence layer, and shared binary/Docker deployment conventions.

## Current milestone

The repository contains the initial foundation:

- Sub-Store storage research with source evidence in `docs/research/sub-store-storage.md`.
- SQLite schema migrations and repository boundaries.
- Health, version, settings, and subscription CRUD APIs.
- Vue 3 + TypeScript + Vite + Naive UI dashboard and subscription management UI.
- Localized interface in Simplified Chinese, English, and Traditional Chinese.
- Embedded frontend assets for a standalone Go binary.
- Multi-stage Docker image and persistent Compose volume.
- Makefile entry points for development, tests, builds, and release checks.

Subscription parsing, processing chains, format adapters, sing-box compatibility, backup/restore, and scheduled tasks are planned milestones. The current UI only exposes implemented subscription CRUD behaviour.

## Development

Prerequisites: Go 1.25+, Node.js 22+, npm, and GNU Make.

```bash
# Install dependencies
make init

# Run the frontend development server
make dev

# Build frontend and embedded Go binary
make build

# Run tests and quality checks
make check
```

The frontend development server proxies `/api` to `http://127.0.0.1:8080`. Start the backend separately with `go run .` when using the dev server.

## Localization

The interface ships with Simplified Chinese (`zh-CN`), English (`en`), and Traditional Chinese (`zh-TW`). The active language is detected from the browser, can be switched from the language button in the navigation bar or the selector in Settings, and is remembered in `localStorage` (`subflow.locale`).

Message catalogs live in `web/src/locales/` and are resolved by `web/src/i18n.ts`. To add a language, create a catalog with the same keys, register it in `i18n.ts`, and add it to `supportedLocales`.

## Configuration

Configuration priority is command-line flags, environment variables, JSON config file, then defaults.

| Setting | Flag | Environment | Default |
| --- | --- | --- | --- |
| Listen host | `--host` | `SUBFLOW_HOST` | `0.0.0.0` |
| Listen port | `--port` | `SUBFLOW_PORT` | `8080` |
| Data directory | `--data-dir` | `SUBFLOW_DATA_DIR` | `./data` |
| Database path | `--db` | `SUBFLOW_DB_PATH` | `<data-dir>/subflow.db` |
| Log level | `--log-level` | `SUBFLOW_LOG_LEVEL` | `info` |
| Development mode | `--dev` | `SUBFLOW_DEV` | `false` |
| JSON config path | `--config` | `SUBFLOW_CONFIG` | unset |

The database and future user files live under the configured data directory. Keep this directory on persistent storage in production.

## Docker

```bash
# Build and start with a persistent named volume
docker compose up -d --build

# Check service health
curl --fail http://127.0.0.1:8080/api/health

# View logs
docker compose logs -f

# Stop without removing the persistent volume
docker compose down
```

To migrate between binary and Docker deployments, stop writes, copy the complete configured data directory (including `subflow.db` and its SQLite WAL/SHM files when present), then start the target mode with the same database path. Keep a backup before migration and allow SQLite to close cleanly before copying.

## API

- `GET /api/health`
- `GET /api/version`
- `GET /api/subscriptions`
- `POST /api/subscriptions`
- `GET /api/subscriptions/{id}`
- `PUT /api/subscriptions/{id}`
- `DELETE /api/subscriptions/{id}`
- `GET /api/settings`
- `PUT /api/settings`

Responses use `{ "data": ..., "error": null }` and errors use a stable `code` plus user-readable `message`.
