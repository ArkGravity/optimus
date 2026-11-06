#!/usr/bin/env bash
set -euo pipefail
# Recheck eligibility immediately before registry or Release writes.
python3 -B "$(dirname "$0")/validate.py"
exec python3 -B "$(dirname "$0")/publish.py" "$@"
