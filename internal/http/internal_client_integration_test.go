//go:build integration

package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"auth-server/internal/store"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"

	oauth "github.com/go-oauth2/oauth2/v4"
)

func TestLiveInternalClientRegistrationAndScopePolicy(t *testing.T) {
	fixture := integrationServer(t)
	ctx := context.Background()
	originalID := internal.AuthServerConfig.Admin.ClientID
	var unrelatedID int64
	if err := internal.DB.Get(&unrelatedID, `INSERT INTO auth_client(display_name,secret,scopes,redirect_uris,is_public) VALUES('Existing application','keep-secret','{manage:system}','{https://app.example.invalid/callback}',TRUE) RETURNING id`); err != nil {
		t.Fatal(err)
	}
	internal.AuthServerConfig.Admin.ClientID = unrelatedID
	if err := store.EnsureAdminClient(); err != nil {
		t.Fatal(err)
	}
	if internal.AuthServerConfig.Admin.ClientID != originalID {
		t.Fatal("seed selected unrelated numeric ID")
	}
	var secret string
	if err := internal.DB.Get(&secret, "SELECT secret FROM auth_client WHERE id=$1", unrelatedID); err != nil || secret != "keep-secret" {
		t.Fatalf("unrelated client overwritten: %v", err)
	}
	var before, after int64
	if err := internal.DB.Get(&before, "SELECT security_version FROM auth_client WHERE internal_key=$1", model.AdminClientKey); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureAdminClient(); err != nil {
		t.Fatal(err)
	}
	if err := internal.DB.Get(&after, "SELECT security_version FROM auth_client WHERE internal_key=$1", model.AdminClientKey); err != nil || before != after {
		t.Fatalf("unchanged seed invalidated tokens: %v", err)
	}
	verifier := strings.Repeat("v", 43)
	digest := sha256.Sum256([]byte(verifier))
	authorize := func(clientID, userID string) (oauth.TokenInfo, error) {
		uri := internal.AuthServerConfig.Admin.RedirectURIs[0]
		if clientID != fixture.clientID() {
			uri = "https://app.example.invalid/callback"
		}
		return fixture.srv.Manager.GenerateAuthToken(ctx, oauth.Code, &oauth.TokenGenerateRequest{ClientID: clientID, UserID: userID, RedirectURI: uri, Scope: "manage:system", CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: oauth.CodeChallengeS256})
	}
	userID := strconv.FormatInt(fixture.userID, 10)
	if _, err := authorize(strconv.FormatInt(unrelatedID, 10), userID); err == nil {
		t.Fatal("ordinary client received management grant")
	}
	code, err := authorize(fixture.clientID(), userID)
	if err != nil {
		t.Fatal(err)
	}
	token := fixture.token(t, "manage:system")
	if _, err := internal.DB.Exec("DELETE FROM auth_admin WHERE user_id=$1", fixture.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := authorize(fixture.clientID(), userID); err == nil {
		t.Fatal("non-administrator received authorization code")
	}
	if _, err := fixture.srv.Manager.GenerateAccessToken(ctx, oauth.AuthorizationCode, &oauth.TokenGenerateRequest{ClientID: fixture.clientID(), Code: code.GetCode(), RedirectURI: internal.AuthServerConfig.Admin.RedirectURIs[0], CodeVerifier: verifier}); err == nil {
		t.Fatal("removed administrator exchanged code")
	}
	if _, err := fixture.srv.Manager.RefreshAccessToken(ctx, &oauth.TokenGenerateRequest{ClientID: fixture.clientID(), Refresh: token.GetRefresh()}); err == nil {
		t.Fatal("removed administrator refreshed inherited scopes")
	}
}
