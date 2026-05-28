// Package api wires the HTTP handlers exposed by the plugin backend.
package api

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openeverest/plugin-audit/backend/internal/store"
)

func Register(mux *http.ServeMux, st store.Store, distFS embed.FS, icon []byte) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /main.js", func(w http.ResponseWriter, _ *http.Request) {
		data, err := distFS.ReadFile("dist/main.js")
		if err != nil {
			http.Error(w, "bundle not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /icon.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(icon)
	})

	h := &handlers{st: st}
	mux.HandleFunc("GET /api/events", h.list)
	mux.HandleFunc("GET /api/events/{id}", h.get)
	mux.HandleFunc("GET /api/events/types", h.types)
	mux.HandleFunc("GET /api/events/namespaces", h.namespaces)
	mux.HandleFunc("GET /api/stats", h.stats)
}

type handlers struct {
	st store.Store
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	events, err := h.st.Query(r.Context(), f)
	if err != nil {
		log.Printf("query events: %v", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	resp := map[string]any{
		"items": events,
	}
	if n := len(events); n == f.Limit {
		resp["nextBeforeID"] = events[n-1].ID
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	e, err := h.st.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if e == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *handlers) types(w http.ResponseWriter, r *http.Request) {
	vals, err := h.st.Types(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": vals})
}

func (h *handlers) namespaces(w http.ResponseWriter, r *http.Request) {
	vals, err := h.st.Namespaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": vals})
}

func (h *handlers) stats(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s, err := h.st.Stats(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats failed")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func parseFilter(r *http.Request) (store.Filter, error) {
	q := r.URL.Query()
	f := store.Filter{
		Types:      csv(q.Get("types")),
		Namespaces: csv(q.Get("namespaces")),
		Actors:     csv(q.Get("actors")),
		Search:     strings.TrimSpace(q.Get("search")),
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			return f, errBadParam("limit must be 1..500")
		}
		f.Limit = n
	} else {
		f.Limit = 100
	}
	if v := q.Get("beforeID"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return f, errBadParam("invalid beforeID")
		}
		f.BeforeID = n
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, errBadParam("since must be RFC3339")
		}
		f.Since = &t
	}
	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, errBadParam("until must be RFC3339")
		}
		f.Until = &t
	}
	return f, nil
}

type errBadParam string

func (e errBadParam) Error() string { return string(e) }

func csv(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
