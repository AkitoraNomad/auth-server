package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	adminoauth2 "auth-server/internal/oauth2"

	"github.com/go-oauth2/oauth2/v4/server"
)

func getIntrospectHandler(srv *server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")

		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="introspection"`)
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Missing client authentication")
			return
		}
		cli, err := authenticateConfidentialClient(r.Context(), srv.Manager, username, password)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="introspection"`)
			slog.Warn("introspect: invalid client authentication", "client_id", username)
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
			return
		}

		_ = r.ParseMultipartForm(1 << 20)
		token := r.PostForm.Get("token")
		if token == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Token parameter is missing")
			return
		}
		ti, err := srv.Manager.LoadAccessToken(r.Context(), token)
		if err != nil || ti == nil || ti.GetUserID() != "" && !adminoauth2.UserIsActive(r.Context(), ti.GetUserID()) {
			_ = json.NewEncoder(w).Encode(map[string]bool{"active": false})
			return
		}
		// A resource server may only introspect tokens issued to itself. A
		// broader policy requires an explicit resource-server authorization
		// model, which this service does not have yet.
		if ti.GetClientID() != cli.GetID() {
			slog.Warn("introspect: caller does not own token", "caller", cli.GetID(), "token_client", ti.GetClientID())
			_ = json.NewEncoder(w).Encode(map[string]bool{"active": false})
			return
		}

		response := map[string]any{
			"active":    true,
			"sub":       ti.GetUserID(),
			"scope":     ti.GetScope(),
			"client_id": ti.GetClientID(),
			"exp":       ti.GetAccessCreateAt().Add(ti.GetAccessExpiresIn()).Unix(),
			"iat":       ti.GetAccessCreateAt().Unix(),
		}
		slog.Info("introspect: successful introspection", "client_id", ti.GetClientID(), "user_id", ti.GetUserID())
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": description})
}
