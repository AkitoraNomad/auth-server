package model_test

import (
	"auth-server/internal/model"
	"testing"
)

func TestParseScopesManagement(t *testing.T) {
	scopes, err := model.ParseScopes("manage:system manage:clients")
	if err != nil {
		t.Fatalf("ParseScopes returned error: %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(scopes))
	}
	if scopes[0].Name != model.ScopeManageSystem || scopes[1].Name != model.ScopeManageClients {
		t.Fatalf("unexpected scopes: %#v", scopes)
	}
	for _, scope := range scopes {
		if scope.Name == model.ScopeProfile {
			t.Fatal("admin scopes must not implicitly include profile")
		}
	}
}

func TestParseScopesRejectsUnknown(t *testing.T) {
	if _, err := model.ParseScopes("manage:system unknown"); err == nil {
		t.Fatal("expected unknown scope to be rejected")
	}
}

func TestParseScopesDefaults(t *testing.T) {
	scopes, err := model.ParseScopes("")
	if err != nil {
		t.Fatalf("ParseScopes returned error: %v", err)
	}
	if len(scopes) != 1 || scopes[0].Name != model.ScopeProfile {
		t.Fatalf("expected profile default, got %#v", scopes)
	}
}

func TestAuthClientExactRedirectURIs(t *testing.T) {
	client := &model.AuthClient{
		RedirectURIs: model.TextArray{
			"http://localhost:5173/admin-page/callback",
			"https://admin.example.com/admin-page/callback",
		},
	}
	if !client.AllowsRedirectURI("https://admin.example.com/admin-page/callback") {
		t.Fatal("expected registered redirect URI to be allowed")
	}
	if client.AllowsRedirectURI("https://admin.example.com/admin-page/callback/extra") {
		t.Fatal("redirect URI must match exactly")
	}
	if client.GetDomain() != "" {
		t.Fatalf("clients with an exact allow-list must not expose a legacy domain, got %q", client.GetDomain())
	}
}

func TestAuthClientLegacyDomainFallback(t *testing.T) {
	client := &model.AuthClient{Domain: "https://legacy.example.com/callback"}
	if !client.AllowsRedirectURI("https://legacy.example.com/callback") {
		t.Fatal("legacy domain should be allowed")
	}
	if client.AllowsRedirectURI("https://legacy.example.com/other") {
		t.Fatal("legacy redirect URI must match exactly")
	}
	if client.GetDomain() != "https://legacy.example.com/callback" {
		t.Fatalf("legacy domain lost: %q", client.GetDomain())
	}
}

func TestAuthClientGrantAndPublicFlags(t *testing.T) {
	public := &model.AuthClient{Public: true, GrantTypes: model.TextArray{"authorization_code"}}
	if !public.IsPublic() {
		t.Fatal("public client must report IsPublic()==true")
	}
	if !public.AllowsGrantType("authorization_code") {
		t.Fatal("authorization_code should be allowed")
	}
	if public.AllowsGrantType("client_credentials") {
		t.Fatal("client_credentials should not be allowed")
	}
}
