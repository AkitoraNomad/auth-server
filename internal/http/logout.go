package http

import (
	authoauth2 "auth-server/internal/oauth2"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-session/session/v3"
	"net/http"
	"strconv"
	"strings"
)

// Logout revokes all existing grants and SSO sessions for the authenticated
// user. Bearer authentication supports the management SPA; cookie-only clients
// must supply the session-bound CSRF header obtained from GET /logout.
func getLogoutHandler(srv *server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		s, err := session.Start(r.Context(), w, r)
		if err != nil {
			http.Error(w, "session unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodGet {
			token, err := csrfToken(s)
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			responseJson(w, 0, map[string]string{"csrf_token": token})
			return
		}
		var uid int64
		if raw := r.Header.Get("Authorization"); raw != "" {
			if !strings.HasPrefix(raw, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ti, err := srv.Manager.LoadAccessToken(r.Context(), strings.TrimPrefix(raw, "Bearer "))
			if err != nil || ti == nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			uid, err = strconv.ParseInt(ti.GetUserID(), 10, 64)
			if err != nil || uid <= 0 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		} else {
			if validateCSRF(r, s) != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			user := currentSessionUser(r.Context(), s)
			if user != nil {
				uid = user.User.ID
			}
		}
		if uid > 0 {
			if err := authoauth2.RevokeUserSessions(r.Context(), uid); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		if err := session.Destroy(r.Context(), w, r); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
