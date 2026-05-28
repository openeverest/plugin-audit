// Package store persists audit events.
//
// The MVP ships a single SQLite-backed implementation. Phase 2 will introduce
// a PostgreSQL driver behind the same Store interface.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Event is the persisted form of an audit event. It mirrors the plugin-facing
// envelope defined in spec 003 §10.5 with a couple of indexed columns lifted
// out for fast filtering.
type Event struct {
	ID              int64           `json:"id"`
	ResourceVersion string          `json:"resourceVersion"`
	Type            string          `json:"type"`
	OccurredAt      time.Time       `json:"occurredAt"`
	Namespace       string          `json:"namespace,omitempty"`
	ResourceKind    string          `json:"resourceKind,omitempty"`
	ResourceName    string          `json:"resourceName,omitempty"`
	ActorType       string          `json:"actorType,omitempty"`
	ActorID         string          `json:"actorID,omitempty"`
	Envelope        json.RawMessage `json:"envelope"`
}

// Filter narrows a Query. Empty fields are ignored.
type Filter struct {
	Types      []string
	Namespaces []string
	Actors     []string
	Search     string
	Since      *time.Time
	Until      *time.Time
	Limit      int
	BeforeID   int64 // cursor — return events with id < BeforeID
}

// Stats summarises a filtered slice of the audit log.
type Stats struct {
	Total       int64            `json:"total"`
	ByType      map[string]int64 `json:"byType"`
	ByNamespace map[string]int64 `json:"byNamespace"`
	ByActor     map[string]int64 `json:"byActor"`
	LastEventAt *time.Time       `json:"lastEventAt,omitempty"`
}

// Store is the abstraction over the persistence layer.
type Store interface {
	Insert(ctx context.Context, e *Event) error
	Get(ctx context.Context, id int64) (*Event, error)
	Query(ctx context.Context, f Filter) ([]Event, error)
	Types(ctx context.Context) ([]string, error)
	Namespaces(ctx context.Context) ([]string, error)
	Stats(ctx context.Context, f Filter) (*Stats, error)
	LatestCursor(ctx context.Context) (string, error)
	SaveCursor(ctx context.Context, cursor string) error
	Close() error
}

// OpenSQLite opens (and migrates) the SQLite database at path.
func OpenSQLite(path string) (Store, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer; keep things simple.
	s := &sqliteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

type sqliteStore struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS events (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  resource_version TEXT NOT NULL,
  type             TEXT NOT NULL,
  occurred_at      TEXT NOT NULL,
  namespace        TEXT,
  resource_kind    TEXT,
  resource_name    TEXT,
  actor_type       TEXT,
  actor_id         TEXT,
  envelope         TEXT NOT NULL,
  UNIQUE(resource_version, type, resource_kind, resource_name)
);
CREATE INDEX IF NOT EXISTS idx_events_occurred_at ON events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_type        ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_namespace   ON events(namespace);
CREATE INDEX IF NOT EXISTS idx_events_actor       ON events(actor_id);

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

func (s *sqliteStore) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

func (s *sqliteStore) Insert(ctx context.Context, e *Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO events
		  (resource_version, type, occurred_at, namespace, resource_kind, resource_name, actor_type, actor_id, envelope)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ResourceVersion, e.Type, e.OccurredAt.UTC().Format(time.RFC3339Nano),
		nullable(e.Namespace), nullable(e.ResourceKind), nullable(e.ResourceName),
		nullable(e.ActorType), nullable(e.ActorID), string(e.Envelope),
	)
	return err
}

func (s *sqliteStore) Get(ctx context.Context, id int64) (*Event, error) {
	row := s.db.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	e, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func (s *sqliteStore) Query(ctx context.Context, f Filter) ([]Event, error) {
	q, args := buildWhere(baseSelect, f)
	q += ` ORDER BY id DESC LIMIT ?`
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *sqliteStore) Types(ctx context.Context) ([]string, error) {
	return s.distinct(ctx, `SELECT DISTINCT type FROM events ORDER BY type`)
}

func (s *sqliteStore) Namespaces(ctx context.Context) ([]string, error) {
	return s.distinct(ctx, `SELECT DISTINCT namespace FROM events WHERE namespace IS NOT NULL AND namespace <> '' ORDER BY namespace`)
}

func (s *sqliteStore) distinct(ctx context.Context, q string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if v.Valid {
			out = append(out, v.String)
		}
	}
	return out, rows.Err()
}

