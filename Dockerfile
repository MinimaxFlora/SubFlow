# Frontend and Go build stage.
FROM node:22-alpine AS frontend
WORKDIR /src/web
# pnpm-workspace.yaml and .npmrc carry the dependency build allowlist, so the
# frozen install succeeds instead of aborting with ignored build scripts.
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

FROM golang:1.27-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/subflow-org/subflow/internal/version.Version=${VERSION} -X github.com/subflow-org/subflow/internal/version.Commit=${COMMIT} -X github.com/subflow-org/subflow/internal/version.BuildTime=${BUILD_TIME}" -o /out/subflow .

# Minimal non-root runtime.
FROM alpine:3.21
RUN addgroup -S subflow && adduser -S -G subflow subflow
WORKDIR /app
COPY --from=backend /out/subflow /app/subflow
RUN mkdir -p /data && chown -R subflow:subflow /data /app
USER subflow
ENV SUBFLOW_HOST=0.0.0.0
ENV SUBFLOW_PORT=8080
ENV SUBFLOW_DATA_DIR=/data
ENV SUBFLOW_DB_PATH=/data/subflow.db
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/subflow"]
