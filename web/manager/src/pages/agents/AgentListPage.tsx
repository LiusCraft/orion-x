import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { voicebotApi, modelApi, voiceApi, languageApi, type Voicebot } from '@/lib/api'
import { Bot, Plus, Settings, Cpu, Mic, Volume2, Sparkles, Brain, LayoutGrid, List } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface ParsedConfig {
  llmModel: string
  asrModel: string
  ttsVoice: string
  memoryMode: string
  mcpCount: number
  language: string
  prompt: string
  vadMode: string
}

function parseConfig(configJson: string): ParsedConfig {
  try {
    const c = JSON.parse(configJson)
    if (c.llm?.model_id || c.asr?.model_id) {
      return {
        llmModel: c.llm?.model_id || '',
        asrModel: c.asr?.model_id || '',
        ttsVoice: c.tts?.voice_id || '',
        memoryMode: c.memory?.mode || 'session',
        mcpCount: Array.isArray(c.mcp) ? c.mcp.length : 0,
        language: c.language || '',
        prompt: c.llm?.prompt || '',
        vadMode: c.asr?.vad_mode || '',
      }
    }
    return {
      llmModel: c?.provider?.llm?.openai?.model ?? '',
      asrModel: c?.provider?.asr?.aliyun?.model ?? '',
      ttsVoice: c?.provider?.tts?.aliyun?.voice ?? '',
      memoryMode: c?.memory?.mode ?? 'session',
      mcpCount: Array.isArray(c?.tools?.mcp) ? c.tools.mcp.length : 0,
      language: '',
      prompt: '',
      vadMode: '',
    }
  } catch {
    return { llmModel: '', asrModel: '', ttsVoice: '', memoryMode: 'session', mcpCount: 0, language: '', prompt: '', vadMode: '' }
  }
}

const MEMORY_LABEL: Record<string, string> = {
  none: '无记忆',
  session: '会话记忆',
  long_term: '长期记忆',
}

