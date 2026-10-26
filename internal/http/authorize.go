package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	storeImpl "auth-server/internal/store"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-session/session/v3"
)

func getAuthorizeHandler(srv *server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := session.Start(r.Context(), w, r)
		if err != nil {
			slog.Error("failed to start authorize session", "err", err)
			writeAuthorizeError(w, "session_error", "failed to start session")
			return
		}
		if err := r.ParseForm(); err != nil {
			writeAuthorizeError(w, "invalid_request", "failed to parse request")
			return
		}

		requestID := r.FormValue("request_id")
		_, hasConsent := r.Form["consented"]
		_, hasDeny := r.Form["deny"]
		isConsentSubmission := r.FormValue("consent") == "1" || hasConsent || hasDeny
		if requestID != "" && isConsentSubmission {
			handleConsentSubmission(w, r, s, srv, requestID)
			return
		}
		handleInitialAuthorize(w, r, s, srv)
	}
}

func handleInitialAuthorize(w http.ResponseWriter, r *http.Request, s session.Store, srv *server.Server) {
	req, err := srv.ValidationAuthorizeRequest(r)
	if err != nil {
		slog.Warn("invalid authorize request", "err", err)
		writeAuthorizeError(w, "invalid_request", err.Error())
		return
	}

	clientInfo, err := storeImpl.ClientRepo.GetByID(r.Context(), req.ClientID)
	if err != nil {
		slog.Error("failed to load authorize client", "client_id", req.ClientID, "err", err)
		writeAuthorizeError(w, "server_error", "failed to load client")
		return
	}
	client, ok := clientInfo.(*model.AuthClient)
	if !ok || client == nil {
		writeAuthorizeError(w, "invalid_request", "unknown client")
		return
	}

	// The admin SPA and new clients must use an exactly registered redirect URI.
	// The legacy single domain column is accepted only for clients without the
	// redirect_uris allow-list.
	if !client.AllowsRedirectURI(req.RedirectURI) {
		slog.Warn("authorize redirect_uri rejected", "client_id", req.ClientID, "redirect_uri", req.RedirectURI)
		writeAuthorizeError(w, "invalid_request", "redirect_uri is not registered for this client")
		return
	}

	grantName := grantNameForResponseType(req.ResponseType)
	if grantName == "" || !client.AllowsGrantType(grantName) {
		writeAuthorizeError(w, "unauthorized_client", "grant type is not enabled for this client")
		return
	}

	requestedScopes, err := model.ParseScopes(req.Scope)
	if err != nil {
		writeAuthorizeError(w, "invalid_scope", err.Error())
		return
	}
	for _, scope := range requestedScopes {
		if !client.HasScope(scope.Name) {
			writeAuthorizeError(w, "invalid_scope", "scope is not allowed for this client: "+scope.Name)
			return
		}
	}

	state := &authorizeRequestState{
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		ResponseType:        string(req.ResponseType),
		Scope:               model.ScopeNames(requestedScopes),
		State:               req.State,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: string(req.CodeChallengeMethod),
	}
	if err := putAuthorizeRequest(s, state); err != nil {
		slog.Error("failed to store authorize request", "err", err)
		writeAuthorizeError(w, "server_error", "failed to store authorization request")
		return
	}

	if _, ok := s.Get(internal.SessionKeyUserID); !ok {
		redirectToLogin(w, r, state.ID)
		return
	}
	redirectToConsent(w, r, state.ID)
}

func handleConsentSubmission(w http.ResponseWriter, r *http.Request, s session.Store, srv *server.Server, requestID string) {
	state := findAuthorizeRequest(s, requestID)
	if state == nil {
		writeAuthorizeError(w, "invalid_request", "authorization request not found or expired")
		return
	}
	if err := validateConsentForm(r, state); err != nil {
		slog.Warn("consent form does not match authorize request", "err", err, "request_id", state.ID)
		writeAuthorizeError(w, "invalid_request", err.Error())
		return
	}
	if _, ok := s.Get(internal.SessionKeyUserID); !ok {
		redirectToLogin(w, r, state.ID)
		return
	}
	if err := validateStoredAuthorizeRequest(r, state); err != nil {
		writeAuthorizeError(w, "invalid_request", err.Error())
		removeAuthorizeRequest(s, state.ID)
		return
	}

	if _, deny := r.Form["deny"]; deny {
		redirectAuthorizationError(w, state, "access_denied", "The user denied the authorization request")
		removeAuthorizeRequest(s, state.ID)
		return
	}

	consented, err := consentedScopes(r.Form["consented"], state.Scope)
	if err != nil {
		redirectToConsentWithError(w, r, state.ID, err.Error())
		return
	}
	if len(consented) == 0 {
		redirectToConsentWithError(w, r, state.ID, "at least one scope must be approved")
		return
	}

	setRequestConsent(s, state.ID, model.ScopeNames(consented))
	setActiveAuthorizeRequest(s, state.ID)
	if err := s.Save(); err != nil {
		slog.Error("failed to save consent", "err", err)
		writeAuthorizeError(w, "server_error", "failed to save authorization decision")
		return
	}

	// Replace client-controlled form values with the stored request values. The
	// hidden fields were checked above; this also covers scope, which is derived
	// from the stored request rather than trusted from the browser.
	r.Form.Set("scope", strings.Join(state.Scope, " "))
	r.Form.Set("request_id", state.ID)

	if err := srv.HandleAuthorizeRequest(w, r); err != nil {
		slog.Error("failed to handle authorized request", "request_id", state.ID, "err", err)
		if w.Header().Get("Location") == "" {
			writeAuthorizeError(w, "server_error", "failed to issue authorization code")
		}
	}
	removeAuthorizeRequest(s, state.ID)
}

