import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { voicebotApi, type Voicebot } from '@/lib/api'
import { Bot, Plus, Settings, Cpu, Mic, Volume2, Sparkles, Brain } from 'lucide-react'
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
}

function parseConfig(configJson: string): ParsedConfig {
  try {
    const c = JSON.parse(configJson)
    return {
      llmModel: c?.provider?.llm?.openai?.model ?? '—',
      asrModel: c?.provider?.asr?.aliyun?.model ?? '—',
      ttsVoice: c?.provider?.tts?.aliyun?.voice ?? '—',
      memoryMode: c?.memory?.mode ?? 'session',
      mcpCount: Array.isArray(c?.tools?.mcp) ? c.tools.mcp.length : 0,
    }
  } catch {
    return { llmModel: '—', asrModel: '—', ttsVoice: '—', memoryMode: 'session', mcpCount: 0 }
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
  const navigate = useNavigate()

  const fetchBots = async () => {
    try {
      const { data } = await voicebotApi.list()
      setBots(data)
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
            <p className="text-sm text-zinc-500 mt-0.5">管理语音智能体实例及其 LLM / ASR / TTS 配置</p>
          </div>
          <Button
            onClick={() => setCreateOpen(true)}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            新建智能体
          </Button>
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
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {bots.map((bot) => {
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
                        <p className="text-[10px] text-zinc-600">LLM</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 font-mono truncate">{cfg.llmModel}</p>
                    </div>
                    <div className="bg-zinc-800/60 rounded-lg px-2.5 py-2">
                      <div className="flex items-center gap-1 mb-0.5">
                        <Mic className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
                        <p className="text-[10px] text-zinc-600">ASR</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 font-mono truncate">{cfg.asrModel}</p>
                    </div>
                    <div className="bg-zinc-800/60 rounded-lg px-2.5 py-2">
                      <div className="flex items-center gap-1 mb-0.5">
                        <Volume2 className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
                        <p className="text-[10px] text-zinc-600">TTS 音色</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 font-mono truncate">{cfg.ttsVoice}</p>
                    </div>
                    <div className="bg-zinc-800/60 rounded-lg px-2.5 py-2">
                      <div className="flex items-center gap-1 mb-0.5">
                        <Brain className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
                        <p className="text-[10px] text-zinc-600">记忆</p>
                      </div>
                      <p className="text-[11px] text-zinc-300 truncate">{MEMORY_LABEL[cfg.memoryMode] ?? cfg.memoryMode}</p>
                    </div>
                  </div>

                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      {cfg.mcpCount > 0 && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-600/15 text-violet-400 border border-violet-500/20">
                          {cfg.mcpCount} MCP
                        </span>
                      )}
                    </div>
                    <p className="text-[11px] text-zinc-600 font-mono">
                      {new Date(bot.created_at).toLocaleDateString('zh-CN')}
                    </p>
                  </div>
                </button>
              )
            })}
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
