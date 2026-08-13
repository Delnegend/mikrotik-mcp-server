#!/usr/bin/env bash
# Provisions a disposable MikroTik Cloud Hosted Router (CHR) in QEMU for
# integration tests, using the OFFICIAL CHR image from download.mikrotik.com.
#
# Intended to run inside the devcontainer, where qemu/unzip/netcat/curl are
# preinstalled (see .devcontainer/Dockerfile) and /dev/kvm is made usable by
# .devcontainer/postinstall.sh. Falls back to software emulation otherwise.
#
# RouterOS 7 CHR boots with the API service enabled by default, so after the
# VM is up the router is immediately usable at 127.0.0.1:8728 with user
# "admin" and an EMPTY password. Optionally harden it afterwards:
#
#   go run ./cmd/chrprovision -password admin      # set pw, enable services
#
# Usage:
#   bash scripts/chr/up.sh [--fresh]
#     --fresh  destroy existing CHR state and start from a clean image
#
# Env:
#   CHR_VERSION     RouterOS version (default 7.23.3)
#   CHR_API_PORT    host port for plain API   (default 8728)
#   CHR_APISSL_PORT host port for SSL API     (default 8729)
#   CHR_SSH_PORT    host port for SSH         (default 2222)
#   CHR_SERIAL_PORT host port for serial console (default 5555)
#
# Stop:  bash scripts/chr/down.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VER="${CHR_VERSION:-7.23.3}"
API_PORT="${CHR_API_PORT:-8728}"
SSL_PORT="${CHR_APISSL_PORT:-8729}"
SSH_PORT="${CHR_SSH_PORT:-2222}"
WINBOX_PORT="${CHR_WINBOX_PORT:-8291}"
SERIAL_PORT="${CHR_SERIAL_PORT:-5555}"

FRESH=0
for a in "$@"; do
  case "$a" in
    --fresh) FRESH=1 ;;
    *) echo "unknown argument: $a" >&2; exit 2 ;;
  esac
done

IMG_ZIP="$HERE/chr-$VER.img.zip"
IMG="$HERE/chr-$VER.img"
COW="$HERE/chr-test.qcow2"
PIDFILE="$HERE/chr-test.pid"
LOG="$HERE/chr-test.log"

port_open() {
  # true when a TCP connection to host:port succeeds (no external nc needed)
  (exec 3<>"/dev/tcp/$1/$2") 2>/dev/null
}

running() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }

# --fresh means: stop any running instance and start from a clean image.
if [ "$FRESH" = 1 ] && running; then
  echo "Stopping existing CHR for --fresh..."
  kill "$(cat "$PIDFILE")"; rm -f "$PIDFILE"
  sleep 1
fi

if ! running; then
  # Image download (official). --retry-all-errors covers mid-stream
  # connection resets (seen on GitHub Actions), which plain --retry misses.
  if [ ! -f "$IMG" ]; then
    echo "Downloading official CHR $VER (~40 MB)..."
    curl -fL --retry 5 --retry-all-errors --retry-delay 2 -o "$IMG_ZIP" "https://download.mikrotik.com/routeros/$VER/chr-$VER.img.zip"
    unzip -tq "$IMG_ZIP" || { echo "downloaded image failed integrity check" >&2; rm -f "$IMG_ZIP"; exit 1; }
    unzip -o "$IMG_ZIP" -d "$HERE"
  fi
  [ -f "$IMG" ] || { echo "expected $IMG after unzip" >&2; ls -la "$HERE" >&2; exit 1; }

  # Fresh state => recreate the overlay (reset).
  if [ "$FRESH" = 1 ] || [ ! -f "$COW" ]; then
    rm -f "$COW"
    qemu-img create -f qcow2 -F raw -b "$IMG" "$COW" >/dev/null
  fi

  # KVM acceleration when usable; postinstall.sh makes /dev/kvm world-usable
  # in the devcontainer, otherwise fall back to TCG (slower boot).
  ACCEL=()
  if [ -r /dev/kvm ] && [ -w /dev/kvm ]; then ACCEL=(-enable-kvm); else
    echo "KVM unavailable - using software emulation (slower boot)."
  fi

  # Boot detached (setsid) so the VM survives if the launching shell dies.
  # SLIRP user networking: no NAT/bridge config; guest reaches host via DHCP.
  setsid qemu-system-x86_64 "${ACCEL[@]}" -m 1024 -smp 1 \
    -drive "file=$COW,format=qcow2,if=ide" \
    -netdev "user,id=n0,hostfwd=tcp:127.0.0.1:$API_PORT-:8728,hostfwd=tcp:127.0.0.1:$SSL_PORT-:8729,hostfwd=tcp:127.0.0.1:$SSH_PORT-:22,hostfwd=tcp:127.0.0.1:$WINBOX_PORT-:8291" \
    -device e1000,netdev=n0 \
    -display none \
    -serial "tcp:127.0.0.1:$SERIAL_PORT,server,nowait" \
    -pidfile "$PIDFILE" >"$LOG" 2>&1 &
  echo "Booting CHR $VER (pid $(cat "$PIDFILE" 2>/dev/null || echo "?"), log $LOG)."
fi

# Wait for the API port to accept connections.
echo "Waiting for RouterOS API on 127.0.0.1:$API_PORT..."
for _ in $(seq 1 120); do
  if port_open 127.0.0.1 "$API_PORT"; then
    cat <<EOF

CHR ready.
  API      tcp://127.0.0.1:$API_PORT   (user: admin / password: EMPTY by default)
  API SSL  tcp://127.0.0.1:$SSL_PORT
  SSH      tcp://127.0.0.1:$SSH_PORT
  WinBox   tcp://127.0.0.1:$WINBOX_PORT   (connect by IP; discovery/L2 won't work over SLIRP)
  console  nc 127.0.0.1 $SERIAL_PORT

Run the integration tests:
  just test                    # whole suite against the VM
  MIKROTIK_TEST_HOST=127.0.0.1 MIKROTIK_TEST_USER=admin go test ./internal/integration/ -v

Optionally harden the router (set admin password, enable services, identity):
  go run ./cmd/chrprovision -password admin

Stop:   bash scripts/chr/down.sh
Reset:  bash scripts/chr/up.sh --fresh
EOF
    exit 0
  fi
  sleep 2
done
echo "Timed out waiting for the API port; boot log:" >&2
tail -n 40 "$LOG" >&2 || true
echo "Stop with: bash scripts/chr/down.sh" >&2
exit 1
