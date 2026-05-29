// Package consumer holds the SSE event-stream client.
//
// It implements the snapshot-then-watch restart pattern from spec 003 §10.6:
// when the persisted cursor is empty or rejected by the host as stale, the
// consumer drops back to a cold-start and resumes from the resourceVersion
// returned by the most recent event.
package consumer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openeverest/plugin-audit/backend/internal/config"
	"github.com/openeverest/plugin-audit/backend/internal/store"
)

type Consumer struct {
	cfg   config.Config
	store store.Store
	http  *http.Client
}

func New(cfg config.Config, st store.Store) *Consumer {
	return &Consumer{
		cfg:   cfg,
		store: st,
		http:  &http.Client{Timeout: 0}, // SSE — no overall timeout.
	}
}

// Run loops forever, reconnecting on every drop with exponential backoff.
func (c *Consumer) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := c.streamOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("event stream error: %v (retry in %s)", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = time.Second
	}
}

func (c *Consumer) streamOnce(ctx context.Context) error {
	cursor, err := c.store.LatestCursor(ctx)
	if err != nil {
		return fmt.Errorf("load cursor: %w", err)
	}

	q := url.Values{}
	if cursor != "" {
		q.Set("since", cursor)
	}
	if c.cfg.EventTypes != "" {
		q.Set("types", c.cfg.EventTypes)
	}
	if c.cfg.Namespaces != "" {
		q.Set("namespaces", c.cfg.Namespaces)
	}

	endpoint := c.cfg.EverestAPIURL + "/v1/events"
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	token, tokenErr := c.token()
	if tokenErr != nil {
		return fmt.Errorf("read token from %s: %w", c.cfg.TokenPath, tokenErr)
	}
	if token == "" {
		log.Printf("WARNING: no token found at %s — request will be unauthenticated", c.cfg.TokenPath)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusBadRequest {
		// Cursor is past the watch cache window. Wipe and cold-start next time.
		log.Printf("stale cursor (status %d); resetting", resp.StatusCode)
		_ = c.store.SaveCursor(ctx, "")
		return fmt.Errorf("stale cursor")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	log.Printf("event stream connected (cursor=%q)", cursor)
	return c.readSSE(ctx, resp.Body)
}

// readSSE parses a minimal SSE stream: lines starting with "data: " carry one
// JSON event each; "event:" / "id:" lines are ignored — the resourceVersion in
// the JSON payload is the source of truth for the cursor.
func (c *Consumer) readSSE(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var dataBuf strings.Builder
	flush := func() error {
		if dataBuf.Len() == 0 {
			return nil
		}
		raw := dataBuf.String()
		dataBuf.Reset()
		return c.handleEvent(ctx, []byte(raw))
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				log.Printf("handle event: %v", err)
			}
		case strings.HasPrefix(line, "data:"):
			dataBuf.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		default:
			// id:, event:, retry:, comments (":") — ignored.
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	_ = flush()
	return io.EOF
}

func (c *Consumer) handleEvent(ctx context.Context, raw []byte) error {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}

	occurred := env.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}

	e := &store.Event{
		ResourceVersion: env.ResourceVersion,
		Type:            env.Type,
		OccurredAt:      occurred,
		Namespace:       env.Namespace,
		ResourceKind:    env.Resource.Kind,
		ResourceName:    env.Resource.Name,
		ActorType:       env.Actor.Type,
		ActorID:         env.Actor.ID,
		Envelope:        raw,
	}
	if err := c.store.Insert(ctx, e); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	if env.ResourceVersion != "" {
		if err := c.store.SaveCursor(ctx, env.ResourceVersion); err != nil {
			return fmt.Errorf("save cursor: %w", err)
		}
	}
	return nil
}

func (c *Consumer) token() (string, error) {
	if c.cfg.TokenPath == "" {
		return "", nil
	}
	b, err := os.ReadFile(c.cfg.TokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func nextBackoff(d time.Duration) time.Duration {
	const max = 30 * time.Second
	d *= 2
	if d > max {
		d = max
	}
	return d
}

// envelope is the subset of the spec-003 §10.5 event envelope we extract for
// indexed columns. The raw JSON is also stored verbatim.
type envelope struct {
	ResourceVersion string    `json:"resourceVersion"`
	Type            string    `json:"type"`
	OccurredAt      time.Time `json:"occurredAt"`
	Namespace       string    `json:"namespace"`
	Resource        struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"resource"`
	Actor struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"actor"`
}
