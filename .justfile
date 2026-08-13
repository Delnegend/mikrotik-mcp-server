# MikroTik MCP server — dev workflows
#
# Devcontainer recipes (Linux shell): run them inside the devcontainer, where
# Go, just and qemu are preinstalled (see .devcontainer/Dockerfile).

# List all recipes
default:
    just --list

# Boot the CHR test router (idempotent)
up:
    bash scripts/chr/up.sh

# Boot with a clean router state (wipes the qcow2 overlay)
fresh:
    bash scripts/chr/up.sh --fresh

# Stop the CHR test router (state kept; up keeps it running)
down:
    bash scripts/chr/down.sh

# Harden the router: set admin password, enable api/api-ssl/ssh/winbox, set identity
# (idempotent: "already provisioned" is a normal no-op)
provision password="admin":
    -go run ./cmd/chrprovision -password {{password}}

# Run the whole test suite against the VM (boots + provisions + tests; VM stays up)
test:
    bash scripts/chr/test.sh

# Same, on a freshly wiped router
test-fresh:
    bash scripts/chr/test.sh --fresh

# Same, stopping the VM afterwards
test-down:
    bash scripts/chr/test.sh --down

# Full clean cycle: wipe, test, stop
test-clean:
    bash scripts/chr/test.sh --fresh --down

# Pure-logic tests only (no VM needed, integration suite self-skips)
test-quick:
    go test ./...

# Static checks: fix deprecated APIs, format, then vet
check:
    go fix ./...
    go fmt ./...
    go vet ./...
