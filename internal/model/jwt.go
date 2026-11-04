package model

import (
	"crypto/rsa"
	"encoding/base64"
	"math/big"
)

type JWTKey interface {
	Kid() string
	Alg() string
	ToJWK() map[string]string
}

type RSAKey struct {
	KeyID      string
	PublicKey  *rsa.PublicKey
	PrivateKey *rsa.PrivateKey
}

func (r *RSAKey) Kid() string {
	return r.KeyID
}

func (r *RSAKey) Alg() string {
	return "RS256"
}

func (r *RSAKey) ToJWK() map[string]string {
	// RFC 7517/7518 require unpadded base64url ("base64url" without '=').
	n := base64.RawURLEncoding.EncodeToString(r.PublicKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(r.PublicKey.E)).Bytes())
	return map[string]string{
		"kty": "RSA",
		"alg": r.Alg(),
		"kid": r.Kid(),
		"n":   n,
		"e":   e,
		"use": "sig",
	}
}

type HMACKey struct {
	KeyID  string
	Secret []byte
}

func (h *HMACKey) Kid() string {
	return h.KeyID
}

func (h *HMACKey) Alg() string {
	return "HS512"
}

func (h *HMACKey) ToJWK() map[string]string {
	return map[string]string{
		"kty": "oct",
		"alg": h.Alg(),
		"kid": h.Kid(),
		//"k":   base64.StdEncoding.EncodeToString(h.Secret),
		"use": "sig",
	}
}
