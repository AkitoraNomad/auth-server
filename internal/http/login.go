package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	authoauth2 "auth-server/internal/oauth2"
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
	"strconv"
	"strings"
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
	if currentSessionUser(c.Request.Context(), s) != nil {
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
	token, err := csrfToken(s)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if err := renderLoginPage(c.Writer, client.DisplayName, nextURL, state.ID, token, http.StatusUnauthorized, nil); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
	}
}

func handleJsonLogin(c *gin.Context, s session.Store) {
	if validateCSRF(c.Request, s) != nil {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if currentSessionUser(c.Request.Context(), s) != nil {
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	if strings.ToLower(c.ContentType()) != "application/json" {
		responseJson(c.Writer, 1, "invalid_request_body")
		return
	}
	req := loginReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		responseJson(c.Writer, 1, "invalid_request_body")
		return
	}
	if req.ProviderType == nil || req.LoginKey == "" {
		responseJson(c.Writer, 1, "invalid_request")
		return
	}
	if !allowAuthAttempt(c.Request.Context(), "account:"+req.ProviderType.String()+":"+strings.ToLower(req.LoginKey), 10) {
		c.Header("Retry-After", "60")
		c.Status(http.StatusTooManyRequests)
		return
	}
	remoteIp := getIp(c.Request)
	if err := verifyRequest(c.Request.Context(), req.CfToken, remoteIp); err != nil {
		slog.Warn("failed to verify request", "ip", remoteIp)
		responseJson(c.Writer, 1, "turnstile_verification_failed")
		return
	}
	userInfo, err := storeImpl.UserRepo.GetUserByCredentials(req.LoginKey, req.ProviderType, req.Data, s)
	if err != nil {
		slog.Warn("failed to verify credentials", "provider", req.ProviderType, "err", err)
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
			slog.Warn("failed to verify credentials", "provider", req.ProviderType, "err", "invalid_credentials")
			responseJson(c.Writer, 1, "invalid_credentials")
			return
		}
		slog.Warn("unregistered credential", "provider", req.ProviderType)
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
	// Rotate the session identifier after successful authentication to close
	// session fixation. Refresh copies the pending OAuth authorization state.
	version, err := authoauth2.UserSessionVersion(c.Request.Context(), strconv.FormatInt(userInfo.ID, 10))
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if err := s.Save(); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if rotated, err := session.Refresh(c.Request.Context(), c.Writer, c.Request); err == nil {
		s = rotated
	} else {
		slog.Warn("failed to rotate session after login", "err", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	sessionUser := model.NewSessionUserInfo(userInfo, req.ProviderType, remoteIp, clientID, time.Now(), map[string]any{
		"login_key": req.LoginKey,
	})
	sessionUser.AuthVersion = version
	s.Delete(csrfSessionKey)
	s.Set(internal.SessionKeyUserID, sessionUser)
	if err := s.Save(); err != nil {
		responseJson(c.Writer, 1, err)
		return
	}
	if err := storeImpl.UserRepo.UpdateLastLogin(userInfo.ID); err != nil {
		slog.Warn("failed to update last login", "user_id", userInfo.ID, "err", err)
	}
	responseJson(c.Writer, 0, map[string]any{
		"user":     sessionUser.User,
		"next_url": nextURL,
	})
}

func getIp(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	if !trustedProxy(host) {
		return host
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" || len(forwarded) > 4096 {
		return host
	}
	chain := strings.Split(forwarded, ",")
	for i := len(chain) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(chain[i])
		if net.ParseIP(ip) == nil {
			return host
		}
		if i == 0 || !trustedProxy(ip) {
			return ip
		}
	}
	return host
}

func trustedProxy(address string) bool {
	ip := net.ParseIP(address)
	if ip == nil {
		return false
	}
	for _, value := range internal.AuthServerConfig.TrustedProxies {
		if candidate := net.ParseIP(value); candidate != nil && candidate.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(value); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func renderLoginPage(w http.ResponseWriter, clientName, nextURL, requestID, csrf string, code int, err error) error {
	data := map[string]any{
		"client_name": clientName,
		"next_url":    nextURL,
		"request_id":  requestID,
		"csrf_token":  csrf,
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
	if internal.AuthServerConfig.Telegram.BotToken == "" {
		return fmt.Errorf("telegram login is disabled")
	}
	botTokenDigest := util.DigestSHA256(internal.AuthServerConfig.Telegram.BotToken)
	tg, err := util.ParseTelegramCredential(tgData, botTokenDigest, time.Now())
	if err != nil {
		slog.Warn("invalid telegram credential", "err", err)
		return fmt.Errorf("invalid_telegram_credentials")
	}
	tgUser := model.TelegramUser{
		Id:        tg.ID,
		FirstName: tg.FirstName,
		LastName:  tg.LastName,
		Username:  tg.Username,
		PhotoUrl:  tg.PhotoURL,
		AuthDate:  tg.AuthDate,
	}
	s.Set(internal.SessionKeyTelegramData, tgUser)
	if err := s.Save(); err != nil {
		return err
	}
	return nil
}
