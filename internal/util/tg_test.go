package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"
)

func signedTelegram(t *testing.T, values map[string]any, token string) json.RawMessage {
	t.Helper()
	keys := make([]string, 0, len(values))
	for k := range values {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, values[k]))
	}
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(joinLines(pairs)))
	values["hash"] = hex.EncodeToString(mac.Sum(nil))
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func joinLines(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += "\n"
		}
		out += v
	}
	return out
}

func TestTelegramCredentialValid(t *testing.T) {
	token := "test-bot-token"
	now := time.Unix(1_800_000_000, 0)
	data := signedTelegram(t, map[string]any{
		"id": 111, "first_name": "Alice", "auth_date": now.Unix(),
	}, token)
	got, err := ParseTelegramCredential(data, []byte(token), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 111 || got.FirstName != "Alice" {
		t.Fatalf("unexpected parsed data: %+v", got)
	}
}

func TestTelegramCredentialRejectsStaleFutureAndMalformed(t *testing.T) {
	token := "test-bot-token"
	now := time.Unix(1_800_000_000, 0)
	base := map[string]any{"id": 111, "first_name": "Alice", "auth_date": now.Unix()}
	for name, values := range map[string]map[string]any{
		"stale":     {"id": 111, "first_name": "Alice", "auth_date": now.Add(-10 * time.Minute).Unix()},
		"future":    {"id": 111, "first_name": "Alice", "auth_date": now.Add(10 * time.Minute).Unix()},
		"no id":     {"first_name": "Alice", "auth_date": now.Unix()},
		"no date":   {"id": 111, "first_name": "Alice"},
		"bad types": {"id": "111", "first_name": "Alice", "auth_date": now.Unix()},
	} {
		t.Run(name, func(t *testing.T) {
			data := signedTelegram(t, values, token)
			if _, err := ParseTelegramCredential(data, []byte(token), now); err == nil {
				t.Fatalf("malformed telegram data accepted: %v", values)
			}
		})
	}
	// A numeric hash must not panic and must fail closed.
	badHash, _ := json.Marshal(map[string]any{"id": 111, "auth_date": now.Unix(), "hash": 42})
	if _, err := ParseTelegramCredential(badHash, []byte(token), now); err == nil {
		t.Fatal("numeric hash accepted")
	}
	// A tampered signed field must fail the HMAC check.
	data := signedTelegram(t, base, token)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	m["first_name"] = "Mallory"
	tampered, _ := json.Marshal(m)
	if _, err := ParseTelegramCredential(tampered, []byte(token), now); err == nil {
		t.Fatal("tampered field accepted")
	}
}
