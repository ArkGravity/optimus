package helmclient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ArkGravity/optimus/internal/infra/gke"
	"github.com/ArkGravity/optimus/internal/modules/apps/helmclient"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/internal/modules/k8s/cluster"
	"github.com/stretchr/testify/require"
)

type gkeConsumer struct {
	*fakeConsumer
	cloudID uint64
	purpose string
	err     error
}

func (c *gkeConsumer) GetCloudKey(_ context.Context, id uint64, purpose string) (*credentials.CloudKey, error) {
	c.cloudID = id
	c.purpose = purpose
	return nil, c.err
}

func TestHelmGKEUsesBoundCloudKeyConsumer(t *testing.T) {
	id := uint64(17)
	want := errors.New("fixture credential unavailable")
	consumer := &gkeConsumer{fakeConsumer: &fakeConsumer{kc: &credentials.Kubeconfig{AuthType: "gke_service_account", CloudKeyID: &id, GKE: &gke.Config{Project: "fixture-project", Location: "us-central1", Cluster: "fixture", Endpoint: "private"}}}, err: want}
	factory := helmclient.NewFactory(consumer, &fakeClusters{c: &cluster.Detail{KubeconfigID: 9, Context: "gke"}})
	_, err := factory.NewForCluster(t.Context(), 1, "fixture", "apps.release.install")
	require.ErrorIs(t, err, want)
	require.EqualValues(t, 17, consumer.cloudID)
	require.Equal(t, "system:apps.release.install.gke", consumer.purpose)
}
