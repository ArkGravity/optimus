//go:build dbtest

package kubeconfig_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/gke"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
	"github.com/ArkGravity/optimus/internal/modules/credentials/kubeconfig"
)

func gkeRequest(id uint64) kubeconfig.CreateRequest {
	return kubeconfig.CreateRequest{Name: "GKE fixture", AuthType: kubeconfig.AuthGKE, CloudKeyID: &id,
		GKE: &gke.Config{Project: "fixture-project", Location: "us-central1", Cluster: "fixture", Endpoint: "default"}}
}

func insertGCPKey(t *testing.T, svc *kubeconfig.Service, name string) uint64 {
	t.Helper()
	key := models.CredentialCloudKey{Name: name, Provider: "gcp", AuthType: "service_account_json", ServiceAccountJSONEnc: []byte("SEALfixture-secret")}
	require.NoError(t, svc.Repo().DB().Create(&key).Error)
	return key.ID
}

func TestGKECredentialBindingMetadataAndRebinding(t *testing.T) {
	svc, cleanup := newSvc(t)
	t.Cleanup(cleanup)
	ctx := t.Context()
	id := insertGCPKey(t, svc, "GCP fixture")
	detail, err := svc.Create(ctx, 0, "", "", gkeRequest(id))
	require.NoError(t, err)
	require.Equal(t, "gke", detail.DefaultContext)
	require.Equal(t, id, *detail.CloudKeyID)
	require.Equal(t, "fixture-project", detail.GKE.Project)
	row, err := svc.Repo().Get(ctx, detail.ID)
	require.NoError(t, err)
	before := append([]byte(nil), row.KubeconfigEnc...)
	description := "metadata only"
	detail, err = svc.Update(ctx, 0, "", "", detail.ID, kubeconfig.UpdateRequest{Description: &description})
	require.NoError(t, err)
	row, err = svc.Repo().Get(ctx, detail.ID)
	require.NoError(t, err)
	require.Equal(t, before, row.KubeconfigEnc)
	consume, err := svc.Consume(ctx, nil, detail.ID, "system:test.gke")
	require.NoError(t, err)
	require.Empty(t, consume.YAML)
	require.Equal(t, id, *consume.CloudKeyID)
	list, err := svc.List(ctx, kubeconfig.ListQuery{})
	require.NoError(t, err)
	encoded, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "fixture-secret")
	other := *detail.GKE
	other.Cluster = "other"
	_, err = svc.Update(ctx, 0, "", "", detail.ID, kubeconfig.UpdateRequest{GKE: &other})
	require.Error(t, err)
	static := kubeconfig.AuthKubeconfig
	_, err = svc.Update(ctx, 0, "", "", detail.ID, kubeconfig.UpdateRequest{AuthType: &static})
	require.Error(t, err)
	key2 := insertGCPKey(t, svc, "replacement GCP fixture")
	detail, err = svc.Update(ctx, 0, "", "", detail.ID, kubeconfig.UpdateRequest{CloudKeyID: &key2})
	require.NoError(t, err)
	require.Equal(t, key2, *detail.CloudKeyID)
	cloud := cloudkey.NewService(cloudkey.NewRepo(svc.Repo().DB()), passthroughCipher{}, audit.NewRecorder(svc.Repo().DB()))
	require.NoError(t, cloud.Delete(ctx, 0, "", "", id))
	err = cloud.Delete(ctx, 0, "", "", key2)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.cloud_key.in_use_by_kubeconfigs", be.MessageKey)
	provider, ak, sk := "aws", "fixture-ak", "fixture-sk"
	_, err = cloud.Update(ctx, 0, "", "", key2, cloudkey.UpdateRequest{Provider: &provider, AccessKeyID: &ak, SecretAccessKey: &sk})
	be, ok = apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.cloud_key.in_use_by_kubeconfigs", be.MessageKey)
	require.NoError(t, svc.Delete(ctx, 0, "", "", detail.ID))
	require.NoError(t, cloud.Delete(ctx, 0, "", "", key2))
}

func TestGKERejectsInvalidCredentialBindings(t *testing.T) {
	svc, cleanup := newSvc(t)
	t.Cleanup(cleanup)
	id := insertGCPKey(t, svc, "GCP fixture")
	aws := models.CredentialCloudKey{Name: "AWS fixture", Provider: "aws", AccessKeyIDEnc: []byte("ak"), SecretAccessKeyEnc: []byte("sk")}
	require.NoError(t, svc.Repo().DB().Create(&aws).Error)
	for name, change := range map[string]func(*kubeconfig.CreateRequest){
		"missing key":         func(r *kubeconfig.CreateRequest) { r.CloudKeyID = nil },
		"wrong provider":      func(r *kubeconfig.CreateRequest) { r.CloudKeyID = &aws.ID },
		"missing target":      func(r *kubeconfig.CreateRequest) { r.GKE = nil },
		"path injection":      func(r *kubeconfig.CreateRequest) { r.GKE.Project = "../other" },
		"supplied kubeconfig": func(r *kubeconfig.CreateRequest) { r.Kubeconfig = validKubeconfigYAML },
		"wrong mode": func(r *kubeconfig.CreateRequest) {
			r.AuthType = kubeconfig.AuthKubeconfig
			r.Kubeconfig = validKubeconfigYAML
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := gkeRequest(id)
			change(&req)
			_, err := svc.Create(t.Context(), 0, "", "", req)
			require.Error(t, err)
		})
	}
	standard, err := svc.Create(t.Context(), 0, "", "", kubeconfig.CreateRequest{Name: "static", Kubeconfig: validKubeconfigYAML})
	require.NoError(t, err)
	_, err = svc.Update(t.Context(), 0, "", "", standard.ID, kubeconfig.UpdateRequest{CloudKeyID: &id})
	require.Error(t, err)
	consume, err := svc.Consume(t.Context(), nil, standard.ID, "system:test.static")
	require.NoError(t, err)
	require.Equal(t, validKubeconfigYAML, string(consume.YAML))
}

func TestGKEBindingAndKeyDeletionSerialize(t *testing.T) {
	svc, cleanup := newSvc(t)
	t.Cleanup(cleanup)
	cloud := cloudkey.NewService(cloudkey.NewRepo(svc.Repo().DB()), passthroughCipher{}, audit.NewRecorder(svc.Repo().DB()))
	for i := range 10 {
		id := insertGCPKey(t, svc, fmt.Sprintf("race key %d", i))
		req := gkeRequest(id)
		req.Name = fmt.Sprintf("race credential %d", i)
		start := make(chan struct{})
		created := make(chan error, 1)
		deleted := make(chan error, 1)
		go func() { <-start; _, err := svc.Create(t.Context(), 0, "", "", req); created <- err }()
		go func() { <-start; deleted <- cloud.Delete(t.Context(), 0, "", "", id) }()
		close(start)
		createErr, deleteErr := <-created, <-deleted
		require.NotEqual(t, createErr == nil, deleteErr == nil, "exactly one operation must win")
		if createErr != nil {
			be, ok := apperr.AsBiz(createErr)
			require.True(t, ok)
			require.Equal(t, "credentials.kubeconfig.invalid_gke", be.MessageKey)
		}
		if deleteErr != nil {
			be, ok := apperr.AsBiz(deleteErr)
			require.True(t, ok)
			require.Equal(t, "credentials.cloud_key.in_use_by_kubeconfigs", be.MessageKey)
		}
	}
}
