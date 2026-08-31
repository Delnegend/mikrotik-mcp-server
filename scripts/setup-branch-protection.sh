#!/usr/bin/env bash
set -euo pipefail

# Setup branch protection for Dependabot -> PR -> CI -> release flow
# Usage: ./scripts/setup-branch-protection.sh Delnegend/mikrotik-mcp-server [--dry-run]
# Requires: gh cli authenticated with admin on repo
# Adapted from https://github.com/Delnegend/actions (pure Go: 6 checks instead of 4+Wails)

REPO="${1:-}"
DRY_RUN=false
if [[ "${2:-}" == "--dry-run" ]]; then
  DRY_RUN=true
fi

if [[ -z "$REPO" ]]; then
  echo "Usage: $0 <owner/repo> [--dry-run]"
  echo "Example: $0 Delnegend/mikrotik-mcp-server"
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI not found. Install from https://cli.github.com/"
  exit 1
fi

BRANCH="main"
echo "Setting up branch protection for $REPO ($BRANCH) — 5 required checks, linear history, rebase only + auto-merge..."

# 1. Branch protection — contexts must match job names in .github/workflows/ci.yml:1
PROTECTION_JSON=$(cat <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["Check (just check)", "Build (linux/amd64)", "Build (darwin/arm64)", "Build (windows/amd64)", "CHR (integration)"]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": null,
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_linear_history": true,
  "allow_auto_merge": true
}
JSON
)

if $DRY_RUN; then
  echo "[dry-run] Would PUT /repos/$REPO/branches/$BRANCH/protection with:"
  echo "$PROTECTION_JSON" | python3 -m json.tool
else
  echo "$PROTECTION_JSON" | gh api "repos/$REPO/branches/$BRANCH/protection" -X PUT --input - > /tmp/protection.out 2>&1
  cat /tmp/protection.out | python3 -m json.tool | head -n 40
  echo "Branch protection set."
fi

# 2. Merge methods + auto-merge
if $DRY_RUN; then
  echo "[dry-run] Would PATCH /repos/$REPO with allow_rebase_merge=true, squash/merge false, auto-merge true"
else
  gh api "repos/$REPO" -X PATCH -f allow_rebase_merge=true -f allow_squash_merge=false -f allow_merge_commit=false -f allow_auto_merge=true > /tmp/merge.out 2>&1
  cat /tmp/merge.out | python3 -m json.tool | grep -E "allow_(rebase|squash|merge|auto)" | head -n 10
  echo "Merge methods set to rebase only + auto-merge enabled."
fi

# 3. Verify
if ! $DRY_RUN; then
  echo "Verifying..."
  gh api "repos/$REPO/branches/$BRANCH/protection" --jq '{required_status_checks, required_linear_history}' 2>&1 | python3 -m json.tool
  gh api "repos/$REPO" --jq '{allow_rebase_merge, allow_squash_merge, allow_merge_commit, allow_auto_merge}' 2>&1 | python3 -m json.tool
fi

echo "Done. PRs to $BRANCH now require 5 checks and linear history; only Rebase and merge is allowed, auto-merge enabled for Dependabot patch/minor."
