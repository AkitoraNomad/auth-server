package http

import (
	"log/slog"
	"net/http"

	"github.com/go-oauth2/oauth2/v4/server"
)

func getTokenHandler(srv *server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasAuth := r.Header.Get("Authorization") != ""
		slog.Info("token request received", "method", r.Method, "has_auth_header", hasAuth)
		if err := srv.HandleTokenRequest(w, r); err != nil {
			slog.Warn("token request failed", "err", err.Error())
		}
	}
}
