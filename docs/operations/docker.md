# Docker hosts over SSH

Optimus manages remote Docker Engines by opening an SSH session to the host and
speaking the Docker API on the configured Unix socket (default
`/var/run/docker.sock`). Image and container lists are live reads; only host
connection metadata is stored.

## Connect a host

1. Prefer an existing SSH Key credential, or choose password auth and enter a
   username/password. Passwords are vault-encrypted on the host row and never
   returned by the API.
2. Enter the SSH address and port, and confirm the socket path if it is not the
   default.
3. Use **Test connection** before Save. Save itself probes first and only
   persists the host after a successful probe; the saved host's Reachable/
   Unreachable status is then recorded. Host key fingerprint pinning is not
   enforced in this version.
4. On Images and Containers, select the host with HostPicker. The picker never
   auto-selects.

Required Optimus permissions: `docker:environment:read` for lists and tests;
`docker:environment:write` for host mutations, image delete/prune and container
start/stop/restart/delete. Selecting an SSH Key also needs
`credentials:ssh_key:read`. Live SSH key references refuse credential deletion.

## Upgrade

Apply migration 00021, then run `optimus seed`. Existing head20 databases upgrade
in place. Custom roles need an explicit write grant for mutations.
