import { useState } from 'react'
import { Key, Plus, Copy, Trash2, Eye, EyeOff, CheckCircle2, Shield } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const SCOPES = ['全部权限', '只读', '智能体调用', 'MCP 调用', '数据读取']

interface ApiKey {
  id: string
  name: string
  key: string
  scope: string
  createdAt: string
  lastUsed: string
  calls: number
}

const MOCK_KEYS: ApiKey[] = [
  { id: 'k1', name: '生产环境', key: 'ox_sk_prod_8f2a1c3d...e9b4', scope: '全部权限', createdAt: '2025-06-01', lastUsed: '2 分钟前', calls: 28420 },
  { id: 'k2', name: '开发测试', key: 'ox_sk_dev_4e7f2a1b...c3d8', scope: '智能体调用', createdAt: '2025-06-15', lastUsed: '1 小时前', calls: 1240 },
  { id: 'k3', name: 'CI/CD 流水线', key: 'ox_sk_ci_2b9a4f1c...7d2e', scope: '只读', createdAt: '2025-07-01', lastUsed: '昨天', calls: 84 },
]

export default function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>(MOCK_KEYS)
  const [createOpen, setCreateOpen] = useState(false)
  const [newCreated, setNewCreated] = useState<string | null>(null)
  const [form, setForm] = useState({ name: '', scope: '全部权限' })
  const [visibleId, setVisibleId] = useState<string | null>(null)
  const [copied, setCopied] = useState<string | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const handleCreate = () => {
    if (!form.name.trim()) return
    const fullKey = `ox_sk_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 14)}`
    const newKey: ApiKey = {
      id: `k_${Date.now()}`,
      name: form.name.trim(),
      key: `${fullKey.slice(0, 20)}...${fullKey.slice(-4)}`,
      scope: form.scope,
      createdAt: new Date().toISOString().slice(0, 10),
      lastUsed: '从未使用',
      calls: 0,
    }
    setKeys((prev) => [newKey, ...prev])
    setNewCreated(fullKey)
    setForm({ name: '', scope: '全部权限' })
    setCreateOpen(false)
  }

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text)
    setCopied(id)
    setTimeout(() => setCopied(null), 2000)
  }

  const SCOPE_BADGE: Record<string, string> = {
    '全部权限': 'bg-violet-600/15 text-violet-400 border-violet-500/20',
    '只读': 'bg-zinc-700/50 text-zinc-400 border-zinc-700',
    '智能体调用': 'bg-blue-400/10 text-blue-400 border-blue-400/20',
    'MCP 调用': 'bg-emerald-400/10 text-emerald-400 border-emerald-400/20',
    '数据读取': 'bg-amber-400/10 text-amber-400 border-amber-400/20',
  }

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">API Keys</h1>
            <p className="text-sm text-zinc-500 mt-0.5">管理用于调用 Orion-X API 的访问密钥</p>
          </div>
          <Button
            onClick={() => setCreateOpen(true)}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            新建 Key
          </Button>
        </div>
      </div>

      <div className="px-8 py-6 space-y-4">
        {/* New key banner */}
        {newCreated && (
          <div className="bg-emerald-400/5 border border-emerald-400/20 rounded-xl px-5 py-4 flex items-center gap-3">
            <CheckCircle2 className="w-5 h-5 text-emerald-400 shrink-0" />
            <div className="flex-1 min-w-0">
              <p className="text-sm text-white font-medium mb-1">API Key 已创建，请立即复制保存</p>
              <p className="text-xs font-mono text-emerald-300 truncate">{newCreated}</p>
            </div>
            <Button
              size="sm"
              onClick={() => handleCopy(newCreated, 'new')}
              className="h-7 px-3 text-xs bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-400 border border-emerald-500/30 shrink-0"
              variant="outline"
            >
              {copied === 'new' ? <><CheckCircle2 className="w-3 h-3 mr-1" />已复制</> : <><Copy className="w-3 h-3 mr-1" />复制</>}
            </Button>
            <button onClick={() => setNewCreated(null)} className="text-zinc-500 hover:text-zinc-300 cursor-pointer ml-2 text-lg leading-none">×</button>
          </div>
        )}

        {keys.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Key className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">还没有 API Key</p>
            <p className="text-zinc-600 text-xs mt-1 mb-4">创建 Key 以通过 API 调用 Orion-X</p>
            <Button onClick={() => setCreateOpen(true)} className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5">
              <Plus className="w-3.5 h-3.5" />新建 Key
            </Button>
          </div>
        ) : (
          <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-zinc-800">
                  {['名称', 'Key（脱敏）', '权限范围', '调用次数', '最后使用', '创建时间', '操作'].map((h) => (
                    <th key={h} className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {keys.map((k, i) => (
                  <tr key={k.id} className={`border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors ${i === keys.length - 1 ? 'border-0' : ''}`}>
                    <td className="px-4 py-3.5">
                      <div className="flex items-center gap-2">
                        <div className="w-7 h-7 rounded-lg bg-zinc-800 border border-zinc-700/50 flex items-center justify-center">
                          <Key className="w-3.5 h-3.5 text-zinc-500" strokeWidth={1.5} />
                        </div>
                        <span className="text-sm text-white font-medium">{k.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-mono text-zinc-400">
                          {visibleId === k.id ? k.key : k.key.replace(/[^.]/g, '•').slice(0, 20) + '...'}
                        </span>
                        <button
                          onClick={() => setVisibleId((v) => (v === k.id ? null : k.id))}
                          className="text-zinc-600 hover:text-zinc-400 cursor-pointer transition-colors"
                        >
                          {visibleId === k.id ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                        </button>
                        <button
                          onClick={() => handleCopy(k.key, k.id)}
                          className="text-zinc-600 hover:text-zinc-400 cursor-pointer transition-colors"
                        >
                          {copied === k.id ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                        </button>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <span className={`text-[11px] px-2 py-0.5 rounded border ${SCOPE_BADGE[k.scope] ?? 'bg-zinc-800 text-zinc-400 border-zinc-700'}`}>
                        {k.scope}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{k.calls.toLocaleString()}</td>
                    <td className="px-4 py-3.5 text-xs text-zinc-500">{k.lastUsed}</td>
                    <td className="px-4 py-3.5 text-xs text-zinc-600 font-mono">{k.createdAt}</td>
                    <td className="px-4 py-3.5">
                      <button
                        onClick={() => setDeleteId(k.id)}
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
          <Shield className="w-4 h-4 text-zinc-500 shrink-0 mt-0.5" strokeWidth={1.5} />
          <p className="text-xs text-zinc-500 leading-relaxed">
            API Key 创建后仅显示一次，请立即复制保存到安全位置。不要将 Key 提交到代码库或分享给他人。如发现泄露请立即删除并重建。
          </p>
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
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">名称</Label>
              <Input value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="例：生产环境" className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500" autoFocus onKeyDown={(e) => e.key === 'Enter' && handleCreate()} />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">权限范围</Label>
              <select value={form.scope} onChange={(e) => setForm((f) => ({ ...f, scope: e.target.value }))} className="w-full h-9 rounded-md bg-zinc-800 border border-zinc-700 text-white text-sm px-2.5 focus:outline-none focus:ring-1 focus:ring-violet-500 cursor-pointer">
                {SCOPES.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">取消</Button>
            <Button onClick={handleCreate} disabled={!form.name.trim()} className="bg-violet-600 hover:bg-violet-500 text-white">创建</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirm */}
      <Dialog open={!!deleteId} onOpenChange={() => setDeleteId(null)}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
          <DialogHeader>
            <DialogTitle className="text-white">删除 API Key</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-zinc-400 py-2">删除后使用此 Key 的服务将立即无法调用，此操作不可撤销。</p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteId(null)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800">取消</Button>
            <Button onClick={() => { setKeys((prev) => prev.filter((k) => k.id !== deleteId)); setDeleteId(null) }} className="bg-red-600 hover:bg-red-500 text-white">删除</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
