---
name: github-project-worktree
description: Manage GitHub project issues and develop with worktrees
compatibility: opencode
metadata:
  audience: maintainers
  workflow: github
---

## What I do

- Standardize new requirement intake through GitHub Issues + Project fields
- Ensure issue execution starts with Project status checks
- Create branch names from issue type using Conventional Commit style
- Enforce main-based worktree development by default

## Defaults

- Repository: detect from current git remote (or pass `-R owner/repo`)
- Project owner/number: detect from repo or prompt once per session
- Base branch: detect from `defaultBranchRef` or prompt
- Worktree root: `~/.worktrees` (full path: `~/.worktrees/<branch-name>`)
- Assignment policy for new issues: unassigned by default

## Preflight (avoid gh command errors)

1. Ensure GitHub CLI auth has `project` scope:
   - `gh auth status`
   - If scope is missing: `gh auth refresh -s project`
2. Detect repo/default branch from current remote:
   - `gh repo view --json nameWithOwner,defaultBranchRef`
3. Resolve project ID once before field edits:
   - `gh project view <project-number> --owner <project-owner> --format json --jq '.id'`

## Session naming (title agent)

1. Once requirement context is known (issue number + service + short scope), try calling `title agent` to rename current session.
2. Use a short, searchable title format:
   - `<type>/<service> #<issue-number> <short-slug>`
   - Example: `feat/manager #8 tool-market-offers-entitlements`
3. If `title agent` is unavailable or fails, continue workflow (non-blocking) and mention it in progress output.

## Workflow A: New requirement

1. Create issue first (do not start coding before issue exists).
2. Use issue template `service_delivery_request.yml` (if available) and ensure required fields are filled:
   - Priority
   - Service (or equivalent grouping field)
   - Background
   - Current state
   - Expected outcome
3. Keep issue unassigned by default (if needed, remove accidental assignment).
4. After issue is created and requirement is clear, try `title agent` to rename current session.
5. Add issue to target GitHub Project:
   - Use `gh project item-add ... --format json --jq '.id'` to get `item-id`.
   - `item-add` is idempotent for existing issue/PR items.
6. Set Project fields (lookup field/option IDs by name, do not hardcode IDs):
   - `Status=Todo`
   - Service/grouping field from issue form
   - Priority from issue form
   - Size if available, otherwise leave blank

## Workflow B: Start working on an issue

1. Validate issue state (`open`, not duplicated, clear scope).
2. After requirement is confirmed from issue metadata, try `title agent` to rename current session.
3. Ensure issue is in Project before coding:
   - Run `item-add` to get `item-id` (works whether item is new or already in project).
   - Set `Status=In Progress` using `item-edit`.
4. Determine branch type from issue labels/title:
   - `type:feature` -> `feat`
   - `bug` or `type:bug` -> `fix`
   - `docs` -> `docs`
   - `refactor` -> `refactor`
   - `test` -> `test`
   - fallback -> `chore`
5. Build branch name:
   - Format: `<type>/issue-<number>-<short-slug>`
   - Example: `feat/issue-25-manager-auth-rbac`
   - Slug derivation: lowercase, kebab-case, remove prefixes like `[service:]`, limit to ~50 chars
6. Create branch from synced default branch (e.g., `origin/main`):
   - `git fetch origin <default-branch>`
   - `WORKTREE_PATH="$HOME/.worktrees/<branch-name>"`
   - `mkdir -p "$(dirname "$WORKTREE_PATH")"`
   - `git worktree add -b "<branch-name>" "$WORKTREE_PATH" origin/<default-branch>`
7. If user explicitly says no worktree, skip worktree and create branch in current tree.
8. After creation, ask one targeted question: whether to switch and continue coding in that worktree path.

## Conventional Commit alignment

- Branch type should match planned commit prefix (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`).
- Keep first commit scope close to issue service/component when possible, e.g. `feat(manager): ...`.

## Command references

```bash
# === Setup variables (detect or prompt per session) ===
REPO="<owner>/<repo>"                    # e.g., LiusCraft/orion-x
PROJECT_OWNER="<project-owner>"          # e.g., LiusCraft (or @me)
PROJECT_NUMBER="<project-number>"        # e.g., 3
DEFAULT_BRANCH="<default-branch>"        # e.g., main
ISSUE_NUMBER="<issue-number>"            # e.g., 8
ISSUE_URL="https://github.com/${REPO}/issues/${ISSUE_NUMBER}"

# === Preflight ===
gh auth status
# if missing project scope: gh auth refresh -s project

# === Validate issue ===
gh issue view "$ISSUE_NUMBER" -R "$REPO" --json number,title,state,stateReason,labels,url

# === Optional: rename current session via title agent ===
# target title format: <type>/<service> #<issue> <short-slug>
# Example prompt: "feat/manager #8 tool-market-offers-entitlements"

# === Resolve project and item IDs ===
PROJECT_ID=$(gh project view "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.id')
ITEM_ID=$(gh project item-add "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --url "$ISSUE_URL" --format json --jq '.id')

# === Lookup field/option IDs by name (avoid hardcoding) ===
# Example for Status field:
STATUS_FIELD_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.fields[] | select(.name=="Status") | .id')
STATUS_IN_PROGRESS_OPTION_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.fields[] | select(.name=="Status") | .options[] | select(.name=="In Progress") | .id')

# Set Status=In Progress (before coding)
gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" --field-id "$STATUS_FIELD_ID" --single-select-option-id "$STATUS_IN_PROGRESS_OPTION_ID"

# === Create worktree from default branch ===
BRANCH_TYPE="feat"        # determine from issue labels
BRANCH_NAME="$BRANCH_TYPE/issue-$ISSUE_NUMBER-short-slug"
WORKTREE_PATH="$HOME/.worktrees/$BRANCH_NAME"
git fetch origin "$DEFAULT_BRANCH"
mkdir -p "$(dirname "$WORKTREE_PATH")"
git worktree add -b "$BRANCH_NAME" "$WORKTREE_PATH" "origin/$DEFAULT_BRANCH"
```

## Common gh pitfalls

- Missing `project` scope causes project edit/add failures.
- `gh project item-edit` requires `project-id` (not project number).
- `gh project item-list` defaults to 30 items; use `-L` for larger projects.
- Field IDs and option IDs are per-project and can change; always resolve by name.
- Prefer idempotent `item-add` to guarantee `item-id` before `item-edit`.

## Output checklist before coding starts

- Issue URL and number
- Project item status update result
- Branch name and type rationale
- Worktree absolute path
- Session name update result (`title agent` success/fallback)
- Confirmation question to switch into worktree
