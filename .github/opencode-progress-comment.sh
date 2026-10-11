#!/usr/bin/env bash
# opencode 状态评论的统一入口，三个 opencode 工作流共用。
# 需要环境变量：GH_TOKEN、GITHUB_REPOSITORY、GITHUB_RUN_ID。
# 智能体侧通过 OPENCODE_PROGRESS_MARKER（值与这里生成的 marker 相同）在每次更新时保留标记行，
# 见 .github/opencode-instructions.md。
#
#   create <issue-number> <状态行>   找本 issue/PR 上最新一条首行以 `<!-- opencode:status` 开头的状态评论：
#                                   只有它仍未收尾（仍有行首 ⏳ 状态行）才复用它，否则新建；首行写入含
#                                   本次运行 id 的标记，向 stdout 输出 `id=<评论 id>`。
#   finalize                        仅当首行标记属于本次运行、且评论仍有行首 ⏳ 状态行时收尾：用未收尾说明
#                                   替换该状态行；分享链接、todo 与标记行原样保留。已收尾、标记属于其它
#                                   运行或评论已删除则不动——已收尾的评论作为历史保留，异常中断的评论
#                                   留给后续运行接管。
set -eu

endpoint="repos/$GITHUB_REPOSITORY/issues"
run_url="https://github.com/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"
marker="<!-- opencode:status:$GITHUB_RUN_ID -->"

case "${1:-}" in
  create)
    number="$2"
    status="$3"
    body="$marker
$status（[运行日志]($run_url)）"
    existing=$(gh api --paginate "$endpoint/$number/comments?per_page=100" \
      --jq '.[] | select(.user.login == "github-actions[bot]" and (.body | startswith("<!-- opencode:status"))) | .id' \
      | tail -1 || true)
    if [ -n "$existing" ]; then
      existing_body=$(gh api "$endpoint/comments/$existing" --jq .body 2>/dev/null || true)
      # 只接管未收尾的评论；已收尾的是历史记录，不能再被下次运行改写成 ⏳。
      if ! printf '%s\n' "$existing_body" | grep -q '^⏳'; then
        existing=""
      fi
    fi
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
    if [ "${body%%$'\n'*}" != "$marker" ]; then
      exit 0
    fi
    if ! printf '%s\n' "$body" | grep -q '^⏳'; then
      exit 0
    fi
    note="❌ opencode 运行未正常收尾（可能失败、超时、被取消或未按约定更新），详见[运行日志]($run_url)。"
    updated=$(printf '%s\n' "$body" | awk -v note="$note" '!done && /^⏳/ { print note; done = 1; next } { print }')
    gh api -X PATCH "$endpoint/comments/$id" -F body="$updated" || true
    ;;
  *)
    echo "usage: $0 create <issue-number> <status-line> | finalize" >&2
    exit 2
    ;;
esac
