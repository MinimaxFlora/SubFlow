package api

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/subflow-org/subflow/internal/fetch"
	"github.com/subflow-org/subflow/internal/storage"
	"github.com/subflow-org/subflow/internal/version"
)

// Deps bundles the dependencies required by the HTTP API.
type Deps struct {
	Subscriptions *storage.SubscriptionRepo
	Settings      *storage.SettingsRepo
	// Resources stores Sub-Store style documents (subs, collections, files,
	// artifacts, tokens, settings).
	Resources *storage.ResourceRepo
	// Fetcher retrieves remote subscription content. It may be nil, in which
	// case URL-based conversions return an error.
	Fetcher *fetch.Fetcher
	// Assets is the filesystem rooted at the built frontend. It may be nil,
	// in which case a placeholder page is served.
	Assets fs.FS
	// Dev enables permissive CORS for local development.
	Dev bool
}

// Server wires HTTP routes to the storage layer.
type Server struct {
	deps    Deps
	handler http.Handler
}

// New builds a Server with all routes registered.
func New(deps Deps) *Server {
	s := &Server{deps: deps}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("GET /api/subscriptions", s.handleListSubscriptions)
	mux.HandleFunc("POST /api/subscriptions", s.handleCreateSubscription)
	mux.HandleFunc("GET /api/subscriptions/{id}", s.handleGetSubscription)
	mux.HandleFunc("GET /api/subscriptions/{id}/content", s.handleSubscriptionContent)
	mux.HandleFunc("PUT /api/subscriptions/{id}", s.handleUpdateSubscription)
	mux.HandleFunc("DELETE /api/subscriptions/{id}", s.handleDeleteSubscription)
	// Flat key/value settings kept on a distinct path; the Sub-Store style
	// document API owns /api/settings.
	mux.HandleFunc("GET /api/legacy/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/legacy/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/platforms", s.handleListPlatforms)
	mux.HandleFunc("POST /api/convert", s.handleConvert)
	mux.HandleFunc("POST /api/fetch", s.handleFetch)

	// Sub-Store compatible API, consumed by the bundled frontend.
	mux.HandleFunc("GET /api/utils/env", s.handleUtilEnv)
	mux.HandleFunc("GET /api/utils/refresh", s.handleUtilRefresh)
	mux.HandleFunc("POST /api/utils/node-info", s.handleNodeInfo)
	mux.HandleFunc("GET /api/utils/backup", s.handleNotImplemented("Gist backup and sync"))
	mux.HandleFunc("POST /api/utils/age/key-pair", s.handleNotImplemented("age key generation"))
	mux.HandleFunc("POST /api/utils/age/public-key", s.handleNotImplemented("age public key derivation"))

	mux.HandleFunc("GET /api/settings", s.handleSettingsGet)
	mux.HandleFunc("PATCH /api/settings", s.handleSettingsPatch)
	mux.HandleFunc("GET /api/storage", s.handleStorageExport)
	mux.HandleFunc("POST /api/storage", s.handleStorageImport)

	mux.HandleFunc("POST /api/preview/{type}", s.handlePreviewRoute)
	mux.HandleFunc("POST /api/sort/{type}", s.handleSortResource)
	mux.HandleFunc("GET /api/sub/flow/{name}", s.handleSubFlowRoute)

	mux.HandleFunc("GET /api/sync/artifact/{name}", s.handleNotImplemented("artifact sync"))
	mux.HandleFunc("GET /api/sync/artifacts", s.handleNotImplemented("artifact sync"))
	mux.HandleFunc("GET /api/artifacts/restore", s.handleNotImplemented("artifact sync"))

	mux.HandleFunc("GET /api/logs", s.handleLogsList)
	mux.HandleFunc("DELETE /api/logs", s.handleLogsClear)
	mux.HandleFunc("DELETE /api/archives/{id}", s.handleNotImplemented("archive management"))
	mux.HandleFunc("POST /api/archives/{id}/restore", s.handleNotImplemented("archive restore"))

	mux.HandleFunc("GET /download/collection/{name}", s.handleDownloadCollection)
	mux.HandleFunc("GET /download/{name}", s.handleDownloadSub)

	mux.HandleFunc("/api/{type}", s.handleResourceCollection)
	mux.HandleFunc("/api/{type}/{name}", s.handleResourceItem)

	mux.Handle("/", s.spaHandler())

	s.handler = s.withMiddleware(mux)
	return s
}

// Handler returns the composed HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return recoverMiddleware(logMiddleware(corsMiddleware(next, s.deps.Dev)))
}

func corsMiddleware(next http.Handler, dev bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dev {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
		)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "error", rec, "path", r.URL.Path)
				respondError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondOK(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	respondOK(w, http.StatusOK, version.Get())
}

// spaHandler serves the built frontend with a single-page fallback.
func (s *Server) spaHandler() http.Handler {
	if s.deps.Assets == nil {
		return http.HandlerFunc(placeholderPage)
	}
	if _, err := fs.Stat(s.deps.Assets, "index.html"); err != nil {
		return http.HandlerFunc(placeholderPage)
	}

	fileServer := http.FileServer(http.FS(s.deps.Assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "unknown API route")
			return
		}
		cleaned := strings.TrimPrefix(r.URL.Path, "/")
		if cleaned == "" {
			cleaned = "index.html"
		}
		if f, err := s.deps.Assets.Open(cleaned); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, s.deps.Assets)
	})
}

func serveIndex(w http.ResponseWriter, assets fs.FS) {
	f, err := assets.Open("index.html")
	if err != nil {
		placeholderPage(w, nil)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.Copy(w, f); err != nil {
		slog.Error("serve index.html", "error", err)
	}
}

func placeholderPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>SubFlow</title></head>
<body style="font-family:system-ui;padding:2rem">
<h1>SubFlow backend is running</h1>
<p>The frontend has not been built yet. Run <code>make frontend</code> and rebuild.</p>
<p>API health: <a href="/api/health">/api/health</a></p>
</body></html>`))
}

// ShutdownTimeout bounds graceful shutdown.
const ShutdownTimeout = 10 * time.Second

// Run starts the server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
