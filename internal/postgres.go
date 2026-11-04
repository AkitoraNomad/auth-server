package internal

import (
	"log/slog"
	"net/url"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// DB is the global database handle (PostgreSQL)
var DB *sqlx.DB

func OpenDatabase() error {
	// AuthServerConfig.DB expected format example:
	// postgres://user:pass@host:5432/auth?sslmode=disable
	db, err := sqlx.Connect("pgx", AuthServerConfig.DB)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		return err
	}
	DB = db

	logConnectionInfo(AuthServerConfig.DB)
	return nil
}

func logConnectionInfo(dsn string) {
	u, err := url.Parse(dsn)
	if err != nil {
		slog.Info("DB connected", "dsn", maskRawDSN(dsn))
		return
	}
	user := ""
	if u.User != nil {
		user = u.User.Username()
	}
	host := u.Host
	dbName := strings.TrimPrefix(u.Path, "/")
	slog.Info("DB connected",
		"scheme", u.Scheme,
		"user", user,
		"host", host,
		"db", dbName,
	)
}

func maskRawDSN(dsn string) string {
	// naive masking: replace :password@ pattern
	// find '://' then next '@'
	if i := strings.Index(dsn, "://"); i >= 0 {
		rest := dsn[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			cred := rest[:at]
			if colon := strings.Index(cred, ":"); colon >= 0 {
				maskedCred := cred[:colon] + ":****"
				return dsn[:i+3] + maskedCred + rest[at:]
			}
		}
	}
	return dsn
}
