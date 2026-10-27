package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"auth-server/internal/password"
	"auth-server/internal/render"
	storeImpl "auth-server/internal/store"
	"auth-server/internal/util"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-session/session/v3"
)

type loginReq struct {
	ProviderType *model.ProviderType `json:"provider_type,omitempty"`
	LoginKey     string              `json:"login_key,omitempty"`
	Data         json.RawMessage     `json:"data,omitempty"`
	CfToken      string              `json:"cf_token,omitempty"`
	RequestID    string              `json:"request_id,omitempty"`
}

func getLoginHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionStore, err := session.Start(c.Request.Context(), c.Writer, c.Request)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		switch c.Request.Method {
		case http.MethodPost:
			handleJsonLogin(c, sessionStore)
		case http.MethodGet:
			handleLoginPage(c, sessionStore)
		default:
			c.Status(http.StatusMethodNotAllowed)
		}
	}
}

func handleLoginPage(c *gin.Context, s session.Store) {
	requestID := c.Query("request_id")
	if requestID == "" {
		requestID = activeAuthorizeRequestID(s)
	}
	state := findAuthorizeRequest(s, requestID)
	if state == nil {
		writeAuthorizeError(c.Writer, "invalid_request", "authorization request not found or expired")
		return
	}
	setActiveAuthorizeRequest(s, state.ID)

	nextURL := internal.PathAuth + "?request_id=" + url.QueryEscape(state.ID)
	if _, ok := s.Get(internal.SessionKeyUserID); ok {
		_ = s.Save()
		c.Redirect(http.StatusFound, nextURL)
		return
	}

	clientInfo, err := storeImpl.ClientRepo.GetByID(c.Request.Context(), state.ClientID)
	if err != nil {
		slog.Error("failed to load login client", "client_id", state.ClientID, "err", err)
		c.String(http.StatusInternalServerError, "failed to get client info")
		return
	}
	client, ok := clientInfo.(*model.AuthClient)
	if !ok || client == nil {
		writeAuthorizeError(c.Writer, "invalid_request", "unknown client")
		return
	}

	if err := s.Save(); err != nil {
		c.String(http.StatusInternalServerError, "failed to save session")
		return
	}
	if err := renderLoginPage(c.Writer, client.DisplayName, nextURL, state.ID, http.StatusUnauthorized, nil); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
	}
}

func handleJsonLogin(c *gin.Context, s session.Store) {
	req := loginReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		responseJson(c.Writer, 1, "invalid_request_body")
		return
	}
	if req.ProviderType == nil || req.LoginKey == "" {
		responseJson(c.Writer, 1, "invalid_request")
		return
	}
	remoteIp := getIp(c.Request)
	if err := verifyRequest(req.CfToken, remoteIp); err != nil {
		slog.Warn("failed to verify request", "ip", remoteIp)
		responseJson(c.Writer, 1, "turnstile_verification_failed")
		return
	}
	userInfo, err := storeImpl.UserRepo.GetUserByCredentials(req.LoginKey, req.ProviderType, req.Data, s)
	if err != nil {
		slog.Warn("failed to verify credentials", "loginKey", req.LoginKey, "provider", req.ProviderType, "err", err)
		responseJson(c.Writer, 1, "invalid_credentials")
		return
	}
	if userInfo == nil {
		// Email/password failures must be indistinguishable from a wrong
		// password, including timing. Telegram unregistered users still need
		// the registration continuation flow.
		if *req.ProviderType == model.ProviderEmailPassword {
			dummy := model.EmailPasswordProviderData{}
			_ = json.Unmarshal(req.Data, &dummy)
			password.DummyVerify(dummy.Password)
			slog.Warn("failed to verify credentials", "loginKey", req.LoginKey, "provider", req.ProviderType, "err", "invalid_credentials")
			responseJson(c.Writer, 1, "invalid_credentials")
			return
		}
		slog.Warn("unregistered credential", "loginKey", req.LoginKey, "provider", req.ProviderType)
		if err := saveTelegramSessionOnUnregistered(req.Data, s); err != nil {
			responseJson(c.Writer, 1, err.Error())
			return
		}
		responseJson(c.Writer, 0, map[string]any{"user": nil})
		return
	}

	clientID := ""
	nextURL := ""
	requestID := req.RequestID
	if requestID == "" {
		requestID = activeAuthorizeRequestID(s)
	}
	if state := findAuthorizeRequest(s, requestID); state != nil {
		clientID = state.ClientID
		nextURL = internal.PathAuth + "?request_id=" + url.QueryEscape(state.ID)
		setActiveAuthorizeRequest(s, state.ID)
	}
	sessionUser := model.NewSessionUserInfo(userInfo, req.ProviderType, remoteIp, clientID, time.Now(), map[string]any{
		"login_key": req.LoginKey,
	})
	s.Set(internal.SessionKeyUserID, sessionUser)
	if err := s.Save(); err != nil {
		responseJson(c.Writer, 1, err)
		return
	}
	responseJson(c.Writer, 0, map[string]any{
		"user":     sessionUser.User,
		"next_url": nextURL,
	})
}

func getIp(r *http.Request) string {
	remoteIp := r.Header.Get("CF-Connecting-IP")
	if remoteIp == "" {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		remoteIp = host
	}
	return remoteIp
}

func renderLoginPage(w http.ResponseWriter, clientName, nextURL, requestID string, code int, err error) error {
	data := map[string]any{
		"client_name": clientName,
		"next_url":    nextURL,
		"request_id":  requestID,
		"site_key":    internal.AuthServerConfig.Cloudflare.Turnstile.Key,
		"bot_name":    internal.AuthServerConfig.Telegram.BotName,
	}
	if err != nil {
		data["err"] = err
	}
	return render.Html(w, "login.gohtml", code, data)
}

// saveTelegramSessionOnUnregistered validates Telegram credential and stores it into session
// so that the registration flow can proceed. It mirrors the previous inline logic.
func saveTelegramSessionOnUnregistered(tgData json.RawMessage, s session.Store) error {
	botTokenDigest := util.DigestSHA256(internal.AuthServerConfig.Telegram.BotToken)
	if !util.ValidateTelegramCredential(tgData, botTokenDigest) {
		slog.Warn("Invalid Telegram Credentials")
		return fmt.Errorf("invalid_telegram_credentials")
	}
	var tgUser model.TelegramUser
	if err := json.Unmarshal(tgData, &tgUser); err != nil {
		return err
	}
	s.Set(internal.SessionKeyTelegramData, tgUser)
	if err := s.Save(); err != nil {
		return err
	}
	return nil
}
