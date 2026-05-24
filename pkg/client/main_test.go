package client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResponseStatusErrorIncludesBody(t *testing.T) {
	resp := &http.Response{
		Status: "500 Internal Server Error",
		Body:   io.NopCloser(strings.NewReader(`{"error":"database is locked (5) (SQLITE_BUSY)"}`)),
	}

	err := responseStatusError("heartbeat", resp)
	if err == nil {
		t.Fatal("expected error")
	}
	got := err.Error()
	if !strings.Contains(got, "heartbeat: server returned 500 Internal Server Error") {
		t.Fatalf("expected operation and status in error, got %q", got)
	}
	if !strings.Contains(got, "SQLITE_BUSY") {
		t.Fatalf("expected response body detail in error, got %q", got)
	}
}

func TestResponseStatusErrorTruncatesBody(t *testing.T) {
	resp := &http.Response{
		Status: "500 Internal Server Error",
		Body:   io.NopCloser(strings.NewReader(strings.Repeat("x", maxErrorResponseBytes+100))),
	}

	err := responseStatusError("heartbeat", resp)
	if err == nil {
		t.Fatal("expected error")
	}
	got := err.Error()
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker in error, got %q", got)
	}
	if len(got) > maxErrorResponseBytes+200 {
		t.Fatalf("error was not bounded: length %d", len(got))
	}
}
