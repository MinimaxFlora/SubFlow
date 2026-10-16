# SubFlow

SubFlow is a self-hosted subscription management and node-processing platform.
The backend is an independent Go implementation of the Sub-Store HTTP API
contract, and the web interface is a rebranded, modified copy of the Sub-Store
frontend. Persistence uses SQLite, and the frontend is embedded into a single
standalone binary.

## Features

- Sub-Store compatible REST API for subscriptions, collections, files,
  artifacts, settings, logs and node preview.
- Subscription parsing and node processing: fetch, parse, transform and
  generate.
- Configuration generation verified against real sing-box (stable and
  prerelease) and mihomo clients.
- Embedded Vue 3 frontend served from the same origin; no separate web server
  is required.
- Fully static binary built with a pure-Go SQLite driver.
- Interface localized in Simplified Chinese, English and Traditional Chinese.
- Multi-architecture release binaries and container images.

## Install

One-click install on a Linux server (systemd or OpenRC):

```bash
curl -fsSL https://raw.githubusercontent.com/MinimaxFlora/SubFlow/master/install.sh | bash
```

The script detects the operating system and CPU architecture, downloads the
matching release archive from GitHub, verifies it against the published
`SHA256SUMS`, installs the binary to `/usr/local/bin/subflow`, writes
`/etc/subflow/subflow.env`, stores data in `/var/lib/subflow`, and registers a
service that starts on boot.

Manage an existing installation:

```bash
subflowctl update      # upgrade to the latest release, keeping config and data
subflowctl status      # service status
subflowctl restart     # restart the service
subflowctl log         # last 200 log lines
subflowctl uninstall   # stop and remove the service and binary (keeps data)
```

Supported install targets: `linux-386`, `linux-amd64`, `linux-arm64`,
`linux-armv5`, `linux-armv6`, `linux-armv7`, `linux-s390x`, `darwin-amd64`
and `darwin-arm64`.

## Container images

```bash
# Docker Hub
docker run -d --name subflow -p 8080:8080 -v subflow-data:/data \
  dockorae/subflow:latest

# GitHub Container Registry
docker run -d --name subflow -p 8080:8080 -v subflow-data:/data \
  ghcr.io/minimaxflora/subflow:latest
```

Or with Compose:

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

Container platforms: `linux/386`, `linux/amd64`, `linux/arm64`,
`linux/arm/v6`, `linux/arm/v7` and `linux/s390x`. There is no Alpine base image
for `arm/v5`, so `armv5` is available as a release binary only.

To migrate between binary and Docker deployments, stop writes, copy the
complete configured data directory (including `subflow.db` and its SQLite
WAL/SHM files when present), then start the target mode with the same database
path. Keep a backup before migration and allow SQLite to close cleanly before
copying.

## Configuration

Configuration priority is command-line flags, environment variables, JSON
config file, then defaults.

| Setting | Flag | Environment | Default |
| --- | --- | --- | --- |
| Listen host | `--host` | `SUBFLOW_HOST` | `0.0.0.0` |
| Listen port | `--port` | `SUBFLOW_PORT` | `8080` |
| Data directory | `--data-dir` | `SUBFLOW_DATA_DIR` | `./data` |
| Database path | `--db` | `SUBFLOW_DB_PATH` | `<data-dir>/subflow.db` |
| Log level | `--log-level` | `SUBFLOW_LOG_LEVEL` | `info` |
| Development mode | `--dev` | `SUBFLOW_DEV` | `false` |
| Fetch User-Agent | `--fetch-user-agent` | `SUBFLOW_FETCH_USER_AGENT` | unset |
| Allow private fetch hosts | `--fetch-allow-private-hosts` | `SUBFLOW_FETCH_ALLOW_PRIVATE_HOSTS` | `false` |
| JSON config path | `--config` | `SUBFLOW_CONFIG` | unset |

The database and user files live under the configured data directory. Keep this
directory on persistent storage in production.

## Development

Prerequisites: Go 1.27+, Node.js 22+, pnpm 11 (via Corepack), and GNU Make.

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

The frontend development server proxies `/api` to `http://127.0.0.1:8080`.
Start the backend separately with `go run .` when using the dev server.

## Localization

The interface ships with Simplified Chinese (`zh-CN`), English (`en`), and
Traditional Chinese (`zh-TW`). The active language is detected from the
browser, can be switched from the language button in the navigation bar or the
selector in Settings, and is remembered in `localStorage` (`subflow.locale`).

Message catalogs live in `web/src/locales/` and are resolved by
`web/src/i18n.ts`. To add a language, create a catalog with the same keys,
register it in `i18n.ts`, and add it to `supportedLocales`.

## API

- `GET /api/health`
- `GET /api/version`
- `GET|POST /api/subscriptions`
- `GET|PUT|DELETE /api/subscriptions/{name}`
- `GET|POST /api/collections`
- `GET /api/preview` and `POST /api/preview/{type}`
- `GET|POST /api/files`, `GET /api/file/{name}`
- `GET|POST /api/artifacts`
- `GET|PUT /api/settings`
- `GET /api/logs`

Responses use `{ "status": ..., "data": ..., "error": null }` and errors use a
stable `code`, a `type`, and a user-readable `message`, matching the Sub-Store
contract.

## License and attribution

SubFlow is a modified derivative of the Sub-Store project and is distributed
under the **GNU Affero General Public License v3.0**; see [`LICENSE`](./LICENSE).

- The Go backend is an independent reimplementation of the Sub-Store backend
  HTTP contract. The Sub-Store backend is licensed under the AGPL-3.0.
- The frontend under `web/` is a modified copy of the Sub-Store frontend
  (baseline `v2.34.1`), which is licensed under the GPL-3.0. Its original
  license text is preserved at [`web/LICENSE`](./web/LICENSE).

Because the AGPL-3.0 and GPL-3.0 are compatible and a combined work must be
conveyed under the AGPL-3.0, the whole project is licensed under the AGPL-3.0.
See [`NOTICE`](./NOTICE) for the full attribution and [`web/README.md`](./web/README.md)
for the list of frontend modifications.

SubFlow is not affiliated with, endorsed by, or sponsored by the Sub-Store
project or its maintainers.
