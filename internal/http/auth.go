package http

import (
	"auth-server/internal/model"
	"auth-server/internal/render"
	storeImpl "auth-server/internal/store"
	"log/slog"
	"net/http"

	"github.com/go-session/session/v3"
)

func getAuthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := session.Start(r.Context(), w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		requestID := r.URL.Query().Get("request_id")
		state := findAuthorizeRequest(s, requestID)
		if state == nil {
			writeAuthorizeError(w, "invalid_request", "authorization request not found or expired")
			return
		}
		sessionUser := currentSessionUser(r.Context(), s)
		if sessionUser == nil {
			redirectToLogin(w, r, state.ID)
			return
		}

		clientInfo, err := storeImpl.ClientRepo.GetByID(r.Context(), state.ClientID)
		if err != nil {
			slog.Error("failed to load consent client", "client_id", state.ClientID, "err", err)
			http.Error(w, "failed to get client info", http.StatusInternalServerError)
			return
		}
		client, ok := clientInfo.(*model.AuthClient)
		if !ok || client == nil {
			writeAuthorizeError(w, "invalid_request", "unknown client")
			return
		}

		scopes := model.ParseScopeNames(state.Scope)
		if len(scopes) != len(state.Scope) {
			writeAuthorizeError(w, "invalid_scope", "the requested scopes are no longer available")
			return
		}

		setActiveAuthorizeRequest(s, state.ID)
		_ = s.Save()

		data := map[string]any{
			"clientInfo":          client,
			"responseType":        state.ResponseType,
			"scopeRequested":      scopes,
			"user":                sessionUser.User,
			"sessionUser":         sessionUser,
			"state":               state.State,
			"codeChallenge":       state.CodeChallenge,
			"codeChallengeMethod": state.CodeChallengeMethod,
			"redirectURI":         state.RedirectURI,
			"requestID":           state.ID,
			"error":               r.URL.Query().Get("error"),
		}

		slog.Debug("rendering consent page", "client_id", state.ClientID, "scopes_count", len(scopes))
		_ = render.Html(w, "auth.gohtml", http.StatusOK, data)
	}
}
