// Plugin-audit backend.
//
// Operates in three modes simultaneously (see spec 003 §6.2 / §10):
//
//  1. Event consumer — holds an open SSE connection to GET /v1/events and
//     persists every event into local SQLite (configurable storage in later
//     phases).
//  2. Request handler — serves /api/events, /api/events/{id},
//     /api/events/types, /api/stats so the UI can browse the audit log.
//  3. Bundle host — serves /main.js and /icon.png for the frontend.
//
// The daemon authenticates to the OpenEverest API with the plugin service
// token mounted at /var/run/secrets/everest/token (see spec 003 §10.4).
package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openeverest/plugin-audit/backend/internal/api"
	"github.com/openeverest/plugin-audit/backend/internal/config"
	"github.com/openeverest/plugin-audit/backend/internal/consumer"
	"github.com/openeverest/plugin-audit/backend/internal/store"
)

//go:embed dist/main.js
var distFS embed.FS

//go:embed dist/icon.png
var iconData []byte

func main() {
	cfg := config.FromEnv()

	st, err := store.OpenSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the SSE event consumer in the background.
	cons := consumer.New(cfg, st)
	go cons.Run(ctx)

	mux := http.NewServeMux()
	api.Register(mux, st, distFS, iconData)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("plugin-audit listening on :%s (everest API: %s, db: %s)",
			cfg.Port, cfg.EverestAPIURL, cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	cancel()
}
