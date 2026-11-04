// Command hash-password prints an Argon2id hash for an admin/bootstrap password.
//
// Read the password from stdin without echoing it:
//
//	read -r -s PASSWORD
//	printf '%s' "$PASSWORD" | go run ./cmd/hash-password
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"auth-server/internal/password"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, "read password:", err)
		os.Exit(1)
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	hash, err := password.Hash(line)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash password:", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
