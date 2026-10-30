package admin

import (
	"auth-server/internal/model"
	"auth-server/internal/store"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

func storeError(c *gin.Context, err error) {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		responseErr(c, 404, "record not found")
	case errors.Is(err, store.ErrLastAdmin):
		responseErr(c, 409, "cannot remove or disable the last active administrator")
	case errors.Is(err, store.ErrNoPassword):
		responseErr(c, 409, err.Error())
	case errors.Is(err, store.ErrProtectedClient), errors.Is(err, store.ErrClientType), errors.Is(err, store.ErrPublicSecret):
		responseErr(c, 400, err.Error())
	case strings.Contains(err.Error(), "user_exists"):
		responseErr(c, 409, "email is already in use")
	case errors.As(err, &pg):
		switch pg.Code {
		case "23505":
			responseErr(c, 409, "email, login identity or administrator already exists")
		case "23503":
			responseErr(c, 400, "referenced user does not exist")
		default:
			slog.Error("admin database operation failed", "err", err)
			responseErr(c, 500, "unable to complete database operation")
		}
	default:
		slog.Error("admin operation failed", "err", err)
		responseErr(c, 500, "unable to complete operation")
	}
}
func recordID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		responseErr(c, 400, "invalid id")
		return 0, false
	}
	return id, true
}
func validEmail(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	return value, err == nil && parsed.Address == value && len(value) <= 254
}
func validPassword(value string) bool {
	return utf8.RuneCountInString(value) >= 12 && len(value) <= 1024
}
func listOptions(c *gin.Context) (store.ListOptions, bool) {
	options := store.ListOptions{Page: 1, PageSize: 20, Query: strings.TrimSpace(c.Query("q"))}
	for _, p := range []struct {
		name   string
		target *int
		max    int
	}{{"page", &options.Page, 1000000}, {"page_size", &options.PageSize, 100}} {
		if value, exists := c.GetQuery(p.name); exists {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > p.max {
				responseErr(c, 400, "invalid "+p.name)
				return options, false
			}
			*p.target = n
		}
	}
	if utf8.RuneCountInString(options.Query) > 200 {
		responseErr(c, 400, "search query is too long")
		return options, false
	}
	switch c.Query("status") {
	case "", "all":
	case "active":
		disabled := false
		options.Disabled = &disabled
	case "disabled":
		disabled := true
		options.Disabled = &disabled
	default:
		responseErr(c, 400, "invalid status filter")
		return options, false
	}
	return options, true
}
func listHandler[T any](load func(context.Context, store.ListOptions) (store.Page[T], error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		options, ok := listOptions(c)
		if !ok {
			return
		}
		result, err := load(c.Request.Context(), options)
		if err != nil {
			storeError(c, err)
			return
		}
		responseJSON(c, 200, result)
	}
}
func PatchUserAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := recordID(c)
		if !ok {
			return
		}
		var req store.UserPatch
		if err := c.ShouldBindJSON(&req); err != nil {
			responseErr(c, 400, "invalid body")
			return
		}
		if req.Email == nil && req.DisplayName == nil && req.IsDisabled == nil {
			responseErr(c, 400, "no user changes provided")
			return
		}
		if req.Email != nil {
			email, ok := validEmail(*req.Email)
			if !ok {
				responseErr(c, 400, "valid email required")
				return
			}
			req.Email = &email
		}
		if req.DisplayName != nil {
			name := strings.TrimSpace(*req.DisplayName)
			if utf8.RuneCountInString(name) > 200 {
				responseErr(c, 400, "display name is too long")
				return
			}
			req.DisplayName = &name
		}
		if err := store.UpdateUser(c.Request.Context(), id, req); err != nil {
			storeError(c, err)
			return
		}
		responseJSON(c, 200, nil)
	}
}
func ResetUserPasswordAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := recordID(c)
		if !ok {
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || !validPassword(req.Password) {
			responseErr(c, 400, "password must contain at least 12 characters and at most 1024 bytes")
			return
		}
		data, err := hashPassword(req.Password)
		if err != nil {
			responseErr(c, http.StatusServiceUnavailable, "password service is busy; try again")
			return
		}
		if err := store.ResetUserPassword(c.Request.Context(), id, data); err != nil {
			storeError(c, err)
			return
		}
		responseJSON(c, 200, nil)
	}
}

type clientRequest struct {
	DisplayName  string   `json:"display_name"`
	Domain       string   `json:"domain"`
	Secret       string   `json:"secret"`
	Scopes       []string `json:"scopes"`
	TokenType    int      `json:"token_type"`
	Public       bool     `json:"is_public"`
	IsDisabled   bool     `json:"is_disabled"`
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types"`
}

func clientFromRequest(c *gin.Context) (*model.AuthClient, *clientRequest) {
	var req clientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responseErr(c, 400, "invalid body")
		return nil, nil
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" || utf8.RuneCountInString(req.DisplayName) > 200 {
		responseErr(c, 400, "display name must contain 1 to 200 characters")
		return nil, nil
	}
	if req.TokenType != int(model.OpaqueToken) && req.TokenType != int(model.JWT) {
		responseErr(c, 400, "token_type must be 0 (opaque) or 1 (JWT)")
		return nil, nil
	}
	uris, err := normalizeRedirectURIs(req.RedirectURIs, req.Domain)
	if err != nil {
		responseErr(c, 400, err.Error())
		return nil, nil
	}
	scopes, err := normalizeScopes(req.Scopes)
	if err != nil {
		responseErr(c, 400, err.Error())
		return nil, nil
	}
	grants, err := normalizeGrantTypes(req.GrantTypes, req.Public)
	if err != nil {
		responseErr(c, 400, err.Error())
		return nil, nil
	}
	tt := model.TokenType(req.TokenType)
	return &model.AuthClient{DisplayName: req.DisplayName, Domain: uris[0], Scopes: scopes, TokenType: &tt, Public: req.Public, IsDisabled: req.IsDisabled, RedirectURIs: uris, GrantTypes: grants}, &req
}
func PutClientAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := recordID(c)
		if !ok {
			return
		}
		client, req := clientFromRequest(c)
		if client == nil {
			return
		}
		if req.Secret != "" {
			responseErr(c, 400, "use the secret reset action to change credentials")
			return
		}
		client.ID = id
		if err := store.UpdateClient(c.Request.Context(), client); err != nil {
			storeError(c, err)
			return
		}
		responseJSON(c, 200, nil)
	}
}
func PatchClientStateAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := recordID(c)
		if !ok {
			return
		}
		var req struct {
			Disabled *bool `json:"is_disabled"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Disabled == nil {
			responseErr(c, 400, "is_disabled is required")
			return
		}
		if err := store.SetClientDisabled(c.Request.Context(), id, *req.Disabled); err != nil {
			storeError(c, err)
			return
		}
		responseJSON(c, 200, nil)
	}
}
func ResetClientSecretAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := recordID(c)
		if !ok {
			return
		}
		secret := generateRandomSecret(32)
		if secret == "" {
			responseErr(c, 500, "unable to generate secret")
			return
		}
		if err := store.ResetClientSecret(c.Request.Context(), id, secret); err != nil {
			storeError(c, err)
			return
		}
		responseJSON(c, 200, gin.H{"id": id, "secret": secret})
	}
}
