package cloudkey

import "strings"

const (
	AuthTypeAccessKey          = "access_key"
	AuthTypeServiceAccountJSON = "service_account_json"
	AuthTypeLegacyInvalid      = "legacy_invalid"
)

// ResolveAuthType accepts only the explicit provider authentication matrix.
// LegacyInvalid is reserved for migrated records and is never a request type.
func ResolveAuthType(provider, requested string) (string, error) {
	var expected string
	switch provider {
	case "aws", "aliyun":
		expected = AuthTypeAccessKey
	case "gcp":
		expected = AuthTypeServiceAccountJSON
	default:
		return "", validationError()
	}
	if requested != "" && requested != expected {
		return "", validationError()
	}
	return expected, nil
}

type secretInput struct {
	accessKeyID, secretAccessKey, serviceAccountJSON *string
}

// secretFields seals replacements and explicitly clears stale fields when the
// authentication shape changes. Metadata-only updates return an empty map.
func (s *Service) secretFields(authType string, input secretInput, requireFull bool) (map[string]any, []string, error) {
	fields := map[string]any{}
	var changed []string
	switch authType {
	case AuthTypeLegacyInvalid:
		if input.accessKeyID != nil || input.secretAccessKey != nil || input.serviceAccountJSON != nil {
			return nil, nil, validationError()
		}
	case AuthTypeAccessKey:
		if input.serviceAccountJSON != nil || (requireFull && (!nonempty(input.accessKeyID) || !nonempty(input.secretAccessKey))) {
			return nil, nil, validationError()
		}
		for _, field := range []struct {
			name, column string
			value        *string
		}{
			{"access_key_id", "access_key_id_enc", input.accessKeyID},
			{"secret_access_key", "secret_access_key_enc", input.secretAccessKey},
		} {
			if field.value == nil {
				continue
			}
			if !nonempty(field.value) {
				return nil, nil, validationError()
			}
			enc, err := s.seal([]byte(*field.value))
			if err != nil {
				return nil, nil, err
			}
			fields[field.column] = enc
			changed = append(changed, field.name)
		}
		if requireFull {
			fields["service_account_json_enc"] = nil
			fields["service_account_email"] = ""
			fields["credential_project_id"] = ""
		}
	case AuthTypeServiceAccountJSON:
		if input.accessKeyID != nil || input.secretAccessKey != nil || (requireFull && input.serviceAccountJSON == nil) {
			return nil, nil, validationError()
		}
		if input.serviceAccountJSON != nil {
			canonical, metadata, err := ValidateServiceAccountJSON([]byte(*input.serviceAccountJSON))
			if err != nil {
				return nil, nil, err
			}
			defer clear(canonical)
			enc, err := s.seal(canonical)
			if err != nil {
				return nil, nil, err
			}
			fields["service_account_json_enc"] = enc
			fields["service_account_email"] = metadata.Email
			fields["credential_project_id"] = metadata.ProjectID
			fields["access_key_id_enc"] = nil
			fields["secret_access_key_enc"] = nil
			changed = append(changed, "service_account_json")
		}
	default:
		return nil, nil, validationError()
	}
	return fields, changed, nil
}

func nonempty(value *string) bool { return value != nil && strings.TrimSpace(*value) != "" }

func optionalSecret(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
