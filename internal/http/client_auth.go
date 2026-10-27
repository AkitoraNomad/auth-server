package http

import (
	"context"
	"crypto/subtle"
	"errors"

	oauth2 "github.com/go-oauth2/oauth2/v4"
)

var errInvalidClientAuth = errors.New("invalid client authentication")

// authenticateConfidentialClient validates Basic/resource-server credentials.
// Public clients are intentionally not allowed to authenticate as resource
// servers, even though their OAuth client_id is public.
func authenticateConfidentialClient(ctx context.Context, manager oauth2.Manager, clientID, secret string) (oauth2.ClientInfo, error) {
	if clientID == "" {
		return nil, errInvalidClientAuth
	}
	cli, err := manager.GetClient(ctx, clientID)
	if err != nil || cli == nil || cli.IsPublic() || cli.GetSecret() == "" {
		return nil, errInvalidClientAuth
	}
	if subtle.ConstantTimeCompare([]byte(cli.GetSecret()), []byte(secret)) != 1 {
		return nil, errInvalidClientAuth
	}
	return cli, nil
}
