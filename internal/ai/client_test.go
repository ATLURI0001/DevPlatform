package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientTrimsTrailingBaseURLSlash(t *testing.T) {
	c, err := NewClient(Config{
		BaseURL: "https://example.com/v1///",
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if c.config.BaseURL != "https://example.com/v1" {
		t.Fatalf("expected trimmed BaseURL, got %q", c.config.BaseURL)
	}
}

func TestChatStreamHandlesCRLFLargeChunksAndNilCallback(t *testing.T) {
	largeContent := strings.Repeat("a", 80*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi \"}}]}\r\n\r\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"" + largeContent + "\"}}]}\r\n"))
		_, _ = w.Write([]byte("data: [DONE]\r\n"))
	}))
	defer srv.Close()

	c, err := NewClient(Config{BaseURL: srv.URL + "/v1/", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	var out strings.Builder
	if err := c.ChatStream(context.Background(), []ChatMessage{{Role: "user", Content: "ping"}}, func(s string) {
		out.WriteString(s)
	}); err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	if out.String() != "hi "+largeContent {
		t.Fatalf("unexpected streamed length %d", out.Len())
	}

	// Passing nil callback must not panic.
	if err := c.ChatStream(context.Background(), []ChatMessage{{Role: "user", Content: "ping"}}, nil); err != nil {
		t.Fatalf("ChatStream with nil callback failed: %v", err)
	}
}
