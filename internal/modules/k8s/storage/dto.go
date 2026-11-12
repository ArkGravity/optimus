package storage

import (
	"time"

	"github.com/ArkGravity/optimus/internal/modules/k8s/clusterscoped"
)

type Summary struct {
	Name          string    `json:"name"`
	Namespace     string    `json:"namespace,omitempty"`
	Capacity      string    `json:"capacity,omitempty"`
	AccessModes   []string  `json:"access_modes"`
	ReclaimPolicy string    `json:"reclaim_policy,omitempty"`
	Status        string    `json:"status,omitempty"`
	Claim         string    `json:"claim,omitempty"`
	StorageClass  string    `json:"storage_class,omitempty"`
	Volume        string    `json:"volume,omitempty"`
	Provisioner   string    `json:"provisioner,omitempty"`
	BindingMode   string    `json:"binding_mode,omitempty"`
	Age           time.Time `json:"age"`
}

type ListResponse = clusterscoped.ListResponse[Summary]
type ListQuery = clusterscoped.ListQuery
