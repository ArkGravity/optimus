package aliyun

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/stretchr/testify/require"
)

const dbList = `{"PageNumber":1,"PageRecordCount":1,"TotalRecordCount":2,"Items":{"DBInstance":[{"DBInstanceId":"rm-a","RegionId":"cn-hangzhou"}]}}`
const dbList2 = `{"PageNumber":2,"PageRecordCount":1,"TotalRecordCount":2,"Items":{"DBInstance":[{"DBInstanceId":"rm-b"}]}}`
const dbAttribute = `{"Items":{"DBInstanceAttribute":[{"DBInstanceId":"rm-a","DBInstanceDescription":"db name","Engine":"MySQL","EngineVersion":"8.0","DBInstanceClass":"unknown.large","DBInstanceStatus":"Running","RegionId":"cn-hangzhou","ZoneId":"cn-hangzhou-b","DBInstanceCPU":"8","DBInstanceMemory":8589934592,"DBInstanceStorage":500,"Category":"HighAvailability","DBInstanceType":"Primary","DBInstanceStorageType":"cloud_essd","Password":"discard-me"}]}}`
const dbNet = `{"DBInstanceNetInfos":{"DBInstanceNetInfo":[{"ConnectionString":"internal.example","Port":"3306","IPType":"Private"},{"ConnectionString":"public.example","Port":"3306","IPType":"Public"},{"IPAddress":"2001:db8::3","IPType":"Private"}]}}`

func databaseFixture(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) http.RoundTripper {
	return fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if override != nil && override(w, r) {
			return
		}
		switch r.Header.Get("x-acs-action") {
		case "GetCallerIdentity":
			_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
		case "DescribeDBInstances":
			if r.FormValue("PageSize") != "100" {
				t.Errorf("RDS page bound %s", r.FormValue("PageSize"))
			}
			payload := dbList
			if r.FormValue("PageNumber") == "2" {
				payload = dbList2
			}
			_, _ = w.Write([]byte(payload))
		case "DescribeDBInstanceAttribute":
			payload := dbAttribute
			if r.FormValue("DBInstanceId") == "rm-b" {
				payload = `{"Items":{"DBInstanceAttribute":[{"DBInstanceId":"rm-b","Engine":"PostgreSQL","EngineVersion":"16"}]}}`
			}
			_, _ = w.Write([]byte(payload))
		case "DescribeDBInstanceNetInfo":
			_, _ = w.Write([]byte(dbNet))
		default:
			t.Errorf("unexpected SDK action %s", r.Header.Get("x-acs-action"))
			http.Error(w, "unexpected", 500)
		}
	})
}

func TestDatabaseSnapshotRequiresEveryDetailAndPreservesNativeFacts(t *testing.T) {
	p := New(Options{Transport: databaseFixture(t, nil)})
	snapshot, err := p.Fetch(context.Background(), fixtureUnit("database"), fixtureKey())
	require.NoError(t, err)
	require.True(t, snapshot.Complete)
	require.NotNil(t, snapshot.Publish)
	rows, err := p.databases(context.Background(), fixtureKey(), "cn-hangzhou")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "MySQL", rows[0].Engine)
	require.Equal(t, "rds", rows[0].Product)
	require.Equal(t, "db name", rows[0].Name)
	require.EqualValues(t, 500, *rows[0].StorageGB)
	require.Equal(t, "HighAvailability", *rows[0].HAMode)
	require.True(t, *rows[0].PubliclyAccessible)
	require.Nil(t, rows[1].StorageGB)
	require.Nil(t, rows[1].HAMode)
	require.Contains(t, string(rows[0].NativeDetails), `"memory_mb":8589934592`)
	require.NotContains(t, string(rows[0].NativeDetails), "discard-me")
	var endpoints []resource.DatabaseEndpoint
	require.NoError(t, json.Unmarshal(rows[0].Endpoints, &endpoints))
	require.Len(t, endpoints, 3)
	require.Equal(t, "Private", endpoints[0].Type)
	require.EqualValues(t, 3306, *endpoints[0].Port)
	require.Nil(t, endpoints[2].Port)
	require.Equal(t, "2001:db8::3", endpoints[2].Address)
}

