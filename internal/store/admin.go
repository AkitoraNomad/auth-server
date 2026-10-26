package store

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
)

// IsAdmin reports whether the user is present in auth_admin. This check is
// intentionally done on every admin API request so removing an administrator
// takes effect immediately, without waiting for a token to expire.
func IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	if err := internal.DB.GetContext(ctx, &exists,
		`SELECT EXISTS (SELECT 1 FROM auth_admin WHERE user_id = $1)`, userID); err != nil {
		return false, err
	}
	return exists, nil
}

// CountAdmins returns the number of administrator records.
func CountAdmins(ctx context.Context) (int64, error) {
	var count int64
	if err := internal.DB.GetContext(ctx, &count, `SELECT COUNT(*) FROM auth_admin`); err != nil {
		return 0, err
	}
	return count, nil
}

type adminClientSeed struct {
	ID           int64           `db:"id"`
	DisplayName  string          `db:"display_name"`
	Domain       string          `db:"domain"`
	Scopes       model.TextArray `db:"scopes"`
	RedirectURIs model.TextArray `db:"redirect_uris"`
	GrantTypes   model.TextArray `db:"grant_types"`
}

// EnsureAdminClient idempotently creates or updates the first-party management
// client. The OAuth client id remains a BIGINT (as selected for this project);
// the server owns this row and keeps its public/PKCE settings in sync with
// configuration on every start.
func EnsureAdminClient() error {
	cfg := internal.AuthServerConfig.Admin

	seed, err := buildAdminClientSeed(cfg.ClientID, cfg.DisplayName, cfg.RedirectURIs, cfg.Scopes, cfg.GrantTypes)
	if err != nil {
		return err
	}

	tx, err := internal.DB.BeginTxx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	var existingID int64
	err = tx.Get(&existingID, `SELECT id FROM auth_client WHERE id = $1 FOR UPDATE`, seed.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.NamedExec(`INSERT INTO auth_client
			(id, display_name, secret, domain, scopes, token_type, is_public, redirect_uris, grant_types, created_at, updated_at)
			VALUES (:id, :display_name, '', :domain, :scopes, 'JWT', TRUE, :redirect_uris, :grant_types, now(), now())`, seed)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("create admin client: %w", err)
		}
	case err != nil:
		_ = tx.Rollback()
		return fmt.Errorf("load admin client: %w", err)
	default:
		_, err = tx.NamedExec(`UPDATE auth_client SET
			display_name = :display_name,
			secret = '',
			domain = :domain,
			scopes = :scopes,
			token_type = 'JWT',
			is_public = TRUE,
			redirect_uris = :redirect_uris,
			grant_types = :grant_types,
			updated_at = now()
			WHERE id = :id`, seed)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("update admin client: %w", err)
		}
		slog.Info("admin client updated", "client_id", seed.ID)
	}

	// The client id is explicit so existing installations and VITE_ADMIN_CLIENT_ID
	// stay stable. Advance the identity sequence past it for future clients.
	var sequenceValue int64
	if err = tx.Get(&sequenceValue,
		`SELECT setval(
			pg_get_serial_sequence('auth.auth_client', 'id'),
			GREATEST((SELECT COALESCE(MAX(id), 0) FROM auth_client), $1),
			true
		)`, seed.ID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("advance auth_client id sequence: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	slog.Info("admin client ensured",
		"client_id", seed.ID,
		"redirect_uris", strings.Join(seed.RedirectURIs, ","),
		"scopes", strings.Join(seed.Scopes, " "),
		"grant_types", strings.Join(seed.GrantTypes, ","),
	)
	return nil
}

func buildAdminClientSeed(clientID int64, displayName string, redirectURIs, scopes, grantTypes []string) (adminClientSeed, error) {
	if clientID <= 0 {
		return adminClientSeed{}, errors.New("admin.client-id must be a positive integer")
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = "Auth Server Admin"
	}
	if len(redirectURIs) == 0 {
		return adminClientSeed{}, errors.New("admin.redirect-uris must not be empty")
	}
	seenURI := make(map[string]struct{}, len(redirectURIs))
	cleanURIs := make([]string, 0, len(redirectURIs))
	for _, raw := range redirectURIs {
		uri := strings.TrimSpace(raw)
		u, err := url.Parse(uri)
		if err != nil || u.Scheme == "" || u.Host == "" || u.Fragment != "" {
			return adminClientSeed{}, fmt.Errorf("invalid admin redirect URI: %q", raw)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return adminClientSeed{}, fmt.Errorf("admin redirect URI must use http or https: %q", raw)
		}
		if _, ok := seenURI[uri]; ok {
			continue
		}
		seenURI[uri] = struct{}{}
		cleanURIs = append(cleanURIs, uri)
	}

	seenScope := make(map[string]struct{}, len(scopes))
	cleanScopes := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, err := model.ParseScope(name); err != nil {
			return adminClientSeed{}, fmt.Errorf("unknown admin scope: %s", name)
		}
		if _, ok := seenScope[name]; ok {
			continue
		}
		seenScope[name] = struct{}{}
		cleanScopes = append(cleanScopes, name)
	}
	if len(cleanScopes) == 0 {
		return adminClientSeed{}, errors.New("admin.scopes must not be empty")
	}
	if !slices.Contains(cleanScopes, model.ScopeManageSystem) || !slices.Contains(cleanScopes, model.ScopeManageClients) {
		return adminClientSeed{}, errors.New("admin.scopes must include both manage:system and manage:clients")
	}

	allowedGrants := []string{"authorization_code", "refresh_token"}
	seenGrant := make(map[string]struct{}, len(grantTypes))
	cleanGrants := make([]string, 0, len(grantTypes))
	for _, raw := range grantTypes {
		grant := strings.TrimSpace(raw)
		if grant == "" {
			continue
		}
		if !slices.Contains(allowedGrants, grant) {
			return adminClientSeed{}, fmt.Errorf("admin grant type %q is not allowed; use authorization_code and/or refresh_token", grant)
		}
		if _, ok := seenGrant[grant]; ok {
			continue
		}
		seenGrant[grant] = struct{}{}
		cleanGrants = append(cleanGrants, grant)
	}
	if !slices.Contains(cleanGrants, "authorization_code") {
		return adminClientSeed{}, errors.New("admin grant types must include authorization_code")
	}

	return adminClientSeed{
		ID:          clientID,
		DisplayName: displayName,
		// Redirect validation is exact and handled by the authorization
		// handlers; leave the legacy domain empty for the admin client.
		Domain:       "",
		Scopes:       model.TextArray(cleanScopes),
		RedirectURIs: model.TextArray(cleanURIs),
		GrantTypes:   model.TextArray(cleanGrants),
	}, nil
}
