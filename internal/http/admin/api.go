package admin

import (
	"auth-server/internal"
	"auth-server/internal/model"
	adminoauth2 "auth-server/internal/oauth2"
	"auth-server/internal/password"
	"auth-server/internal/store"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-oauth2/oauth2/v4"
)

func responseJSON(c *gin.Context, code int, data any) {
	c.JSON(code, gin.H{"code": 0, "data": data})
}

func responseErr(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"code": 1, "msg": msg})
}

func adminClientID() string {
	return strconv.FormatInt(internal.AuthServerConfig.Admin.ClientID, 10)
}

// RequireAdmin authenticates the management API. It deliberately performs all
// of the following checks on every request:
//
//   - a signed JWT with the configured issuer/audience and access-token claims;
//   - a Redis lookup, so revoked/expired tokens fail immediately;
//   - the token belongs to the first-party management client;
//   - the token has a real user (client credentials tokens are rejected);
//   - that user still exists in auth_admin (removal takes effect immediately).
func RequireAdmin(mgr oauth2.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			responseErr(c, http.StatusUnauthorized, "missing bearer token")
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		expectedClientID := adminClientID()

		claims, err := adminoauth2.ParseAccessToken(token, expectedClientID)
		if err != nil {
			slog.Warn("admin API JWT rejected", "err", err, "ip", c.ClientIP())
			responseErr(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}
		if claims.ClientID != expectedClientID {
			responseErr(c, http.StatusForbidden, "token was not issued to the management client")
			c.Abort()
			return
		}
		if claims.Subject == "" {
			responseErr(c, http.StatusForbidden, "client credentials tokens cannot access the management API")
			c.Abort()
			return
		}

		ti, err := mgr.LoadAccessToken(c.Request.Context(), token)
		if err != nil || ti == nil {
			responseErr(c, http.StatusUnauthorized, "token is revoked or expired")
			c.Abort()
			return
		}
		if ti.GetClientID() != expectedClientID || ti.GetUserID() != claims.Subject {
			responseErr(c, http.StatusUnauthorized, "token claims do not match the token store")
			c.Abort()
			return
		}
		if !sameScopeSet(claims.Scope, ti.GetScope()) {
			responseErr(c, http.StatusUnauthorized, "token scope claims do not match")
			c.Abort()
			return
		}

		userID, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil || userID <= 0 {
			responseErr(c, http.StatusUnauthorized, "invalid user in token")
			c.Abort()
			return
		}
		isAdmin, err := store.IsAdmin(c.Request.Context(), userID)
		if err != nil {
			slog.Error("failed to check administrator membership", "user_id", userID, "err", err)
			responseErr(c, http.StatusInternalServerError, "failed to verify administrator")
			c.Abort()
			return
		}
		if !isAdmin {
			responseErr(c, http.StatusForbidden, "user is not an administrator")
			c.Abort()
			return
		}

		if authenticated, ok := ti.(interface{ AuthenticationTime() time.Time }); ok {
			c.Set("admin_auth_time", authenticated.AuthenticationTime())
		}
		c.Set("admin_user_id", userID)
		c.Set("admin_scope", ti.GetScope())
		c.Set("admin_client_id", expectedClientID)
		c.Next()
	}
}

// RequireScope enforces the route-level management scope. It must run after
// RequireAdmin.
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, ok := c.Get("admin_scope")
		if !ok {
			responseErr(c, http.StatusUnauthorized, "missing administrator context")
			c.Abort()
			return
		}
		scopeValue, _ := value.(string)
		if !model.HasScope(scopeValue, scope) {
			responseErr(c, http.StatusForbidden, "missing required scope: "+scope)
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireRecentAuthentication(mgr oauth2.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		// Reuse the request snapshot; other Manager implementations keep the
		// existing fail-closed interface check.
		recent := false
		if value, exists := c.Get("admin_auth_time"); exists {
			authTime, ok := value.(time.Time)
			recent = ok && adminoauth2.IsRecentAuthentication(authTime)
		} else if checker, ok := mgr.(interface {
			RecentlyAuthenticated(context.Context, string) bool
		}); ok {
			recent = checker.RecentlyAuthenticated(c.Request.Context(), strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		}
		if !recent {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 1, "msg": "Recent sign-in required. Sign out and sign in again before changing security settings."})
			return
		}
		c.Next()
	}
}

func GetMeAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		value, ok := c.Get("admin_user_id")
		userID, _ := value.(int64)
		if !ok || userID <= 0 {
			responseErr(c, http.StatusUnauthorized, "missing administrator context")
			return
		}
		user, err := store.UserRepo.GetUserByID(userID)
		if err != nil {
			responseErr(c, http.StatusInternalServerError, "failed to load administrator")
			return
		}
		scopeValue := ""
		if v, ok := c.Get("admin_scope"); ok {
			scopeValue, _ = v.(string)
		}
		responseJSON(c, http.StatusOK, gin.H{
			"user":      user,
			"scopes":    strings.Fields(scopeValue),
			"client_id": adminClientID(),
		})
	}
}

// ---------- Users ----------

func GetUsersAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := store.UserRepo.(*store.DbUserStore).List()
		if err != nil {
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		responseJSON(c, http.StatusOK, users)
	}
}

func PostUserAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
			Password    string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Email) == "" || utf8.RuneCountInString(req.Password) < 12 {
			responseErr(c, http.StatusBadRequest, "email and a password of at least 12 characters are required")
			return
		}
		u := &model.User{Email: strings.TrimSpace(req.Email), DisplayName: strings.TrimSpace(req.DisplayName)}
		providerData, err := hashPassword(req.Password)
		if err != nil {
			responseErr(c, http.StatusBadRequest, "invalid password")
			return
		}
		pt := model.ProviderEmailPassword
		provider := model.AuthUserProvider{
			LoginKey:     u.Email,
			ProviderType: &pt,
			ProviderData: providerData,
		}
		if err := store.UserRepo.AddUser(u, provider); err != nil {
			if strings.Contains(err.Error(), "user_exists") {
				responseErr(c, http.StatusConflict, "user already exists")
				return
			}
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		responseJSON(c, http.StatusOK, gin.H{"id": u.ID})
	}
}

func DeleteUserAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			responseErr(c, http.StatusBadRequest, "invalid id")
			return
		}

		if err := store.UserRepo.(*store.DbUserStore).Delete(id); err != nil {
			if errors.Is(err, store.ErrLastAdmin) {
				responseErr(c, http.StatusConflict, err.Error())
				return
			}
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		// Also advance the persistent security generation, invalidating existing
		// codes, token families and SSO sessions for this user.
		if err := adminoauth2.RevokeUserSessions(c.Request.Context(), id); err != nil {
			slog.Error("failed to revoke sessions after user deletion", "user_id", id, "err", err)
			responseErr(c, http.StatusInternalServerError, "user deleted but session revocation failed")
			return
		}
		responseJSON(c, http.StatusOK, nil)
	}
}

// ---------- Clients ----------

func GetClientsAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		clients, err := store.ClientRepo.(*store.DbClientStore).List(c.Request.Context())
		if err != nil {
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		// AuthClient.Secret is json:"-" so secrets never appear in this list.
		responseJSON(c, http.StatusOK, clients)
	}
}

func PostClientAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			DisplayName  string   `json:"display_name"`
			Domain       string   `json:"domain"`
			Secret       string   `json:"secret"`
			Scopes       []string `json:"scopes"`
			TokenType    int      `json:"token_type"`
			Public       bool     `json:"is_public"`
			RedirectURIs []string `json:"redirect_uris"`
			GrantTypes   []string `json:"grant_types"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			responseErr(c, http.StatusBadRequest, "invalid body")
			return
		}
		if strings.TrimSpace(req.DisplayName) == "" {
			responseErr(c, http.StatusBadRequest, "display_name required")
			return
		}
		if req.TokenType != int(model.OpaqueToken) && req.TokenType != int(model.JWT) {
			responseErr(c, http.StatusBadRequest, "token_type must be 0 (opaque) or 1 (JWT)")
			return
		}
		redirectURIs, err := normalizeRedirectURIs(req.RedirectURIs, req.Domain)
		if err != nil {
			responseErr(c, http.StatusBadRequest, err.Error())
			return
		}
		scopes, err := normalizeScopes(req.Scopes)
		if err != nil {
			responseErr(c, http.StatusBadRequest, err.Error())
			return
		}
		grantTypes, err := normalizeGrantTypes(req.GrantTypes, req.Public)
		if err != nil {
			responseErr(c, http.StatusBadRequest, err.Error())
			return
		}
		if req.Public && slices.Contains(grantTypes, "client_credentials") {
			responseErr(c, http.StatusBadRequest, "public clients cannot use client_credentials")
			return
		}
		if req.Public {
			req.Secret = ""
		} else if req.Secret == "" {
			req.Secret = generateRandomSecret(32)
			if req.Secret == "" {
				responseErr(c, http.StatusInternalServerError, "failed to generate client secret")
				return
			}
		}
		tt := model.TokenType(req.TokenType)
		client := &model.AuthClient{
			DisplayName:  strings.TrimSpace(req.DisplayName),
			Domain:       redirectURIs[0],
			Secret:       req.Secret,
			Scopes:       scopes,
			TokenType:    &tt,
			Public:       req.Public,
			RedirectURIs: redirectURIs,
			GrantTypes:   grantTypes,
		}
		if err := store.ClientRepo.(*store.DbClientStore).Create(c.Request.Context(), client); err != nil {
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		secret := ""
		if !client.Public {
			secret = client.Secret
		}
		responseJSON(c, http.StatusOK, gin.H{"id": client.ID, "secret": secret})
	}
}

func DeleteClientAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		clientID, err := strconv.ParseInt(id, 10, 64)
		if err != nil || clientID <= 0 {
			responseErr(c, http.StatusBadRequest, "invalid id")
			return
		}
		if clientID == internal.AuthServerConfig.Admin.ClientID {
			responseErr(c, http.StatusBadRequest, "the management client cannot be deleted")
			return
		}
		if err := store.ClientRepo.(*store.DbClientStore).Delete(c.Request.Context(), id); err != nil {
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		responseJSON(c, http.StatusOK, nil)
	}
}

// ---------- Admins ----------

func GetAdminsAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		var rows []struct {
			ID     int64  `db:"id" json:"id"`
			UserID int64  `db:"user_id" json:"user_id"`
			Email  string `db:"email" json:"email"`
		}
		query := `SELECT a.id, a.user_id, u.email
			FROM auth_admin a JOIN auth_user u ON a.user_id = u.id
			ORDER BY a.created_at DESC`
		if err := internal.DB.Select(&rows, query); err != nil {
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		responseJSON(c, http.StatusOK, rows)
	}
}

func PostAdminAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			UserID int64 `json:"user_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.UserID <= 0 {
			responseErr(c, http.StatusBadRequest, "user_id required")
			return
		}
		result, err := internal.DB.Exec(
			`INSERT INTO auth_admin (user_id, created_at, updated_at)
			 VALUES ($1, now(), now())
			 ON CONFLICT (user_id) DO NOTHING`, req.UserID)
		if err != nil {
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			responseErr(c, http.StatusConflict, "user is already an administrator")
			return
		}
		responseJSON(c, http.StatusOK, nil)
	}
}

func DeleteAdminAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
		if err != nil || uid <= 0 {
			responseErr(c, http.StatusBadRequest, "invalid user_id")
			return
		}
		err = store.DeleteAdmin(c.Request.Context(), uid)
		if err != nil {
			if errors.Is(err, store.ErrLastAdmin) {
				responseErr(c, http.StatusConflict, err.Error())
				return
			}
			responseErr(c, http.StatusInternalServerError, "failed to remove administrator")
			return
		}

		responseJSON(c, http.StatusOK, nil)
	}
}

// ---------- Sessions ----------
//
// The list exposes user records; revocation invalidates all existing user
// grants and SSO sessions through the persistent security generation.
func GetSessionsAPI(_ oauth2.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := store.UserRepo.(*store.DbUserStore).List()
		if err != nil {
			responseErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		responseJSON(c, http.StatusOK, users)
	}
}

func DeleteSessionAPI(_ oauth2.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
		if err != nil || uid <= 0 {
			responseErr(c, http.StatusBadRequest, "invalid user_id")
			return
		}
		if err := adminoauth2.RevokeUserSessions(c.Request.Context(), uid); err != nil {
			responseErr(c, http.StatusServiceUnavailable, "failed to revoke sessions")
			return
		}
		responseJSON(c, http.StatusOK, nil)
	}
}

func hashPassword(plain string) ([]byte, error) {
	hash, err := password.Hash(plain)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"password": hash})
}

func sameScopeSet(a, b string) bool {
	left := strings.Fields(a)
	right := strings.Fields(b)
	if len(left) != len(right) {
		return false
	}
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func normalizeRedirectURIs(input []string, legacyDomain string) (model.TextArray, error) {
	values := append([]string{}, input...)
	if len(values) == 0 && strings.TrimSpace(legacyDomain) != "" {
		values = append(values, strings.TrimSpace(legacyDomain))
	}
	if len(values) == 0 {
		return nil, errors.New("at least one redirect_uri is required")
	}
	seen := make(map[string]struct{}, len(values))
	result := make(model.TextArray, 0, len(values))
	for _, raw := range values {
		uri := strings.TrimSpace(raw)
		if !model.ValidRedirectURI(uri) {
			return nil, errors.New("invalid or insecure redirect_uri: " + raw)
		}
		if _, ok := seen[uri]; ok {
			continue
		}
		seen[uri] = struct{}{}
		result = append(result, uri)
	}
	return result, nil
}

func normalizeScopes(input []string) (model.TextArray, error) {
	values := input
	if len(values) == 0 {
		values = []string{model.ScopeProfile}
	}
	seen := make(map[string]struct{}, len(values))
	result := make(model.TextArray, 0, len(values))
	for _, raw := range values {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, err := model.ParseScope(name); err != nil {
			return nil, errors.New("unknown scope: " + name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil, errors.New("at least one scope is required")
	}
	return result, nil
}

func normalizeGrantTypes(input []string, public bool) (model.TextArray, error) {
	values := input
	if len(values) == 0 {
		if public {
			values = []string{"authorization_code", "refresh_token"}
		} else {
			values = []string{"authorization_code", "client_credentials", "refresh_token"}
		}
	}
	allowed := []string{"authorization_code", "client_credentials", "refresh_token"}
	seen := make(map[string]struct{}, len(values))
	result := make(model.TextArray, 0, len(values))
	for _, raw := range values {
		grant := strings.TrimSpace(raw)
		if grant == "" {
			continue
		}
		if !slices.Contains(allowed, grant) {
			return nil, errors.New("unknown grant type: " + grant)
		}
		if public && grant == "client_credentials" {
			return nil, errors.New("public clients cannot use client_credentials")
		}
		if _, ok := seen[grant]; ok {
			continue
		}
		seen[grant] = struct{}{}
		result = append(result, grant)
	}
	if len(result) == 0 {
		return nil, errors.New("at least one grant type is required")
	}
	return result, nil
}
