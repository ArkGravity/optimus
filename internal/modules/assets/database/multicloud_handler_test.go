package database

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

func TestHandlerListsNativeMetadataAndNullableFacts(t *testing.T) {
	native := fromProjection(query.Database{Base: query.Base{Provider: "gcp", AssetKey: "gcp:database:42", NativeID: "123", ProjectID: "project"}, Endpoints: []resource.DatabaseEndpoint{{Address: "10.0.0.8", Type: "PRIVATE"}}})
	storage := int32(100)
	aws := Summary{Provider: "aws", MultiAZ: true, PubliclyAccessible: false, StorageGB: &storage, Endpoint: "orders.internal"}
	repo := &stubRepository{items: []Summary{native, aws}, total: 2}

	router := gin.New()
	router.GET("/databases", NewHandler(NewService(repo)).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/databases?provider=gcp&project_id=project&zone=us-central1-a", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Items) != 2 {
		t.Fatalf("body=%s", recorder.Body.String())
	}
	nativeJSON, awsJSON := envelope.Data.Items[0], envelope.Data.Items[1]
	if nativeJSON["provider"] != "gcp" || nativeJSON["project_id"] != "project" || nativeJSON["native_id"] != "123" {
		t.Fatalf("metadata=%#v", nativeJSON)
	}
	for _, key := range []string{"multi_az", "publicly_accessible", "storage_gb", "ha_mode"} {
		if nativeJSON[key] != nil {
			t.Fatalf("unknown %s=%#v", key, nativeJSON[key])
		}
	}
	if awsJSON["multi_az"] != true || awsJSON["publicly_accessible"] != false || awsJSON["storage_gb"] != float64(100) || awsJSON["endpoint"] != "orders.internal" {
		t.Fatalf("AWS aliases=%#v", awsJSON)
	}
	f := repo.filter
	if f.Provider != "gcp" || f.ProjectID != "project" || f.Zone != "us-central1-a" {
		t.Fatalf("filter=%#v", f)
	}
}
