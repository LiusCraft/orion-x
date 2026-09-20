import { useEffect, useMemo, useState } from "react";
import {
  Key,
  Plus,
  Copy,
  Trash2,
  CheckCircle2,
  Shield,
  AlertCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiKeyApi, type ApiKey, type ApiKeyScope } from "@/lib/api";

// 取值与后端 internal/apikey 的 Scope 常量一一对应，顺序就是这里的展示顺序。
const SCOPE_META: Record<string, { label: string; hint: string; badge: string }> =
  {
    "apikey:scope:all": {
      label: "全部权限",
      hint: "可读可写：智能体 / MCP / 数据",
      badge: "bg-violet-600/15 text-violet-400 border-violet-500/20",
    },
    "apikey:scope:read": {
      label: "只读",
      hint: "只放行 GET 等安全方法",
      badge: "bg-zinc-700/50 text-zinc-400 border-zinc-700",
    },
    "apikey:scope:agent": {
      label: "智能体调用",
      hint: "智能体、设备、智能体模板",
      badge: "bg-blue-400/10 text-blue-400 border-blue-400/20",
    },
    "apikey:scope:mcp": {
      label: "MCP 调用",
      hint: "MCP 市场、私有 MCP 服务与工具调用",
      badge: "bg-emerald-400/10 text-emerald-400 border-emerald-400/20",
    },
    "apikey:scope:data": {
      label: "数据读取",
      hint: "记忆、知识库与资源",
      badge: "bg-amber-400/10 text-amber-400 border-amber-400/20",
    },
    "apikey:scope:voice": {
      label: "语音接入",
      hint: "连接 wsserver（小智 WS）时的设备凭据",
      badge: "bg-cyan-400/10 text-cyan-400 border-cyan-400/20",
    },
  };

const SCOPE_VALUES = Object.keys(SCOPE_META) as ApiKeyScope[];

function scopeMeta(scope: string) {
  return (
    SCOPE_META[scope] ?? {
      label: scope,
      hint: "",
      badge: "bg-zinc-800 text-zinc-400 border-zinc-700",
    }
  );
}

function errorMessage(err: unknown, fallback: string): string {
  return (
    (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
    fallback
  );
}

function formatDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleDateString("zh-CN");
}

function formatLastUsed(iso?: string): string {
  if (!iso) return "从未使用";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "—";
  const diff = Date.now() - then;
  if (diff < 0) return formatDate(iso);
  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff < minute) return "刚刚";
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < 30 * day) return `${Math.floor(diff / day)} 天前`;
  return formatDate(iso);
}

