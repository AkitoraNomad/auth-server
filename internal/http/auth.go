package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"auth-server/internal/render"
	storeImpl "auth-server/internal/store"
	"auth-server/internal/util"
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
		if _, ok := s.Get(internal.SessionKeyUserID); !ok {
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

		uVal, ok := s.Get(internal.SessionKeyUserID)
		if !ok {
			redirectToLogin(w, r, state.ID)
			return
		}
		var sessionUser *model.SessionUserInfo
		if v, ok := uVal.(map[string]any); ok {
			sessionUser, err = util.MapToStruct[*model.SessionUserInfo](v)
			if err != nil {
				http.Error(w, "failed to parse session user info", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "invalid session user info type", http.StatusInternalServerError)
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
