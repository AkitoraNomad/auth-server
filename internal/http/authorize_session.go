package http

import (
	"auth-server/internal"
	"auth-server/internal/util"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"time"

	"github.com/go-session/session/v3"
)

const (
	authorizeRequestTTL     = 10 * time.Minute
	maxAuthorizeRequests    = 5
	authorizeRequestIDBytes = 32
)

// authorizeRequestState is the complete, per-request authorization intent. It
// is kept in the server-side session and referenced from the login/consent
// pages by a random request id. Keeping the full request here (including the
// exact redirect_uri) avoids trusting hidden form fields after login.
type authorizeRequestState struct {
	ID                  string    `json:"id"`
	ClientID            string    `json:"client_id"`
	RedirectURI         string    `json:"redirect_uri"`
	ResponseType        string    `json:"response_type"`
	Scope               []string  `json:"scope"`
	State               string    `json:"state"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

func newAuthorizeRequestID() (string, error) {
	raw := make([]byte, authorizeRequestIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func loadAuthorizeRequests(s session.Store) []*authorizeRequestState {
	raw, ok := s.Get(internal.SessionKeyAuthorizeRequests)
	if !ok || raw == nil {
		return nil
	}
	if typed, ok := raw.([]*authorizeRequestState); ok {
		return typed
	}
	requests, err := util.Decode[[]*authorizeRequestState](raw)
	if err != nil {
		slog.Error("failed to decode authorize requests from session", "err", err)
		return nil
	}
	return requests
}

func activeAuthorizeRequestID(s session.Store) string {
	raw, ok := s.Get(internal.SessionKeyActiveAuthorizeRequest)
	if !ok {
		return ""
	}
	id, _ := raw.(string)
	return id
}

func findAuthorizeRequest(s session.Store, id string) *authorizeRequestState {
	requests := loadAuthorizeRequests(s)
	if id == "" {
		id = activeAuthorizeRequestID(s)
	}
	var newest *authorizeRequestState
	for _, request := range requests {
		if request == nil || time.Now().After(request.ExpiresAt) {
			continue
		}
		if id != "" && request.ID == id {
			return request
		}
		if newest == nil || request.CreatedAt.After(newest.CreatedAt) {
			newest = request
		}
	}
	if id == "" {
		return newest
	}
	return nil
}

func putAuthorizeRequest(s session.Store, request *authorizeRequestState) error {
	if request.ID == "" {
		id, err := newAuthorizeRequestID()
		if err != nil {
			return err
		}
		request.ID = id
	}
	now := time.Now()
	if request.CreatedAt.IsZero() {
		request.CreatedAt = now
	}
	if request.ExpiresAt.IsZero() {
		request.ExpiresAt = now.Add(authorizeRequestTTL)
	}

	requests := loadAuthorizeRequests(s)
	kept := make([]*authorizeRequestState, 0, len(requests)+1)
	for _, existing := range requests {
		if existing == nil || time.Now().After(existing.ExpiresAt) || existing.ID == request.ID {
			continue
		}
		kept = append(kept, existing)
	}
	kept = append(kept, request)
	if len(kept) > maxAuthorizeRequests {
		kept = kept[len(kept)-maxAuthorizeRequests:]
	}
	s.Set(internal.SessionKeyAuthorizeRequests, kept)
	s.Set(internal.SessionKeyActiveAuthorizeRequest, request.ID)
	return s.Save()
}

func removeAuthorizeRequest(s session.Store, id string) {
	if id == "" {
		return
	}
	requests := loadAuthorizeRequests(s)
	kept := make([]*authorizeRequestState, 0, len(requests))
	for _, existing := range requests {
		if existing != nil && existing.ID != id {
			kept = append(kept, existing)
		}
	}
	if len(kept) == 0 {
		s.Delete(internal.SessionKeyAuthorizeRequests)
	} else {
		s.Set(internal.SessionKeyAuthorizeRequests, kept)
	}
	if activeAuthorizeRequestID(s) == id {
		s.Delete(internal.SessionKeyActiveAuthorizeRequest)
	}
	deleteRequestConsent(s, id)
	if err := s.Save(); err != nil {
		slog.Error("failed to save session after removing authorize request", "err", err)
	}
}

func setActiveAuthorizeRequest(s session.Store, id string) {
	if id == "" {
		s.Delete(internal.SessionKeyActiveAuthorizeRequest)
	} else {
		s.Set(internal.SessionKeyActiveAuthorizeRequest, id)
	}
}

func setRequestConsent(s session.Store, requestID string, scopes []string) {
	if requestID == "" {
		return
	}
	consents := loadRequestConsents(s)
	consents[requestID] = append([]string(nil), scopes...)
	s.Set(internal.SessionKeyConsentByRequest, consents)
}

func deleteRequestConsent(s session.Store, requestID string) {
	if requestID == "" {
		return
	}
	consents := loadRequestConsents(s)
	if _, ok := consents[requestID]; !ok {
		return
	}
	delete(consents, requestID)
	if len(consents) == 0 {
		s.Delete(internal.SessionKeyConsentByRequest)
		return
	}
	s.Set(internal.SessionKeyConsentByRequest, consents)
}

func loadRequestConsents(s session.Store) map[string][]string {
	raw, ok := s.Get(internal.SessionKeyConsentByRequest)
	if !ok || raw == nil {
		return make(map[string][]string)
	}
	if typed, ok := raw.(map[string][]string); ok {
		return typed
	}
	consents, err := util.Decode[map[string][]string](raw)
	if err != nil {
		slog.Error("failed to decode request consents from session", "err", err)
		return make(map[string][]string)
	}
	if consents == nil {
		return make(map[string][]string)
	}
	return consents
}
