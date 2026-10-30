package store

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

var (
	ErrProtectedClient = errors.New("the management client is configured by the server and cannot be changed here")
	ErrClientType      = errors.New("client type cannot be changed; register a new client")
	ErrPublicSecret    = errors.New("public clients have no secret")
	ErrNoPassword      = errors.New("this account has no email/password sign-in")
)

type ListOptions struct {
	Page     int
	PageSize int
	Query    string
	Disabled *bool
}
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// SQL fragments are fixed by callers, never taken from request parameters.
func listPage[T any](ctx context.Context, selectSQL, countSQL string, options ListOptions) (Page[T], error) {
	result := Page[T]{Items: []T{}, Page: options.Page, PageSize: options.PageSize}
	if err := internal.DB.GetContext(ctx, &result.Total, countSQL, options.Query, options.Disabled); err != nil {
		return result, err
	}
	last := max(1, (result.Total+options.PageSize-1)/options.PageSize)
	result.Page = min(result.Page, last)
	err := internal.DB.SelectContext(ctx, &result.Items, selectSQL, options.Query, options.Disabled, options.PageSize, (result.Page-1)*options.PageSize)
	return result, err
}

const userFilter = ` WHERE ($1 = '' OR strpos(lower(coalesce(email,'') || ' ' || coalesce(display_name,'') || ' ' || id::text), lower($1)) > 0) AND ($2::boolean IS NULL OR is_disabled = $2)`

func ListUsers(ctx context.Context, options ListOptions) (Page[model.User], error) {
	rows, err := listPage[model.AuthUser](ctx, "SELECT * FROM auth_user"+userFilter+" ORDER BY id DESC LIMIT $3 OFFSET $4", "SELECT count(*) FROM auth_user"+userFilter, options)
	result := Page[model.User]{Items: []model.User{}, Total: rows.Total, Page: rows.Page, PageSize: rows.PageSize}
	for _, row := range rows.Items {
		result.Items = append(result.Items, model.NewUserFromAuth(row, row.LastLoginAt))
	}
	return result, err
}

const clientFilter = ` WHERE ($1 = '' OR strpos(lower(coalesce(display_name,'') || ' ' || id::text), lower($1)) > 0) AND ($2::boolean IS NULL OR is_disabled = $2)`

func ListClients(ctx context.Context, options ListOptions) (Page[model.AuthClient], error) {
	return listPage[model.AuthClient](ctx, "SELECT * FROM auth_client"+clientFilter+" ORDER BY id DESC LIMIT $3 OFFSET $4", "SELECT count(*) FROM auth_client"+clientFilter, options)
}

type AdminEntry struct {
	ID         int64  `db:"id" json:"id"`
	UserID     int64  `db:"user_id" json:"user_id"`
	Email      string `db:"email" json:"email"`
	IsDisabled bool   `db:"is_disabled" json:"is_disabled"`
}

func ListAdmins(ctx context.Context, options ListOptions) (Page[AdminEntry], error) {
	from := ` FROM auth_admin a JOIN auth_user u ON u.id = a.user_id WHERE ($1 = '' OR strpos(lower(coalesce(u.email,'') || ' ' || coalesce(u.display_name,'') || ' ' || u.id::text), lower($1)) > 0) AND ($2::boolean IS NULL OR u.is_disabled = $2)`
	return listPage[AdminEntry](ctx, "SELECT a.id,a.user_id,u.email,u.is_disabled"+from+" ORDER BY a.id DESC LIMIT $3 OFFSET $4", "SELECT count(*)"+from, options)
}

type UserPatch struct {
	Email       *string `json:"email"`
	DisplayName *string `json:"display_name"`
	IsDisabled  *bool   `json:"is_disabled"`
}

func UpdateUser(ctx context.Context, id int64, patch UserPatch) error {
	tx, err := internal.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Use the same lock order as user/admin deletion, before locking a user row.
	if patch.IsDisabled != nil && *patch.IsDisabled {
		if err := protectLastAdmin(tx, id); err != nil {
			return err
		}
	}
	var old model.AuthUser
	if err := tx.GetContext(ctx, &old, "SELECT * FROM auth_user WHERE id=$1 FOR UPDATE", id); err != nil {
		return err
	}
	email, display, disabled := old.Email, old.DisplayName, old.IsDisabled
	if patch.Email != nil {
		email = *patch.Email
	}
	if patch.DisplayName != nil {
		display = *patch.DisplayName
	}
	if patch.IsDisabled != nil {
		disabled = *patch.IsDisabled
	}
	increment := 0
	if email != old.Email || disabled != old.IsDisabled {
		increment = 1
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auth_user SET email=$2,display_name=$3,is_disabled=$4,security_version=security_version+$5,updated_at=now() WHERE id=$1`, id, email, display, disabled, increment); err != nil {
		return err
	}
	if email != old.Email {
		if _, err := tx.ExecContext(ctx, `UPDATE auth_user_provider SET login_key=$2,updated_at=now() WHERE user_id=$1 AND provider_type='EmailPassword'`, id, email); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func ResetUserPassword(ctx context.Context, id int64, providerData []byte) error {
	tx, err := internal.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked int64
	if err := tx.GetContext(ctx, &locked, "SELECT id FROM auth_user WHERE id=$1 FOR UPDATE", id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE auth_user_provider SET provider_data=$2,updated_at=now() WHERE user_id=$1 AND provider_type='EmailPassword'`, id, string(providerData))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNoPassword
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auth_user SET security_version=security_version+1,updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func lockClient(ctx context.Context, tx *sqlx.Tx, id int64) (model.AuthClient, error) {
	var client model.AuthClient
	if id == internal.AuthServerConfig.Admin.ClientID {
		return client, ErrProtectedClient
	}
	err := tx.GetContext(ctx, &client, "SELECT * FROM auth_client WHERE id=$1 FOR UPDATE", id)
	return client, err
}
func UpdateClient(ctx context.Context, client *model.AuthClient) error {
	tx, err := internal.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := lockClient(ctx, tx, client.ID)
	if err != nil {
		return err
	}
	if client.Public != old.Public {
		return ErrClientType
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_client SET display_name=$2,domain=$3,scopes=$4,token_type=$5,redirect_uris=$6,grant_types=$7,security_version=security_version+1,updated_at=now() WHERE id=$1`, client.ID, client.DisplayName, client.Domain, client.Scopes, client.TokenType, client.RedirectURIs, client.GrantTypes)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func SetClientDisabled(ctx context.Context, id int64, disabled bool) error {
	tx, err := internal.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := lockClient(ctx, tx, id)
	if err != nil {
		return err
	}
	if disabled == old.IsDisabled {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auth_client SET is_disabled=$2,security_version=security_version+1,updated_at=now() WHERE id=$1`, id, disabled); err != nil {
		return err
	}
	return tx.Commit()
}
func ResetClientSecret(ctx context.Context, id int64, secret string) error {
	tx, err := internal.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	client, err := lockClient(ctx, tx, id)
	if err != nil {
		return err
	}
	if client.Public {
		return ErrPublicSecret
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auth_client SET secret=$2,security_version=security_version+1,updated_at=now() WHERE id=$1`, id, secret); err != nil {
		return err
	}
	return tx.Commit()
}

// Distinguish unknown IDs from successful writes instead of reporting no-op deletion.
func requireAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
