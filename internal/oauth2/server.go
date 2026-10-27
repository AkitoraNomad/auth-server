package oauth2

import (
	"auth-server/internal"
	"auth-server/internal/model"
	storeImpl "auth-server/internal/store"
	"auth-server/internal/util"
	"context"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/errors"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-session/redis/v3"
	"github.com/go-session/session/v3"
)

func InitServer() *server.Server {
	manager := manage.NewDefaultManager()

	// manage.DefaultAuthorizeCodeTokenCfg
	manager.SetAuthorizeCodeTokenCfg(&manage.Config{
		AccessTokenExp:    time.Hour * 2,
		RefreshTokenExp:   time.Hour * 24 * 3,
		IsGenerateRefresh: true,
	})

	manager.MustTokenStorage(storeImpl.NewRedisStoreWithCli(internal.Rdb), nil)
	session.InitManager(sessionOptions()...)
	manager.MapAccessGenerate(&ClientConfigTokenGenerate{})
	manager.MapClientStorage(storeImpl.ClientRepo)

	oauth2ServerConfig := server.Config{
		TokenType: "Bearer",
		AllowedResponseTypes: []oauth2.ResponseType{
			oauth2.Code,
		},
		AllowedGrantTypes: []oauth2.GrantType{
			oauth2.AuthorizationCode,
			oauth2.ClientCredentials,
			oauth2.Refreshing,
		},
		AllowedCodeChallengeMethods: []oauth2.CodeChallengeMethod{
			oauth2.CodeChallengeS256,
		},
		ForcePKCE: true,
	}
	boundManager := newClientBoundManager(manager, internal.Rdb)
	srv := server.NewServer(&oauth2ServerConfig, boundManager)
	srv.SetClientInfoHandler(clientInfoHandler)
	srv.SetClientAuthorizedHandler(clientAuthorizedHandler)
	srv.SetClientScopeHandler(clientScopeHandler)
	// Refresh scope validation is performed inside clientBoundManager, after
	// client authentication and replay detection. The library's scope hook
	// preloads the old token and would bypass replay handling for scoped retries.
	srv.SetUserAuthorizationHandler(userAuthorizeHandler)
	srv.SetAuthorizeScopeHandler(scopeHandler)
	srv.SetResponseTokenHandler(getResponseTokenHandler(boundManager))
	srv.SetInternalErrorHandler(func(err error) (re *errors.Response) {
		// go-oauth2 does not put ErrInvalidRedirectURI in its public
		// descriptions map, which would otherwise turn a token request with a
		// mismatched redirect_uri into an opaque 500 server_error.
		if err == errors.ErrInvalidRedirectURI {
			return &errors.Response{
				Error:       errors.ErrInvalidGrant,
				Description: errors.Descriptions[errors.ErrInvalidGrant],
				StatusCode:  http.StatusBadRequest,
			}
		}
		slog.Error("Internal Error", "err", err)
		return
	})
	srv.SetResponseErrorHandler(func(re *errors.Response) {
		slog.Error("Response Error", "err", re.Error.Error())
	})
	srv.SetPreRedirectErrorHandler(preRedirectErrorHandler)
	return srv
}

func preRedirectErrorHandler(w http.ResponseWriter, req *server.AuthorizeRequest, err error) error {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusBadRequest)

	if req == nil {
		msg := "Error: invalid_request\nDescription: " + err.Error() + "\n"
		msg += "Hint: check that response_type, client_id, and redirect_uri are present and valid.\n"
		slog.Error("Authorization PreRedirect Error (req is nil)", "err", err)
		_, _ = w.Write([]byte(msg))
		return nil
	}

	slog.Error("Authorization PreRedirect Error",
		"client_id", req.ClientID,
		"response_type", string(req.ResponseType),
		"redirect_uri", req.RedirectURI,
		"err", err,
	)

	body := "Error: access_denied\nDescription: " + err.Error() + "\n"
	if req.State != "" {
		body += "State: " + req.State + "\n"
	}
	_, _ = w.Write([]byte(body))
	return nil
}

