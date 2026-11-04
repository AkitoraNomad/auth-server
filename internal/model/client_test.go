package model_test

import (
	"auth-server/internal/model"
	"testing"

	"github.com/go-oauth2/oauth2/v4"
)

func TestAuthClientIsNotPublic(t *testing.T) {
	c := &model.AuthClient{Secret: "s"}
	if c.IsPublic() {
		t.Fatal("AuthClient.IsPublic() should return false to enforce client secret validation")
	}
}

func TestAuthClientInterfaces(t *testing.T) {
	c := &model.AuthClient{
		ID:        42,
		Secret:    "s3cret",
		Domain:    "https://example.com/cb",
		TokenType: func() *model.TokenType { t := model.TokenType(1); return &t }(),
	}
	if c.GetID() != "42" {
		t.Fatalf("GetID = %s", c.GetID())
	}
	if c.GetSecret() != "s3cret" {
		t.Fatalf("GetSecret = %s", c.GetSecret())
	}
	if c.GetDomain() != "https://example.com/cb" {
		t.Fatalf("GetDomain = %s", c.GetDomain())
	}
	if c.IsPublic() {
		t.Fatal("IsPublic should be false")
	}
	var _ oauth2.ClientInfo = c
	var _ model.ScopedClientInfo = c
}
