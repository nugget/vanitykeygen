package server

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

var (
	logger      *slog.Logger
	logLevel    *slog.LevelVar
	target      vkg.Target
	matchLogger *slog.Logger

	listenPort    int
	listenAddress string
	matchLogFile  string
)

func FlagSet() *flag.FlagSet {
	f := flag.NewFlagSet("server", flag.ExitOnError)

	f.IntVar(&listenPort, "p", 8080, "Specifies the port on which the server listens for connections")
	f.StringVar(&listenAddress, "b", "", "Bind this address on the local machine when listening for connections (default '' for all addresses)")
	f.StringVar(&matchLogFile, "l", "matchfile.log", "Log successful matches to this file")

	return f
}

// getTarget handles GET /target
func getTarget(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(target)
}

// postMatch handles POST /match
func postMatch(w http.ResponseWriter, r *http.Request) {
	var m vkg.Match

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&m)
	if err != nil {
		logger.Error("Decoder Failed", "error", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	logger.Info("received match",
		"hostname", m.Hostname,
		"seekerID", m.SeekerID,
		"authKey", m.Key.AuthorizedString,
		"finger", m.Key.Fingerprint,
	)

	matchLogger.Info("match reported", "match", m)

	w.WriteHeader(http.StatusOK)
}

// setupRouter creates the HTTP handler with routes
func setupRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /target", getTarget)
	mux.HandleFunc("POST /match", postMatch)

	// Add logging middleware
	return loggingMiddleware(mux)
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Debug("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

// Run is the real main, but one where we can exit with an error.
func Run(ctx context.Context, l *slog.Logger, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	logger = l

	logger.Info("Starting Server")

	myFlags := FlagSet()
	err := myFlags.Parse(args)
	if err != nil {
		return err
	}

	target.MatchString = `(?i)[\/\+](nugget|slacker|wheelsdown|hollowoak|ferrari|porsche|gt3rs|portofino|longhorn|miata|equiraptor|nugget-info|vanitykey|vanity-nugget)=?$`
	val := getenv("VKG_TARGET")
	if val != "" {
		target.MatchString = val
	}

	matchFile, err := os.OpenFile(matchLogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer matchFile.Close()
	matchLogger = slog.New(slog.NewJSONHandler(matchFile, nil))
	logger.Info("Logging matches to file", "matchLogFile", matchLogFile)

	// Setup HTTP server
	addr := fmt.Sprintf("%s:%d", listenAddress, listenPort)
	server := &http.Server{
		Addr:    addr,
		Handler: setupRouter(),
	}

	// Run server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting", "address", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for interrupt or server error
	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("Shutdown signal received")

		// Graceful shutdown with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
		logger.Info("Server stopped gracefully")
	}

	return nil
}
