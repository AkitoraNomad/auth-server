package oauth2

import (
	"auth-server/internal/model"
	"context"
	"fmt"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/generates"
)

var tokenGenerator = map[model.TokenType]oauth2.AccessGenerate{
	model.OpaqueToken: &generates.AccessGenerate{},
	model.JWT:         NewJWTAccessGenerate(),
}

type ClientConfigTokenGenerate struct {
}

func (c *ClientConfigTokenGenerate) Token(ctx context.Context, data *oauth2.GenerateBasic, isGenRefresh bool) (access, refresh string, err error) {
	var tokenType model.TokenType
	if ci, ok := data.Client.(model.ScopedClientInfo); ok {
		tokenType = ci.GetTokenType()
	} else {
		tokenType = model.OpaqueToken
	}
	generator, ok := tokenGenerator[tokenType]
	if !ok || generator == nil {
		return "", "", fmt.Errorf("unsupported token type: %s", tokenType.String())
	}
	return generator.Token(ctx, data, isGenRefresh)
}
