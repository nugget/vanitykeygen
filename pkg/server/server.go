package server

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/nugget/vanitykeygen/pkg/store"
	"github.com/nugget/vanitykeygen/pkg/vkg"
	"github.com/nugget/vanitykeygen/web"
)

var (
	listenPort    int
	listenAddress string
	dbPath        string
	defaultTarget string
	wordPrefix    string
	wordSuffix    string
)

func FlagSet() *flag.FlagSet {
	f := flag.NewFlagSet("server", flag.ExitOnError)
	f.IntVar(&listenPort, "p", 8080, "Listen port")
	f.StringVar(&listenAddress, "b", "", "Bind address (default '' for all)")
	f.StringVar(&dbPath, "d", "vkg.db", "SQLite database path")
	f.StringVar(&defaultTarget, "t", "", "Default target pattern (creates if DB is empty)")
	f.StringVar(&wordPrefix, "word-prefix", `[\/\+]`, "Regex prefix for word targets")
	f.StringVar(&wordSuffix, "word-suffix", `=?$`, "Regex suffix for word targets")
	return f
}

// Version is set at build time via ldflags.
var Version = "dev"

// Server is the VKG server.
type Server struct {
	logger *slog.Logger
	store  *store.Store
	hub    *Hub
}

func newServer(logger *slog.Logger, st *store.Store) *Server {
	return &Server{
		logger: logger,
		store:  st,
		hub:    NewHub(),
	}
}

func (s *Server) setupRouter() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/targets", s.handleListTargets)
	mux.HandleFunc("POST /api/targets", s.handleCreateTarget)
	mux.HandleFunc("GET /api/targets/active", s.handleGetActiveTargets)
	mux.HandleFunc("GET /api/targets/{id}", s.handleGetTarget)
	mux.HandleFunc("PUT /api/targets/{id}", s.handleUpdateTarget)
	mux.HandleFunc("DELETE /api/targets/{id}", s.handleDeleteTarget)

	mux.HandleFunc("GET /api/matches", s.handleListMatches)
	mux.HandleFunc("GET /api/matches/{id}", s.handleGetMatch)
	mux.HandleFunc("POST /api/matches", s.handlePostMatch)

	mux.HandleFunc("POST /api/clients/register", s.handleRegister)
	mux.HandleFunc("POST /api/clients/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("GET /api/clients", s.handleListClients)

	mux.HandleFunc("GET /api/events", s.handleSSE)

	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/version", s.handleVersion)

	// Static web UI
	mux.Handle("GET /", http.FileServerFS(web.FS))

	return s.loggingMiddleware(mux)
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

// writeJSON writes a JSON response.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("failed to encode JSON response", "error", err)
	}
}

// readJSON decodes a JSON request body into v.
func (s *Server) readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// writeError writes a JSON error response.
func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

// Run starts the server. It blocks until interrupted or a fatal error occurs.
func Run(ctx context.Context, l *slog.Logger, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	myFlags := FlagSet()
	if err := myFlags.Parse(args); err != nil {
		return err
	}

	if val := getenv("VKG_DB_PATH"); val != "" {
		dbPath = val
	}

	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	srv := newServer(l, st)

	// Seed default target if DB is empty
	if err := srv.seedDefaultTarget(ctx, getenv); err != nil {
		return err
	}

	// Background: mark stale clients offline every 60s
	go srv.clientReaper(ctx)

	addr := fmt.Sprintf("%s:%d", listenAddress, listenPort)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.setupRouter(),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	serverErr := make(chan error, 1)
	go func() {
		l.Info("HTTP server starting", "address", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		l.Info("Shutdown signal received")
		// Close SSE hub first so all streaming connections return
		srv.hub.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			l.Warn("HTTP shutdown timeout", "error", err)
		}
		l.Info("Server stopped")
	}

	return nil
}

func (s *Server) seedDefaultTarget(ctx context.Context, getenv func(string) string) error {
	targets, err := s.store.ListTargets(ctx)
	if err != nil {
		return err
	}
	if len(targets) > 0 {
		return nil
	}

	// If a custom pattern is specified, seed it as a raw regex target.
	if pattern := defaultTarget; pattern != "" {
		return s.seedRegexTarget(ctx, pattern, "Default")
	}
	if val := getenv("VKG_TARGET"); val != "" {
		return s.seedRegexTarget(ctx, val, "Default")
	}

	// Otherwise seed default word targets.
	words := []string{
		"nugget", "slacker", "wheelsdown", "hollowoak",
		"ferrari", "porsche", "gt3rs", "portofino",
		"longhorn", "miata", "equiraptor",
	}
	for _, w := range words {
		t := &vkg.Target{
			Type:    "word",
			Pattern: w,
			Label:   w,
			Active:  true,
		}
		if err := s.store.CreateTarget(ctx, t); err != nil {
			return fmt.Errorf("seed word target %q: %w", w, err)
		}
	}
	s.logger.Info("Seeded default word targets", "count", len(words))
	return nil
}

func (s *Server) seedRegexTarget(ctx context.Context, pattern, label string) error {
	t := &vkg.Target{
		Type:    "regex",
		Pattern: pattern,
		Label:   label,
		Active:  true,
	}
	if err := s.store.CreateTarget(ctx, t); err != nil {
		return fmt.Errorf("seed regex target: %w", err)
	}
	s.logger.Info("Seeded regex target", "id", t.ID, "pattern", t.Pattern)
	return nil
}

func (s *Server) clientReaper(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Use a detached context for DB operations so they aren't
			// cancelled by the signal context mid-query.
			dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := s.store.MarkOfflineClients(dbCtx, 90*time.Second); err != nil {
				s.logger.Error("client reaper error", "error", err)
			}
			if err := s.store.DeleteOfflineClients(dbCtx, 5*time.Minute); err != nil {
				s.logger.Error("client cleanup error", "error", err)
			}
			cancel()
		}
	}
}
