package password

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	if !Verify(hash, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if Verify(hash, "wrong password") {
		t.Fatal("wrong password accepted")
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	a, err := Hash("same password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash("same password")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("hashes for the same password must use different salts")
	}
}

func TestVerifyRejectsMalformedAndUnsupportedHashes(t *testing.T) {
	hash, err := Hash("password")
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{
		"",
		"not-a-hash",
		strings.Replace(hash, "$argon2id$", "$argon2i$", 1),
		strings.Replace(hash, "v=19", "v=16", 1),
		strings.Replace(hash, "m=19456", "m=999999999", 1),
	} {
		if Verify(candidate, "password") {
			t.Fatalf("malformed hash accepted: %s", candidate)
		}
	}
}

func TestHashRejectsInvalidPasswords(t *testing.T) {
	if _, err := Hash(""); err == nil {
		t.Fatal("empty password accepted")
	}
	if _, err := Hash(strings.Repeat("x", maxPasswordBytes+1)); err == nil {
		t.Fatal("oversized password accepted")
	}
}

func TestDummyVerifyDoesNotPanic(t *testing.T) {
	DummyVerify("anything")
}