func (s *sqliteStore) Stats(ctx context.Context, f Filter) (*Stats, error) {
	out := &Stats{
		ByType:      map[string]int64{},
		ByNamespace: map[string]int64{},
		ByActor:     map[string]int64{},
	}

	if q, args := buildWhere(`SELECT COUNT(*) FROM events`, f); true {
		if err := s.db.QueryRowContext(ctx, q, args...).Scan(&out.Total); err != nil {
			return nil, err
		}
	}

	type bucket struct {
		col   string
		dst   map[string]int64
		where string
	}
	buckets := []bucket{
		{"type", out.ByType, ""},
		{"namespace", out.ByNamespace, "namespace IS NOT NULL AND namespace <> ''"},
		{"actor_id", out.ByActor, "actor_id IS NOT NULL AND actor_id <> ''"},
	}
	for _, b := range buckets {
		base := fmt.Sprintf(`SELECT %s, COUNT(*) FROM events`, b.col)
		q, args := buildWhere(base, f)
		if b.where != "" {
			if strings.Contains(q, "WHERE") {
				q += " AND " + b.where
			} else {
				q += " WHERE " + b.where
			}
		}
		q += fmt.Sprintf(" GROUP BY %s ORDER BY COUNT(*) DESC LIMIT 25", b.col)
		rows, err := s.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				rows.Close()
				return nil, err
			}
			b.dst[k] = n
		}
		rows.Close()
	}

	var lastStr sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(occurred_at) FROM events`).Scan(&lastStr); err == nil && lastStr.Valid {
		if t, perr := time.Parse(time.RFC3339Nano, lastStr.String); perr == nil {
			out.LastEventAt = &t
		}
	}
	return out, nil
}

func (s *sqliteStore) LatestCursor(ctx context.Context) (string, error) {
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'cursor'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v.String, nil
}

func (s *sqliteStore) SaveCursor(ctx context.Context, cursor string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO meta(key, value) VALUES ('cursor', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, cursor)
	return err
}

func (s *sqliteStore) Close() error { return s.db.Close() }

// ---- helpers ----

const baseSelect = `SELECT id, resource_version, type, occurred_at, namespace,
   resource_kind, resource_name, actor_type, actor_id, envelope FROM events`

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(r scanner) (*Event, error) {
	var (
		e            Event
		ns, rk, rn   sql.NullString
		at, aid      sql.NullString
		occurred     string
		envelopeText string
	)
	if err := r.Scan(&e.ID, &e.ResourceVersion, &e.Type, &occurred,
		&ns, &rk, &rn, &at, &aid, &envelopeText); err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339Nano, occurred)
	if err != nil {
		return nil, fmt.Errorf("parse occurred_at: %w", err)
	}
	e.OccurredAt = t
	e.Namespace = ns.String
	e.ResourceKind = rk.String
	e.ResourceName = rn.String
	e.ActorType = at.String
	e.ActorID = aid.String
	e.Envelope = json.RawMessage(envelopeText)
	return &e, nil
}

func buildWhere(base string, f Filter) (string, []any) {
	var (
		conds []string
		args  []any
	)
	if len(f.Types) > 0 {
		conds = append(conds, "type IN ("+placeholders(len(f.Types))+")")
		for _, v := range f.Types {
			args = append(args, v)
		}
	}
	if len(f.Namespaces) > 0 {
		conds = append(conds, "namespace IN ("+placeholders(len(f.Namespaces))+")")
		for _, v := range f.Namespaces {
			args = append(args, v)
		}
	}
	if len(f.Actors) > 0 {
		conds = append(conds, "actor_id IN ("+placeholders(len(f.Actors))+")")
		for _, v := range f.Actors {
			args = append(args, v)
		}
	}
	if f.Search != "" {
		conds = append(conds, "(resource_name LIKE ? OR envelope LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	if f.Since != nil {
		conds = append(conds, "occurred_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if f.Until != nil {
		conds = append(conds, "occurred_at <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if f.BeforeID > 0 {
		conds = append(conds, "id < ?")
		args = append(args, f.BeforeID)
	}
	q := base
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	return q, args
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
