package store

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

type seedDriver struct{ failCommit bool }
type seedConn struct{ failCommit bool }
type seedTx struct{ failCommit bool }
type seedRows struct{ done bool }

func (d seedDriver) Open(string) (driver.Conn, error)  { return seedConn{d.failCommit}, nil }
func (c seedConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected prepare") }
func (c seedConn) Close() error                        { return nil }
func (c seedConn) Begin() (driver.Tx, error)           { return seedTx{c.failCommit}, nil }
func (c seedConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if query != `SELECT id FROM auth_client WHERE internal_key = $1 FOR UPDATE` || len(args) != 1 || args[0].Value != model.AdminClientKey {
		return nil, fmt.Errorf("seed selected a record without its internal key: %s", query)
	}
	return &seedRows{}, nil
}
func (c seedConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == `INSERT INTO auth_client (internal_key) VALUES ($1) ON CONFLICT (internal_key) DO NOTHING` && len(args) == 1 && args[0].Value == model.AdminClientKey {
		return driver.RowsAffected(1), nil
	}
	if strings.HasPrefix(query, "UPDATE auth_client SET") && strings.Contains(query, "WHERE internal_key = ") && args[len(args)-1].Value == model.AdminClientKey {
		return driver.RowsAffected(1), nil
	}
	return nil, fmt.Errorf("unexpected seed mutation: %s", query)
}
func (tx seedTx) Commit() error {
	if tx.failCommit {
		return errors.New("commit failed")
	}
	return nil
}
func (tx seedTx) Rollback() error     { return nil }
func (r *seedRows) Columns() []string { return []string{"id"} }
func (r *seedRows) Close() error      { return nil }
func (r *seedRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = int64(81)
	return nil
}

func init() {
	sql.Register("admin-seed-success", seedDriver{})
	sql.Register("admin-seed-failure", seedDriver{failCommit: true})
}

func TestAdminClientSeedUsesInternalKeyAndPublishesIDAfterCommit(t *testing.T) {
	originalConfig, originalDB := internal.AuthServerConfig, internal.DB
	t.Cleanup(func() { internal.AuthServerConfig, internal.DB = originalConfig, originalDB })
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			name := "admin-seed-success"
			if fail {
				name = "admin-seed-failure"
			}
			db, err := sqlx.Open(name, "")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			internal.DB = db
			internal.AuthServerConfig.Admin.ClientID = 2
			internal.AuthServerConfig.Admin.RedirectURIs = []string{"https://auth.example.invalid/admin-page/callback"}
			internal.AuthServerConfig.Admin.Scopes = []string{"manage:system", "manage:clients"}
			internal.AuthServerConfig.Admin.GrantTypes = []string{"authorization_code", "refresh_token"}
			err = EnsureAdminClient()
			if fail {
				if err == nil || internal.AuthServerConfig.Admin.ClientID != 2 {
					t.Fatalf("failed commit published client ID: %v", err)
				}
				return
			}
			if err != nil || internal.AuthServerConfig.Admin.ClientID != 81 {
				t.Fatalf("seed failed to resolve generated ID: %v", err)
			}
		})
	}
}
