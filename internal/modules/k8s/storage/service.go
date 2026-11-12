// Package storage provides read-only PV, PVC and StorageClass inventory.
package storage

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	k8serrs "github.com/ArkGravity/optimus/internal/modules/k8s/apierr"
)

type Clientsetter interface {
	Clientset(context.Context, uint64, string) (kubernetes.Interface, error)
}

type Service struct{ clients Clientsetter }

func NewService(clients Clientsetter) *Service { return &Service{clients: clients} }

func modes(values []corev1.PersistentVolumeAccessMode) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func storageError(err error) error {
	// Reuse the shared classification without exposing upstream response text.
	business, _ := apperr.AsBiz(k8serrs.MapAPIError(err))
	return apperr.Wrap(err, business.Code, business.MessageKey, "Kubernetes storage request failed")
}

func (s *Service) List(ctx context.Context, clusterID uint64, kind string, q ListQuery) (*ListResponse, error) {
	if kind != "persistentvolumes" && kind != "persistentvolumeclaims" && kind != "storageclasses" {
		return nil, apperr.New(apperr.CodeBadRequest, "k8s.storage.unsupported_kind", "unsupported storage kind")
	}
	cs, err := s.clients.Clientset(ctx, clusterID, "k8s.storage.list")
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	opts := metav1.ListOptions{Limit: limit, Continue: q.Continue}
	result := &ListResponse{Items: make([]Summary, 0)}
	switch kind {
	case "persistentvolumes":
		list, err := cs.CoreV1().PersistentVolumes().List(ctx, opts)
		if err != nil {
			return nil, storageError(err)
		}
		result.Continue = list.Continue
		for _, volume := range list.Items {
			capacity := volume.Spec.Capacity[corev1.ResourceStorage]
			claim := ""
			if volume.Spec.ClaimRef != nil {
				claim = volume.Spec.ClaimRef.Namespace + "/" + volume.Spec.ClaimRef.Name
			}
			result.Items = append(result.Items, Summary{
				Name: volume.Name, Capacity: capacity.String(), AccessModes: modes(volume.Spec.AccessModes),
				ReclaimPolicy: string(volume.Spec.PersistentVolumeReclaimPolicy), Status: string(volume.Status.Phase),
				Claim: claim, StorageClass: volume.Spec.StorageClassName, Age: volume.CreationTimestamp.Time,
			})
		}
	case "persistentvolumeclaims":
		list, err := cs.CoreV1().PersistentVolumeClaims(q.Namespace).List(ctx, opts)
		if err != nil {
			return nil, storageError(err)
		}
		result.Continue = list.Continue
		for _, claim := range list.Items {
			capacity := ""
			if quantity, ok := claim.Status.Capacity[corev1.ResourceStorage]; ok {
				capacity = quantity.String()
			}
			class := ""
			if claim.Spec.StorageClassName != nil {
				class = *claim.Spec.StorageClassName
			}
			result.Items = append(result.Items, Summary{
				Name: claim.Name, Namespace: claim.Namespace, Capacity: capacity, AccessModes: modes(claim.Spec.AccessModes),
				Status: string(claim.Status.Phase), Volume: claim.Spec.VolumeName, StorageClass: class, Age: claim.CreationTimestamp.Time,
			})
		}
	case "storageclasses":
		list, err := cs.StorageV1().StorageClasses().List(ctx, opts)
		if err != nil {
			return nil, storageError(err)
		}
		result.Continue = list.Continue
		for _, class := range list.Items {
			policy, binding := "", ""
			if class.ReclaimPolicy != nil {
				policy = string(*class.ReclaimPolicy)
			}
			if class.VolumeBindingMode != nil {
				binding = string(*class.VolumeBindingMode)
			}
			result.Items = append(result.Items, Summary{
				Name: class.Name, AccessModes: []string{}, Provisioner: class.Provisioner,
				ReclaimPolicy: policy, BindingMode: binding, Age: class.CreationTimestamp.Time,
			})
		}
	}
	result.Truncated = result.Continue != ""
	return result, nil
}
