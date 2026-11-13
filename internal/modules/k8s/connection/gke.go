// Package connection shares native GKE authentication between Kubernetes and Helm.
package connection

import (
	"context"

	"k8s.io/client-go/rest"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/gke"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/internal/modules/k8s/cluster"
)

func GKEConfig(ctx context.Context, consumer credentials.Consumer, kc *credentials.Kubeconfig, contextName, purpose string) (*rest.Config, error) {
	if err := cluster.ValidateCredential(kc, contextName); err != nil {
		return nil, err
	}
	if kc.AuthType != "gke_service_account" {
		return nil, invalid()
	}
	key, err := consumer.GetCloudKey(ctx, *kc.CloudKeyID, purpose+".gke")
	if err != nil {
		return nil, err
	}
	defer credentials.Wipe(key)
	if key == nil || key.Provider != "gcp" || key.AuthType != "service_account_json" || key.AccessKeyID != "" || key.SecretAccessKey != "" {
		return nil, invalid()
	}
	canonical, err := credentials.ValidateServiceAccountJSON(key.ServiceAccountJSON)
	if err != nil {
		return nil, invalid()
	}
	defer clear(canonical)
	return gke.RESTConfig(ctx, *kc.GKE, canonical, gke.Options{})
}

func invalid() error {
	return apperr.New(apperr.CodeValidation, "k8s.gke.invalid_config", "invalid GKE connection configuration")
}