func userAuthorizeHandler(w http.ResponseWriter, r *http.Request) (userID string, err error) {
	s, err := session.Start(r.Context(), w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	uid, ok := s.Get(internal.SessionKeyUserID)
	if !ok {
		http.Error(w, "failed to read session data", http.StatusInternalServerError)
		return
	}
	var sessionUser *model.SessionUserInfo
	if v, ok := uid.(map[string]any); ok {
		var mapErr error
		sessionUser, mapErr = util.MapToStruct[*model.SessionUserInfo](v)
		if mapErr != nil {
			http.Error(w, "failed to parse session user info", http.StatusInternalServerError)
			err = mapErr
			return
		}
	} else if v, ok := uid.(*model.SessionUserInfo); ok {
		sessionUser = v
	} else {
		http.Error(w, "invalid session user info type", http.StatusInternalServerError)
		return
	}
	if sessionUser == nil || sessionUser.User == nil {
		return "", errors.New("invalid session user")
	}
	userID = strconv.FormatInt(sessionUser.User.ID, 10)
	if !SessionIsActive(r.Context(), sessionUser) {
		http.Error(w, "user account is not active", http.StatusUnauthorized)
		return "", errors.New("user account is not active")
	}
	*r = *r.WithContext(context.WithValue(r.Context(), authenticatedVersionKey{}, sessionUser.AuthVersion))
	*r = *r.WithContext(context.WithValue(r.Context(), authenticatedTimeKey{}, sessionUser.LoginAt))
	return
}

func scopeHandler(w http.ResponseWriter, r *http.Request) (string, error) {
	s, err := session.Start(r.Context(), w, r)
	if err != nil {
		return "", err
	}
	requestID := r.FormValue("request_id")
	if requestID == "" {
		return "", errors.New("failed to get authorize request id")
	}
	raw, ok := s.Get(internal.SessionKeyConsentByRequest)
	if !ok || raw == nil {
		return "", errors.New("failed to get scope")
	}
	var consents map[string][]string
	if typed, ok := raw.(map[string][]string); ok {
		consents = typed
	} else {
		decoded, err := util.Decode[map[string][]string](raw)
		if err != nil {
			return "", err
		}
		consents = decoded
	}
	names := consents[requestID]
	if len(names) == 0 {
		return "", errors.New("failed to get scope")
	}
	return strings.Join(names, " "), nil
}

func getResponseTokenHandler(manager oauth2.Manager) server.ResponseTokenHandler {
	return func(w http.ResponseWriter, data map[string]interface{}, header http.Header, statusCode ...int) error {
		t := data["access_token"]
		if ts, ok := t.(string); ok {
			ti, err := manager.LoadAccessToken(context.Background(), ts)
			if err == nil {
				userId := ti.GetUserID()
				if userId != "" {
					slog.Info("access token generated", "user_id", userId)
					// update user last login time for user-bound tokens only
					updateUserLastLogin(userId)
				} else {
					slog.Debug("client credentials access token generated")
				}
			} else {
				slog.Warn("failed to load access token after gen token", "err", err)
			}
		}
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")

		for key := range header {
			w.Header().Set(key, header.Get(key))
		}

		status := http.StatusOK
		if len(statusCode) > 0 && statusCode[0] > 0 {
			status = statusCode[0]
		}

		w.WriteHeader(status)
		return json.NewEncoder(w).Encode(data)
	}
}

func updateUserLastLogin(userId string) {
	if uid, err := strconv.ParseInt(userId, 10, 64); err == nil {
		if err := storeImpl.UserRepo.UpdateLastLogin(uid); err != nil {
			slog.Error("failed to update user last login time", "err", err)
		} else {
			slog.Info("updated user last login time", "user_id", uid)
		}
	} else {
		slog.Error("failed to parse user id", "user_id", userId, "err", err)
	}
}

func sessionOptions() []session.Option {
	signKey := sha256.Sum256([]byte(internal.AuthServerConfig.JWT[0].Sec))
	return []session.Option{
		session.SetStore(redis.NewRedisStoreWithCli(internal.Rdb)),
		session.SetExpired(15 * 60),
		session.SetCookieLifeTime(15 * 60),
		session.SetSecure(strings.HasPrefix(internal.AuthServerConfig.Issuer, "https://")),
		session.SetSameSite(http.SameSiteLaxMode),
		session.SetEnableSIDInURLQuery(false),
		session.SetEnableSIDInHTTPHeader(false),
		session.SetSign(signKey[:]),
	}
}
