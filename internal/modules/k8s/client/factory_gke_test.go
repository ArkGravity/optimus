package client_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ArkGravity/optimus/internal/infra/gke"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/internal/modules/k8s/client"
	"github.com/stretchr/testify/require"
)

type gkeConsumer struct {
	*fakeConsumer
	cloudID uint64
	purpose string
	err     error
}

func (c *gkeConsumer) GetKubeconfig(context.Context, uint64, string) (*credentials.Kubeconfig, error) {
	id := uint64(17)
	return &credentials.Kubeconfig{AuthType: "gke_service_account", CloudKeyID: &id, GKE: &gke.Config{Project: "fixture-project", Location: "us-central1", Cluster: "fixture", Endpoint: "private"}}, nil
}
func (c *gkeConsumer) GetCloudKey(_ context.Context, id uint64, purpose string) (*credentials.CloudKey, error) {
	c.cloudID = id
	c.purpose = purpose
	return nil, c.err
}

func TestKubernetesGKEUsesBoundCloudKeyConsumer(t *testing.T) {
	want := errors.New("fixture credential unavailable")
	consumer := &gkeConsumer{fakeConsumer: &fakeConsumer{}, err: want}
	factory := client.NewFactory(consumer, &fakeRepo{meta: &client.ClusterMeta{KubeconfigID: 9, Context: "gke"}})
	_, err := factory.RestConfig(t.Context(), 1, "k8s.storage.list")
	require.ErrorIs(t, err, want)
	require.EqualValues(t, 17, consumer.cloudID)
	require.Equal(t, "k8s.storage.list.gke", consumer.purpose)
}
