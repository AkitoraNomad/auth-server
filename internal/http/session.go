package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	authoauth2 "auth-server/internal/oauth2"
	"auth-server/internal/util"
	"context"
	"github.com/go-session/session/v3"
)

func currentSessionUser(ctx context.Context, s session.Store) *model.SessionUserInfo {
	value, ok := s.Get(internal.SessionKeyUserID)
	if !ok {
		return nil
	}
	var user *model.SessionUserInfo
	switch v := value.(type) {
	case *model.SessionUserInfo:
		user = v
	case map[string]any:
		user, _ = util.MapToStruct[*model.SessionUserInfo](v)
	}
	if !authoauth2.SessionIsActive(ctx, user) {
		s.Delete(internal.SessionKeyUserID)
		s.Delete(internal.SessionKeyTelegramData)
		_ = s.Save()
		return nil
	}
	return user
}
