package model_test

import (
	"auth-server/internal/model"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
)

func TestRSAKeyToJWKUsesUnpaddedBase64URL(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwk := (&model.RSAKey{
		KeyID:      "test",
		PublicKey:  &key.PublicKey,
		PrivateKey: key,
	}).ToJWK()

	if jwk["kid"] != "test" || jwk["alg"] != "RS256" || jwk["kty"] != "RSA" {
		t.Fatalf("unexpected JWK metadata: %#v", jwk)
	}
	if strings.Contains(jwk["n"], "=") || strings.Contains(jwk["e"], "=") {
		t.Fatalf("JWK must use unpadded base64url: %#v", jwk)
	}
	if jwk["e"] != "AQAB" {
		t.Fatalf("unexpected RSA exponent: %q", jwk["e"])
	}
}
