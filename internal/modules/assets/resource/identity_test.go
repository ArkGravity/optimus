package resource

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAssetKey(t *testing.T) {
	key, err := ParseAssetKey("gcp:database:42", "database")
	require.NoError(t, err)
	require.Equal(t, AssetKey{Provider: "gcp", Family: "database", ID: 42}, key)
	legacy, err := ParseAssetKey("42", "vpc")
	require.NoError(t, err)
	require.Equal(t, AssetKey{Provider: "aws", Family: "vpc", ID: 42}, legacy)
	for _, raw := range []string{
		"gcp:instance:42", "azure:database:42", "gcp:database:0",
		"gcp:database:9223372036854775808", "gcp:database:18446744073709551616",
		"", "0", "042", "-42", "+42", " 42", "42 ", "4.2", "４２",
		"gcp:database:042", "gcp:database:+42", "gcp:database:-42",
		"gcp:database: 42", "gcp:database:42 ", "gcp:database:",
		"gcp:42", "gcp::42", ":database:42", "gcp:database:42:1",
		"GCP:database:42", "gcp:Database:42", "gcp/database/42",
	} {
		t.Run(raw, func(t *testing.T) {
			key, err := ParseAssetKey(raw, "database")
			require.Error(t, err)
			require.Equal(t, AssetKey{}, key)
		})
	}
	for _, family := range []string{"", "security_group", "Database", "database "} {
		_, err := ParseAssetKey("42", family)
		require.Error(t, err, family)
	}
}

func TestAssetKeyRoundTrip(t *testing.T) {
	seen := make(map[string]bool)
	for _, provider := range []string{"aws", "aliyun", "gcp"} {
		for _, family := range []string{"instance", "vpc", "subnet", "database", "firewall"} {
			for _, id := range []uint64{1, 42, math.MaxInt64} {
				key := AssetKey{Provider: provider, Family: family, ID: id}
				raw := key.String()
				require.False(t, seen[raw], "duplicate asset key %s", raw)
				seen[raw] = true
				parsed, err := ParseAssetKey(raw, family)
				require.NoError(t, err)
				require.Equal(t, key, parsed)
			}
		}
	}
	legacy, err := ParseAssetKey("42", "vpc")
	require.NoError(t, err)
	require.Equal(t, "aws:vpc:42", legacy.String())
}
