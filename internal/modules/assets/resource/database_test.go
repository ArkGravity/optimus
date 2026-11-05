package resource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatabaseEndpointJSONPreservesZeroAndUnknownPort(t *testing.T) {
	zero := int32(0)
	for _, port := range []*int32{nil, &zero} {
		original := DatabaseEndpoint{Address: "database.example.test", Port: port, Type: "PRIVATE"}
		data, err := json.Marshal(original)
		require.NoError(t, err)
		if port == nil {
			require.JSONEq(t, `{"address":"database.example.test","port":null,"type":"PRIVATE"}`, string(data))
		} else {
			require.JSONEq(t, `{"address":"database.example.test","port":0,"type":"PRIVATE"}`, string(data))
		}
		var decoded DatabaseEndpoint
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Equal(t, original, decoded)
	}
}
