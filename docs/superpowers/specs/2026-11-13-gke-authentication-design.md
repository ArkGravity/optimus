# Native GKE authentication

At design time the operator confirmed v0.1.2 Prod acceptance steps 1–6 and control-plane
connectivity. Helm acceptance is pending because the imported GKE kubeconfig
uses an exec plugin. The operator authorized code changes and selected reuse
of the existing GCP Service Account JSON credentials.

Delivery follow-up: the operator deployed and successfully verified implementation
commit `709db86` in Prod, then authorized PR creation and merge to main. See the
[implementation plan](../plans/2026-11-13-gke-authentication.md) for the separate
local and operator evidence.

## Connection contract

Add a GKE authentication option to Kubernetes credentials. Select an existing
GCP service-account Cloud Key, target project, location, cluster name and endpoint
kind (default/public/private/DNS). Optimus retrieves connection metadata from
the official GKE API, so copying a gcloud-generated kubeconfig is unnecessary.
The generated context is `gke`, offered automatically when adding a cluster.
Ordinary uploaded kubeconfigs retain their existing behavior and plugin ban.

Keep cluster/application references to Kubernetes credentials. Append migration
00020, preserving every published migration and existing credential. Persist
only nonsecret GKE binding metadata; OAuth tokens are never stored or returned.
Prevent deletion/provider conversion of referenced Cloud Keys, with the same
advisory-lock boundary used by their existing consumers. Mode and target changes
require a new Kubernetes credential; metadata edits and rebinding a valid GCP
key remain possible. Current references consequently retain context identity.

## Runtime and security

Share GKE REST configuration construction between Kubernetes and Helm. Load keys
only through credentials.Consumer, retaining actor/purpose audit attribution.
Validate the explicit service-account format, use fixed Google OAuth and GKE
endpoints and cloud-platform/userinfo.email scopes. Never execute a program,
discover ambient credentials, honor credential-supplied endpoints or disable TLS.
Resolve the Kubernetes endpoint and CA exclusively from authenticated GKE API
metadata, preventing arbitrary user endpoints from receiving a Cloud Key's token.
Bound metadata/token I/O, deny redirects and constrain bearer-token delivery to
the resolved HTTPS authority. Use per-operation OAuth token reuse/automatic
refresh and fresh credential material for subsequent operations; no token or
private-key cache spans operations.

Expose safe domain errors for invalid configuration, authorization and connection
failures, without provider bodies or token/SDK error strings. Use normal request
contexts for cancellation, including token exchange. Support public/private IP
endpoints with cluster CA validation and DNS endpoints with normal PKI trust.
Google identity permissions remain independent of Optimus RBAC. The Google
identity needs container.clusters.get and the necessary Kubernetes/IAM resource
permissions; Helm also needs Secret and chart-resource writes in a test namespace.

## Scope and evidence

No agent-operated Production actions, real-cloud calls or release publication
are included. Commit/push and subsequent PR/merge are authorized by the operator.
Verify migration19-to20/fresh20, binding/deletion/conversion/rotation, existing
kubeconfig compatibility, TLS/redirect/context/token refresh/error handling,
Kubernetes and Helm wiring, frontend forms/locales and generated Swagger.
The operator's successful branch verification supersedes the initial pending
GKE acceptance status; later deployments retain their own environment checks.

Official references:
- https://docs.cloud.google.com/kubernetes-engine/docs/how-to/api-server-authentication
- https://docs.cloud.google.com/kubernetes-engine/docs/reference/rest/v1/projects.locations.clusters/get
- https://docs.cloud.google.com/kubernetes-engine/docs/reference/rest/v1/projects.locations.clusters