func validateConsentForm(r *http.Request, state *authorizeRequestState) error {
	checks := []struct {
		field string
		want  string
	}{
		{"client_id", state.ClientID},
		{"redirect_uri", state.RedirectURI},
		{"response_type", state.ResponseType},
		{"state", state.State},
		{"code_challenge", state.CodeChallenge},
		{"code_challenge_method", state.CodeChallengeMethod},
	}
	for _, check := range checks {
		if got := r.FormValue(check.field); got != check.want {
			return fmt.Errorf("%s does not match the original authorization request", check.field)
		}
	}
	return nil
}

func consentedScopes(values []string, requested []string) ([]*model.Scope, error) {
	allowed := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		allowed[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]*model.Scope, 0, len(values))
	for _, raw := range values {
		scope, err := model.ParseScope(strings.TrimSpace(raw))
		if err != nil {
			return nil, err
		}
		if _, ok := allowed[scope.Name]; !ok {
			return nil, fmt.Errorf("scope was not requested: %s", scope.Name)
		}
		if _, ok := seen[scope.Name]; ok {
			continue
		}
		seen[scope.Name] = struct{}{}
		result = append(result, scope)
	}
	return result, nil
}

func grantNameForResponseType(responseType oauth2.ResponseType) string {
	switch responseType {
	case oauth2.Code:
		return "authorization_code"
	case oauth2.Token:
		return "implicit"
	default:
		return ""
	}
}

func redirectToLogin(w http.ResponseWriter, r *http.Request, requestID string) {
	location := internal.PathLogin
	if requestID != "" {
		location += "?request_id=" + url.QueryEscape(requestID)
	}
	http.Redirect(w, r, location, http.StatusFound)
}

func redirectToConsent(w http.ResponseWriter, r *http.Request, requestID string) {
	http.Redirect(w, r, internal.PathAuth+"?request_id="+url.QueryEscape(requestID), http.StatusFound)
}

func redirectToConsentWithError(w http.ResponseWriter, r *http.Request, requestID, message string) {
	location := internal.PathAuth + "?request_id=" + url.QueryEscape(requestID) + "&error=" + url.QueryEscape(message)
	http.Redirect(w, r, location, http.StatusFound)
}

func redirectAuthorizationError(w http.ResponseWriter, state *authorizeRequestState, code, description string) {
	target, err := url.Parse(state.RedirectURI)
	if err != nil {
		writeAuthorizeError(w, "invalid_request", "invalid redirect_uri")
		return
	}
	query := target.Query()
	query.Set("error", code)
	query.Set("error_description", description)
	if state.State != "" {
		query.Set("state", state.State)
	}
	target.RawQuery = query.Encode()
	w.Header().Set("Location", target.String())
	w.WriteHeader(http.StatusFound)
}

func writeAuthorizeError(w http.ResponseWriter, code, description string) {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(w, "Error: %s\nDescription: %s\n", code, description)
}

// validateStoredAuthorizeRequest re-checks the stored intent at consent time.
// This prevents a client's redirect URI/scope/grant configuration from being
// weakened between the initial authorization request and the user's decision.
func validateStoredAuthorizeRequest(r *http.Request, state *authorizeRequestState) error {
	clientInfo, err := storeImpl.ClientRepo.GetByID(r.Context(), state.ClientID)
	if err != nil {
		return fmt.Errorf("failed to load client")
	}
	client, ok := clientInfo.(*model.AuthClient)
	if !ok || client == nil {
		return fmt.Errorf("unknown client")
	}
	if !client.AllowsRedirectURI(state.RedirectURI) {
		return fmt.Errorf("redirect_uri is no longer registered for this client")
	}
	grantName := grantNameForResponseType(oauth2.ResponseType(state.ResponseType))
	if grantName == "" || !client.AllowsGrantType(grantName) {
		return fmt.Errorf("grant type is no longer enabled for this client")
	}
	for _, name := range state.Scope {
		scope, err := model.ParseScope(name)
		if err != nil {
			return fmt.Errorf("invalid scope: %s", name)
		}
		if !client.HasScope(scope.Name) {
			return fmt.Errorf("scope is no longer allowed for this client: %s", scope.Name)
		}
	}
	return nil
}
