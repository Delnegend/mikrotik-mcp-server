#!/usr/bin/env bash
# scripts/chr/test.sh - run the full test suite against the RouterOS CHR VM.
# Linux/devcontainer twin of test.ps1 (which stays as the Windows-host path).
#
# Boots the CHR (idempotent via up.sh), provisions it if fresh, then runs
# `go test ./...` with the VM environment set. The behavioral integration
# suite (internal/integration) runs against the live router; the pure-logic
# suite stays green without a VM.
#
# Usage:
#   bash scripts/chr/test.sh             # boot + provision + full suite (VM stays up)
#   bash scripts/chr/test.sh --fresh     # wipe CHR state and start clean
#   bash scripts/chr/test.sh --down      # stop the VM after the suite
#   bash scripts/chr/test.sh --tag CHR   # pass -run to go test
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

FRESH=0
DOWN=0
TAG=""
PASSWORD="${CHR_TEST_PASSWORD:-admin}"
while [ $# -gt 0 ]; do
  case "$1" in
    --fresh) FRESH=1 ;;
    --down) DOWN=1 ;;
    --tag) TAG="$2"; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

UP_ARGS=(--no-install)
[ "$FRESH" = 1 ] && UP_ARGS+=(--fresh)

echo "==> booting CHR (idempotent)"
bash "$HERE/up.sh" "${UP_ARGS[@]}"

cd "$REPO"
echo "==> ensuring provisioned (best effort; 'already provisioned' is fine)"
go run ./cmd/chrprovision -password "$PASSWORD" >/dev/null 2>&1 || true

echo "==> running full test suite against the VM"
export MIKROTIK_TEST_HOST=127.0.0.1
export MIKROTIK_TEST_USER=admin
export MIKROTIK_TEST_PASSWORD="$PASSWORD"
export MIKROTIK_TEST_PORT=8728

if [ -n "$TAG" ]; then
  go test ./... -count=1 -run "$TAG"
else
  go test ./... -count=1
fi
CODE=$?

if [ "$DOWN" = 1 ]; then
  echo "==> stopping CHR"
  bash "$HERE/down.sh"
fi

exit $CODE
