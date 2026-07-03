import { useState } from 'react'
import { Layers, Plus, Trash2, Edit2, CheckCircle2, Eye, EyeOff } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const MODEL_TYPES = ['文本', '视觉', '语音', '全模态', '向量']

const TYPE_BADGE: Record<string, string> = {
  '文本': 'bg-violet-600/15 text-violet-400 border-violet-500/20',
  '视觉': 'bg-blue-400/10 text-blue-400 border-blue-400/20',
  '语音': 'bg-emerald-400/10 text-emerald-400 border-emerald-400/20',
  '全模态': 'bg-fuchsia-400/10 text-fuchsia-400 border-fuchsia-400/20',
  '向量': 'bg-amber-400/10 text-amber-400 border-amber-400/20',
}

interface Model {
  id: string
  name: string
  type: string
  provider: string
  baseUrl: string
  modelId: string
  createdAt: string
}

const MOCK_MODELS: Model[] = [
  { id: 'md1', name: '生产文本模型', type: '文本', provider: 'Anthropic', baseUrl: 'https://api.anthropic.com', modelId: 'claude-sonnet-4-6', createdAt: '2025-06-01' },
  { id: 'md2', name: '快速文本模型', type: '文本', provider: 'Anthropic', baseUrl: 'https://api.anthropic.com', modelId: 'claude-haiku-4-5', createdAt: '2025-06-01' },
  { id: 'md3', name: '语音识别', type: '语音', provider: 'Aliyun', baseUrl: 'https://dashscope.aliyuncs.com', modelId: 'paraformer-realtime-v2', createdAt: '2025-06-10' },
  { id: 'md4', name: '语音合成', type: '语音', provider: 'Aliyun', baseUrl: 'https://dashscope.aliyuncs.com', modelId: 'cosyvoice-v2', createdAt: '2025-06-10' },
  { id: 'md5', name: '文本向量化', type: '向量', provider: 'OpenAI', baseUrl: 'https://api.openai.com', modelId: 'text-embedding-3-small', createdAt: '2025-06-15' },
]

