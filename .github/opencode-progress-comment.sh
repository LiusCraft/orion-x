#!/usr/bin/env bash
# opencode 状态评论的统一入口，三个 opencode 工作流共用。
# 需要环境变量：GH_TOKEN、GITHUB_REPOSITORY、GITHUB_RUN_ID。
#
#   create <issue-number> <状态行>   复用本 issue/PR 上已有的状态评论（按首行标记定位），没有则新建；
#                                   向 stdout 输出 `id=<评论 id>`，供 GITHUB_OUTPUT 使用。
#   finalize                        智能体未收尾（评论仍以 ⏳ 开头）时保留原内容、前置未收尾说明，
#                                   仅把状态行标记为 ⛔；已收尾（无 ⏳ 状态行）则不动。
#
# 评论首行标记由 .github/opencode-instructions.md 要求智能体在每次更新时保留。
set -eu

endpoint="repos/$GITHUB_REPOSITORY/issues"
run_url="https://github.com/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"

case "${1:-}" in
  create)
    number="$2"
    status="$3"
    body='<!-- opencode:status -->
'"$status"'（[运行日志]('"$run_url"')）'
    existing=$(gh api --paginate "$endpoint/$number/comments?per_page=100" \
      --jq '.[] | select(.user.login == "github-actions[bot]" and (.body | startswith("<!-- opencode:status -->"))) | .id' \
      | tail -1 || true)
    if [ -n "$existing" ]; then
      id=$(gh api -X PATCH "$endpoint/comments/$existing" -F body="$body" --jq .id)
    else
      id=$(gh api "$endpoint/$number/comments" -F body="$body" --jq .id)
    fi
    echo "id=$id"
    ;;
  finalize)
    id="${PROGRESS_COMMENT_ID:-}"
    if [ -z "$id" ]; then
      exit 0
    fi
    body=$(gh api "$endpoint/comments/$id" --jq .body 2>/dev/null || true)
    if ! printf '%s\n' "$body" | grep -q '^⏳'; then
      exit 0
    fi
    gh api -X PATCH "$endpoint/comments/$id" \
      -F body="❌ opencode 运行未正常收尾（可能失败、超时、被取消或未按约定更新），详见[运行日志]($run_url)。

---

${body//⏳/⛔}" || true
    ;;
  *)
    echo "usage: $0 create <issue-number> <status-line> | finalize" >&2
    exit 2
    ;;
esac
