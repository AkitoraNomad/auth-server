package http

import (
	"auth-server/internal"
	"auth-server/internal/http/admin"
	"auth-server/internal/model"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-session/session/v3"
)

// CreateGinEngine builds a gin.Engine replacing the previous net/http mux.
// Gradually wraps existing handler funcs to reduce refactor impact.
func CreateGinEngine(srv *server.Server) *gin.Engine {
	r := gin.New()
	// discard default gin text logger, we use slog
	gin.DefaultWriter = io.Discard
	// ensure panic stack traces are printed to the console
	gin.DefaultErrorWriter = os.Stderr
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic recovered", "err", fmt.Sprintf("%v", recovered), "path", c.Request.URL.Path)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code": 1,
			"msg":  fmt.Sprintf("%v", recovered),
		})
	}))
	r.Use(ginSlogLogger())

	// keep previous session expiration behavior (15 minutes)
	session.SetExpired(15 * 60)

	// Root & static placeholder
	r.GET("/", gin.WrapF(indexHandler))

	// Vendored CSS used by the server-rendered auth pages. Keeping this route
	// explicit avoids exposing the assets directory and does not depend on a
	// public CDN.
	r.GET("/assets/bulma.min.css", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=86400")
		c.File("assets/bulma.min.css")
	})

	// Auth related
	loginHandler := getLoginHandler()
	r.GET(internal.PathLogin, loginHandler)
	r.POST(internal.PathLogin, loginHandler)
	r.GET(internal.PathAuthorize, gin.WrapF(getAuthorizeHandler(srv)))
	r.POST(internal.PathAuthorize, gin.WrapF(getAuthorizeHandler(srv)))
	r.GET(internal.PathAuth, gin.WrapF(getAuthHandler()))
	r.POST(internal.PathToken, gin.WrapF(getTokenHandler(srv)))
	r.POST(internal.PathIntrospect, gin.WrapF(getIntrospectHandler(srv)))
	r.POST(internal.PathUserinfo, gin.WrapF(getUserinfoHandler(srv)))
	r.GET(internal.PathDiscovery, gin.WrapF(discoveryHandler))
	r.GET(internal.PathJwkSet, gin.WrapF(jwkSetHandler))
	r.GET(internal.PathRegistration, gin.WrapF(getRegistrationHandler(srv)))
	r.POST(internal.PathRegistration, gin.WrapF(getRegistrationHandler(srv)))

	// Admin JSON API. RequireAdmin verifies the JWT, the management client, the
	// Redis revocation state and auth_admin membership. Each route then enforces
	// its own management scope.
	apiAdmin := r.Group("/api/admin")
	apiAdmin.Use(admin.RequireAdmin(srv.Manager))
	{
		apiAdmin.GET("/me", admin.GetMeAPI())

		apiAdmin.GET("/users", admin.RequireScope(model.ScopeManageSystem), admin.GetUsersAPI())
		apiAdmin.POST("/users", admin.RequireScope(model.ScopeManageSystem), admin.PostUserAPI())
		apiAdmin.DELETE("/users/:id", admin.RequireScope(model.ScopeManageSystem), admin.DeleteUserAPI())

		apiAdmin.GET("/clients", admin.RequireScope(model.ScopeManageClients), admin.GetClientsAPI())
		apiAdmin.POST("/clients", admin.RequireScope(model.ScopeManageClients), admin.PostClientAPI())
		apiAdmin.DELETE("/clients/:id", admin.RequireScope(model.ScopeManageClients), admin.DeleteClientAPI())

		apiAdmin.GET("/admins", admin.RequireScope(model.ScopeManageSystem), admin.GetAdminsAPI())
		apiAdmin.POST("/admins", admin.RequireScope(model.ScopeManageSystem), admin.PostAdminAPI())
		apiAdmin.DELETE("/admins/:user_id", admin.RequireScope(model.ScopeManageSystem), admin.DeleteAdminAPI())

		apiAdmin.GET("/sessions", admin.RequireScope(model.ScopeManageSystem), admin.GetSessionsAPI(srv.Manager))
		apiAdmin.DELETE("/sessions/:user_id", admin.RequireScope(model.ScopeManageSystem), admin.DeleteSessionAPI(srv.Manager))
	}

	// 404 fallback
	r.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "Not Found")
	})

	return r
}

// ginSlogLogger provides structured logging via slog.
func ginSlogLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery
		method := c.Request.Method
		clientIP := c.ClientIP()
		uaShort, uaHash, uaLen := compactUA(c.Request.UserAgent())
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		size := c.Writer.Size()
		var errMsg string
		if len(c.Errors) > 0 {
			errMsg = c.Errors.String()
		}
		if rawQuery != "" {
			path = path + "?" + sanitizeQuery(rawQuery)
		}
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}
		slog.Log(c, level, "http_request",
			"status", status,
			"method", method,
			"path", path,
			"ip", clientIP,
			// Compact UA to avoid excessively long log lines while keeping correlation
			"ua", uaShort,
			"ua_len", uaLen,
			"ua_hash", uaHash,
			"latency_ms", latency.Milliseconds(),
			"resp_bytes", size,
			"err", errMsg,
		)
	}
}

// maxUALength caps the length of the UA string recorded in logs.
const maxUALength = 128

// compactUA returns a truncated UA (at most maxUALength runes),
// a short stable hash for correlation, and the original length.
func compactUA(ua string) (short string, hash string, length int) {
	length = len(ua)
	// Stable short hash (first 8 bytes of sha256 => 16 hex chars)
	sum := sha256.Sum256([]byte(ua))
	hash = hex.EncodeToString(sum[:8])

	// Truncate by runes to avoid breaking multibyte characters
	r := []rune(ua)
	if len(r) > maxUALength {
		short = string(r[:maxUALength]) + "…"
	} else {
		short = ua
	}
	return
}

var sensitiveQueryKeys = map[string]struct{}{
	"state":          {},
	"code":           {},
	"code_challenge": {},
	"code_verifier":  {},
	"access_token":   {},
	"refresh_token":  {},
	"token":          {},
	"client_secret":  {},
	"password":       {},
	"cf_token":       {},
}

// sanitizeQuery keeps useful routing/debug parameters while redacting values
// that must not be written to logs (state, PKCE material, credentials/tokens).
func sanitizeQuery(rawQuery string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "[unparseable query redacted]"
	}
	for key := range sensitiveQueryKeys {
		if values.Has(key) {
			values.Set(key, "[redacted]")
		}
	}
	return values.Encode()
}
