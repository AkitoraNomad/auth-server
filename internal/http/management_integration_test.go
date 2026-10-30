//go:build integration

package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	authoauth "auth-server/internal/oauth2"
	"auth-server/internal/store"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	oauth "github.com/go-oauth2/oauth2/v4"
)

func runPhase3Regression(t *testing.T, f *integrationFixture) {
	ctx := context.Background()
	admin := f.token(t, "manage:system manage:clients").GetAccess()
	request := func(method, path string, body any, status int) *httptest.ResponseRecorder {
		t.Helper()
		var data *strings.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			data = strings.NewReader(string(encoded))
		}
		var w *httptest.ResponseRecorder
		if data == nil {
			w = f.request(method, path, admin, nil)
		} else {
			w = f.request(method, path, admin, data)
		}
		if w.Code != status {
			t.Fatalf("%s %s returned %d: %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	payload := func(w *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var result struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	createUser := func(email string) int64 {
		t.Helper()
		data := payload(request("POST", "/api/admin/users", map[string]any{"email": email, "display_name": "Phase3", "password": "Original-password-123!"}, 200))
		return int64(data["id"].(float64))
	}
	clientBody := map[string]any{"display_name": "Phase3 client", "is_public": false, "token_type": 1, "redirect_uris": []string{"https://app.example.invalid/cb"}, "scopes": []string{"profile"}, "grant_types": []string{"authorization_code", "refresh_token", "client_credentials"}}
	created := payload(request("POST", "/api/admin/clients", clientBody, 200))
	clientID := strconv.FormatInt(int64(created["id"].(float64)), 10)
	secret := created["secret"].(string)
	makeCode := func(userID int64) (oauth.TokenInfo, string) {
		t.Helper()
		verifier := strings.Repeat("v", 43)
		digest := sha256.Sum256([]byte(verifier))
		code, err := f.srv.Manager.GenerateAuthToken(ctx, oauth.Code, &oauth.TokenGenerateRequest{ClientID: clientID, UserID: strconv.FormatInt(userID, 10), RedirectURI: clientBody["redirect_uris"].([]string)[0], Scope: "profile", CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: oauth.CodeChallengeS256})
		if err != nil {
			t.Fatal(err)
		}
		return code, verifier
	}
	exchange := func(code oauth.TokenInfo, verifier string) (oauth.TokenInfo, error) {
		return f.srv.Manager.GenerateAccessToken(ctx, oauth.AuthorizationCode, &oauth.TokenGenerateRequest{ClientID: clientID, ClientSecret: secret, Code: code.GetCode(), CodeVerifier: verifier, RedirectURI: code.GetRedirectURI()})
	}
	mint := func(userID int64) oauth.TokenInfo {
		t.Helper()
		code, v := makeCode(userID)
		token, err := exchange(code, v)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	invalid := func(token oauth.TokenInfo) {
		t.Helper()
		if _, err := f.srv.Manager.LoadAccessToken(ctx, token.GetAccess()); err == nil {
			t.Fatal("old access token remained valid")
		}
		if token.GetRefresh() != "" {
			if _, err := f.srv.Manager.LoadRefreshToken(ctx, token.GetRefresh()); err == nil {
				t.Fatal("old refresh remained valid")
			}
		}
	}
	userID := createUser("phase3@example.invalid")
	userPath := "/api/admin/users/" + strconv.FormatInt(userID, 10)
	t.Run("UserEditDisablePasswordAndSSO", func(t *testing.T) {
		request("POST", "/api/admin/users", map[string]any{"email": "PHASE3@example.invalid", "password": "Original-password-123!"}, 409)
		request("POST", "/api/admin/users", map[string]any{"email": "bad-email", "password": "Original-password-123!"}, 400)
		token := mint(userID)
		code, v := makeCode(userID)
		user, err := store.UserRepo.GetUserByID(userID)
		if err != nil {
			t.Fatal(err)
		}
		session := model.NewSessionUserInfo(user, nil, "127.0.0.1", clientID, time.Now(), nil)
		session.AuthVersion, err = authoauth.UserSessionVersion(ctx, strconv.FormatInt(userID, 10))
		if err != nil {
			t.Fatal(err)
		}
		request("PATCH", userPath, map[string]any{"display_name": "Edited user"}, 200)
		if _, err := f.srv.Manager.LoadAccessToken(ctx, token.GetAccess()); err != nil {
			t.Fatal("display name edit unexpectedly revoked grants", err)
		}
		request("PATCH", userPath, map[string]any{"is_disabled": true}, 200)
		invalid(token)
		if _, err := exchange(code, v); err == nil {
			t.Fatal("disabled user's code exchanged")
		}
		if authoauth.SessionIsActive(ctx, session) {
			t.Fatal("disabled user's SSO survived")
		}
		pt := model.ProviderEmailPassword
		credentials := json.RawMessage(`{"password":"Original-password-123!"}`)
		if result, err := store.UserRepo.GetUserByCredentials("phase3@example.invalid", &pt, credentials, nil); err == nil && result != nil {
			t.Fatal("disabled user authenticated")
		}
		request("PATCH", userPath, map[string]any{"is_disabled": false}, 200)
		invalid(token)
		if authoauth.SessionIsActive(ctx, session) {
			t.Fatal("re-enabling revived old SSO")
		}
		token = mint(userID)
		request("PATCH", userPath, map[string]any{"email": "Renamed@example.invalid"}, 200)
		invalid(token)
		if result, err := store.UserRepo.GetUserByCredentials("PHASE3@example.invalid", &pt, credentials, nil); err == nil && result != nil {
			t.Fatal("old email authenticated after edit")
		}
		if result, err := store.UserRepo.GetUserByCredentials("RENAMED@example.invalid", &pt, credentials, nil); err != nil || result == nil || result.ID != userID {
			t.Fatalf("new case-insensitive email failed: %v", err)
		}
		token = mint(userID)
		code, v = makeCode(userID)
		user, _ = store.UserRepo.GetUserByID(userID)
		session = model.NewSessionUserInfo(user, nil, "127.0.0.1", clientID, time.Now(), nil)
		session.AuthVersion, _ = authoauth.UserSessionVersion(ctx, strconv.FormatInt(userID, 10))
		request("POST", userPath+"/password", map[string]any{"password": "short"}, 400)
		request("POST", userPath+"/password", map[string]any{"password": "Replacement-password-456!"}, 200)
		invalid(token)
		if _, err := exchange(code, v); err == nil {
			t.Fatal("password reset left code usable")
		}
		if authoauth.SessionIsActive(ctx, session) {
			t.Fatal("password reset left SSO usable")
		}
		if result, err := store.UserRepo.GetUserByCredentials("renamed@example.invalid", &pt, credentials, nil); err == nil && result != nil {
			t.Fatal("old password authenticated")
		}
		if result, err := store.UserRepo.GetUserByCredentials("renamed@example.invalid", &pt, json.RawMessage(`{"password":"Replacement-password-456!"}`), nil); err != nil || result == nil {
			t.Fatalf("new password failed: %v", err)
		}
		request("PATCH", "/api/admin/users/"+strconv.FormatInt(f.userID, 10), map[string]any{"is_disabled": true}, 409)
		request("PATCH", userPath, map[string]any{"email": "regression@example.invalid"}, 409)
		var passwordless int64
		if err := internal.DB.Get(&passwordless, "INSERT INTO auth_user(email) VALUES('passwordless@example.invalid') RETURNING id"); err != nil {
			t.Fatal(err)
		}
		passwordlessPath := "/api/admin/users/" + strconv.FormatInt(passwordless, 10)
		request("POST", passwordlessPath+"/password", map[string]any{"password": "Replacement-password-456!"}, 409)
		request("DELETE", passwordlessPath, nil, 200)
		limited := f.token(t, "manage:clients").GetAccess()
		denied := f.request("PATCH", userPath, limited, strings.NewReader(`{"display_name":"denied"}`))
		if denied.Code != 403 {
			t.Fatalf("user mutation without scope: %d", denied.Code)
		}
		request("PATCH", "/api/admin/users/999999999", map[string]any{"display_name": "Missing"}, 404)
		request("POST", "/api/admin/users/999999999/password", map[string]any{"password": "Replacement-password-456!"}, 404)
	})
	t.Run("ClientEditDisableRotateAndProtect", func(t *testing.T) {
		token := mint(userID)
		code, v := makeCode(userID)
		machine, err := f.srv.Manager.GenerateAccessToken(ctx, oauth.ClientCredentials, &oauth.TokenGenerateRequest{ClientID: clientID, ClientSecret: secret, Scope: "profile"})
		if err != nil {
			t.Fatal(err)
		}
		request("PATCH", "/api/admin/clients/"+clientID+"/state", map[string]any{"is_disabled": true}, 200)
		invalid(token)
		invalid(machine)
		introspectReq := httptest.NewRequest("POST", "/introspect", strings.NewReader(url.Values{"token": {machine.GetAccess()}}.Encode()))
		introspectReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		introspectReq.SetBasicAuth(clientID, secret)
		denied := httptest.NewRecorder()
		f.engine.ServeHTTP(denied, introspectReq)
		if denied.Code != 401 {
			t.Fatalf("disabled client introspection authentication returned %d", denied.Code)
		}

		if _, err := exchange(code, v); err == nil {
			t.Fatal("disabled client exchanged code")
		}
		if _, err := f.srv.Manager.GenerateAccessToken(ctx, oauth.ClientCredentials, &oauth.TokenGenerateRequest{ClientID: clientID, ClientSecret: secret, Scope: "profile"}); err == nil {
			t.Fatal("disabled client authenticated")
		}
		// Saving a stale editor must never implicitly re-enable a client that
		// another administrator disabled. State changes have their own endpoint.
		clientBody["is_disabled"] = false
		request("PUT", "/api/admin/clients/"+clientID, clientBody, 200)
		current, err := store.ClientRepo.GetByID(ctx, clientID)
		if err != nil || !current.(*model.AuthClient).IsDisabled {
			t.Fatal("metadata save re-enabled disabled client", err)
		}
		request("PATCH", "/api/admin/clients/"+clientID+"/state", map[string]any{"is_disabled": false}, 200)
		invalid(token)
		invalid(machine)
		token = mint(userID)
		code, v = makeCode(userID)
		clientBody["redirect_uris"] = []string{"https://app.example.invalid/new-cb", "https://app.example.invalid/other-cb"}
		clientBody["scopes"] = []string{"profile", "avatar"}
		clientBody["is_disabled"] = false
		request("PUT", "/api/admin/clients/"+clientID, clientBody, 200)
		invalid(token)
		if _, err := exchange(code, v); err == nil {
			t.Fatal("client edit left old code usable")
		}
		token = mint(userID)
		code, v = makeCode(userID)
		rotated := payload(request("POST", "/api/admin/clients/"+clientID+"/secret", nil, 200))
		newSecret := rotated["secret"].(string)
		if newSecret == secret || len(newSecret) < 32 {
			t.Fatal("secret did not rotate")
		}
		invalid(token)
		if _, err := exchange(code, v); err == nil {
			t.Fatal("secret reset left code usable")
		}
		if _, err := f.srv.Manager.GenerateAccessToken(ctx, oauth.ClientCredentials, &oauth.TokenGenerateRequest{ClientID: clientID, ClientSecret: secret, Scope: "profile"}); err == nil {
			t.Fatal("old secret authenticated")
		}
		secret = newSecret
		if _, err := f.srv.Manager.GenerateAccessToken(ctx, oauth.ClientCredentials, &oauth.TokenGenerateRequest{ClientID: clientID, ClientSecret: secret, Scope: "profile"}); err != nil {
			t.Fatal("new secret failed", err)
		}
		// Public/confidential identity changes are explicit new registrations.
		clientBody["is_public"] = true
		request("PUT", "/api/admin/clients/"+clientID, clientBody, 400)
		clientBody["is_public"] = false
		protected := map[string]any{"display_name": "broken", "is_public": true, "token_type": 1, "redirect_uris": []string{"https://app.example.invalid/cb"}, "scopes": []string{"manage:system", "manage:clients"}, "grant_types": []string{"authorization_code", "refresh_token"}}
		request("PUT", "/api/admin/clients/2", protected, 400)
		request("PATCH", "/api/admin/clients/2/state", map[string]any{"is_disabled": true}, 400)
		request("POST", "/api/admin/clients/2/secret", nil, 400)
		request("DELETE", "/api/admin/clients/2", nil, 400)
		protected["display_name"] = "Phase3 public"
		data := payload(request("POST", "/api/admin/clients", protected, 200))
		publicID := strconv.FormatInt(int64(data["id"].(float64)), 10)
		request("POST", "/api/admin/clients/"+publicID+"/secret", nil, 400)
		request("DELETE", "/api/admin/clients/"+publicID, nil, 200)
		w := request("GET", "/api/admin/clients?q=Phase3", nil, 200)
		if strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), `"secret"`) {
			t.Fatal("client list exposed secret")
		}
		token = mint(userID)
		request("DELETE", "/api/admin/clients/"+clientID, nil, 200)
		invalid(token)
		request("DELETE", "/api/admin/clients/"+clientID, nil, 404)
	})
	t.Run("PaginationSearchStatusAndInputErrors", func(t *testing.T) {
		for i := 0; i < 23; i++ {
			createUser(fmt.Sprintf("page-%02d@example.invalid", i))
		}
		q := "/api/admin/users?q=" + url.QueryEscape("page-")
		data := payload(request("GET", q+"&page=2&page_size=10", nil, 200))
		if data["total"] != float64(23) || len(data["items"].([]any)) != 10 || data["page"] != float64(2) {
			t.Fatalf("bad pagination: %#v", data)
		}
		data = payload(request("GET", q+"&page=99&page_size=10", nil, 200))
		if data["page"] != float64(3) || len(data["items"].([]any)) != 3 {
			t.Fatal("out-of-range page was not clamped")
		}
		request("PATCH", userPath, map[string]any{"is_disabled": true}, 200)
		data = payload(request("GET", "/api/admin/users?q=renamed&status=disabled", nil, 200))
		if data["total"] != float64(1) {
			t.Fatal("status filter failed")
		}
		for _, path := range []string{"users", "clients", "admins", "sessions"} {
			data = payload(request("GET", "/api/admin/"+path+"?page_size=1", nil, 200))
			if len(data["items"].([]any)) > 1 {
				t.Fatal("page_size ignored")
			}
		}
		request("DELETE", "/api/admin/admins/999999999", nil, 404)
		request("DELETE", "/api/admin/sessions/999999999", nil, 404)
		for _, q := range []string{"page=0", "page=-1", "page_size=101", "page_size=bad", "status=wrong"} {
			request("GET", "/api/admin/users?"+q, nil, 400)
		}
		data = payload(request("GET", "/api/admin/users?q="+url.QueryEscape("' OR 1=1 --"), nil, 200))
		if data["total"] != float64(0) {
			t.Fatal("search was not treated literally")
		}
		request("DELETE", userPath, nil, 200)
		request("DELETE", userPath, nil, 404)
		// Restore the small inventory used by the baseline browser regression.
		if _, err := internal.DB.Exec("DELETE FROM auth_user WHERE email LIKE 'page-%@example.invalid'"); err != nil {
			t.Fatal(err)
		}
	})
}
