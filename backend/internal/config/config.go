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
	TokenPath string

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
		Port:          getenv("PORT", "8080"),
		EverestAPIURL: discoverEverestAPI(),
		TokenPath:     getenv("EVEREST_TOKEN_PATH", "/var/run/secrets/everest/token"),
		DBPath:        getenv("AUDIT_DB_PATH", "/data/audit.db"),
		EventTypes:    os.Getenv("AUDIT_EVENT_TYPES"),
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
