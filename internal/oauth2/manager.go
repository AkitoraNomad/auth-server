package oauth2

import (
	"context"

	oauth2 "github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/errors"
)

// clientBoundManager enforces that a refresh token can only be redeemed by the
// OAuth2 client that originally received it.
type clientBoundManager struct {
	oauth2.Manager
}

func (m *clientBoundManager) RefreshAccessToken(ctx context.Context, tgr *oauth2.TokenGenerateRequest) (oauth2.TokenInfo, error) {
	if tgr == nil || tgr.ClientID == "" || tgr.Refresh == "" {
		return nil, errors.ErrInvalidRefreshToken
	}
	ti, err := m.Manager.LoadRefreshToken(ctx, tgr.Refresh)
	if err != nil {
		return nil, err
	}
	if ti == nil || ti.GetClientID() != tgr.ClientID {
		return nil, errors.ErrInvalidRefreshToken
	}
	return m.Manager.RefreshAccessToken(ctx, tgr)
}
