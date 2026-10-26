package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"encoding/json"
	"net/http"
	"strings"
)

func discoveryHandler(w http.ResponseWriter, _ *http.Request) {
	issuer := strings.TrimRight(internal.AuthServerConfig.Issuer, "/")

	scopes := make([]string, 0)
	for _, scope := range model.AllScopes() {
		scopes = append(scopes, scope.Name)
	}

	metadata := map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + internal.PathAuthorize,
		"token_endpoint":                        issuer + internal.PathToken,
		"jwks_uri":                              issuer + internal.PathJwkSet,
		"userinfo_endpoint":                     issuer + internal.PathUserinfo,
		"introspection_endpoint":                issuer + internal.PathIntrospect,
		"response_types_supported":              []string{"code", "token"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials"},
		"code_challenge_methods_supported":      []string{"S256"},
		"subject_types_supported":               []string{"public"},
		"scopes_supported":                      scopes,
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_basic", "client_secret_post"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(metadata)
}
