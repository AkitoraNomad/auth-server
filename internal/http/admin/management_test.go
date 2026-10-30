package admin

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestListOptionsRejectUnboundedAndInvalidInput(t *testing.T) {
	for _, query := range []string{"page=0", "page=-1", "page=1000001", "page_size=0", "page_size=101", "page_size=abc", "status=other", "q=" + strings.Repeat("x", 201)} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/admin/users?"+query, nil)
		if _, ok := listOptions(c); ok || w.Code != 400 {
			t.Fatalf("query %q was not rejected", query)
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/admin/users?page=2&page_size=10&q=Alice&status=disabled", nil)
	opts, ok := listOptions(c)
	if !ok || opts.Page != 2 || opts.PageSize != 10 || opts.Disabled == nil || !*opts.Disabled || opts.Query != "Alice" {
		t.Fatal("valid filter did not survive parsing")
	}
}

func TestPasswordResetValidatesBeforeDatabaseAccess(t *testing.T) {
	for _, body := range []string{`{}`, `{"password":"short"}`, `{"password":false}`, `{"password":"` + strings.Repeat("x", 1025) + `"}`} {
		r := gin.New()
		r.POST("/users/:id/password", ResetUserPasswordAPI())
		req := httptest.NewRequest("POST", "/users/1/password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("invalid password returned %d", w.Code)
		}
	}
}

func TestStoreErrorsAreSafeAndActionable(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{&pgconn.PgError{Code: "23505", Detail: "private database detail"}, 409},
		{&pgconn.PgError{Code: "23503", Detail: "private database detail"}, 400},
		{errors.New("private database detail"), 500},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		storeError(c, tc.err)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private database detail") {
			t.Fatalf("unsafe response: %d %s", w.Code, w.Body.String())
		}
	}
}
