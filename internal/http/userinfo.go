package http

import (
	"auth-server/internal/model"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	adminoauth2 "auth-server/internal/oauth2"

	"github.com/go-oauth2/oauth2/v4/server"
)

func getUserinfoHandler(srv *server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		if authorization := r.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
			ti, err := srv.Manager.LoadAccessToken(r.Context(), strings.TrimPrefix(authorization, "Bearer "))
			if err != nil || ti == nil || ti.GetUserID() == "" || !adminoauth2.UserIsActive(r.Context(), ti.GetUserID()) {
				writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "Token is invalid or revoked")
				return
			}
			if !model.HasScope(ti.GetScope(), model.ScopeProfile) {
				writeOAuthError(w, http.StatusForbidden, "insufficient_scope", "profile scope required")
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": ti.GetUserID()})
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing client authentication")
			return
		}
		cli, err := authenticateConfidentialClient(r.Context(), srv.Manager, username, password)
		if err != nil {
			slog.Warn("userinfo: invalid client authentication", "client_id", username)
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
		if err != nil || ti == nil || ti.GetUserID() == "" || !adminoauth2.UserIsActive(r.Context(), ti.GetUserID()) {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "Token is invalid or revoked")
			return
		}
		if ti.GetClientID() != cli.GetID() {
			slog.Warn("userinfo: caller does not own token", "caller", cli.GetID(), "token_client", ti.GetClientID())
			writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "Token is not owned by this client")
			return
		}

		if !model.HasScope(ti.GetScope(), model.ScopeProfile) {
			writeOAuthError(w, http.StatusForbidden, "insufficient_scope", "profile scope required")
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": ti.GetUserID()})
	}
}
