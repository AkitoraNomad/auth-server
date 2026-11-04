package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func testDist() fs.FS {
	return fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("<!doctype html><div id=root>admin</div>")},
		"assets/app.js":  &fstest.MapFile{Data: []byte("console.log('admin')")},
		"favicon.svg":    &fstest.MapFile{Data: []byte("<svg></svg>")},
		"icons.svg":      &fstest.MapFile{Data: []byte("<svg></svg>")},
		"assets/app.css": &fstest.MapFile{Data: []byte("body{}")},
	}
}

func testRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, testDist())
	return r
}

func TestServesIndexAndClientRoutes(t *testing.T) {
	r := testRouter(t)

	for _, route := range []string{
		"/admin-page/",
		"/admin-page/users",
		"/admin-page/clients",
		"/admin-page/callback",
	} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", route, w.Code)
		}
		if strings.Contains(w.Body.String(), "console.log") {
			t.Fatalf("%s: got an asset instead of the SPA shell", route)
		}
		if !strings.Contains(w.Body.String(), `id=root`) {
			t.Fatalf("%s: missing SPA shell", route)
		}
		if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Fatalf("%s: Cache-Control = %q, want no-store", route, got)
		}
	}
}

func TestRedirectsBarePrefix(t *testing.T) {
	r := testRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/admin-page", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/admin-page/" {
		t.Fatalf("Location = %q, want /admin-page/", got)
	}
}

func TestServesHashedAssets(t *testing.T) {
	r := testRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/admin-page/assets/app.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "console.log") {
		t.Fatalf("unexpected asset body: %q", w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control = %q, want immutable", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type = %q, want JavaScript", got)
	}
}

func TestMissingAssetDoesNotFallBackToIndex(t *testing.T) {
	r := testRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/admin-page/assets/missing.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), `id=root`) {
		t.Fatal("missing asset unexpectedly returned the SPA shell")
	}
}

func TestNilFilesystemIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin-page/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 without an embedded SPA", w.Code)
	}
}
