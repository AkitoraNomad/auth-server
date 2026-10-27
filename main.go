package main

import (
	"auth-server/internal"
	oauth2Http "auth-server/internal/http"
	"auth-server/internal/oauth2"
	"auth-server/internal/store"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	if err := store.EnsureAdminClient(); err != nil {
		slog.Error("failed to initialize admin client", "err", err)
		os.Exit(1)
	}

	srv := oauth2.InitServer()
	engine := oauth2Http.CreateGinEngine(srv, spaAssetsFS())
	addr := fmt.Sprintf("%s:%d", internal.AuthServerConfig.Host, internal.AuthServerConfig.Port)
	slog.Info("Server started.", "addr", addr)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	// Process signal handling is left to the supervisor (Docker/Kubernetes/
	// systemd); the explicit server sets bounds on slow clients.
	if err := httpSrv.ListenAndServe(); err != nil {
		slog.Error("server exit", "err", err)
	}
}
