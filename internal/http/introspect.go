package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-oauth2/oauth2/v4/server"
)

func getIntrospectHandler(srv *server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		username, password, ok := r.BasicAuth()
		if !ok {
			slog.Warn("introspect: missing client authentication")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request", "error_description": "Missing client authentication"})
			return
		}

		cli, err := srv.Manager.GetClient(r.Context(), username)
		if err != nil {
			slog.Warn("introspect: failed to get client", "client_id", username, "err", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client", "error_description": "AuthClient authentication failed"})
			return
		}

		if cli.GetSecret() != password {
			slog.Warn("introspect: invalid client secret", "client_id", username)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client", "error_description": "AuthClient authentication failed"})
			return
		}

		_ = r.ParseMultipartForm(1 << 20)
		token := r.PostForm.Get("token")
		if token == "" {
			slog.Warn("introspect: missing token parameter")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request", "error_description": "Token parameter is missing"})
			return
		}

		ti, err := srv.Manager.LoadAccessToken(r.Context(), token)
		if err != nil {
			slog.Warn("introspect: invalid or expired token", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_token", "error_description": "Token is invalid or expired"})
			return
		}

		m := map[string]any{
			"active":    true,
			"sub":       ti.GetUserID(),
			"scope":     ti.GetScope(),
			"client_id": cli.GetID(),
			"exp":       ti.GetAccessCreateAt().Add(ti.GetAccessExpiresIn()).Unix(),
			"iat":       ti.GetAccessCreateAt().Unix(),
		}
		slog.Info("introspect: successful introspection", "client_id", cli.GetID(), "user_id", ti.GetUserID())
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(m)
	}
}
