#!/usr/bin/env bash
set -euo pipefail

# Setup branch protection for autonomous-upgrade pipeline
# Usage: ./scripts/setup-branch-protection.sh Delnegend/mikrotik-mcp-server [--dry-run]
# Requires: gh cli authenticated with admin on repo
# Strictly follows ~/.agents/skills/autonomous-upgrade skill

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
echo "Setting up branch protection for $REPO ($BRANCH) — single 'Check (just check)' gate, linear history, rebase only + auto-merge..."

# 1. Enable auto-merge and rebase-only merges
if $DRY_RUN; then
  echo "[dry-run] Would edit $REPO with --enable-auto-merge --enable-rebase-merge --delete-branch-on-merge"
else
  gh repo edit "$REPO" \
    --enable-auto-merge \
    --enable-rebase-merge \
    --delete-branch-on-merge
  echo "Repository merge settings updated."
fi

# 2. Branch protection — single 'Check (just check)' gate
PROTECTION_JSON=$(cat <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Check (just check)"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": null,
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "allow_auto_merge": true
}
JSON
)

if $DRY_RUN; then
  echo "[dry-run] Would PUT /repos/$REPO/branches/$BRANCH/protection with:"
  echo "$PROTECTION_JSON" | python3 -m json.tool
else
  echo "$PROTECTION_JSON" | gh api "repos/$REPO/branches/$BRANCH/protection" -X PUT -H "Accept: application/vnd.github+json" --input - > /tmp/protection.out 2>&1
  cat /tmp/protection.out | python3 -m json.tool | head -n 40
  echo "Branch protection set."
fi

# 3. Verify
if ! $DRY_RUN; then
  echo "Verifying..."
  gh api "repos/$REPO/branches/$BRANCH/protection" --jq '{required_status_checks, required_linear_history}' 2>&1 | python3 -m json.tool
  gh repo view "$REPO" --json deleteBranchOnMerge,rebaseMergeAllowed,squashMergeAllowed,mergeCommitAllowed 2>&1 | python3 -m json.tool
fi

echo "Done. PRs to $BRANCH now require 'Check (just check)' and linear history; rebase merges only, auto-merge enabled."
