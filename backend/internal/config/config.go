package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds runtime configuration sourced from environment variables and
// projected files.
type Config struct {
	// Port is the HTTP port the backend listens on.
	Port string

	// EverestAPIURL is the base URL of the OpenEverest API (e.g.
	// http://everest-server.everest-system.svc.cluster.local:8080).
	EverestAPIURL string

	// TokenPath is the path to the projected plugin service token (spec 003 §10.4).
	// Used when the host implements the daemon token service (Phase 3).
	TokenPath string

	// ServiceUser and ServicePassword are credentials for authenticating against
	// the Everest API via POST /v1/session. Used as a fallback until the host
	// provides projected service tokens.
	ServiceUser     string
	ServicePassword string

	// DBPath is the on-disk location of the SQLite database.
	DBPath string

	// EventTypes is an optional comma-separated list of event types to subscribe
	// to. Empty means "all".
	EventTypes string

	// Namespaces optionally restricts the subscription to a comma-separated list
	// of namespaces.
	Namespaces string
}

func FromEnv() Config {
	return Config{
		Port:            getenv("PORT", "8080"),
		EverestAPIURL:   discoverEverestAPI(),
		TokenPath:       getenv("EVEREST_TOKEN_PATH", "/var/run/secrets/everest/token"),
		ServiceUser:     os.Getenv("EVEREST_SERVICE_USER"),
		ServicePassword: os.Getenv("EVEREST_SERVICE_PASSWORD"),
		DBPath:          getenv("AUDIT_DB_PATH", "/data/audit.db"),
		EventTypes:      os.Getenv("AUDIT_EVENT_TYPES"),
		Namespaces:    os.Getenv("AUDIT_NAMESPACES"),
	}
}

func discoverEverestAPI() string {
	if v := os.Getenv("EVEREST_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	host := os.Getenv("EVEREST_SERVICE_HOST")
	port := os.Getenv("EVEREST_SERVICE_PORT")
	if host != "" && port != "" {
		return fmt.Sprintf("http://%s:%s", host, port)
	}
	return "http://everest-server.everest-system.svc.cluster.local:8080"
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
