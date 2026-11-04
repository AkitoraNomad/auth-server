package admin

import (
	"crypto/rand"
	"encoding/base64"
)

// generateRandomSecret returns a URL-safe, unguessable client secret. It is
// shown to the administrator exactly once at creation time.
func generateRandomSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
