package oauth2

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

func LoadJWTConfig() error {
	var keys []model.JWTKey
	for _, keyConfig := range internal.AuthServerConfig.JWT {
		switch keyConfig.Alg {
		case "RS256":
			publicKey, privateKey, err := parseRSAPublicKeyFromPEM(keyConfig.Sec)
			if err != nil {
				return err
			}
			keys = append(keys, &model.RSAKey{KeyID: keyConfig.Kid, PublicKey: publicKey, PrivateKey: privateKey})
		case "HS512":
			keys = append(keys, &model.HMACKey{KeyID: keyConfig.Kid, Secret: []byte(keyConfig.Sec)})
		default:
			return fmt.Errorf("unknown alg: %s", keyConfig.Alg)
		}
	}
	JWTKeys = keys
	return nil
}

// Helper function to parse RSA public key
func parseRSAPublicKeyFromPEM(pemStr string) (*rsa.PublicKey, *rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, nil, fmt.Errorf("failed to parse PEM block")
	}

	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return &priv.PublicKey, priv, nil
}

var JWTKeys []model.JWTKey
