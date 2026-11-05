package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAliyunNativeDetailsPreserveTypedFactsAndDiscardUnknownKeys(t *testing.T) {
	var db DatabaseNativeDetails
	require.NoError(t, json.Unmarshal([]byte(`{"cpu_count":8,"memory_mb":8589934592,"category":"HighAvailability","instance_type":"Primary","storage_type":"cloud_essd","password":"discard-me"}`), &db))
	raw, err := json.Marshal(db)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"memory_mb":8589934592`)
	require.Contains(t, string(raw), `"cpu_count":8`)
	require.NotContains(t, string(raw), "discard-me")
	var fw FirewallNativeDetails
	require.NoError(t, json.Unmarshal([]byte(`{"aliyun_outbound_rule_facts":[{"rule_index":0,"rule_id":"sgr-a","source_group_id":"sg-src","destination_group_id":"sg-dst","source_prefix_list_id":"pl-src","destination_prefix_list_id":"pl-dst","destination_group_owner_account":"owner","source_port_range":"1/2","port_range_list_id":"prl-a","port_range_list_name":"service ports","password":"discard-me"}],"token":"discard-me"}`), &fw))
	raw, err = json.Marshal(fw)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"source_group_id":"sg-src"`)
	require.Contains(t, string(raw), `"destination_group_id":"sg-dst"`)
	require.Contains(t, string(raw), `"source_prefix_list_id":"pl-src"`)
	require.Contains(t, string(raw), `"destination_prefix_list_id":"pl-dst"`)
	require.Contains(t, string(raw), `"port_range_list_id":"prl-a"`)
	require.Contains(t, string(raw), `"port_range_list_name":"service ports"`)
	require.NotContains(t, string(raw), "discard-me")
}
