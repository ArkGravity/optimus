package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// generate writes a fresh 32-byte base64-encoded vault master key. Usage:
//
//	$ optimus vault-keygen > .vault-key
//	$ echo "OPTIMUS_VAULT_MASTER_KEY=$(optimus vault-keygen)" >> .env
func generate(w io.Writer) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("read random: %w", err)
	}
	enc := base64.StdEncoding.EncodeToString(key)
	_, err := fmt.Fprintln(w, enc)
	return err
}
