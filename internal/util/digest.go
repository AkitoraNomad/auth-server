package util

import (
	"crypto/sha256"
)

func DigestSHA256(input string) []byte {
	hash := sha256.Sum256([]byte(input))
	return hash[:]
}
