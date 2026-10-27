package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/go-session/session/v3"
)

const csrfSessionKey = "csrfToken"

func csrfToken(s session.Store) (string, error) {
	if value, ok := s.Get(csrfSessionKey); ok {
		if token, ok := value.(string); ok && token != "" {
			return token, nil
		}
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	s.Set(csrfSessionKey, token)
	if err := s.Save(); err != nil {
		return "", err
	}
	return token, nil
}

func validateCSRF(r *http.Request, s session.Store) error {
	value, ok := s.Get(csrfSessionKey)
	token, valid := value.(string)
	supplied := r.Header.Get("X-CSRF-Token")
	if !ok || !valid || token == "" || supplied == "" || subtle.ConstantTimeCompare([]byte(token), []byte(supplied)) != 1 {
		return errors.New("invalid csrf token")
	}
	return nil
}
