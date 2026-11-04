package oauth2_test

import (
	"auth-server/internal/model"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/server"
)

// ---------- in-memory stores ----------

type memClientStore struct {
	mu      sync.RWMutex
	clients map[string]oauth2.ClientInfo
}

func newMemClientStore() *memClientStore {
	return &memClientStore{clients: make(map[string]oauth2.ClientInfo)}
}

func (s *memClientStore) GetByID(_ context.Context, id string) (oauth2.ClientInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clients[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (s *memClientStore) Set(id string, cli oauth2.ClientInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[id] = cli
}

type memTokenStore struct {
	mu      sync.RWMutex
	tokens  map[string]oauth2.TokenInfo
	codes   map[string]string
	refresh map[string]string
}

func newMemTokenStore() *memTokenStore {
	return &memTokenStore{
		tokens:  make(map[string]oauth2.TokenInfo),
		codes:   make(map[string]string),
		refresh: make(map[string]string),
	}
}

func (s *memTokenStore) Create(_ context.Context, info oauth2.TokenInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[info.GetAccess()] = info
	if code := info.GetCode(); code != "" {
		s.codes[code] = info.GetAccess()
	}
	if refresh := info.GetRefresh(); refresh != "" {
		s.refresh[refresh] = info.GetAccess()
	}
	return nil
}

func (s *memTokenStore) RemoveByCode(_ context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if access, ok := s.codes[code]; ok {
		delete(s.tokens, access)
		delete(s.codes, code)
	}
	return nil
}

func (s *memTokenStore) RemoveByAccess(_ context.Context, access string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, access)
	return nil
}

func (s *memTokenStore) RemoveByRefresh(_ context.Context, refresh string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if access, ok := s.refresh[refresh]; ok {
		delete(s.tokens, access)
		delete(s.refresh, refresh)
	}
	return nil
}

func (s *memTokenStore) GetByCode(_ context.Context, code string) (oauth2.TokenInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if access, ok := s.codes[code]; ok {
		return s.tokens[access], nil
	}
	return nil, nil
}

func (s *memTokenStore) GetByAccess(_ context.Context, access string) (oauth2.TokenInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if info, ok := s.tokens[access]; ok {
		return info, nil
	}
	return nil, nil
}

func (s *memTokenStore) GetByRefresh(_ context.Context, refresh string) (oauth2.TokenInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if access, ok := s.refresh[refresh]; ok {
		return s.tokens[access], nil
	}
	return nil, nil
}

type testAccessGen struct{}

func (g *testAccessGen) Token(_ context.Context, _ *oauth2.GenerateBasic, isGenRefresh bool) (string, string, error) {
	access := "test-access-" + time.Now().Format("150405.000000000")
	refresh := ""
	if isGenRefresh {
		refresh = "test-refresh-" + time.Now().Format("150405.000000000")
	}
	return access, refresh, nil
}

// ---------- helpers ----------

func setupServer() *server.Server {
	manager := manage.NewDefaultManager()
	manager.MapTokenStorage(newMemTokenStore())
	manager.MapAccessGenerate(&testAccessGen{})
	cs := newMemClientStore()
	cs.Set("test-client", &model.AuthClient{
		ID:        1,
		Secret:    "correct-secret",
		Domain:    "http://localhost",
		TokenType: func() *model.TokenType { t := model.TokenType(1); return &t }(),
	})
	manager.MapClientStorage(cs)
	return server.NewServer(server.NewConfig(), manager)
}

func tokenReq(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func basicAuth(clientID, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(clientID+":"+secret))
}

// ---------- Basic Auth tests ----------

func TestBasicAuth_ValidSecret_ClientCredentials(t *testing.T) {
	srv := setupServer()
	req := tokenReq("grant_type=client_credentials&scope=read")
	req.Header.Set("Authorization", basicAuth("test-client", "correct-secret"))
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "access_token") {
		t.Fatalf("expected access_token: %s", w.Body.String())
	}
}

func TestBasicAuth_InvalidSecret_ClientCredentials(t *testing.T) {
	srv := setupServer()
	req := tokenReq("grant_type=client_credentials&scope=read")
	req.Header.Set("Authorization", basicAuth("test-client", "wrong-secret"))
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_client") {
		t.Fatalf("expected invalid_client: %s", w.Body.String())
	}
}

func TestBasicAuth_EmptySecret_ClientCredentials(t *testing.T) {
	srv := setupServer()
	req := tokenReq("grant_type=client_credentials&scope=read")
	req.Header.Set("Authorization", basicAuth("test-client", ""))
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_client") {
		t.Fatalf("expected invalid_client: %s", w.Body.String())
	}
}

func TestBasicAuth_NoAuthHeader(t *testing.T) {
	srv := setupServer()
	req := tokenReq("grant_type=client_credentials&scope=read")
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_client") {
		t.Fatalf("expected invalid_client: %s", w.Body.String())
	}
}

func TestBasicAuth_UnknownClient(t *testing.T) {
	srv := setupServer()
	req := tokenReq("grant_type=client_credentials&scope=read")
	req.Header.Set("Authorization", basicAuth("ghost", "secret"))
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_client") {
		t.Fatalf("expected invalid_client: %s", w.Body.String())
	}
}

func TestBasicAuth_ValidSecret_AuthCode_InvalidCode(t *testing.T) {
	srv := setupServer()
	body := "grant_type=authorization_code&code=invalid-code&redirect_uri=http://localhost/cb"
	req := tokenReq(body)
	req.Header.Set("Authorization", basicAuth("test-client", "correct-secret"))
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "invalid_client") {
		t.Fatalf("should NOT reject valid client: %s", w.Body.String())
	}
}

func TestBasicAuth_InvalidSecret_AuthCode(t *testing.T) {
	srv := setupServer()
	body := "grant_type=authorization_code&code=test-code&redirect_uri=http://localhost/cb"
	req := tokenReq(body)
	req.Header.Set("Authorization", basicAuth("test-client", "wrong-secret"))
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_client") {
		t.Fatalf("expected invalid_client: %s", w.Body.String())
	}
}

func TestBasicAuth_MalformedAuthHeader(t *testing.T) {
	srv := setupServer()
	req := tokenReq("grant_type=client_credentials&scope=read")
	req.Header.Set("Authorization", "NotBasic xyz")
	w := httptest.NewRecorder()
	_ = srv.HandleTokenRequest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_client") {
		t.Fatalf("expected invalid_client: %s", w.Body.String())
	}
}
