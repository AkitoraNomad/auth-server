// Package webui serves the embedded management SPA without coupling the
// authorization handlers to frontend assets.
package webui

import (
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	routePrefix = "/admin-page"
	indexFile   = "index.html"
)

// Register adds the management SPA routes to r when dist is available. A nil
// filesystem is a no-op, which keeps the default (non-embedded) build and the
// Vite development workflow unchanged.
//
// The SPA owns only the /admin-page/ prefix. OAuth2, login, registration,
// discovery, /assets and /api routes remain untouched.
func Register(r *gin.Engine, dist fs.FS) {
	if dist == nil {
		return
	}

	index, err := fs.ReadFile(dist, indexFile)
	if err != nil {
		slog.Error("embedded SPA index is missing", "err", err)
		return
	}

	files := http.FileServer(http.FS(dist))

	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	}

	r.GET(routePrefix, func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, routePrefix+"/")
	})

	r.GET(routePrefix+"/*filepath", func(c *gin.Context) {
		cleanPath := path.Clean("/" + strings.TrimPrefix(c.Request.URL.Path, routePrefix))
		if !strings.HasPrefix(cleanPath, "/") {
			c.Status(http.StatusNotFound)
			return
		}

		// The SPA root and index document always render the shell.
		if cleanPath == "/" || cleanPath == "/index.html" {
			serveIndex(c)
			return
		}

		relativePath := strings.TrimPrefix(cleanPath, "/")
		info, statErr := fs.Stat(dist, relativePath)

		if statErr == nil && !info.IsDir() {
			switch {
			case strings.HasPrefix(cleanPath, "/assets/"):
				// Vite hashes asset names, so this response is immutable.
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			case path.Ext(cleanPath) != "":
				c.Header("Cache-Control", "public, max-age=86400")
			default:
				c.Header("Cache-Control", "no-cache")
			}

			// ServeFile expects the path relative to the filesystem root. Clone
			// the request so the router's original URL is not mutated.
			request := c.Request.Clone(c.Request.Context())
			request.URL.Path = cleanPath
			request.URL.RawPath = ""
			files.ServeHTTP(c.Writer, request)
			return
		}

		// Never fall back to index.html for a missing asset: a JS/CSS request
		// receiving HTML produces a confusing MIME type error in the browser.
		if (statErr == nil && info.IsDir()) ||
			strings.HasPrefix(cleanPath, "/assets/") ||
			path.Ext(cleanPath) != "" {
			c.Status(http.StatusNotFound)
			return
		}

		// Client-side routes such as /admin-page/users and
		// /admin-page/callback are handled by the SPA router.
		serveIndex(c)
	})
}
