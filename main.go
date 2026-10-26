package main

import (
	"auth-server/internal"
	oauth2Http "auth-server/internal/http"
	"auth-server/internal/oauth2"
	"auth-server/internal/store"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	if err := store.EnsureAdminClient(); err != nil {
		slog.Error("failed to initialize admin client", "err", err)
		os.Exit(1)
	}

	srv := oauth2.InitServer()
	engine := oauth2Http.CreateGinEngine(srv)
	addr := fmt.Sprintf("%s:%d", internal.AuthServerConfig.Host, internal.AuthServerConfig.Port)
	slog.Info("Server started.", "addr", addr)
	// gin.Engine.Run uses http.ListenAndServe; process signal handling is left to
	// the supervisor (Docker/Kubernetes/systemd) for now.
	if err := engine.Run(addr); err != nil {
		slog.Error("server exit", "err", err)
	}
}
