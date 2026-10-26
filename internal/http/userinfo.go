package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-oauth2/oauth2/v4/server"
)

func getUserinfoHandler(srv *server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		username, password, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request", "error_description": "Missing client authentication"})
			return
		}

		cli, err := srv.Manager.GetClient(r.Context(), username)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client", "error_description": "AuthClient authentication failed"})
			return
		}

		if cli.GetSecret() != password {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client", "error_description": "AuthClient authentication failed"})
			return
		}

		_ = r.ParseMultipartForm(1 << 20)
		token := r.PostForm.Get("token")
		if token == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request", "error_description": "Token parameter is missing"})
			return
		}

		ti, err := srv.Manager.LoadAccessToken(r.Context(), token)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_token", "error_description": "Token is invalid or expired"})
			return
		}

		m := map[string]any{
			"active": true,
			"sub":    ti.GetUserID(),
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(m)
	}
}
