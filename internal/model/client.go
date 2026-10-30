package model

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/go-oauth2/oauth2/v4"
)

type ScopedClientInfo interface {
	oauth2.ClientInfo
	GetDisplayName() string
	GetScopes() []string
	GetTokenType() TokenType
	GetRedirectURIs() []string
	GetGrantTypes() []string
}

type AuthClient struct {
	IsDisabled      bool       `db:"is_disabled" json:"is_disabled"`
	SecurityVersion int64      `db:"security_version" json:"-"`
	ID              int64      `db:"id" json:"id"`
	Secret          string     `db:"secret" json:"-"`
	Domain          string     `db:"domain" json:"domain"`
	DisplayName     string     `db:"display_name" json:"display_name"`
	Scopes          TextArray  `db:"scopes" json:"scopes"`
	TokenType       *TokenType `db:"token_type" json:"token_type"`
	Public          bool       `db:"is_public" json:"is_public"`
	RedirectURIs    TextArray  `db:"redirect_uris" json:"redirect_uris"`
	GrantTypes      TextArray  `db:"grant_types" json:"grant_types"`
	BaseModel
}

func (c *AuthClient) GetID() string {
	return fmt.Sprint(c.ID)
}

func (c *AuthClient) GetSecret() string {
	return c.Secret
}

func (c *AuthClient) GetDomain() string {
	// Clients with an explicit redirect allow-list are checked by the HTTP
	// authorization handlers using exact string matching. Returning an empty
	// legacy domain here disables go-oauth2's weak host-suffix check while those
	// handlers remain the source of truth. Clients without redirect_uris keep the
	// legacy domain behaviour.
	if len(c.RedirectURIs) > 0 {
		return ""
	}
	return c.Domain
}

// IsPublic reports whether this is a public client (no client secret).
func (c *AuthClient) IsPublic() bool {
	return c.Public
}

func (c *AuthClient) GetUserID() string {
	return ""
}

func (c *AuthClient) GetDisplayName() string {
	return c.DisplayName
}

func (c *AuthClient) GetScopes() []string {
	return c.Scopes
}

func (c *AuthClient) GetRedirectURIs() []string {
	return c.RedirectURIs
}

func (c *AuthClient) GetGrantTypes() []string {
	return c.GrantTypes
}

func (c *AuthClient) HasScope(scope string) bool {
	return slices.Contains(c.Scopes, scope)
}

// AllowsRedirectURI performs exact redirect URI matching. Existing clients that
// predate redirect_uris keep working through the legacy single domain value.
func (c *AuthClient) AllowsRedirectURI(uri string) bool {
	if !ValidRedirectURI(uri) {
		return false
	}
	if len(c.RedirectURIs) > 0 {
		return slices.Contains(c.RedirectURIs, uri)
	}
	return c.Domain != "" && c.Domain == uri
}

// ValidRedirectURI applies to stored clients as well as new configuration.
func ValidRedirectURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" || u.User != nil || strings.Contains(uri, "#") || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}

// AllowsGrantType reports whether the client may use a grant type. An empty
// grant list is treated as backward-compatible "all server grants".
func (c *AuthClient) AllowsGrantType(grant string) bool {
	if c.IsDisabled {
		return false
	}
	if len(c.GrantTypes) == 0 {
		return true
	}
	return slices.Contains(c.GrantTypes, grant)
}

func (c *AuthClient) GetTokenType() TokenType {
	if c.TokenType == nil {
		return OpaqueToken
	}
	return *c.TokenType
}
