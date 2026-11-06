#!/usr/bin/env bash
set -euo pipefail
if [[ $# != 4 ]]; then echo "usage: prepare.sh IMAGE VERSION ARCH OUTPUT" >&2; exit 2; fi
image=$1 version=$2 arch=$3 output=$4
mkdir -p "$output"
container="optimus-release-extract-$(python3 -B -c 'import secrets; print(secrets.token_hex(12))')"
create_pid=''
create_known=0
smoke_pid=''
cleanup() {
  local status=$?
  trap '' TERM HUP INT
  # A trap can run between launching the helper and assigning its PID.
  if [[ "$create_pid" == pending ]]; then create_pid=${!:-}; fi
  if [[ -n "$create_pid" ]]; then
    if wait "$create_pid"; then create_known=1; fi
  fi
  # Client failure/timeout does not settle a detached daemon request. Reconcile
  # only this name and ownership label, with isolated and bounded CLI children.
  if ! python3 -B scripts/release/extraction.py "$container" "$create_known"; then
    if [[ "$status" == 0 ]]; then status=1; fi
  fi
  rm -f "$output/optimus" "$output/optimus_${version}_deploy.tar.gz"
  exit "$status"
}
terminate() {
  local termination_signal=$1 termination_status=$2 smoke_status=0
  trap '' TERM HUP INT
  if [[ -n "$smoke_pid" ]]; then
    kill -s "$termination_signal" "$smoke_pid" >/dev/null 2>&1 || true
    wait "$smoke_pid" || smoke_status=$?
    # A group signal may reach Python before Bash services its own trap.
    # Preserve that first termination status even if another signal follows.
    case "$smoke_status" in 129|130|143) termination_status=$smoke_status ;; esac
  fi
  exit "$termination_status"
}
trap cleanup EXIT
trap 'terminate TERM 143' TERM
trap 'terminate HUP 129' HUP
trap 'terminate INT 130' INT
# Keep creation alive through group termination and await its bounded result
# before removing by the preassigned name, even if no container ID is returned.
create_pid=pending
(
  trap '' TERM HUP INT
  exec python3 -B scripts/release/process.py docker create --name "$container" --label "io.optimus.release.extraction=$container" "$image"
) &
create_pid=$!
wait "$create_pid"
create_known=1
create_pid=''
docker cp "$container:/usr/local/bin/optimus" "$output/optimus"
chmod 700 "$output/optimus"
epoch=$(git show -s --format=%ct HEAD)
python3 -B scripts/release/package.py native --version "$version" --arch "$arch" --binary "$output/optimus" --output "$output" --epoch "$epoch"
python3 -B scripts/release/package.py deploy --version "$version" --output "$output" --epoch "$epoch"
# An explicit wait lets Bash forward signals promptly while Python unwinds its
# Compose project and private temporary directory.
bash scripts/release/smoke.sh --image "$image" --version "$version" --arch "$arch" --artifacts "$output" &
smoke_pid=$!
wait "$smoke_pid"
smoke_pid=''
# Save only after the exact image and extracted binary passed real runtime smoke.
docker save --output "$output/image-${arch}.tar" "$image"
python3 -B scripts/release/package.py arch-sums --version "$version" --arch "$arch" --output "$output"
