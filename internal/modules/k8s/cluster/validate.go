package cluster

import (
	"fmt"

	"k8s.io/client-go/tools/clientcmd"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/gke"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

// ValidateCredential checks the explicit connection mode before any I/O.
func ValidateCredential(kc *credentials.Kubeconfig, contextName string) error {
	if kc == nil {
		return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.invalid", "invalid Kubernetes credential")
	}
	if kc.AuthType == "gke_service_account" {
		if kc.GKE == nil || !kc.GKE.Valid() || kc.CloudKeyID == nil || *kc.CloudKeyID == 0 || len(kc.YAML) != 0 {
			return apperr.New(apperr.CodeValidation, "k8s.gke.invalid_config", "invalid GKE connection configuration")
		}
		if contextName != gke.Context {
			return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.context_not_found", "GKE connections use the gke context")
		}
		return nil
	}
	if (kc.AuthType != "" && kc.AuthType != "kubeconfig") || kc.GKE != nil || kc.CloudKeyID != nil {
		return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.invalid", "invalid Kubernetes credential")
	}
	return ValidateContextAndAuth(kc.YAML, contextName)
}

// ValidateContextAndAuth parses a kubeconfig YAML and rejects it if:
//   - it fails to parse;
//   - any AuthInfo uses an exec plugin or an auth-provider plugin;
//   - the requested context name is not present.
//
// Promoted to exported in Task 7 so client.Factory can reuse it as
// defense-in-depth at run time (same check on every per-request rebuild).
func ValidateContextAndAuth(raw []byte, contextName string) error {
	apiCfg, err := clientcmd.Load(raw)
	if err != nil {
		return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.invalid", err.Error())
	}
	for name, u := range apiCfg.AuthInfos {
		if u == nil {
			continue
		}
		if u.Exec != nil {
			return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.exec_forbidden",
				fmt.Sprintf("user %q uses exec auth plugin", name))
		}
		if u.AuthProvider != nil {
			return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.authprovider_forbidden",
				fmt.Sprintf("user %q uses auth-provider plugin", name))
		}
	}
	if _, ok := apiCfg.Contexts[contextName]; !ok {
		return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.context_not_found",
			fmt.Sprintf("context %q not found", contextName))
	}
	return nil
}
