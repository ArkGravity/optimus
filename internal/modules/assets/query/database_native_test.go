package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGCPDatabaseProvenanceIsTypedAndAllowlisted(t *testing.T) {
	var details DatabaseNativeDetails
	require.NoError(t, json.Unmarshal([]byte(`{"gcp_ip_addresses":[{"address":"203.0.113.9","type":"OUTGOING","password":"secret"}],"gcp_dns_names":[{"name":"psc.example.test","connection_type":"PRIVATE_SERVICE_CONNECT","dns_scope":"INSTANCE","record_manager":"CUSTOMER","private_key":"secret"}],"dns_name":"sql.example.test","connection_name":"project:region:instance","psc_service_attachment_link":"projects/project/regions/region/serviceAttachments/sql","sql_network_architecture":"NEW_NETWORK_ARCHITECTURE","database_installed_version":"POSTGRES_16_3","settings":{"password":"secret"},"root_password":"secret"}`), &details))
	raw, err := json.Marshal(details)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"type":"OUTGOING"`)
	require.Contains(t, string(raw), `"record_manager":"CUSTOMER"`)
	require.Contains(t, string(raw), `"database_installed_version":"POSTGRES_16_3"`)
	require.NotContains(t, string(raw), "secret")
	require.NotContains(t, string(raw), "password")
	require.NotContains(t, string(raw), "settings")
}
