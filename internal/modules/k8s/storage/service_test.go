package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
)

type factory struct {
	client *fake.Clientset
	calls  int
	err    error
}

func (f *factory) Clientset(_ context.Context, id uint64, purpose string) (kubernetes.Interface, error) {
	if id != 7 || purpose != "k8s.storage.list" {
		return nil, errors.New("unexpected credential scope")
	}
	f.calls++
	return f.client, f.err
}

func TestStorageProjections(t *testing.T) {
	class := "fast"
	policy := corev1.PersistentVolumeReclaimRetain
	binding := storagev1.VolumeBindingWaitForFirstConsumer
	f := &factory{client: fake.NewSimpleClientset(
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "disk"}, Spec: corev1.PersistentVolumeSpec{
			Capacity:    corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("32Gi")},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, PersistentVolumeReclaimPolicy: policy,
			StorageClassName: class, ClaimRef: &corev1.ObjectReference{Namespace: "prod", Name: "data"},
		}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "prod"}, Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "disk", StorageClassName: &class, AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("32Gi")}}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "other"}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: class}, Provisioner: "example.csi", ReclaimPolicy: &policy, VolumeBindingMode: &binding},
	)}
	s := NewService(f)
	volumes, err := s.List(context.Background(), 7, "persistentvolumes", ListQuery{Namespace: "ignored"})
	require.NoError(t, err)
	require.Len(t, volumes.Items, 1)
	require.Equal(t, "32Gi", volumes.Items[0].Capacity)
	require.Equal(t, []string{"ReadWriteOnce"}, volumes.Items[0].AccessModes)
	require.Equal(t, "Retain", volumes.Items[0].ReclaimPolicy)
	require.Equal(t, "prod/data", volumes.Items[0].Claim)
	require.Equal(t, "Bound", volumes.Items[0].Status)
	claims, err := s.List(context.Background(), 7, "persistentvolumeclaims", ListQuery{Namespace: "prod"})
	require.NoError(t, err)
	require.Len(t, claims.Items, 1)
	require.Equal(t, "disk", claims.Items[0].Volume)
	require.Equal(t, class, claims.Items[0].StorageClass)
	require.Equal(t, "32Gi", claims.Items[0].Capacity)
	all, err := s.List(context.Background(), 7, "persistentvolumeclaims", ListQuery{})
	require.NoError(t, err)
	require.Len(t, all.Items, 2)
	for _, claim := range all.Items {
		if claim.Name == "pending" {
			require.Empty(t, claim.Capacity, "unknown capacity must not become zero")
			require.NotNil(t, claim.AccessModes)
		}
	}
	classes, err := s.List(context.Background(), 7, "storageclasses", ListQuery{})
	require.NoError(t, err)
	require.Equal(t, "example.csi", classes.Items[0].Provisioner)
	require.Equal(t, "WaitForFirstConsumer", classes.Items[0].BindingMode)
	require.Equal(t, 4, f.calls, "each list must construct a fresh client")
	for _, action := range f.client.Actions() {
		require.Equal(t, "list", action.GetVerb(), "storage never mutates Kubernetes")
	}
}

type clientFunc func(context.Context, uint64, string) (kubernetes.Interface, error)

func (f clientFunc) Clientset(ctx context.Context, id uint64, purpose string) (kubernetes.Interface, error) {
	return f(ctx, id, purpose)
}

func TestStorageBoundsContinuationAndErrors(t *testing.T) {
	for _, limit := range []int64{0, -1, 2001, 73} {
		t.Run(resource.NewQuantity(limit, resource.DecimalSI).String(), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/namespaces/prod/persistentvolumeclaims", r.URL.Path)
				want := int64(500)
				if limit == 73 {
					want = 73
				}
				require.Equal(t, fmt.Sprint(want), r.URL.Query().Get("limit"))
				require.Equal(t, "start", r.URL.Query().Get("continue"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"PersistentVolumeClaimList","metadata":{"continue":"next"},"items":[]}`))
			}))
			defer server.Close()
			clients := clientFunc(func(context.Context, uint64, string) (kubernetes.Interface, error) {
				return kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			})
			result, err := NewService(clients).List(context.Background(), 7, "persistentvolumeclaims", ListQuery{Namespace: "prod", Limit: limit, Continue: "start"})
			require.NoError(t, err)
			require.Equal(t, "next", result.Continue)
			require.True(t, result.Truncated)
			require.NotNil(t, result.Items)
		})
	}
	f := &factory{client: fake.NewSimpleClientset()}
	_, err := NewService(f).List(context.Background(), 7, "secrets", ListQuery{})
	require.Error(t, err)
	require.Zero(t, f.calls, "invalid kind must not read credentials")
	f.client.PrependReactor("list", "*", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "persistentvolumes"}, "disk", errors.New("private upstream detail"))
	})
	_, err = NewService(f).List(context.Background(), 7, "persistentvolumes", ListQuery{})
	var business *apperr.BizError
	require.ErrorAs(t, err, &business)
	require.NotContains(t, business.Message, "private upstream detail")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.err = ctx.Err()
	_, err = NewService(f).List(ctx, 7, "storageclasses", ListQuery{})
	require.ErrorIs(t, err, context.Canceled)
}
