package http

import (
	"auth-server/internal"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxRequestBody = 1 << 20 // 1 MiB

// go-session infers Secure from the transport peer, even with SetSecure(true).
// Enforce the public issuer policy at the response boundary as well, including
// TLS-terminating proxies connected over HTTP.
type secureCookieWriter struct{ gin.ResponseWriter }

func (w *secureCookieWriter) secureCookies() {
	values := w.Header().Values("Set-Cookie")
	if len(values) == 0 {
		return
	}
	w.Header().Del("Set-Cookie")
	for _, value := range values {
		response := &http.Response{Header: http.Header{"Set-Cookie": []string{value}}}
		cookies := response.Cookies()
		if len(cookies) == 1 && cookies[0].Name == "go_session_id" {
			cookie := cookies[0]
			cookie.Secure = true
			cookie.HttpOnly = true
			cookie.SameSite = http.SameSiteLaxMode
			value = cookie.String()
		}
		w.Header().Add("Set-Cookie", value)
	}
}
func (w *secureCookieWriter) WriteHeader(code int) {
	w.secureCookies()
	w.ResponseWriter.WriteHeader(code)
}
func (w *secureCookieWriter) WriteHeaderNow() { w.secureCookies(); w.ResponseWriter.WriteHeaderNow() }
func (w *secureCookieWriter) Write(data []byte) (int, error) {
	w.secureCookies()
	return w.ResponseWriter.Write(data)
}
func (w *secureCookieWriter) WriteString(data string) (int, error) {
	w.secureCookies()
	return w.ResponseWriter.WriteString(data)
}

func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(internal.AuthServerConfig.Issuer, "https://") {
			c.Writer = &secureCookieWriter{c.Writer}
		}
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self' 'unsafe-inline' https://challenges.cloudflare.com https://telegram.org",
			"style-src 'self' 'unsafe-inline' https://cdn.bootcss.com",
			"font-src 'self' data: https://cdn.bootcss.com",
			"img-src 'self' data: https:",
			"connect-src 'self' https://challenges.cloudflare.com https://telegram.org https://oauth.telegram.org",
			"frame-src https://challenges.cloudflare.com https://telegram.org https://oauth.telegram.org",
			"base-uri 'self'",
			"frame-ancestors 'none'",
		}, "; "))
		if isNoStorePath(c.Request.URL.Path) {
			h.Set("Cache-Control", "no-store")
			h.Set("Pragma", "no-cache")
		}
		c.Next()
	}
}

func isNoStorePath(path string) bool {
	for _, prefix := range []string{"/login", "/logout", "/auth", "/registration", "/oauth2/", "/introspect", "/userinfo", "/api/admin"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// sameOriginMiddleware blocks browser cross-origin state-changing requests
// when an Origin header is present. Non-browser clients without Origin remain
// supported, but the endpoints should still be protected by CSRF-aware session
// rules where applicable.
func sameOriginMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			c.Next()
			return
		}
		origin := strings.TrimSpace(c.Request.Header.Get("Origin"))
		if c.GetHeader("Sec-Fetch-Site") == "cross-site" && (c.Request.URL.Path == "/login" || c.Request.URL.Path == "/registration" || c.Request.URL.Path == "/logout" || c.Request.URL.Path == "/oauth2/authorize") {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if origin == "" {
			c.Next()
			return
		}
		u, err := url.Parse(origin)
		expected, expectedErr := url.Parse(internal.AuthServerConfig.Issuer)
		if err != nil || expectedErr != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !strings.EqualFold(u.Scheme, expected.Scheme) || !strings.EqualFold(u.Host, expected.Host) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 1, "msg": "cross-origin request rejected"})
			return
		}
		c.Next()
	}
}

func bodyLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBody)
		}
		c.Next()
	}
}
