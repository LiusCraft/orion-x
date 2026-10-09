#!/usr/bin/env bash
# .github/opencode-progress-comment.sh 的契约测试：stub gh，不访问网络。
# 运行：bash .github/opencode-progress-comment.test.sh
set -u

script="$(cd "$(dirname "$0")" && pwd)/opencode-progress-comment.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
export GH_LOG="$work/gh.log"
mkdir -p "$work/bin"

cat > "$work/bin/gh" <<'EOF'
#!/usr/bin/env bash
echo "GH: $*" >> "$GH_LOG"
args="$*"
if [[ "$args" == *"--paginate"* ]]; then
  [ "${FAKE_LIST_FAIL:-}" = "1" ] && exit 1
  printf '%s\n' "${FAKE_EXISTING_ID:-}"
  exit 0
fi
if [[ "$args" == *"-X PATCH"* ]]; then
  [ "${FAKE_PATCH_FAIL:-}" = "1" ] && exit 1
  printf '77\n'
  exit 0
fi
if [[ "$args" == *"-F body="* ]]; then
  printf '9999\n'
  exit 0
fi
[ "${FAKE_GET_FAIL:-}" = "1" ] && exit 1
printf '%s' "$FAKE_BODY"
EOF
chmod +x "$work/bin/gh"
export PATH="$work/bin:$PATH"
export GITHUB_REPOSITORY="example/repo" GITHUB_RUN_ID="42"

marker='<!-- opencode:status:42 -->'
passed=0
failed=0

check() { # <name> <want> <got>
  if [ "$2" = "$3" ]; then
    passed=$((passed + 1))
    echo "ok   $1"
  else
    failed=$((failed + 1))
    echo "FAIL $1"
    echo "  want: $2"
    echo "  got:  $3"
  fi
}

check_contains() { # <name> <log> <needle>
  case "$2" in
    *"$3"*) check "$1" yes yes ;;
    *) check "$1" yes no ;;
  esac
}

check_patch_count() { # <name> <expected>
  check "$1" "$2" "$(grep -c -- '-X PATCH' "$GH_LOG" | tr -d '\n')"
}

# create：已有状态评论 -> 复用并 PATCH
: > "$GH_LOG"
out=$(FAKE_EXISTING_ID=555 bash "$script" create 82 "⏳ opencode 正在处理…" 2>&1)
check "create 输出复用的评论 id" "id=77" "$out"
check_contains "create 按首行前缀定位评论" "$(cat "$GH_LOG")" 'startswith("<!-- opencode:status")'
check_contains "create PATCH 已有评论" "$(cat "$GH_LOG")" "-X PATCH repos/example/repo/issues/comments/555"

# create：没有 -> 新建
: > "$GH_LOG"
out=$(FAKE_EXISTING_ID= bash "$script" create 82 "⏳ opencode 正在处理…" 2>&1)
check "create 新建评论并输出 id" "id=9999" "$out"
check_contains "create 新建走 POST" "$(cat "$GH_LOG")" "comments -F body="

# create：查找失败 -> 退回新建，不报错
: > "$GH_LOG"
out=$(FAKE_LIST_FAIL=1 bash "$script" create 82 "⏳ opencode 正在处理…" 2>&1)
check "create 查找失败退回新建" "id=9999" "$out"

# finalize：本运行未收尾 -> 就地替换状态行，保留标记/分享链接/todo
: > "$GH_LOG"
body=''"$marker"'
🔗 [会话分享](https://opncd.ai/share/ab12cd34)
⏳ 正在审查

- [x] 读取 AGENTS.md
- [ ] 获取 diff'
FAKE_BODY="$body" PROGRESS_COMMENT_ID=555 bash "$script" finalize > /dev/null 2>&1
log="$(cat "$GH_LOG")"
check_patch_count "finalize 标记未收尾" "1"
check_contains "finalize 前置未收尾说明" "$log" "❌ opencode 运行未正常收尾"
check_contains "finalize 保留分享链接" "$log" "share/ab12cd34"
check_contains "finalize 保留 todo" "$log" "- [ ] 获取 diff"
check "finalize 不再残留 ⏳" "0" "$(printf '%s' "$log" | grep -c '⏳' || true)"
check_contains "finalize 后的评论仍可被复用（标记行在首行）" "$log" "-F body=$marker"

# finalize：标记属于其它运行 -> 不动（避免陈旧运行误标新运行）
: > "$GH_LOG"
FAKE_BODY='<!-- opencode:status:99 -->
⏳ 正在审查' PROGRESS_COMMENT_ID=555 bash "$script" finalize > /dev/null 2>&1
check_patch_count "finalize 对其它运行的评论不动" "0"

# finalize：已收尾 -> 不动
: > "$GH_LOG"
FAKE_BODY=''"$marker"'
✅ 已完成：设计文档已产出' PROGRESS_COMMENT_ID=555 bash "$script" finalize > /dev/null 2>&1
check_patch_count "finalize 对已收尾评论不动" "0"

# finalize：正文提及 ⏳ 但状态行已收尾 -> 不动（行首锚定）
: > "$GH_LOG"
FAKE_BODY=''"$marker"'
✅ 已完成：报告里提到 ⏳ 只是引用' PROGRESS_COMMENT_ID=555 bash "$script" finalize > /dev/null 2>&1
check_patch_count "finalize 忽略正文中的 ⏳" "0"

# finalize：评论已删 / 读取失败 -> 不动
: > "$GH_LOG"
FAKE_GET_FAIL=1 PROGRESS_COMMENT_ID=555 bash "$script" finalize > /dev/null 2>&1
check_patch_count "finalize 容忍评论已删" "0"

# finalize：没有评论 id -> 不调用 gh
: > "$GH_LOG"
PROGRESS_COMMENT_ID= bash "$script" finalize > /dev/null 2>&1
check "finalize 无 id 时不调用 gh" "0" "$(wc -l < "$GH_LOG" | tr -d ' ')"

# 未知子命令 -> 退出码 2
bash "$script" bogus >/dev/null 2>&1
check "未知子命令退出码" "2" "$?"

echo
echo "passed=$passed failed=$failed"
[ "$failed" -eq 0 ]
