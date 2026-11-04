package oauth2

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessTokenClaims is the explicit JWT contract for access tokens. The
// management API verifies these claims and then performs a Redis lookup so a
// revoked token cannot be used before its exp claim.
type AccessTokenClaims struct {
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
	TokenUse string `json:"token_use"`
	jwt.RegisteredClaims
}

// JWTAccessGenerate signs access tokens with the first configured RSA key. It
// intentionally implements a small, explicit claim set instead of relying on
// go-oauth2's default (sub/aud/exp only).
type JWTAccessGenerate struct{}

func NewJWTAccessGenerate() *JWTAccessGenerate {
	return &JWTAccessGenerate{}
}

func (g *JWTAccessGenerate) Token(_ context.Context, data *oauth2.GenerateBasic, isGenRefresh bool) (string, string, error) {
	key, err := accessSigningKey()
	if err != nil {
		return "", "", err
	}

	issuedAt := data.TokenInfo.GetAccessCreateAt()
	if issuedAt.IsZero() {
		issuedAt = time.Now()
	}
	expiresAt := issuedAt.Add(data.TokenInfo.GetAccessExpiresIn())
	claims := AccessTokenClaims{
		ClientID: data.Client.GetID(),
		Scope:    data.TokenInfo.GetScope(),
		TokenUse: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    strings.TrimRight(internal.AuthServerConfig.Issuer, "/"),
			Subject:   data.UserID,
			Audience:  jwt.ClaimStrings{data.Client.GetID()},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = key.Kid()
	access, err := token.SignedString(key.PrivateKey)
	if err != nil {
		return "", "", err
	}

	refresh := ""
	if isGenRefresh {
		random := uuid.Must(uuid.NewRandom()).String()
		refresh = base64.URLEncoding.EncodeToString([]byte(uuid.NewSHA1(uuid.Must(uuid.NewRandom()), []byte(random)).String()))
		refresh = strings.ToUpper(strings.TrimRight(refresh, "="))
	}
	return access, refresh, nil
}

// ParseAccessToken validates the JWT signature, issuer, audience, expiry and
// token_use claim. It is used by the admin API before the Redis revocation
// lookup.
func ParseAccessToken(raw, expectedAudience string) (*AccessTokenClaims, error) {
	if raw == "" || strings.Count(raw, ".") != 2 {
		return nil, errors.New("access token is not a JWT")
	}
	claims := &AccessTokenClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != "RS256" {
			return nil, fmt.Errorf("unexpected JWT signing method: %s", token.Method.Alg())
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("access token kid is missing")
		}
		key, err := accessVerificationKeyForKid(kid)
		if err != nil {
			return nil, err
		}
		return key.PublicKey, nil
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(strings.TrimRight(internal.AuthServerConfig.Issuer, "/")),
		jwt.WithAudience(expectedAudience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid access token")
	}
	if claims.TokenUse != "access" {
		return nil, errors.New("unexpected token_use")
	}
	if claims.ClientID == "" || claims.Subject == "" {
		return nil, errors.New("access token is missing required claims")
	}
	return claims, nil
}

func accessSigningKey() (*model.RSAKey, error) {
	for _, key := range JWTKeys {
		if rsaKey, ok := key.(*model.RSAKey); ok {
			return rsaKey, nil
		}
	}
	return nil, errors.New("no RSA JWT key configured")
}

func accessVerificationKeyForKid(kid string) (*model.RSAKey, error) {
	if kid == "" {
		return nil, errors.New("empty JWT kid")
	}
	for _, key := range JWTKeys {
		if rsaKey, ok := key.(*model.RSAKey); ok && rsaKey.Kid() == kid {
			return rsaKey, nil
		}
	}
	return nil, fmt.Errorf("unknown JWT kid: %s", kid)
}
