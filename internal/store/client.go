package store

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-oauth2/oauth2/v4"
)

var ClientRepo oauth2.ClientStore = &DbClientStore{}

type DbClientStore struct {
}

func (m *DbClientStore) GetByID(_ context.Context, id string) (oauth2.ClientInfo, error) {
	clientID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || clientID <= 0 {
		// Client ids are BIGINT in this deployment; an unknown/non-numeric id is
		// treated like an unknown client rather than a database error.
		return nil, nil
	}
	var client model.AuthClient
	err = internal.DB.Get(&client, "select * from auth_client where id = $1", clientID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// List returns all clients (admin usage).
func (m *DbClientStore) List(ctx context.Context) ([]model.AuthClient, error) {
	var clients []model.AuthClient
	if err := internal.DB.SelectContext(ctx, &clients, "select * from auth_client order by created_at desc"); err != nil {
		return nil, err
	}
	return clients, nil
}

// Create inserts a new client and lets PostgreSQL allocate the BIGINT id.
func (m *DbClientStore) Create(ctx context.Context, c *model.AuthClient) error {
	now := time.Now()
	c.CreatedAt = &now
	c.UpdatedAt = &now
	if c.TokenType == nil {
		return errors.New("token type required")
	}
	if c.Public {
		c.Secret = ""
	} else if c.Secret == "" {
		return errors.New("confidential clients require a secret")
	}
	if len(c.RedirectURIs) == 0 && c.Domain != "" {
		c.RedirectURIs = model.TextArray{c.Domain}
	}
	if len(c.GrantTypes) == 0 {
		c.GrantTypes = model.TextArray{"authorization_code", "client_credentials", "refresh_token", "implicit"}
	}

	rows, err := internal.DB.NamedQueryContext(ctx, `insert into auth_client
		(display_name, secret, domain, scopes, token_type, is_public, redirect_uris, grant_types, created_at, updated_at)
		values (:display_name, :secret, :domain, :scopes, :token_type, :is_public, :redirect_uris, :grant_types, :created_at, :updated_at)
		returning id`, c)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return fmt.Errorf("create client: no id returned")
	}
	if err := rows.Scan(&c.ID); err != nil {
		return err
	}
	return rows.Err()
}

// Delete removes a client by numeric id.
func (m *DbClientStore) Delete(ctx context.Context, id string) error {
	clientID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || clientID <= 0 {
		return errors.New("invalid client id")
	}
	_, err = internal.DB.ExecContext(ctx, "delete from auth_client where id = $1", clientID)
	return err
}
