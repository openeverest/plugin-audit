package consumer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/openeverest/plugin-audit/backend/internal/config"
	"github.com/openeverest/plugin-audit/backend/internal/store"
)

// TestConsumer_SurvivesShutdown drives the race main.go's shutdown path used
// to have: an event fully received and persisted before the process gets a
// stop signal must still be on disk after the consumer goroutine stops and
// the store closes, with no error from either step.
//
// Before this PR, main() called cancel() and returned without waiting for
// the consumer goroutine, so st.Close() could run concurrently with an
// in-flight store write. This test doesn't reproduce that race directly
// (it's timing-dependent by nature) - it pins down the contract the fix
// establishes: Run() must have actually returned before the store is safe
// to close.
func TestConsumer_SurvivesShutdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/session" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"token":"test-token"}`)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}

		fmt.Fprint(w, "data: {\"resourceVersion\":\"1\",\"type\":\"instance.created\",\"resource\":{\"kind\":\"Instance\",\"name\":\"x\"}}\n\n")
		flusher.Flush()

		// Hold the connection open with no further data, like a live SSE
		// stream sitting idle between events, until the client disconnects.
		<-r.Context().Done()
	}))
	defer srv.Close()

	dbPath := filepath.Join(t.TempDir(), "audit.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	cfg := config.Config{
		EverestAPIURL:   srv.URL,
		ServiceUser:     "admin",
		ServicePassword: "pass",
	}
	c := New(cfg, st)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx)
	}()

	// Wait for the event to actually land before triggering shutdown - this
	// is the "in-flight event" precondition: work that finished must not be
	// lost by whatever happens next.
	deadline := time.Now().Add(5 * time.Second)
	for {
		events, err := st.Query(context.Background(), store.Filter{Limit: 500})
		if err != nil {
			t.Fatalf("query events: %v", err)
		}
		if len(events) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("event was not persisted before deadline (got %d rows)", len(events))
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Simulate SIGTERM: cancel, then wait for the goroutine to actually stop
	// - the fix under test - before closing the store, same order as main.go.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop after context cancellation")
	}

	if err := st.Close(); err != nil {
		t.Fatalf("store close after shutdown: %v", err)
	}

	// Reopen from disk - not the same in-memory handle - to confirm the
	// event actually made it to disk and Close() didn't drop anything.
	st2, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st2.Close()

	events, err := st2.Query(context.Background(), store.Filter{Limit: 500})
	if err != nil {
		t.Fatalf("query events after reopen: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 persisted event after shutdown, got %d", len(events))
	}
	if events[0].Type != "instance.created" {
		t.Fatalf("unexpected event type: %q", events[0].Type)
	}
}
