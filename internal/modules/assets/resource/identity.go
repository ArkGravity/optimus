package resource

import (
	"errors"
	"strconv"
	"strings"
)

// AssetKey identifies a provider's local resource row within its public family.
type AssetKey struct {
	Provider string
	Family   string
	ID       uint64
}

// ParseAssetKey accepts canonical keys and legacy numeric AWS row IDs.
func ParseAssetKey(raw, family string) (AssetKey, error) {
	invalid := errors.New("invalid asset key")
	switch family {
	case "instance", "vpc", "subnet", "database", "firewall":
	default:
		return AssetKey{}, invalid
	}
	parts := strings.Split(raw, ":")
	if len(parts) == 1 {
		parts = []string{"aws", family, raw}
	}
	if len(parts) != 3 || parts[1] != family {
		return AssetKey{}, invalid
	}
	switch parts[0] {
	case "aws", "aliyun", "gcp":
	default:
		return AssetKey{}, invalid
	}
	id, err := strconv.ParseUint(parts[2], 10, 63)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != parts[2] {
		return AssetKey{}, invalid
	}
	return AssetKey{Provider: parts[0], Family: family, ID: id}, nil
}

func (k AssetKey) String() string {
	return k.Provider + ":" + k.Family + ":" + strconv.FormatUint(k.ID, 10)
}