func TestDatabasesRejectIncompleteListOrRequiredDetails(t *testing.T) {
	for _, mode := range []string{"attribute-failed", "net-failed", "attribute-missing-id", "attribute-wrong-id", "duplicate-id", "repeated-page", "changed-total", "truncated", "unexpected-token", "malformed", "invalid-port"} {
		t.Run(mode, func(t *testing.T) {
			p := New(Options{Transport: databaseFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				action := r.Header.Get("x-acs-action")
				payload := ""
				if (mode == "attribute-failed" && action == "DescribeDBInstanceAttribute") || (mode == "net-failed" && action == "DescribeDBInstanceNetInfo") {
					w.WriteHeader(403)
					_, _ = w.Write([]byte(`{"Code":"Forbidden.RAM","Message":"synthetic-secret"}`))
					return true
				}
				if action == "DescribeDBInstanceAttribute" {
					switch mode {
					case "attribute-missing-id":
						payload = strings.Replace(dbAttribute, `"DBInstanceId":"rm-a",`, "", 1)
					case "attribute-wrong-id":
						payload = strings.Replace(dbAttribute, "rm-a", "other", 1)
					}
				}
				if action == "DescribeDBInstanceNetInfo" && mode == "invalid-port" {
					payload = strings.Replace(dbNet, `"3306"`, `"invalid"`, 1)
				}
				if action == "DescribeDBInstances" && r.FormValue("PageNumber") == "2" {
					switch mode {
					case "duplicate-id":
						payload = strings.Replace(dbList2, "rm-b", "rm-a", 1)
					case "repeated-page":
						payload = dbList
					case "changed-total":
						payload = strings.Replace(dbList2, `"TotalRecordCount":2`, `"TotalRecordCount":3`, 1)
					case "truncated":
						payload = `{"PageNumber":2,"PageRecordCount":1,"TotalRecordCount":2,"Items":{"DBInstance":[]}}`
					case "unexpected-token":
						payload = strings.Replace(dbList2, `"PageNumber":2`, `"PageNumber":2,"NextToken":"unexpected"`, 1)
					case "malformed":
						payload = `{`
					}
				}
				if payload == "" {
					return false
				}
				_, _ = w.Write([]byte(payload))
				return true
			})})
			snapshot, err := p.Fetch(context.Background(), fixtureUnit("database"), fixtureKey())
			require.Error(t, err)
			require.False(t, snapshot.Complete)
			require.Nil(t, snapshot.Publish)
			require.NotContains(t, err.Error(), "synthetic-secret")
		})
	}
}

func TestDatabaseClassicInternalEndpointAndUnknownTypesStayNative(t *testing.T) {
	for _, tc := range []struct {
		native string
		known  bool
		public bool
	}{{"Inner", true, false}, {"Private", true, false}, {"Public", true, true}, {"future-type", false, false}} {
		t.Run(tc.native, func(t *testing.T) {
			p := New(Options{Transport: databaseFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("x-acs-action") != "DescribeDBInstanceNetInfo" {
					return false
				}
				_, _ = w.Write([]byte(`{"DBInstanceNetInfos":{"DBInstanceNetInfo":[{"ConnectionString":"native.example","IPType":"` + tc.native + `"}]}}`))
				return true
			})})
			rows, err := p.databases(context.Background(), fixtureKey(), "cn-hangzhou")
			require.NoError(t, err)
			var endpoints []resource.DatabaseEndpoint
			require.NoError(t, json.Unmarshal(rows[0].Endpoints, &endpoints))
			require.Equal(t, tc.native, endpoints[0].Type)
			require.Nil(t, endpoints[0].Port)
			if tc.known {
				require.NotNil(t, rows[0].PubliclyAccessible)
				require.Equal(t, tc.public, *rows[0].PubliclyAccessible)
			} else {
				require.Nil(t, rows[0].PubliclyAccessible)
			}
		})
	}
}