export default function AgentListPage() {
  const [bots, setBots] = useState<Voicebot[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const [modelMap, setModelMap] = useState<Record<string, string>>({})
  const [voiceMap, setVoiceMap] = useState<Record<string, string>>({})
  const [langMap, setLangMap] = useState<Record<string, string>>({})
  const [search, setSearch] = useState('')
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(() => {
    return (localStorage.getItem('agentViewMode') as 'grid' | 'list') || 'grid'
  })
  const navigate = useNavigate()

  const toggleView = (mode: 'grid' | 'list') => {
    setViewMode(mode)
    localStorage.setItem('agentViewMode', mode)
  }

  const filtered = bots.filter(bot =>
    !search.trim() || bot.name.toLowerCase().includes(search.trim().toLowerCase())
  )

  const fetchBots = async () => {
    try {
      const [botData, modelData, voiceData, langData] = await Promise.all([
        voicebotApi.list(),
        modelApi.list(),
        voiceApi.listSystem(),
        languageApi.list(),
      ])
      setBots(botData.data)
      const mm: Record<string, string> = {}
      modelData.data.forEach(m => { mm[m.id] = m.name })
      setModelMap(mm)
      const vm: Record<string, string> = {}
      voiceData.data.forEach(v => { vm[v.voice_id] = v.name; vm[v.id] = v.name })
      setVoiceMap(vm)
      const lm: Record<string, string> = {}
      langData.data.forEach(l => { lm[l.code] = l.name })
      setLangMap(lm)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchBots() }, [])

  const handleCreate = async () => {
    if (!newName.trim()) return
    setCreating(true)
    try {
      const { data } = await voicebotApi.create(newName.trim())
      setCreateOpen(false)
      setNewName('')
      navigate(`/agents/${data.id}`)
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">我的智能体</h1>
            <p className="text-sm text-zinc-500 mt-0.5">管理你的语音机器人，配置模型与语音</p>
          </div>
          <div className="flex items-center gap-3">
            <button onClick={() => toggleView('grid')}
              className={`p-1.5 rounded-md transition-colors ${viewMode === 'grid' ? 'bg-zinc-700 text-white' : 'text-zinc-500 hover:text-zinc-300'}`}>
              <LayoutGrid className="w-4 h-4" />
            </button>
            <button onClick={() => toggleView('list')}
              className={`p-1.5 rounded-md transition-colors ${viewMode === 'list' ? 'bg-zinc-700 text-white' : 'text-zinc-500 hover:text-zinc-300'}`}>
              <List className="w-4 h-4" />
            </button>
            <input type="text" value={search} onChange={e => setSearch(e.target.value)}
              placeholder="搜索智能体..."
              className="bg-zinc-800 border border-zinc-700 text-white placeholder:text-zinc-500 rounded-lg px-3 py-1.5 text-sm w-48 focus:outline-none focus:border-violet-500 transition-colors" />
            <Button
              onClick={() => setCreateOpen(true)}
              className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
            >
              <Plus className="w-4 h-4" />
              新建智能体
            </Button>
          </div>
        </div>
      </div>

      <div className="px-8 py-6">
        {loading ? (
          <div className="flex items-center justify-center gap-2 py-20 text-zinc-500 text-sm">
            <div className="w-4 h-4 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
            加载中...
          </div>
        ) : bots.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Bot className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">还没有智能体</p>
            <p className="text-zinc-600 text-xs mt-1 mb-4">创建第一个，配置 LLM / ASR / TTS 后绑定设备</p>
            <Button onClick={() => setCreateOpen(true)} className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5">
              <Plus className="w-3.5 h-3.5" />新建智能体
            </Button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Bot className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">没有匹配的智能体</p>
            <p className="text-zinc-600 text-xs mt-1">试试其他关键词</p>
          </div>
        ) : viewMode === 'grid' ? (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {filtered.map((bot) => {
              const cfg = parseConfig(bot.config_json)
              return (
                <button
                  key={bot.id}
                  onClick={() => navigate(`/agents/${bot.id}`)}
                  className="text-left bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-600 hover:bg-zinc-800/50 transition-all group"
                >
                  {/* Header */}
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex items-center gap-3">
                      <div className="w-9 h-9 rounded-xl bg-violet-600/15 border border-violet-500/20 flex items-center justify-center shrink-0">
                        <Bot className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
                      </div>
                      <div>
                        <p className="font-medium text-sm text-white leading-snug">{bot.name}</p>
                        <p className="text-[11px] text-zinc-600 font-mono mt-0.5">{bot.id.slice(0, 8)}…</p>
                      </div>
                    </div>
                    <div className="text-zinc-600 group-hover:text-zinc-400 transition-colors mt-0.5">
                      <Settings className="w-4 h-4" strokeWidth={1.5} />
                    </div>
                  </div>

                  {/* Config summary */}
                  <div className="grid grid-cols-2 gap-2 mb-3">
                    <div className="bg-zinc-800/60 rounded-lg px-2.5 py-2">
                      <div className="flex items-center gap-1 mb-0.5">
                        <Cpu className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
                        <p className="text-[10px] text-zinc-600">聊天模型</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 font-mono truncate">{modelMap[cfg.llmModel] || cfg.llmModel}</p>
                    </div>
                    <div className="bg-zinc-800/60 rounded-lg px-2.5 py-2">
                      <div className="flex items-center gap-1 mb-0.5">
                        <Volume2 className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
                        <p className="text-[10px] text-zinc-600">音色</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 font-mono truncate">{voiceMap[cfg.ttsVoice] || cfg.ttsVoice}</p>
                    </div>
                    <div className="bg-zinc-800/60 rounded-lg px-2.5 py-2">
                      <div className="flex items-center gap-1 mb-0.5">
                        <Mic className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
                        <p className="text-[10px] text-zinc-600">语音识别</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 font-mono truncate">{modelMap[cfg.asrModel] || cfg.asrModel}</p>
                    </div>
                    <div className="bg-zinc-800/60 rounded-lg px-2.5 py-2">
                      <div className="flex items-center gap-1 mb-0.5">
                        <Brain className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
                        <p className="text-[10px] text-zinc-600">记忆</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 truncate">{MEMORY_LABEL[cfg.memoryMode] ?? cfg.memoryMode}</p>
                    </div>
                  </div>

                  {/* Prompt preview */}
                  {cfg.prompt && (
                    <p className="text-[11px] text-zinc-500 leading-relaxed mb-3 line-clamp-2">
                      {cfg.prompt}
                    </p>
                  )}

                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      {cfg.language && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700">
                          {langMap[cfg.language] || cfg.language}
                        </span>
                      )}
                      {cfg.vadMode && cfg.vadMode !== 'auto' && (
                        <span className={`text-[10px] px-1.5 py-0.5 rounded border
                          ${cfg.vadMode === 'manual' ? 'bg-amber-500/10 border-amber-500/20 text-amber-400' : 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400'}`}>
                          {cfg.vadMode === 'manual' ? '手动' : '实时'}
                        </span>
                      )}
                      {cfg.mcpCount > 0 && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-600/15 text-violet-400 border border-violet-500/20">
                          {cfg.mcpCount} MCP
                        </span>
                      )}
                    </div>
                    <p className="text-[11px] text-zinc-600 font-mono">
                      {new Date(bot.updated_at).toLocaleString('zh-CN', { hour12: false })}
                    </p>
                  </div>
                </button>
              )
            })}
          </div>
        ) : (
          <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-800 text-left text-[11px] text-zinc-500 uppercase tracking-wide">
                  <th className="px-4 py-3 font-medium">名称</th>
                  <th className="px-4 py-3 font-medium">聊天模型</th>
                  <th className="px-4 py-3 font-medium">音色</th>
                  <th className="px-4 py-3 font-medium">语音识别</th>
                  <th className="px-4 py-3 font-medium">语言</th>
                  <th className="px-4 py-3 font-medium">记忆</th>
                  <th className="px-4 py-3 font-medium">更新时间</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(bot => {
                  const cfg = parseConfig(bot.config_json)
                  return (
                    <tr key={bot.id} onClick={() => navigate(`/agents/${bot.id}`)}
                      className="border-b border-zinc-800/60 hover:bg-zinc-800/40 transition-colors cursor-pointer">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-3">
                          <div className="w-7 h-7 rounded-lg bg-violet-600/15 border border-violet-500/20 flex items-center justify-center shrink-0">
                            <Bot className="w-3.5 h-3.5 text-violet-400" strokeWidth={1.5} />
                          </div>
                          <div>
                            <p className="text-sm text-white">{bot.name}</p>
                            <p className="text-[10px] text-zinc-600 font-mono">{bot.id.slice(0, 8)}…</p>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-zinc-300 font-mono text-xs">{modelMap[cfg.llmModel] || cfg.llmModel || '—'}</td>
                      <td className="px-4 py-3 text-zinc-300 font-mono text-xs">{voiceMap[cfg.ttsVoice] || cfg.ttsVoice || '—'}</td>
                      <td className="px-4 py-3 text-zinc-300 font-mono text-xs">{modelMap[cfg.asrModel] || cfg.asrModel || '—'}</td>
                      <td className="px-4 py-3">
                        {cfg.language && <span className="text-[11px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700">{langMap[cfg.language] || cfg.language}</span>}
                      </td>
                      <td className="px-4 py-3 text-xs text-zinc-400">{MEMORY_LABEL[cfg.memoryMode] ?? cfg.memoryMode}</td>
                      <td className="px-4 py-3 text-xs text-zinc-600 font-mono">{new Date(bot.updated_at).toLocaleString('zh-CN', { hour12: false })}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Sparkles className="w-4 h-4 text-violet-400" />
              新建智能体
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">名称</Label>
              <Input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="客厅助手 / 车载音箱..."
                className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500"
                onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
                autoFocus
              />
            </div>
            <p className="text-xs text-zinc-600">创建后进入配置页面设置 LLM、ASR、TTS 等参数</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">
              取消
            </Button>
            <Button onClick={handleCreate} disabled={creating || !newName.trim()} className="bg-violet-600 hover:bg-violet-500 text-white">
              {creating ? '创建中...' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
