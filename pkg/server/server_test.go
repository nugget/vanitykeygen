package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorLogsServerFailures(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{
		logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/clients/heartbeat", nil)
	rec := httptest.NewRecorder()

	s.writeError(rec, req, http.StatusInternalServerError, "database is locked")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
	got := logs.String()
	for _, want := range []string{
		`"msg":"HTTP request failed"`,
		`"method":"POST"`,
		`"path":"/api/clients/heartbeat"`,
		`"status":500`,
		`"error":"database is locked"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected log to contain %s, got %s", want, got)
		}
	}
}

func TestWriteErrorDoesNotLogClientFailures(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{
		logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/clients/heartbeat", nil)
	rec := httptest.NewRecorder()

	s.writeError(rec, req, http.StatusBadRequest, "invalid JSON")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if got := logs.String(); got != "" {
		t.Fatalf("expected no log for client failure, got %s", got)
	}
}
