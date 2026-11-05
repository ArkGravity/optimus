package cloudkey

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/mail"
	"strings"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
)

const maxServiceAccountJSONSize = 64 * 1024

type ServiceAccountMetadata struct {
	Email     string
	ProjectID string
}

// ValidateServiceAccountJSON validates locally and retains only the fields
// needed for explicit service-account authentication. It never follows URLs.
func ValidateServiceAccountJSON(data []byte) ([]byte, ServiceAccountMetadata, error) {
	invalid := func() ([]byte, ServiceAccountMetadata, error) {
		return nil, ServiceAccountMetadata{}, validationError()
	}
	if len(data) == 0 || len(data) > maxServiceAccountJSONSize {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSONValue(decoder, 0); err != nil {
		return invalid()
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return invalid()
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return invalid()
	}
	canonical := make(map[string]string, 8)
	for field, value := range raw {
		switch field {
		case "type", "project_id", "private_key_id", "private_key", "client_email", "client_id", "token_uri", "universe_domain":
			var text string
			if bytes.Equal(value, []byte("null")) || json.Unmarshal(value, &text) != nil {
				return invalid()
			}
			canonical[field] = text
		case "auth_uri", "auth_provider_x509_cert_url", "client_x509_cert_url":
			// Downloaded-key metadata is discarded, including any supplied URLs.
		default:
			return invalid()
		}
	}
	if canonical["type"] != "service_account" || strings.TrimSpace(canonical["project_id"]) == "" {
		return invalid()
	}
	email := canonical["client_email"]
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || address.Name != "" || strings.ContainsAny(email, "\r\n\t ") {
		return invalid()
	}
	if endpoint, present := canonical["token_uri"]; present && endpoint != "https://oauth2.googleapis.com/token" {
		return invalid()
	}
	if universe, present := canonical["universe_domain"]; present && universe != "googleapis.com" {
		return invalid()
	}
	if !validRSAKey([]byte(canonical["private_key"])) {
		return invalid()
	}
	out, err := json.Marshal(canonical)
	if err != nil {
		return invalid()
	}
	return out, ServiceAccountMetadata{Email: email, ProjectID: canonical["project_id"]}, nil
}

// uniqueJSONValue checks every object, including discarded metadata, before
// decoding into maps could overwrite duplicate keys.
func uniqueJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 128 {
		return validationError()
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return validationError()
			}
			seen[name] = true
			if err := uniqueJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return validationError()
	}
	_, err = decoder.Token()
	return err
}

func validRSAKey(data []byte) bool {
	block, rest := pem.Decode(data)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 || len(block.Headers) != 0 {
		return false
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return false
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return false
		}
	case "RSA PRIVATE KEY":
		var err error
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return false
		}
	default:
		return false
	}
	return key.Validate() == nil
}

func validationError() error {
	return apperr.New(apperr.CodeValidation, "common.validation", "invalid cloud credential configuration")
}
