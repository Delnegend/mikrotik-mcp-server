#!/usr/bin/env bash
# Stops the CHR test VM started by up.sh. State is kept in chr-test.qcow2;
# use `bash scripts/chr/up.sh --fresh` to start over from a clean image.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PIDFILE="$HERE/chr-test.pid"

if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
  kill "$(cat "$PIDFILE")"
  echo "Stopped CHR (pid $(cat "$PIDFILE"))."
  rm -f "$PIDFILE"
else
  echo "CHR is not running."
fi
