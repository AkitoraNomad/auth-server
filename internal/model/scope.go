package model

import (
	"fmt"
	"slices"
	"strings"
)

const (
	ScopeProfile       = "profile"
	ScopeAvatar        = "avatar"
	ScopeManageSystem  = "manage:system"
	ScopeManageClients = "manage:clients"
)

type Scope struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default"`
}

func (s *Scope) GetName() string {
	return s.Name
}

func (s *Scope) GetDescription() string {
	return s.Description
}

func (s *Scope) IsDefault() bool {
	return s.Default
}

func (s *Scope) String() string {
	return s.Name
}

var (
	Profile = &Scope{
		Name:        ScopeProfile,
		Description: "Basic profile information",
		Default:     true,
	}
	Avatar = &Scope{
		Name:        ScopeAvatar,
		Description: "Avatar image",
	}
	ManageSystem = &Scope{
		Name:        ScopeManageSystem,
		Description: "Manage users, administrators and sessions",
	}
	ManageClients = &Scope{
		Name:        ScopeManageClients,
		Description: "Manage OAuth2 clients",
	}
	allScopes     = []*Scope{Profile, Avatar, ManageSystem, ManageClients}
	defaultScopes = []*Scope{Profile}
)

// AllScopes returns the scopes this server understands.
func AllScopes() []*Scope {
	return slices.Clone(allScopes)
}

// ParseScope resolves one known scope name.
func ParseScope(name string) (*Scope, error) {
	for _, scope := range allScopes {
		if name == scope.Name {
			return scope, nil
		}
	}
	return nil, fmt.Errorf("invalid scope: %s", name)
}

// ParseScopes resolves a space-delimited scope value.
//
// Unknown scopes are rejected instead of silently discarded: an authorization
// server must never grant a different scope set than the one requested. If the
// value is empty the default scopes are returned (currently "profile").
func ParseScopes(s string) ([]*Scope, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return slices.Clone(defaultScopes), nil
	}

	result := make([]*Scope, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, name := range fields {
		if _, ok := seen[name]; ok {
			continue
		}
		scope, err := ParseScope(name)
		if err != nil {
			return nil, err
		}
		seen[name] = struct{}{}
		result = append(result, scope)
	}
	return result, nil
}

// ScopeNames returns the scope names in their original order.
func ScopeNames(scopes []*Scope) []string {
	names := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope != nil {
			names = append(names, scope.Name)
		}
	}
	return names
}

// ScopeString joins scopes with a single space, as used by OAuth2.
func ScopeString(scopes []*Scope) string {
	return strings.Join(ScopeNames(scopes), " ")
}

// ParseScopeNames resolves names returned by a client allow-list. Unknown or
// empty names are skipped so legacy rows with stale scope values do not break
// startup; actual request scopes are still validated strictly.
func ParseScopeNames(names []string) []*Scope {
	result := make([]*Scope, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		scope, err := ParseScope(name)
		if err != nil {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, scope)
	}
	return result
}

// HasScope reports whether a space-delimited scope value contains want.
func HasScope(scopeValue, want string) bool {
	for _, name := range strings.Fields(scopeValue) {
		if name == want {
			return true
		}
	}
	return false
}
