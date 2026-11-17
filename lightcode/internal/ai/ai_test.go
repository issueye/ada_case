package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sseServer(t *testing.T, chunks []string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			w.Write([]byte(`{"error":{"message":"bad key"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			fl.Flush()
			time.Sleep(time.Millisecond)
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
}

func TestStream(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"Hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"}}]}`,
		`{"choices":[{"delta":{"content":"!"}}]}`,
	}
	srv := sseServer(t, chunks, http.StatusOK)
	defer srv.Close()
	var deltas []string
	c := New(Config{BaseURL: srv.URL, Model: "test", APIKey: "k"})
	got, err := c.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, func(s string) {
		deltas = append(deltas, s)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello!" {
		t.Errorf("reply = %q", got)
	}
	if len(deltas) != 3 || deltas[0] != "Hel" {
		t.Errorf("deltas = %v", deltas)
	}
}

func TestStreamSendsRequest(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Model: "test-model", APIKey: "k"})
	if _, err := c.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "test-model" || body["stream"] != true {
		t.Errorf("body = %v", body)
	}
}

func TestStreamNoKey(t *testing.T) {
	c := New(Config{BaseURL: "http://127.0.0.1:1", APIKey: ""})
	if _, err := c.Stream(context.Background(), nil, nil); err != ErrNoKey {
		t.Errorf("err = %v, want ErrNoKey", err)
	}
}

func TestStreamHTTPError(t *testing.T) {
	srv := sseServer(t, nil, http.StatusUnauthorized)
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, APIKey: "bad"})
	_, err := c.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v", err)
	}
}

func TestStreamCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for i := 0; i < 50; i++ {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
			fl.Flush()
			time.Sleep(5 * time.Millisecond)
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	c := New(Config{BaseURL: srv.URL, APIKey: "k"})
	_, err := c.Stream(ctx, []Message{{Role: "user", Content: "hi"}}, func(string) { cancel() })
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want a cancellation", err)
	}
}
