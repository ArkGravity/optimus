# Docker host, image and container management

The operator authorized a new feature branch for Docker management under
Container Management. The empty Docker placeholder becomes a three-section
workspace modelled on Kubernetes navigation. Transport is SSH into the remote
host, then the Docker Engine API over the host Unix socket. Images and
containers are not persisted; only host connection metadata is stored.

## Decisions

- Transport: SSH tunnel to the host Docker Unix socket (default
  `/var/run/docker.sock`), using the official Docker client API.
- Permissions: keep `docker:environment:read` for all reads; add
  `docker:environment:write` for host CRUD, image delete/prune, and container
  start/stop/restart/delete. Viewer retains read; admin/editor get write.
- Password auth: store encrypted `password_enc` on the Docker host row. SSH key
  auth continues to reference existing `credentials_ssh_keys` through
  `credentials.Consumer`.
- Host selection: shared HostPicker on Images and Containers (ClusterPicker
  pattern). Never auto-select; absent selection shows a prompt.
- Container list: all containers including stopped, with state/status columns.
- Container delete is in scope for this version. Exec, logs, create container,
  compose, pull/push/build, Docker TCP/TLS without SSH, and inventory sync are
  out of scope.

## UI contract

Replace `containers/Docker` with a section workspace. Sidebar entry remains
`containers.docker` at `/containers/docker`. Section routes:

| Section | Path | Permission gate |
| --- | --- | --- |
| Hosts | `/containers/docker/hosts` | `docker:environment:read` |
| Images | `/containers/docker/images` | `docker:environment:read` |
| Containers | `/containers/docker/containers` | `docker:environment:read` |

Landing `/containers/docker` shows the first allowed section without changing
the URL until a button click, matching Inventory/Kubernetes. Mutation controls
use `v-permission="'docker:environment:write'"`. Seed keeps the Docker menu
row ID and path; update the permission description from “placeholder” to the
real capability. Existing installs need `optimus seed` after deploy so built-in
roles receive the new write grant where seed rules say so. Custom roles keep
existing grants and need an explicit write grant for mutations.

### Hosts

Create/edit form:

- name, optional description
- host, SSH port (default 22)
- auth type: `ssh_key` or `password`
- ssh_key: select existing SSH key; username defaults from the key and may be overridden
- password: username + password (write-only; never returned)
- socket path (default `/var/run/docker.sock`)
- Test connection (and Save) probe before persisting; Save fails closed on probe failure

List columns: name, address (`host:port`), auth (SSH key name or Password),
username, socket, status (Unknown / Reachable / Unreachable from last check),
last checked, created, actions (edit / test / delete). Status is updated on
list open or explicit test; no background polling.

Deleting a host clears HostPicker selection when that host was selected. Live
`ssh_key` host references refuse SSH key deletion in the same transaction style
as kubeconfig/cloud-key consumers; soft-deleted hosts may detach.

### Images

Requires a selected host. Live list columns: repository:tag, short image ID,
size, created. Actions: refresh (read); delete image and prune unused/dangling
images (write), each with confirmation. Occupied-image delete failures return a
safe domain error.

### Containers

Requires a selected host. Live list of all containers. Columns: name, short ID,
image, status, state, ports, created. Row actions: start, stop, restart, delete
(write; enabled by state where applicable; delete requires confirmation). Detail
drawer (read): inspect summary — ID, name, image, state/status, created/started,
network/IP, mounts, env (folded), command, restart policy. No exec or logs.

## Data and API

Append migration `00021_create_docker_hosts.sql` without changing published
migrations. Table `docker_hosts`:

- identity: id, name (unique among live rows), description
- connection: host, port, socket_path
- auth: auth_type (`ssh_key`|`password`), ssh_key_id (nullable FK to
  `credentials_ssh_keys`), username, password_enc (nullable bytea)
- probe: last_checked_at, last_check_ok (bool), last_check_error (safe text)
- audit: created_by_user_id, created_at, updated_at, deleted_at
- check constraint: ssh_key rows require ssh_key_id; password rows require
  password_enc; mutually exclusive secret material

HTTP under `/api/v1/docker` (composition in `cmd/optimus/server.go`):

- `GET/POST /hosts`, `GET/PUT/DELETE /hosts/:id`, `POST /hosts/:id/test`
- `GET /hosts/:id/images`, `DELETE /hosts/:id/images/:imageId`,
  `POST /hosts/:id/images/prune`
- `GET /hosts/:id/containers`, `GET /hosts/:id/containers/:containerId`,
  `POST /hosts/:id/containers/:containerId/start|stop|restart`,
  `DELETE /hosts/:id/containers/:containerId`

Read routes require `docker:environment:read`; mutations require
`docker:environment:write`. Responses use the fixed envelope. Secrets never
appear in JSON. Live calls open a fresh SSH session, dial the configured
socket, build a Docker client, perform the operation, and close. Bound
timeouts (about 15–30s). Normalize remote failures without leaking credentials
or raw SSH/Docker payloads. Audit mutating host and live actions through the
shared audit recorder.

## Runtime and security

- SSH key material only via `credentials.Consumer.GetSSHKey` with a purpose such
  as `docker.host.<op>` and actor context.
- Password material only via host-service vault open for the dialer; wipe buffers
  after use.
- Reject SSH `exec`/`auth-provider`-style external helpers; use private key or
  password auth only.
- Host key verification: known_hosts-style strict checking is preferred when a
  stored host key fingerprint exists; initial connect may record a fingerprint
  after successful auth for later pin. Document the first-connect behaviour in
  the operating note.
- No ambient Docker host, no long-lived client cache across requests.
- Go 1.25; pin a Docker client module compatible with that toolchain.

## Scope and evidence

Implement on a new branch from current main. Local verification: migration
00021 up/down, host CRUD and SSH-key in-use refusal, dialer unit tests with
fakes, permission/Swagger/locale/frontend tests, and optional Colima SSH smoke
when available. No Production deployment, release tag or application database
recreation is authorized by this design alone. Existing installs apply migrate
then seed after rolling the binary.
