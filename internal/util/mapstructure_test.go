package util_test

import (
	"auth-server/internal/model"
	"auth-server/internal/util"
	"encoding/json"
	"testing"
)

func Test1(t *testing.T) {
	j := `
	{
    "user": {
      "id": 2,
      "email": "test@example.com",
      "display_name": "Test User"
    },
    "provider_type": 1,
    "login_ip": "127.0.0.1",
    "login_at": "2025-11-21T11:30:12.028691521+08:00",
    "metadata": {
      "login_key": "123456"
    }
  }
`
	var m map[string]any
	err := json.Unmarshal([]byte(j), &m)
	if err != nil {
		t.Fatal(err)
	}
	su, err := util.MapToStruct[*model.SessionUserInfo](m)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(su)
}
