// Package consumer holds the SSE event-stream client.
//
// It implements the snapshot-then-watch restart pattern from spec 003 §10.6:
// when the persisted cursor is empty or rejected by the host as stale, the
// consumer drops back to a cold-start and resumes from the resourceVersion
// returned by the most recent event.
package consumer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/openeverest/plugin-audit/backend/internal/config"
	"github.com/openeverest/plugin-audit/backend/internal/store"
)

type Consumer struct {
	cfg   config.Config
	store store.Store
	http  *http.Client

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
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
	token, tokenErr := c.getToken(ctx)
	if tokenErr != nil {
		return fmt.Errorf("obtain auth token: %w", tokenErr)
	}
	req.Header.Set("Authorization", "Bearer "+token)

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
	if err := flush(); err != nil {
		log.Printf("handle event: %v", err)
	}
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

// getToken returns a valid bearer token by authenticating against the Everest
// API via POST /v1/session and caching the JWT until shortly before expiry.
//
// Note: spec 003 §10.4 envisions the host issuing autonomous plugin service
// tokens via a projected Secret. Until that lands, plugins must log in like a
// regular client using credentials supplied via EVEREST_SERVICE_USER /
// EVEREST_SERVICE_PASSWORD.
func (c *Consumer) getToken(ctx context.Context) (string, error) {
	if c.cfg.ServiceUser == "" || c.cfg.ServicePassword == "" {
		return "", fmt.Errorf("EVEREST_SERVICE_USER / EVEREST_SERVICE_PASSWORD must be set")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with 5-minute safety margin).
	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry.Add(-5*time.Minute)) {
		return c.cachedToken, nil
	}

	// Authenticate.
	body, _ := json.Marshal(map[string]string{
		"username": c.cfg.ServiceUser,
		"password": c.cfg.ServicePassword,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.EverestAPIURL+"/v1/session", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("session auth failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}

	c.cachedToken = result.Token
	// Default to 23h expiry if we can't parse the token.
	c.tokenExpiry = time.Now().Add(23 * time.Hour)

	// Try to extract actual expiry from JWT claims (middle segment).
	parts := strings.Split(result.Token, ".")
	if len(parts) == 3 {
		if payload, err := decodeJWTSegment(parts[1]); err == nil {
			var claims struct {
				Exp int64 `json:"exp"`
			}
			if json.Unmarshal(payload, &claims) == nil && claims.Exp > 0 {
				c.tokenExpiry = time.Unix(claims.Exp, 0)
			}
		}
	}

	log.Printf("authenticated to Everest API as %q (token expires %s)", c.cfg.ServiceUser, c.tokenExpiry.Format(time.RFC3339))
	return c.cachedToken, nil
}

// decodeJWTSegment decodes a base64url-encoded JWT segment.
func decodeJWTSegment(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(seg)
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
