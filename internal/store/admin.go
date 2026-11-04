package store

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"context"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log/slog"
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

type adminClientSeed struct {
	ID           int64           `db:"id"`
	InternalKey  string          `db:"internal_key"`
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

	seed, err := buildAdminClientSeed(cfg.DisplayName, cfg.RedirectURIs, cfg.Scopes, cfg.GrantTypes)
	if err != nil {
		return err
	}

	tx, err := internal.DB.BeginTxx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO auth_client (internal_key) VALUES ($1) ON CONFLICT (internal_key) DO NOTHING`, seed.InternalKey)
	if err != nil {
		return fmt.Errorf("create admin client: %w", err)
	}
	err = tx.Get(&seed.ID, `SELECT id FROM auth_client WHERE internal_key = $1 FOR UPDATE`, seed.InternalKey)
	if err != nil {
		return fmt.Errorf("load admin client: %w", err)
	}
	_, err = tx.NamedExec(`UPDATE auth_client SET
   security_version = security_version + CASE WHEN
    secret IS DISTINCT FROM '' OR is_disabled OR NOT is_public OR token_type IS DISTINCT FROM 'JWT'
    OR scopes IS DISTINCT FROM :scopes OR redirect_uris IS DISTINCT FROM :redirect_uris OR grant_types IS DISTINCT FROM :grant_types
    THEN 1 ELSE 0 END,
			display_name = :display_name,
			secret = '',
			domain = :domain,
			scopes = :scopes,
			token_type = 'JWT',
			is_disabled = FALSE,
			is_public = TRUE,
			redirect_uris = :redirect_uris,
			grant_types = :grant_types,
			updated_at = now()
			WHERE internal_key = :internal_key`, seed)
	if err != nil {
		return fmt.Errorf("update admin client: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	internal.AuthServerConfig.Admin.ClientID = seed.ID

	slog.Info("admin client ensured",
		"client_id", seed.ID,
		"redirect_uris", strings.Join(seed.RedirectURIs, ","),
		"scopes", strings.Join(seed.Scopes, " "),
		"grant_types", strings.Join(seed.GrantTypes, ","),
	)
	return nil
}

func buildAdminClientSeed(displayName string, redirectURIs, scopes, grantTypes []string) (adminClientSeed, error) {
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
		if !model.ValidRedirectURI(uri) {
			return adminClientSeed{}, fmt.Errorf("insecure admin redirect URI: %q", raw)
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
		InternalKey: model.AdminClientKey,
		DisplayName: displayName,
		// Redirect validation is exact and handled by the authorization
		// handlers; leave the legacy domain empty for the admin client.
		Domain:       "",
		Scopes:       model.TextArray(cleanScopes),
		RedirectURIs: model.TextArray(cleanURIs),
		GrantTypes:   model.TextArray(cleanGrants),
	}, nil
}

var ErrLastAdmin = errors.New("cannot remove the last administrator")

// All administrator removals (including user deletion and its cascading FK)
// serialize with administrator inserts/removals before counting. The lock is
// held until commit, so concurrent removals cannot both observe two admins.
func protectLastAdmin(tx *sqlx.Tx, userID int64) error {
	if _, err := tx.Exec(`LOCK TABLE auth_admin IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	var target bool
	if err := tx.Get(&target, `SELECT EXISTS (SELECT 1 FROM auth_admin a JOIN auth_user u ON u.id = a.user_id WHERE a.user_id = $1 AND NOT u.is_disabled)`, userID); err != nil {
		return err
	}
	if !target {
		return nil
	}
	var count int
	if err := tx.Get(&count, `SELECT COUNT(*) FROM auth_admin a JOIN auth_user u ON u.id = a.user_id WHERE NOT u.is_disabled`); err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastAdmin
	}
	return nil
}

func DeleteAdmin(ctx context.Context, userID int64) error {
	tx, err := internal.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := protectLastAdmin(tx, userID); err != nil {
		return err
	}
	if err := requireAffected(tx.ExecContext(ctx, `DELETE FROM auth_admin WHERE user_id = $1`, userID)); err != nil {
		return err
	}
	return tx.Commit()
}
