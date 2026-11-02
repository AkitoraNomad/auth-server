package http

import (
	"auth-server/internal"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-oauth2/oauth2/v4/server"
)

func TestAdminConfigPublishesResolvedClientIDWithoutAuthentication(t *testing.T) {
	original := internal.AuthServerConfig
	t.Cleanup(func() { internal.AuthServerConfig = original })
	internal.AuthServerConfig.Admin.ClientID = 81
	engine := CreateGinEngine(&server.Server{}, nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/api/public/admin-config", nil))
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || len(payload) != 1 || payload["client_id"] != "81" {
		t.Fatalf("unexpected configuration: %d %s", response.Code, response.Body.String())
	}
}
