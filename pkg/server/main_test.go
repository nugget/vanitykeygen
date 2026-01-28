package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

func setupTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGetTarget(t *testing.T) {
	// Setup
	target = vkg.Target{
		MatchString: "test-pattern",
	}

	req := httptest.NewRequest(http.MethodGet, "/target", nil)
	w := httptest.NewRecorder()

	// Execute
	getTarget(w, req)

	// Assert
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.StatusCode)
	}

	var result vkg.Target
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.MatchString != "test-pattern" {
		t.Errorf("Expected MatchString 'test-pattern', got '%s'", result.MatchString)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestPostMatch(t *testing.T) {
	// Setup logger
	logger = setupTestLogger()

	// Create temporary match log file
	tmpFile, err := os.CreateTemp("", "matchfile-test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	matchLogger = slog.New(slog.NewJSONHandler(tmpFile, nil))

	// Create test match
	testMatch := vkg.Match{
		Timestamp:            time.Now(),
		Hostname:             "test-host",
		SeekerID:             1,
		MatchString:          "nugget",
		MatchedAuthorizedKey: true,
		MatchedFingerprint:   false,
		Key: vkg.Key{
			AuthorizedString: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAInugget test@example.com",
			Fingerprint:      "SHA256:testnuggetfingerprint",
		},
	}

	body, err := json.Marshal(testMatch)
	if err != nil {
		t.Fatalf("Failed to marshal test match: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/match", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute
	postMatch(w, req)

	// Assert
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.StatusCode)
	}
}

func TestPostMatchInvalidJSON(t *testing.T) {
	// Setup logger
	logger = setupTestLogger()

	req := httptest.NewRequest(http.MethodPost, "/match", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute
	postMatch(w, req)

	// Assert
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status BadRequest, got %v", res.StatusCode)
	}
}

func TestSetupRouter(t *testing.T) {
	// Setup
	logger = setupTestLogger()
	target = vkg.Target{
		MatchString: "test-pattern",
	}

	handler := setupRouter()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "GET /target",
			method:         http.MethodGet,
			path:           "/target",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST /target not allowed",
			method:         http.MethodPost,
			path:           "/target",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "GET /nonexistent",
			method:         http.MethodGet,
			path:           "/nonexistent",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestLoggingMiddleware(t *testing.T) {
	// Setup
	logger = setupTestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := loggingMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Execute
	wrapped.ServeHTTP(w, req)

	// Assert - middleware should not change the response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

func TestFlagSet(t *testing.T) {
	fs := FlagSet()

	if fs == nil {
		t.Fatal("FlagSet returned nil")
	}

	// Test default values by parsing empty args
	err := fs.Parse([]string{})
	if err != nil {
		t.Fatalf("Failed to parse empty args: %v", err)
	}

	// Check defaults
	if listenPort != 8080 {
		t.Errorf("Expected default port 8080, got %d", listenPort)
	}

	if listenAddress != "" {
		t.Errorf("Expected default address '', got '%s'", listenAddress)
	}

	if matchLogFile != "matchfile.log" {
		t.Errorf("Expected default matchLogFile 'matchfile.log', got '%s'", matchLogFile)
	}
}

func TestFlagSetWithCustomValues(t *testing.T) {
	fs := FlagSet()

	args := []string{"-p", "9000", "-b", "127.0.0.1", "-l", "custom.log"}
	err := fs.Parse(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}

	if listenPort != 9000 {
		t.Errorf("Expected port 9000, got %d", listenPort)
	}

	if listenAddress != "127.0.0.1" {
		t.Errorf("Expected address '127.0.0.1', got '%s'", listenAddress)
	}

	if matchLogFile != "custom.log" {
		t.Errorf("Expected matchLogFile 'custom.log', got '%s'", matchLogFile)
	}

	// Reset to defaults for other tests
	listenPort = 8080
	listenAddress = ""
	matchLogFile = "matchfile.log"
}

func TestRunWithContext(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create temporary match log file
	tmpFile, err := os.CreateTemp("", "matchfile-test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	testLogger := setupTestLogger()
	getenv := func(key string) string {
		if key == "VKG_TARGET" {
			return "test-pattern"
		}
		return ""
	}

	args := []string{"-p", "0", "-l", tmpFile.Name()} // Port 0 = random available port

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, testLogger, io.Discard, io.Discard, getenv, args)
	}()

	// Wait a bit for server to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for Run to complete
	select {
	case err := <-errCh:
		// Context cancellation is expected, nil is also acceptable
		if err != nil && err != context.Canceled {
			t.Errorf("Unexpected error from Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not complete in time")
	}
}
