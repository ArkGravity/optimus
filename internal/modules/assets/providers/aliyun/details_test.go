package aliyun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/stretchr/testify/require"
)

func TestDetailRequestsBoundConcurrencyAndDrainAfterCancellation(t *testing.T) {
	for _, kind := range []string{"database", "security_group"} {
		for _, canceled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/canceled=%t", kind, canceled), func(t *testing.T) {
				var active, peak, finished atomic.Int32
				started := make(chan struct{}, 1)
				transport := fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
					_ = r.ParseForm()
					w.Header().Set("Content-Type", "application/json")
					switch r.Header.Get("x-acs-action") {
					case "GetCallerIdentity":
						_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
						return
					case "DescribeDBInstances":
						items := []map[string]string{}
						for i := 0; i < 9; i++ {
							items = append(items, map[string]string{"DBInstanceId": fmt.Sprintf("rm-%d", i)})
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"PageNumber": 1, "PageRecordCount": 9, "TotalRecordCount": 9, "Items": map[string]any{"DBInstance": items}})
						return
					case "DescribeSecurityGroups":
						items := []map[string]string{}
						for i := 0; i < 9; i++ {
							items = append(items, map[string]string{"SecurityGroupId": fmt.Sprintf("sg-%d", i), "VpcId": "vpc-a"})
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"PageNumber": 1, "PageSize": 100, "TotalCount": 9, "SecurityGroups": map[string]any{"SecurityGroup": items}})
						return
					}
					now := active.Add(1)
					defer active.Add(-1)
					for old := peak.Load(); now > old; old = peak.Load() {
						if peak.CompareAndSwap(old, now) {
							break
						}
					}
					if now >= 4 {
						select {
						case started <- struct{}{}:
						default:
						}
					}
					if canceled {
						<-r.Context().Done()
						return
					}
					time.Sleep(10 * time.Millisecond)
					finished.Add(1)
					switch r.Header.Get("x-acs-action") {
					case "DescribeDBInstanceAttribute":
						_ = json.NewEncoder(w).Encode(map[string]any{"Items": map[string]any{"DBInstanceAttribute": []any{map[string]any{"DBInstanceId": r.FormValue("DBInstanceId"), "Engine": "PostgreSQL"}}}})
					case "DescribeDBInstanceNetInfo":
						_, _ = w.Write([]byte(dbNet))
					case "DescribeSecurityGroupAttribute":
						_ = json.NewEncoder(w).Encode(map[string]any{"SecurityGroupId": r.FormValue("SecurityGroupId"), "Permissions": map[string]any{"Permission": []any{}}})
					default:
						t.Errorf("unexpected detail action %s", r.Header.Get("x-acs-action"))
					}
				})
				p := New(Options{Transport: transport, RequestTimeout: time.Second})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				type result struct {
					snapshot assetsync.Snapshot
					err      error
				}
				done := make(chan result, 1)
				go func() { snapshot, err := p.Fetch(ctx, fixtureUnit(kind), fixtureKey()); done <- result{snapshot, err} }()
				if canceled {
					select {
					case <-started:
						cancel()
					case <-time.After(2 * time.Second):
						t.Fatal("details did not start")
					}
				}
				select {
				case result := <-done:
					if canceled {
						require.ErrorIs(t, result.err, context.Canceled)
						require.False(t, result.snapshot.Complete)
						require.Nil(t, result.snapshot.Publish)
					} else {
						require.NoError(t, result.err)
						require.True(t, result.snapshot.Complete)
						expected := int32(9)
						if kind == "database" {
							expected = 18
						}
						require.Equal(t, expected, finished.Load())
					}
				case <-time.After(3 * time.Second):
					t.Fatal("collector did not finish within request budget")
				}
				require.LessOrEqual(t, peak.Load(), int32(4))
				require.Equal(t, int32(4), peak.Load())
				require.Eventually(t, func() bool { return active.Load() == 0 }, time.Second, time.Millisecond, "detail handlers did not drain")
			})
		}
	}
}
