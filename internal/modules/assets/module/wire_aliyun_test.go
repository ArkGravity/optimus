package module

import (
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	aliyunprovider "github.com/ArkGravity/optimus/internal/modules/assets/providers/aliyun"
	awsprovider "github.com/ArkGravity/optimus/internal/modules/assets/providers/aws"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/stretchr/testify/require"
)

func TestDefaultCompositionRegistersAliyunAndPreservesOverrides(t *testing.T) {
	registry := providerRegistry(Input{}, time.Second)
	aws, err := registry.Lookup("aws")
	require.NoError(t, err)
	require.IsType(t, &awsprovider.Provider{}, aws)
	aliyun, err := registry.Lookup("aliyun")
	require.NoError(t, err)
	require.IsType(t, &aliyunprovider.Provider{}, aliyun)
	custom, err := assetsync.NewRegistry(awsprovider.New(awsprovider.Options{}))
	require.NoError(t, err)
	require.Same(t, custom, providerRegistry(Input{ProviderRegistry: custom}, time.Second))
	catalogs := regionCatalogs(Input{}, time.Second)
	require.IsType(t, aliyunprovider.RegionCatalog{}, catalogs["aliyun"])
	override := &awsprovider.RegionCatalog{Timeout: 3 * time.Second}
	require.Equal(t, override, regionCatalogs(Input{RegionCatalogs: map[string]account.RegionCatalog{"aliyun": override}}, time.Second)["aliyun"])
}
