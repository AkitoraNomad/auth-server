// Package password implements the server's password hashing policy.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memoryKiB        uint32 = 19 * 1024
	iterations       uint32 = 2
	parallelism      uint8  = 1
	saltLength              = 16
	keyLength               = 32
	maxPasswordBytes        = 1024
)

var (
	errInvalidHash         = errors.New("invalid password hash")
	errIncompatibleVersion = errors.New("incompatible argon2 version")
)

// Bound aggregate Argon2 memory across all handlers, including admin creation
// and dummy verifications. Reject overload instead of queueing unbounded work.
var hashSlots = make(chan struct{}, 4)

func acquireHashSlot() bool {
	select {
	case hashSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

var dummyHash = func() string {
	hash, err := Hash("not-a-real-password")
	if err != nil {
		return ""
	}
	return hash
}()

// Hash returns a PHC-formatted Argon2id hash.
func Hash(plain string) (string, error) {
	if plain == "" {
		return "", errors.New("password must not be empty")
	}
	if len(plain) > maxPasswordBytes {
		return "", errors.New("password is too long")
	}
	if !acquireHashSlot() {
		return "", errors.New("password service busy")
	}
	defer func() { <-hashSlots }()

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(plain), salt, iterations, memoryKiB, parallelism, keyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memoryKiB,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify reports whether plain matches encoded. Malformed or unsupported
// hashes fail closed.
func Verify(encoded, plain string) bool {
	if encoded == "" || len(plain) > maxPasswordBytes {
		return false
	}
	version, memory, timeCost, threads, salt, expected, err := decode(encoded)
	if err != nil {
		return false
	}
	if version != argon2.Version || memory < 8*1024 || memory > 256*1024 ||
		timeCost < 1 || timeCost > 10 || threads < 1 || threads > 16 ||
		len(salt) < 8 || len(salt) > 64 || len(expected) < 16 || len(expected) > 64 {
		return false
	}
	if !acquireHashSlot() {
		return false
	}
	defer func() { <-hashSlots }()
	actual := argon2.IDKey([]byte(plain), salt, timeCost, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// DummyVerify burns a real Argon2id verification. Auth handlers call it when a
// user is not found so unknown-account and wrong-password requests have similar
// cost, reducing timing-based account enumeration.
func DummyVerify(plain string) {
	if dummyHash == "" {
		return
	}
	_ = Verify(dummyHash, plain)
}

func decode(encoded string) (version int, memory, timeCost uint32, threads uint8, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] == "" {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}
	var p uint32
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &p); err != nil || p < 1 || p > 255 {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}
	threads = uint8(p)
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return 0, 0, 0, 0, nil, nil, errInvalidHash
	}
	if version != argon2.Version {
		return 0, 0, 0, 0, nil, nil, errIncompatibleVersion
	}
	return version, memory, timeCost, threads, salt, key, nil
}
