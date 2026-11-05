package module

import (
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	gcpprovider "github.com/ArkGravity/optimus/internal/modules/assets/providers/gcp"
	"github.com/stretchr/testify/require"
)

func TestDefaultCompositionRegistersGCPAndPreservesCatalogOverride(t *testing.T) {
	provider, err := providerRegistry(Input{}, time.Second).Lookup("gcp")
	require.NoError(t, err)
	require.IsType(t, &gcpprovider.Provider{}, provider)
	catalogs := regionCatalogs(Input{}, time.Second)
	require.IsType(t, gcpprovider.RegionCatalog{}, catalogs["gcp"])
	override := &gcpprovider.RegionCatalog{Timeout: 3 * time.Second}
	require.Same(t, override, regionCatalogs(Input{RegionCatalogs: map[string]account.RegionCatalog{"gcp": override}}, time.Second)["gcp"])
}
