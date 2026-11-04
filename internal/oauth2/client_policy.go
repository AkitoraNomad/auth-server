package oauth2

import (
	"auth-server/internal/model"
	storeImpl "auth-server/internal/store"
	"context"
	"net/http"

	"github.com/go-oauth2/oauth2/v4"
	oerrors "github.com/go-oauth2/oauth2/v4/errors"
)

// clientInfoHandler accepts either HTTP Basic (confidential clients) or the
// form body (public clients such as the admin SPA, which must not carry a
// secret). The token endpoint itself still verifies the secret for confidential
// clients.
func clientInfoHandler(r *http.Request) (string, string, error) {
	if user, password, ok := r.BasicAuth(); ok {
		if user == "" {
			return "", "", oerrors.ErrInvalidClient
		}
		return user, password, nil
	}
	if r.Header.Get("Authorization") != "" {
		// A malformed Authorization header must not silently fall back to form auth.
		return "", "", oerrors.ErrInvalidClient
	}
	clientID := r.FormValue("client_id")
	if clientID == "" {
		return "", "", oerrors.ErrInvalidClient
	}
	return clientID, r.FormValue("client_secret"), nil
}

func clientAuthorizedHandler(clientID string, grant oauth2.GrantType) (bool, error) {
	cli, err := storeImpl.ClientRepo.GetByID(context.Background(), clientID)
	if err != nil {
		return false, err
	}
	if cli == nil {
		return false, nil
	}
	grantName := grant.String()
	if grant == oauth2.Implicit {
		grantName = "implicit"
	}
	if grantName == "" {
		return false, nil
	}
	if c, ok := cli.(*model.AuthClient); ok {
		return c.AllowsGrantType(grantName), nil
	}
	return true, nil
}

func clientScopeHandler(tgr *oauth2.TokenGenerateRequest) (bool, error) {
	cli, err := storeImpl.ClientRepo.GetByID(context.Background(), tgr.ClientID)
	if err != nil {
		return false, err
	}
	if cli == nil {
		return false, nil
	}
	requested, err := model.ParseScopes(tgr.Scope)
	if err != nil {
		return false, nil
	}
	for _, scope := range requested {
		if !cliHasScope(cli, scope.Name) {
			return false, nil
		}
	}
	return true, nil
}

func cliHasScope(cli oauth2.ClientInfo, scope string) bool {
	if c, ok := cli.(*model.AuthClient); ok {
		return c.HasScope(scope)
	}
	if c, ok := cli.(interface{ HasScope(string) bool }); ok {
		return c.HasScope(scope)
	}
	return false
}