export default function MyModelsPage() {
  const [models, setModels] = useState<Model[]>(MOCK_MODELS)
  const [addOpen, setAddOpen] = useState(false)
  const [showKey, setShowKey] = useState(false)
  const [activeType, setActiveType] = useState('文本')
  const [form, setForm] = useState({ name: '', type: '文本', provider: '', baseUrl: '', modelId: '', apiKey: '' })

  const handleAdd = () => {
    if (!form.name.trim() || !form.modelId.trim()) return
    setModels((prev) => [
      {
        id: `md_${Date.now()}`,
        name: form.name,
        type: form.type,
        provider: form.provider,
        baseUrl: form.baseUrl,
        modelId: form.modelId,
        createdAt: new Date().toISOString().slice(0, 10),
      },
      ...prev,
    ])
    setForm({ name: '', type: '文本', provider: '', baseUrl: '', modelId: '', apiKey: '' })
    setAddOpen(false)
  }

  const filtered = models.filter((m) => m.type === activeType)

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">我的模型</h1>
            <p className="text-sm text-zinc-500 mt-0.5">添加和管理自托管或第三方 AI 模型</p>
          </div>
          <Button
            onClick={() => setAddOpen(true)}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            添加模型
          </Button>
        </div>
      </div>

      <div className="px-8 py-6">
        <Tabs value={activeType} onValueChange={setActiveType}>
          <TabsList className="bg-zinc-900 border border-zinc-800 h-9 p-0.5 mb-6 gap-0">
            {MODEL_TYPES.map((t) => (
              <TabsTrigger key={t} value={t} className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4">
                {t}
                <span className="ml-1.5 text-[10px] text-zinc-600">
                  ({models.filter((m) => m.type === t).length})
                </span>
              </TabsTrigger>
            ))}
          </TabsList>

          {MODEL_TYPES.map((type) => (
            <TabsContent key={type} value={type}>
              {filtered.length === 0 ? (
                <div className="flex flex-col items-center py-20">
                  <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
                    <Layers className="w-6 h-6 text-zinc-600" />
                  </div>
                  <p className="text-zinc-400 text-sm">还没有{type}模型</p>
                  <p className="text-zinc-600 text-xs mt-1 mb-4">添加兼容 OpenAI 接口的{type}模型</p>
                  <Button onClick={() => { setForm((f) => ({ ...f, type })); setAddOpen(true) }} className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5">
                    <Plus className="w-3.5 h-3.5" />添加{type}模型
                  </Button>
                </div>
              ) : (
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                  {filtered.map((model) => (
                    <div key={model.id} className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group">
                      <div className="flex items-start justify-between mb-3">
                        <div>
                          <p className="font-medium text-sm text-white">{model.name}</p>
                          <div className="flex items-center gap-2 mt-1">
                            <span className={`text-[10px] px-1.5 py-0.5 rounded border ${TYPE_BADGE[model.type]}`}>{model.type}</span>
                            <span className="text-[11px] text-zinc-500">{model.provider}</span>
                          </div>
                        </div>
                        <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                          <button className="text-zinc-500 hover:text-zinc-300 p-1.5 rounded hover:bg-zinc-800 cursor-pointer transition-colors">
                            <Edit2 className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={() => setModels((prev) => prev.filter((m) => m.id !== model.id))}
                            className="text-zinc-500 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 cursor-pointer transition-colors"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>

                      <div className="space-y-1.5">
                        <div className="bg-zinc-800/60 rounded-lg px-3 py-2">
                          <p className="text-[10px] text-zinc-600 mb-0.5">Model ID</p>
                          <p className="text-xs text-zinc-300 font-mono truncate">{model.modelId}</p>
                        </div>
                        <div className="bg-zinc-800/60 rounded-lg px-3 py-2">
                          <p className="text-[10px] text-zinc-600 mb-0.5">Base URL</p>
                          <p className="text-xs text-zinc-500 font-mono truncate">{model.baseUrl}</p>
                        </div>
                      </div>

                      <div className="flex items-center justify-between mt-3">
                        <span className="flex items-center gap-1 text-[11px] text-emerald-400">
                          <CheckCircle2 className="w-3 h-3" />可用
                        </span>
                        <span className="text-[11px] text-zinc-600 font-mono">{model.createdAt}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </TabsContent>
          ))}
        </Tabs>
      </div>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Layers className="w-4 h-4 text-violet-400" />
              添加模型
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">名称</Label>
                <Input value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="模型别名" className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm focus-visible:ring-violet-500" />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">类型</Label>
                <select value={form.type} onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))} className="w-full h-9 rounded-md bg-zinc-800 border border-zinc-700 text-white text-sm px-2.5 focus:outline-none focus:ring-1 focus:ring-violet-500 cursor-pointer">
                  {MODEL_TYPES.map((t) => <option key={t} value={t}>{t}模型</option>)}
                </select>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">提供商</Label>
                <Input value={form.provider} onChange={(e) => setForm((f) => ({ ...f, provider: e.target.value }))} placeholder="Anthropic / OpenAI..." className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm focus-visible:ring-violet-500" />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">Model ID</Label>
                <Input value={form.modelId} onChange={(e) => setForm((f) => ({ ...f, modelId: e.target.value }))} placeholder="claude-sonnet-4-6" className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm font-mono focus-visible:ring-violet-500" />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">Base URL</Label>
              <Input value={form.baseUrl} onChange={(e) => setForm((f) => ({ ...f, baseUrl: e.target.value }))} placeholder="https://api.anthropic.com" className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm font-mono focus-visible:ring-violet-500" />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">API Key</Label>
              <div className="relative">
                <Input
                  type={showKey ? 'text' : 'password'}
                  value={form.apiKey}
                  onChange={(e) => setForm((f) => ({ ...f, apiKey: e.target.value }))}
                  placeholder="sk-••••••••"
                  className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm font-mono focus-visible:ring-violet-500 pr-10"
                />
                <button onClick={() => setShowKey((v) => !v)} className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300 cursor-pointer transition-colors">
                  {showKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">取消</Button>
            <Button onClick={handleAdd} disabled={!form.name.trim() || !form.modelId.trim()} className="bg-violet-600 hover:bg-violet-500 text-white">添加模型</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
