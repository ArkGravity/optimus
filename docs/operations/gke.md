# Native GKE connections

This source feature is not part of the published v0.1.2 binary or image.
Use a build containing native GKE authentication and apply its additive
00020 migration to an existing head19 database. Preserve the database and vault
key; no recreation, new environment secret or installed gcloud plugin is needed.

## Connect a cluster

1. In Cloud Keys, create or select a GCP credential using Service Account JSON.
   Secret material remains encrypted and is not returned by list/detail APIs.
2. In Kubernetes credentials, choose **GKE Service Account**, select that Cloud
   Key, and enter the target Project ID/number, region or zone, and cluster name.
   The target project can differ from the credential's home project. Choose
   the default, public IP, private IP or DNS control-plane endpoint. No kubeconfig
   upload, manually copied token or installation of gke-gcloud-auth-plugin is
   required. The credential editor requires Cloud Key read access for selection.
3. In Kubernetes / Clusters, select that credential. Its context is **gke**;
   the form fills this value when selecting the credential. Check connectivity,
   then verify inventory and Pod logs according to the identity's permissions.
4. Use an isolated test namespace to verify the Helm lifecycle. Helm needs
   appropriate Secret and chart-resource write permissions there; read-only
   inventory permissions alone are insufficient. Pre-create the namespace if
   the identity has no namespace-creation permission.

The Service Account needs `container.clusters.get` on the target cluster and
the target project's Kubernetes Engine API must be enabled. Resource operations
require their own IAM or Kubernetes RBAC permissions. A role containing
`container.clusters.get` does not automatically authorize Helm installation.
DNS endpoint access also requires `container.clusters.connect` and applicable
network/VPC Service Controls policies. Use an IAM service-account email for RBAC
bindings, not the identity of a Kubernetes ServiceAccount.

The Optimus server needs HTTPS access to `oauth2.googleapis.com`,
`container.googleapis.com` and the selected control-plane endpoint. Access from
an administrator's workstation does not establish server connectivity.

## Runtime and maintenance

Optimus obtains the cluster endpoint and CA from the authenticated Google API,
uses normal TLS validation, and acquires OAuth tokens with cloud-platform and
userinfo.email scopes. Tokens refresh automatically within an operation and
are never saved in the database. New operations retrieve the currently selected
credential again, so key rotation is effective without restarting Optimus.
This initial implementation reuses tokens within an operation, not across
operations; Google token/metadata calls therefore accompany new connections.

Uploaded kubeconfigs continue to support their existing authentication methods.
All exec and auth-provider plugins remain prohibited, including an uploaded
gcloud-generated kubeconfig. Select the native GKE mode instead. No ambient
Google login, gcloud process or Workload Identity discovery is used.

Metadata-only edits do not read/decrypt the Cloud Key. Rotate Service Account
JSON on the same Cloud Key or select another valid GCP key on the connection.
Authentication mode and target cluster/endpoint are immutable; create a new
credential and rebind the cluster to change these. Referenced Cloud Keys cannot
be deleted or converted to another provider/authentication type.

Safe errors distinguish invalid configuration, authentication, denied access,
missing cluster and connection failures. They do not expose Google response
bodies, Service Account JSON or access tokens. Application JWT authentication
and Optimus feature permissions remain independent of Google authorization.

On 2026-11-13 the operator confirmed successful Prod verification of the native
GKE feature branch at implementation commit `709db86`. Local fixture tests
separately verify transport, refresh and client integration without real cloud
credentials. Validate the selected identity and required resource permissions
for each deployment; the operator's report does not replace those environment
prerequisites.

References: [API authentication](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/api-server-authentication),
[cluster metadata API](https://docs.cloud.google.com/kubernetes-engine/docs/reference/rest/v1/projects.locations.clusters/get),
[control-plane endpoints](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/network-isolation).
