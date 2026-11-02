package store

import (
	"auth-server/internal/model"
	"context"
	"strconv"
	"strings"

	"github.com/go-oauth2/oauth2/v4"
	oautherrors "github.com/go-oauth2/oauth2/v4/errors"
)

func ManagementScopePolicy(ctx context.Context, client oauth2.ClientInfo, userID, scope string) error {
	for _, name := range strings.Fields(scope) {
		if name != model.ScopeManageSystem && name != model.ScopeManageClients {
			continue
		}
		registered, ok := client.(*model.AuthClient)
		id, err := strconv.ParseInt(userID, 10, 64)
		if !ok || !registered.IsAdminClient() || registered.IsDisabled || err != nil || id <= 0 {
			return oautherrors.ErrInvalidScope
		}
		allowed, err := IsAdmin(ctx, id)
		if err != nil {
			return err
		}
		if !allowed {
			return oautherrors.ErrInvalidScope
		}
		return nil
	}
	return nil
}
