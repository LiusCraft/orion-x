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
- Project owner/number: use repo convention (for `orion-x`: owner `LiusCraft`, project `#3`), otherwise ask once and continue
- Base branch: `main`
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
2. Use issue template `service_delivery_request.yml` and ensure required fields are filled:
   - Priority
   - Service
   - Background
   - Current state
   - Expected outcome
3. Keep issue unassigned by default (if needed, remove accidental assignment).
4. After issue is created and requirement is clear, try `title agent` to rename current session.
5. Add issue to target GitHub Project (for `orion-x`: `orion-x Service Delivery`, `#3`).
   - Use `gh project item-add ... --format json --jq '.id'` to get `item-id`.
   - `item-add` is idempotent for existing issue/PR items.
6. Set Project fields (lookup field/option IDs by name, do not hardcode IDs):
   - `Status=Todo`
   - `Service` from issue form
   - `Priority` from issue form
   - `Size` if available, otherwise leave blank

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
   - `<type>/issue-<number>-<short-slug>`
   - Example: `feat/issue-25-manager-auth-rbac`
6. Create branch from synced `origin/main` (or ask user if default branch is uncertain):
   - `git fetch origin main`
   - `WORKTREE_PATH="$HOME/.worktrees/<branch-name>"`
   - `mkdir -p "$(dirname "$WORKTREE_PATH")"`
   - `git worktree add -b "<branch-name>" "$WORKTREE_PATH" origin/main`
7. If user explicitly says no worktree, skip worktree and create branch in current tree.
8. After creation, ask one targeted question: whether to switch and continue coding in that worktree path.

## Conventional Commit alignment

- Branch type should match planned commit prefix (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`).
- Keep first commit scope close to issue service when possible, e.g. `feat(manager): ...`.

## Command references

```bash
# required variables
REPO="LiusCraft/orion-x"
PROJECT_OWNER="LiusCraft"
PROJECT_NUMBER=3
ISSUE_NUMBER=8
ISSUE_URL="https://github.com/${REPO}/issues/${ISSUE_NUMBER}"

# preflight
gh auth status
# if missing project scope:
# gh auth refresh -s project

# validate issue
gh issue view "$ISSUE_NUMBER" -R "$REPO" --json number,title,state,stateReason,labels,url

# optional: rename current session via title agent
# target title format: <type>/<service> #<issue> <short-slug>
# Example prompt to title agent:
# "feat/manager #8 tool-market-offers-entitlements"

# resolve project and item IDs
PROJECT_ID=$(gh project view "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.id')
ITEM_ID=$(gh project item-add "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --url "$ISSUE_URL" --format json --jq '.id')

# lookup field/option IDs by name
STATUS_FIELD_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.fields[] | select(.name=="Status") | .id')
STATUS_TODO_OPTION_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.fields[] | select(.name=="Status") | .options[] | select(.name=="Todo") | .id')
STATUS_IN_PROGRESS_OPTION_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.fields[] | select(.name=="Status") | .options[] | select(.name=="In Progress") | .id')
SERVICE_FIELD_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.fields[] | select(.name=="Service") | .id')
PRIORITY_FIELD_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq '.fields[] | select(.name=="Priority") | .id')

# values from issue form
SERVICE_VALUE="manager"
PRIORITY_VALUE="P1"
SERVICE_OPTION_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq ".fields[] | select(.name==\"Service\") | .options[] | select(.name==\"$SERVICE_VALUE\") | .id")
PRIORITY_OPTION_ID=$(gh project field-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --jq ".fields[] | select(.name==\"Priority\") | .options[] | select(.name==\"$PRIORITY_VALUE\") | .id")

# set Status=In Progress (before coding)
gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" --field-id "$STATUS_FIELD_ID" --single-select-option-id "$STATUS_IN_PROGRESS_OPTION_ID"

# set Service/Priority (for new requirement intake)
gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" --field-id "$SERVICE_FIELD_ID" --single-select-option-id "$SERVICE_OPTION_ID"
gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" --field-id "$PRIORITY_FIELD_ID" --single-select-option-id "$PRIORITY_OPTION_ID"

# verify item status (use -L to avoid default 30-item truncation)
gh project item-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" -L 200 --format json --jq ".items[] | select(.id==\"$ITEM_ID\") | {id,status,service,priority,title}"

# create worktree from main
BRANCH_NAME="feat/issue-8-tool-market-offers-entitlements-api"
WORKTREE_PATH="$HOME/.worktrees/$BRANCH_NAME"
git fetch origin main
mkdir -p "$(dirname "$WORKTREE_PATH")"
git worktree add -b "$BRANCH_NAME" "$WORKTREE_PATH" origin/main
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
