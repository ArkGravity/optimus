package vpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
)

func TestHandlerListsNativeMetadataAndNullableFacts(t *testing.T) {
	native := fromProjection(query.VPC{Base: query.Base{Provider: "gcp", AssetKey: "gcp:vpc:42", NativeID: "123", ProjectID: "project"}, CIDRBlocks: []string{"10.0.0.0/16"}})
	aws := Summary{Provider: "aws", IsDefault: false}
	repo := &stubRepo{vpcs: []Summary{native, aws}, vpcTotal: 2}

	router := gin.New()
	router.GET("/vpcs", NewHandler(NewService(repo)).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/vpcs?provider=gcp&project_id=project&zone=us-central1-a", nil))
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
	if nativeJSON["is_default"] != nil || awsJSON["is_default"] != false {
		t.Fatalf("native=%#v aws=%#v", nativeJSON, awsJSON)
	}
	f := repo.listFilter
	if f.Provider != "gcp" || f.ProjectID != "project" || f.Zone != "us-central1-a" {
		t.Fatalf("filter=%#v", f)
	}
}