export default function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [newCreated, setNewCreated] = useState<string | null>(null);
  const [form, setForm] = useState<{ name: string; scopes: ApiKeyScope[] }>({
    name: "",
    scopes: ["apikey:scope:all"],
  });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");
  const [copied, setCopied] = useState<string | null>(null);
  const [revealTarget, setRevealTarget] = useState<ApiKey | null>(null);
  const [revealPassword, setRevealPassword] = useState("");
  const [revealError, setRevealError] = useState("");
  const [revealing, setRevealing] = useState(false);
  // 只在“浏览器不让写剪贴板”时才填：留个手动复制的出口，总比把明文弄丢好。
  const [revealedKey, setRevealedKey] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ApiKey | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");

  const load = () => {
    setLoading(true);
    apiKeyApi
      .list()
      .then((res) => {
        setKeys(res.data);
        setLoadError("");
      })
      .catch((err) => setLoadError(errorMessage(err, "加载 API Key 失败")))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    load();
  }, []);

  const openCreate = () => {
    setForm({ name: "", scopes: ["apikey:scope:all"] });
    setFormError("");
    setCreateOpen(true);
  };

  const toggleScope = (scope: ApiKeyScope) => {
    setForm((f) => ({
      ...f,
      scopes: f.scopes.includes(scope)
        ? f.scopes.filter((s) => s !== scope)
        : [...f.scopes, scope],
    }));
  };

  const handleCreate = async () => {
    if (!form.name.trim() || form.scopes.length === 0) return;
    setSaving(true);
    setFormError("");
    try {
      const res = await apiKeyApi.create(form.name.trim(), form.scopes);
      setNewCreated(res.data.key ?? null);
      setCreateOpen(false);
      load();
    } catch (err) {
      setFormError(errorMessage(err, "创建失败，请重试"));
    } finally {
      setSaving(false);
    }
  };

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopied(id);
    setTimeout(() => setCopied(null), 2000);
  };

  const closeReveal = () => {
    setRevealTarget(null);
    setRevealPassword("");
    setRevealError("");
    setRevealedKey(null);
  };

  const openReveal = (key: ApiKey) => {
    setRevealTarget(key);
    setRevealPassword("");
    setRevealError("");
    setRevealedKey(null);
  };

  // 复制明文：先验账号密码，服务端才会解封密钥。拿到后优先写剪贴板并关掉弹窗；
  // 浏览器拒绝剪贴板（非 https 且非 localhost 的部署上有这种情况）时把明文摊在
  // 弹窗里让用户自己复制。
  const handleReveal = async () => {
    if (!revealTarget || !revealPassword) return;
    setRevealing(true);
    setRevealError("");
    try {
      const res = await apiKeyApi.reveal(revealTarget.id, revealPassword);
      try {
        await navigator.clipboard.writeText(res.data.key);
      } catch {
        setRevealedKey(res.data.key);
        setRevealError("浏览器拒绝了剪贴板写入，请手动复制下面的密钥。");
        return;
      }
      setCopied(revealTarget.id);
      setTimeout(() => setCopied(null), 2000);
      closeReveal();
    } catch (err) {
      const status = (err as { response?: { status?: number } })?.response
        ?.status;
      if (status === 403) {
        setRevealError("密码不正确");
      } else if (status === 409) {
        setRevealError("该账号没有密码（OAuth 登录），请先在账户页设置密码");
      } else {
        setRevealError(errorMessage(err, "验证失败，请重试"));
      }
    } finally {
      setRevealing(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    setDeleteError("");
    try {
      await apiKeyApi.remove(deleteTarget.id);
      setKeys((prev) => prev.filter((k) => k.id !== deleteTarget.id));
      setDeleteTarget(null);
    } catch (err) {
      setDeleteError(errorMessage(err, "删除失败，请重试"));
    } finally {
      setDeleting(false);
    }
  };

  // 选定范围里的命名空间提示：让“这个 Key 到底能碰什么”一目了然。
  const selectedHints = useMemo(
    () =>
      form.scopes.includes("apikey:scope:all")
        ? ["可读可写智能体 / MCP / 数据三类接口，并可接入 wsserver"]
        : form.scopes.map((s) => scopeMeta(s).hint).filter(Boolean),
    [form.scopes],
  );

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">API Keys</h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              管理用于调用 Orion-X API 的访问密钥
            </p>
          </div>
          <Button
            onClick={openCreate}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            新建 Key
          </Button>
        </div>
      </div>

      <div className="px-8 py-6 space-y-4">
        {/* New key banner — 明文只在这里出现一次 */}
        {newCreated && (
          <div className="bg-emerald-400/5 border border-emerald-400/20 rounded-xl px-5 py-4 flex items-center gap-3">
            <CheckCircle2 className="w-5 h-5 text-emerald-400 shrink-0" />
            <div className="flex-1 min-w-0">
              <p className="text-sm text-white font-medium mb-1">
                API Key 已创建，请立即复制保存
              </p>
              <p className="text-xs font-mono text-emerald-300 truncate">
                {newCreated}
              </p>
            </div>
            <Button
              size="sm"
              onClick={() => handleCopy(newCreated, "new")}
              className="h-7 px-3 text-xs bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-400 border border-emerald-500/30 shrink-0"
              variant="outline"
            >
              {copied === "new" ? (
                <>
                  <CheckCircle2 className="w-3 h-3 mr-1" />
                  已复制
                </>
              ) : (
                <>
                  <Copy className="w-3 h-3 mr-1" />
                  复制
                </>
              )}
            </Button>
            <button
              onClick={() => setNewCreated(null)}
              className="text-zinc-500 hover:text-zinc-300 cursor-pointer ml-2 text-lg leading-none"
            >
              ×
            </button>
          </div>
        )}

        {loadError && (
          <div className="flex items-start gap-2 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3">
            <AlertCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
            <p className="text-xs text-red-400 leading-relaxed">{loadError}</p>
          </div>
        )}

        {loading ? (
          <div className="flex items-center justify-center py-20">
            <div className="w-6 h-6 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
          </div>
        ) : keys.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Key className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">还没有 API Key</p>
            <p className="text-zinc-600 text-xs mt-1 mb-4">
              创建 Key 以通过 API 调用 Orion-X
            </p>
            <Button
              onClick={openCreate}
              className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5"
            >
              <Plus className="w-3.5 h-3.5" />
              新建 Key
            </Button>
          </div>
        ) : (
          <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-zinc-800">
                  {[
                    "名称",
                    "Key（脱敏）",
                    "权限范围",
                    "调用次数",
                    "最后使用",
                    "创建时间",
                    "操作",
                  ].map((h) => (
                    <th
                      key={h}
                      className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider"
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {keys.map((k, i) => (
                  <tr
                    key={k.id}
                    className={`border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors ${i === keys.length - 1 ? "border-0" : ""}`}
                  >
                    <td className="px-4 py-3.5">
                      <div className="flex items-center gap-2">
                        <div className="w-7 h-7 rounded-lg bg-zinc-800 border border-zinc-700/50 flex items-center justify-center">
                          <Key
                            className="w-3.5 h-3.5 text-zinc-500"
                            strokeWidth={1.5}
                          />
                        </div>
                        <span className="text-sm text-white font-medium">
                          {k.name}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-mono text-zinc-400">
                          {k.masked_key || "—"}
                        </span>
                        <button
                          onClick={() => openReveal(k)}
                          title="复制完整密钥（需要验证账号密码）"
                          className="text-zinc-600 hover:text-zinc-300 cursor-pointer transition-colors"
                        >
                          {copied === k.id ? (
                            <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
                          ) : (
                            <Copy className="w-3.5 h-3.5" />
                          )}
                        </button>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="flex flex-wrap gap-1">
                        {k.scopes.length === 0 ? (
                          <span className="text-xs text-zinc-600">—</span>
                        ) : (
                          k.scopes.map((scope) => (
                            <span
                              key={scope}
                              title={scopeMeta(scope).hint}
                              className={`text-[11px] px-2 py-0.5 rounded border ${scopeMeta(scope).badge}`}
                            >
                              {scopeMeta(scope).label}
                            </span>
                          ))
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">
                      {k.call_count.toLocaleString()}
                    </td>
                    <td
                      className="px-4 py-3.5 text-xs text-zinc-500"
                      title={k.last_used_at ?? ""}
                    >
                      {formatLastUsed(k.last_used_at)}
                    </td>
                    <td className="px-4 py-3.5 text-xs text-zinc-600 font-mono">
                      {formatDate(k.created_at)}
                    </td>
                    <td className="px-4 py-3.5">
                      <button
                        onClick={() => {
                          setDeleteError("");
                          setDeleteTarget(k);
                        }}
                        className="text-zinc-600 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 transition-all cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div className="bg-zinc-900/50 border border-zinc-800 rounded-xl p-4 flex items-start gap-3">
          <Shield
            className="w-4 h-4 text-zinc-500 shrink-0 mt-0.5"
            strokeWidth={1.5}
          />
          <div className="text-xs text-zinc-500 leading-relaxed space-y-1">
            <p>
              列表页只显示脱敏串。点行尾的复制按钮需要验证账号密码，通过后明文直接进剪贴板（只有浏览器拒绝剪贴板时才临时显示）。
            </p>
            <p>
              Key 只能触达智能体、MCP 与数据三类接口，或作为 wsserver 的接入凭据（语音接入）；供应商密钥、模型、计费与密钥管理仍需要控制台登录。
            </p>
            <p className="font-mono text-zinc-600">
              curl -H "Authorization: Bearer ox:sk:..." .../api/voicebots
              &nbsp;·&nbsp; 也支持 X-API-Key 头
            </p>
          </div>
        </div>
      </div>

      {/* Create Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Key className="w-4 h-4 text-violet-400" />
              新建 API Key
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                名称
              </Label>
              <Input
                value={form.name}
                onChange={(e) =>
                  setForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="例：生产环境"
                autoFocus
                onKeyDown={(e) => e.key === "Enter" && handleCreate()}
              />
            </div>
            <div className="space-y-2">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                权限范围
              </Label>
              <div className="flex flex-wrap gap-1.5">
                {SCOPE_VALUES.map((scope) => {
                  const meta = scopeMeta(scope);
                  const active = form.scopes.includes(scope);
                  return (
                    <button
                      key={scope}
                      type="button"
                      title={meta.hint}
                      onClick={() => toggleScope(scope)}
                      className={`text-[11px] px-2.5 py-1 rounded border transition-colors cursor-pointer ${
                        active
                          ? meta.badge
                          : "bg-zinc-900 text-zinc-500 border-zinc-800 hover:border-zinc-700 hover:text-zinc-400"
                      }`}
                    >
                      {meta.label}
                    </button>
                  );
                })}
              </div>
              <p className="text-[11px] text-zinc-600 leading-relaxed">
                {form.scopes.length === 0
                  ? "至少选择一个权限范围"
                  : selectedHints.join("；")}
              </p>
            </div>
            {formError && (
              <p className="text-xs text-red-400 leading-relaxed">{formError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setCreateOpen(false)}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
            >
              取消
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!form.name.trim() || form.scopes.length === 0 || saving}
              className="bg-violet-600 hover:bg-violet-500 text-white"
            >
              {saving ? "创建中..." : "创建"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reveal & copy (password re-auth) */}
      <Dialog open={!!revealTarget} onOpenChange={closeReveal}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Key className="w-4 h-4 text-violet-400" />
              复制 API Key
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <p className="text-sm text-zinc-400">
              确认是本人操作：输入账号密码后，
              {revealTarget ? `「${revealTarget.name}」` : "该 Key"}
              的完整明文会直接复制到剪贴板。
            </p>
            {revealedKey ? (
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  密钥明文
                </Label>
                <Input
                  readOnly
                  value={revealedKey}
                  onFocus={(e) => e.currentTarget.select()}
                  className="font-mono text-xs"
                />
              </div>
            ) : (
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  账号密码
                </Label>
                <Input
                  type="password"
                  value={revealPassword}
                  onChange={(e) => setRevealPassword(e.target.value)}
                  placeholder="登录密码"
                  autoFocus
                  onKeyDown={(e) => e.key === "Enter" && handleReveal()}
                />
              </div>
            )}
            {revealError && (
              <p className="text-xs text-red-400 leading-relaxed">
                {revealError}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={closeReveal}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
            >
              {revealedKey ? "关闭" : "取消"}
            </Button>
            {!revealedKey && (
              <Button
                onClick={handleReveal}
                disabled={!revealPassword || revealing}
                className="bg-violet-600 hover:bg-violet-500 text-white"
              >
                {revealing ? "验证中..." : "验证并复制"}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirm */}
      <Dialog
        open={!!deleteTarget}
        onOpenChange={() => {
          setDeleteError("");
          setDeleteTarget(null);
        }}
      >
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
          <DialogHeader>
            <DialogTitle className="text-white">删除 API Key</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-zinc-400 py-2">
            删除
            {deleteTarget ? `「${deleteTarget.name}」` : ""}
            后使用此 Key 的服务将立即无法调用，此操作不可撤销。
          </p>
          {deleteError && (
            <p className="text-xs text-red-400 leading-relaxed">{deleteError}</p>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setDeleteError("");
                setDeleteTarget(null);
              }}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
            >
              取消
            </Button>
            <Button
              onClick={handleDelete}
              disabled={deleting}
              className="bg-red-600 hover:bg-red-500 text-white"
            >
              {deleting ? "删除中..." : "删除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
