#!/usr/bin/env bash
# Devcontainer post-create setup.
#
# The container runs privileged so the host's /dev/kvm is mounted, but its
# group id is host-specific. Make it world-accessible so qemu can use KVM
# acceleration; scripts/chr/up.sh falls back to software emulation without it.
set -euo pipefail

if [ -e /dev/kvm ]; then
  sudo chmod 666 /dev/kvm 2>/dev/null || \
    echo "warning: cannot make /dev/kvm world-accessible; KVM acceleration disabled (software emulation fallback active)"
fi

go version
just check
