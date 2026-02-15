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
- Worktree root: `~/.worktrees/<branch-name>/`
- Assignment policy for new issues: unassigned by default

## Workflow A: New requirement

1. Create issue first (do not start coding before issue exists).
2. Use issue template `service_delivery_request.yml` and ensure required fields are filled:
   - Priority
   - Service
   - Background
   - Current state
   - Expected outcome
3. Keep issue unassigned by default.
4. Add issue to target GitHub Project (for `orion-x`: `orion-x Service Delivery`, `#3`).
5. Set Project fields:
   - `Status=Todo`
   - `Service` from issue form
   - `Priority` from issue form
   - `Size` if available, otherwise leave blank

## Workflow B: Start working on an issue

1. Validate issue state (`open`, not duplicated, clear scope).
2. Check if issue is already in Project:
   - If in Project, update `Status=In Progress` before coding.
   - If not in Project, add it, then set `Status=In Progress`.
3. Determine branch type from issue labels/title:
   - `type:feature` -> `feat`
   - `bug` or `type:bug` -> `fix`
   - `docs` -> `docs`
   - `refactor` -> `refactor`
   - `test` -> `test`
   - fallback -> `chore`
4. Build branch name:
   - `<type>/issue-<number>-<short-slug>`
   - Example: `feat/issue-25-manager-auth-rbac`
5. Create branch from `main` (or ask user if default branch is uncertain):
   - Sync `main`
   - Create worktree at `~/.worktrees/<branch-name>/`
   - `git worktree add -b "<branch-name>" "<worktree-path>" main`
6. If user explicitly says no worktree, skip worktree and create branch in current tree.
7. After creation, ask one targeted question: whether to switch and continue coding in that worktree path.

## Conventional Commit alignment

- Branch type should match planned commit prefix (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`).
- Keep first commit scope close to issue service when possible, e.g. `feat(manager): ...`.

## Command references

```bash
# create issue (preferred with template via GitHub UI), then:
gh project item-add <project-number> --owner <project-owner> --url "https://github.com/<owner>/<repo>/issues/<n>"

# inspect project fields and options
gh project field-list <project-number> --owner <project-owner> --format json

# set status / service / priority (requires ids)
gh project item-edit --id <item-id> --project-id <project-id> --field-id <field-id> --single-select-option-id <option-id>

# create worktree from main
git worktree add -b "<branch-name>" "~/.worktrees/<branch-name>" main
```

## Output checklist before coding starts

- Issue URL and number
- Project item status update result
- Branch name and type rationale
- Worktree absolute path
- Confirmation question to switch into worktree
