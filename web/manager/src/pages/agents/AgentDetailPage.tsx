import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { voicebotApi, deviceApi, type Device } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { ChevronLeft, Plus, Trash2 } from 'lucide-react'

interface MCPServer {
  id: string
  transport: 'stdio' | 'sse' | 'streamable'
  command: string
  args: string[]
  endpoint: string
  tool_name_list: string[]
  timeout_ms: number
}

interface BotConfig {
  provider: {
    llm: { openai: { api_key: string; base_url: string; model: string } }
    asr: { aliyun: { api_key: string; model: string; endpoint: string } }
    tts: { aliyun: {
      api_key: string; endpoint: string; model: string; voice: string
      volume: number; rate: number; pitch: number
      voice_map: Record<string, string>
    }}
  }
  audio: { in_pipe: { enable_vad: boolean; vad_threshold: number; vad_min_silence_ms: number; vad_speech_pad_ms: number; sample_rate: number } }
  tools: { mcp: MCPServer[] }
  memory: { mode: string; session_max_turns: number; session_summary_every_n: number; long_term_db_path: string; long_term_max_results: number; retention_days: number }
}

const DC: BotConfig = {
  provider: {
    llm: { openai: { api_key: '', base_url: 'https://open.bigmodel.cn/api/coding/paas/v4', model: 'glm-4-flash' } },
    asr: { aliyun: { api_key: '', model: 'fun-asr-realtime', endpoint: 'wss://dashscope.aliyuncs.com/api-ws/v1/inference' } },
    tts: { aliyun: { api_key: '', endpoint: 'wss://dashscope.aliyuncs.com/api-ws/v1/inference', model: 'cosyvoice-v3-flash', voice: 'longanyang', volume: 50, rate: 1.0, pitch: 1.0, voice_map: { happy: 'longanyang', sad: 'zhichu', angry: 'zhimeng', calm: 'longxiaochun', excited: 'longanyang', default: 'longanyang' } } },
  },
  audio: { in_pipe: { enable_vad: true, vad_threshold: 0.5, vad_min_silence_ms: 500, vad_speech_pad_ms: 300, sample_rate: 16000 } },
  tools: { mcp: [] },
  memory: { mode: 'session', session_max_turns: 10, session_summary_every_n: 20, long_term_db_path: 'data/memory.db', long_term_max_results: 6, retention_days: 365 },
}

function parseCfg(json: string): BotConfig {
  try {
    const c = JSON.parse(json)
    return {
      provider: {
        llm: { openai: { ...DC.provider.llm.openai, ...c?.provider?.llm?.openai } },
        asr: { aliyun: { ...DC.provider.asr.aliyun, ...c?.provider?.asr?.aliyun } },
        tts: { aliyun: { ...DC.provider.tts.aliyun, ...c?.provider?.tts?.aliyun } },
      },
      audio: { in_pipe: { ...DC.audio.in_pipe, ...c?.audio?.in_pipe } },
      tools: { mcp: Array.isArray(c?.tools?.mcp) ? c.tools.mcp : [] },
      memory: { ...DC.memory, ...c?.memory },
    }
  } catch { return structuredClone(DC) }
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs text-zinc-400 uppercase tracking-wide">{label}</Label>
      {children}
    </div>
  )
}

const inp = 'bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500 h-9'

export default function AgentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [cfg, setCfg] = useState<BotConfig>(structuredClone(DC))
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [saveStatus, setSaveStatus] = useState<'idle' | 'ok' | 'err'>('idle')
  const [saveErr, setSaveErr] = useState('')
  const [mcpOpen, setMcpOpen] = useState(false)
  const [newMcp, setNewMcp] = useState<MCPServer>({ id: '', transport: 'stdio', command: '', args: [], endpoint: '', tool_name_list: [], timeout_ms: 30000 })
  const [mcpArgsStr, setMcpArgsStr] = useState('')
  const [mcpToolsStr, setMcpToolsStr] = useState('')
  const [devOpen, setDevOpen] = useState(false)
  const [newDevId, setNewDevId] = useState('')
  const [newDevName, setNewDevName] = useState('')
  const [devAdding, setDevAdding] = useState(false)
  const [devErr, setDevErr] = useState('')

  useEffect(() => {
    if (!id) return
    Promise.all([voicebotApi.get(id), deviceApi.list(id)]).then(([b, d]) => {
      setName(b.data.name); setCfg(parseCfg(b.data.config_json)); setDevices(d.data)
    }).finally(() => setLoading(false))
  }, [id])

  const setLlm = (p: Partial<BotConfig['provider']['llm']['openai']>) =>
    setCfg(c => ({ ...c, provider: { ...c.provider, llm: { openai: { ...c.provider.llm.openai, ...p } } } }))
  const setAsr = (p: Partial<BotConfig['provider']['asr']['aliyun']>) =>
    setCfg(c => ({ ...c, provider: { ...c.provider, asr: { aliyun: { ...c.provider.asr.aliyun, ...p } } } }))
  const setTts = (p: Partial<BotConfig['provider']['tts']['aliyun']>) =>
    setCfg(c => ({ ...c, provider: { ...c.provider, tts: { aliyun: { ...c.provider.tts.aliyun, ...p } } } }))
  const setInPipe = (p: Partial<BotConfig['audio']['in_pipe']>) =>
    setCfg(c => ({ ...c, audio: { in_pipe: { ...c.audio.in_pipe, ...p } } }))
  const setMem = (p: Partial<BotConfig['memory']>) =>
    setCfg(c => ({ ...c, memory: { ...c.memory, ...p } }))

  const handleSave = async () => {
    if (!id) return
    setSaving(true); setSaveStatus('idle'); setSaveErr('')
    try {
      await voicebotApi.update(id, name, JSON.stringify(cfg))
      setSaveStatus('ok'); setTimeout(() => setSaveStatus('idle'), 2000)
    } catch (e: unknown) {
      setSaveErr((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败')
      setSaveStatus('err')
    } finally { setSaving(false) }
  }

  const handleAddMcp = () => {
    setCfg(c => ({ ...c, tools: { mcp: [...c.tools.mcp, { ...newMcp, args: mcpArgsStr.trim() ? mcpArgsStr.split(/\s+/) : [], tool_name_list: mcpToolsStr.trim() ? mcpToolsStr.split(',').map(s => s.trim()).filter(Boolean) : [] }] } }))
    setMcpOpen(false)
    setNewMcp({ id: '', transport: 'stdio', command: '', args: [], endpoint: '', tool_name_list: [], timeout_ms: 30000 })
    setMcpArgsStr(''); setMcpToolsStr('')
  }

  const handleAddDevice = async () => {
    if (!id || !newDevId.trim()) return
    setDevAdding(true); setDevErr('')
    try {
      await deviceApi.create(id, newDevId.trim(), newDevName.trim())
      const { data } = await deviceApi.list(id)
      setDevices(data); setDevOpen(false); setNewDevId(''); setNewDevName('')
    } catch (e: unknown) {
      setDevErr((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '添加失败')
    } finally { setDevAdding(false) }
  }

  const handleDeleteDevice = async (devId: string) => {
    if (!id || !confirm(`确认删除设备 ${devId}？`)) return
    await deviceApi.remove(id, devId)
    setDevices(prev => prev.filter(d => d.id !== devId))
  }

  if (loading) return (
    <div className="flex items-center justify-center py-32">
      <div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
    </div>
  )

  const llm = cfg.provider.llm.openai
  const asr = cfg.provider.asr.aliyun
  const tts = cfg.provider.tts.aliyun
  const ip = cfg.audio.in_pipe
  const mem = cfg.memory

  return (
    <div className="min-h-full">
      {/* Header */}
      <div className="border-b border-zinc-800/80 px-8 py-4 flex items-center gap-3">
        <button onClick={() => navigate('/agents')} className="text-zinc-500 hover:text-zinc-300 transition-colors">
          <ChevronLeft className="w-4 h-4" />
        </button>
        <div className="h-4 w-px bg-zinc-800" />
        <input value={name} onChange={e => setName(e.target.value)}
          className="bg-transparent text-white font-medium text-sm focus:outline-none border-b border-transparent focus:border-zinc-600 py-0.5 min-w-0 max-w-xs" />
        <span className="text-xs text-zinc-600 font-mono hidden sm:block">{id}</span>
        <div className="ml-auto flex items-center gap-2">
          {saveStatus === 'err' && <span className="text-xs text-red-400">{saveErr}</span>}
          <Button onClick={handleSave} disabled={saving}
            className={saveStatus === 'ok' ? 'bg-emerald-600 hover:bg-emerald-600 text-white h-8 px-4 text-sm' : 'bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-sm'}>
            {saving ? '保存中...' : saveStatus === 'ok' ? '已保存 ✓' : '保存'}
          </Button>
        </div>
      </div>

      <div className="px-8 py-6">
        <Tabs defaultValue="llm">
          <TabsList className="bg-zinc-900 border border-zinc-800 mb-6 h-auto flex-wrap gap-0.5 p-1">
            {(['基本', 'LLM', 'ASR', 'TTS', '音频', '记忆', 'MCP', '设备'] as const).map((t, i) => (
              <TabsTrigger key={t} value={['basic','llm','asr','tts','audio','memory','mcp','devices'][i]}
                className="data-[state=active]:bg-zinc-700 data-[state=active]:text-white text-zinc-400 text-sm h-8">{t}</TabsTrigger>
            ))}
          </TabsList>

          {/* ── 基本 ── */}
          <TabsContent value="basic" className="space-y-5 pt-1">
            <Field label="智能体名称">
              <Input value={name} onChange={e => setName(e.target.value)} className={inp} />
            </Field>
          </TabsContent>

          {/* ── LLM ── */}
          <TabsContent value="llm" className="space-y-5 pt-1">
            <Field label="API Key">
              <Input value={llm.api_key} onChange={e => setLlm({ api_key: e.target.value })} type="password" placeholder="sk-..." className={inp} />
            </Field>
            <Field label="Base URL">
              <Input value={llm.base_url} onChange={e => setLlm({ base_url: e.target.value })} placeholder="https://..." className={inp} />
            </Field>
            <Field label="模型">
              <Input value={llm.model} onChange={e => setLlm({ model: e.target.value })} placeholder="glm-4-flash" className={inp} />
            </Field>
          </TabsContent>

          {/* ── ASR ── */}
          <TabsContent value="asr" className="space-y-5 pt-1">
            <Field label="API Key">
              <Input value={asr.api_key} onChange={e => setAsr({ api_key: e.target.value })} type="password" placeholder="Dashscope API Key" className={inp} />
            </Field>
            <Field label="模型">
              <Input value={asr.model} onChange={e => setAsr({ model: e.target.value })} placeholder="fun-asr-realtime" className={inp} />
            </Field>
            <Field label="Endpoint">
              <Input value={asr.endpoint} onChange={e => setAsr({ endpoint: e.target.value })} placeholder="wss://..." className={inp} />
            </Field>
          </TabsContent>

          {/* ── TTS ── */}
          <TabsContent value="tts" className="space-y-5 pt-1">
            <div className="grid grid-cols-2 gap-4">
              <Field label="API Key">
                <Input value={tts.api_key} onChange={e => setTts({ api_key: e.target.value })} type="password" placeholder="Dashscope API Key" className={inp} />
              </Field>
              <Field label="Endpoint">
                <Input value={tts.endpoint} onChange={e => setTts({ endpoint: e.target.value })} placeholder="wss://..." className={inp} />
              </Field>
              <Field label="模型">
                <Input value={tts.model} onChange={e => setTts({ model: e.target.value })} placeholder="cosyvoice-v3-flash" className={inp} />
              </Field>
              <Field label="默认音色">
                <Input value={tts.voice} onChange={e => setTts({ voice: e.target.value })} placeholder="longanyang" className={inp} />
              </Field>
              <Field label="音量 (0-100)">
                <Input type="number" min={0} max={100} value={tts.volume} onChange={e => setTts({ volume: +e.target.value })} className={inp} />
              </Field>
              <Field label="语速 (0.5-2.0)">
                <Input type="number" min={0.5} max={2} step={0.1} value={tts.rate} onChange={e => setTts({ rate: +e.target.value })} className={inp} />
              </Field>
              <Field label="音调 (0.5-2.0)">
                <Input type="number" min={0.5} max={2} step={0.1} value={tts.pitch} onChange={e => setTts({ pitch: +e.target.value })} className={inp} />
              </Field>
            </div>
            <div className="space-y-2 pt-1">
              <div className="flex items-center justify-between">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">情绪音色映射</Label>
                <Button size="sm" variant="outline" className="h-7 px-2 text-xs border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                  onClick={() => setTts({ voice_map: { ...tts.voice_map, '': '' } })}>
                  <Plus className="w-3 h-3 mr-1" />新增
                </Button>
              </div>
              {Object.entries(tts.voice_map).map(([emotion, voice], idx) => (
                <div key={idx} className="flex items-center gap-2">
                  <Input value={emotion} placeholder="情绪 (happy...)" className={`${inp} flex-1`}
                    onChange={e => { const m = { ...tts.voice_map }; delete m[emotion]; setTts({ voice_map: { ...m, [e.target.value]: voice } }) }} />
                  <Input value={voice} placeholder="音色名称" className={`${inp} flex-1`}
                    onChange={e => setTts({ voice_map: { ...tts.voice_map, [emotion]: e.target.value } })} />
                  <button onClick={() => { const m = { ...tts.voice_map }; delete m[emotion]; setTts({ voice_map: m }) }}
                    className="text-zinc-600 hover:text-red-400 transition-colors p-1.5">
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))}
            </div>
          </TabsContent>
          {/* ── 音频/VAD ── */}
          <TabsContent value="audio" className="space-y-5 pt-1">
            <Field label="采样率 (Hz)">
              <Input type="number" value={ip.sample_rate} onChange={e => setInPipe({ sample_rate: +e.target.value })} className={inp} />
            </Field>
            <div className="flex items-center gap-3">
              <Switch checked={ip.enable_vad} onCheckedChange={v => setInPipe({ enable_vad: v })}
                className="data-[state=checked]:bg-violet-600" />
              <Label className="text-sm text-zinc-300 cursor-pointer">启用 VAD 端点检测</Label>
            </div>
            {ip.enable_vad && (
              <div className="grid grid-cols-2 gap-4 pl-3 border-l border-zinc-800">
                <Field label="检测阈值 (0-1)">
                  <Input type="number" min={0} max={1} step={0.05} value={ip.vad_threshold}
                    onChange={e => setInPipe({ vad_threshold: +e.target.value })} className={inp} />
                </Field>
                <Field label="最小静音时长 (ms)">
                  <Input type="number" min={0} value={ip.vad_min_silence_ms}
                    onChange={e => setInPipe({ vad_min_silence_ms: +e.target.value })} className={inp} />
                </Field>
                <Field label="语音填充 (ms)">
                  <Input type="number" min={0} value={ip.vad_speech_pad_ms}
                    onChange={e => setInPipe({ vad_speech_pad_ms: +e.target.value })} className={inp} />
                </Field>
              </div>
            )}
          </TabsContent>

          {/* ── 记忆 ── */}
          <TabsContent value="memory" className="space-y-5 pt-1">
            <Field label="记忆模式">
              <Select value={mem.mode} onValueChange={v => setMem({ mode: v })}>
                <SelectTrigger className="bg-zinc-800 border-zinc-700 text-white focus:ring-violet-500 h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="bg-zinc-800 border-zinc-700 text-white">
                  <SelectItem value="none">无记忆</SelectItem>
                  <SelectItem value="session">会话记忆</SelectItem>
                  <SelectItem value="long_term">长期记忆</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <div className="grid grid-cols-2 gap-4">
              <Field label="会话最大轮数">
                <Input type="number" min={0} value={mem.session_max_turns}
                  onChange={e => setMem({ session_max_turns: +e.target.value })} className={inp} />
              </Field>
              <Field label="每 N 轮摘要">
                <Input type="number" min={0} value={mem.session_summary_every_n}
                  onChange={e => setMem({ session_summary_every_n: +e.target.value })} className={inp} />
              </Field>
            </div>
            {mem.mode === 'long_term' && (
              <div className="grid grid-cols-2 gap-4 pl-3 border-l border-zinc-800">
                <Field label="数据库路径">
                  <Input value={mem.long_term_db_path} onChange={e => setMem({ long_term_db_path: e.target.value })}
                    placeholder="data/memory.db" className={`${inp} col-span-2`} />
                </Field>
                <Field label="最大检索条数">
                  <Input type="number" min={0} value={mem.long_term_max_results}
                    onChange={e => setMem({ long_term_max_results: +e.target.value })} className={inp} />
                </Field>
                <Field label="保留天数">
                  <Input type="number" min={0} value={mem.retention_days}
                    onChange={e => setMem({ retention_days: +e.target.value })} className={inp} />
                </Field>
              </div>
            )}
          </TabsContent>
          {/* ── MCP ── */}
          <TabsContent value="mcp" className="space-y-4 pt-1">
            <div className="flex justify-end">
              <Button onClick={() => setMcpOpen(true)}
                className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-3 text-sm gap-1.5">
                <Plus className="w-3.5 h-3.5" />添加 MCP 服务
              </Button>
            </div>
            {cfg.tools.mcp.length === 0 ? (
              <div className="text-center py-14 border border-dashed border-zinc-800 rounded-xl">
                <p className="text-zinc-500 text-sm">暂无 MCP 工具</p>
              </div>
            ) : (
              <div className="space-y-2">
                {cfg.tools.mcp.map(m => (
                  <div key={m.id} className="flex items-start justify-between bg-zinc-900 border border-zinc-800 rounded-xl px-4 py-3">
                    <div className="space-y-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-mono text-white">{m.id}</span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700">{m.transport}</span>
                      </div>
                      <p className="text-xs text-zinc-500 truncate">
                        {m.transport === 'stdio' ? `${m.command} ${m.args.join(' ')}` : m.endpoint}
                      </p>
                      {m.tool_name_list.length > 0 && (
                        <p className="text-xs text-zinc-600">工具：{m.tool_name_list.join(', ')}</p>
                      )}
                    </div>
                    <button onClick={() => setCfg(c => ({ ...c, tools: { mcp: c.tools.mcp.filter(x => x.id !== m.id) } }))}
                      className="text-zinc-600 hover:text-red-400 transition-colors p-1.5 ml-3 shrink-0">
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>

          {/* ── 设备 ── */}
          <TabsContent value="devices" className="space-y-4 pt-1">
            <div className="flex justify-end">
              <Button onClick={() => setDevOpen(true)}
                className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-3 text-sm gap-1.5">
                <Plus className="w-3.5 h-3.5" />绑定设备
              </Button>
            </div>
            {devices.length === 0 ? (
              <div className="text-center py-14 border border-dashed border-zinc-800 rounded-xl">
                <p className="text-zinc-500 text-sm">暂无绑定设备</p>
                <p className="text-zinc-600 text-xs mt-1">填写设备 ID 即可绑定硬件</p>
              </div>
            ) : (
              <div className="space-y-2">
                {devices.map(d => (
                  <div key={d.id} className="flex items-center justify-between bg-zinc-900 border border-zinc-800 rounded-xl px-4 py-3">
                    <div>
                      <p className="text-sm font-mono text-white">{d.id}</p>
                      <p className="text-xs text-zinc-500 mt-0.5">
                        {d.name && <span className="mr-2">{d.name}</span>}
                        {new Date(d.created_at).toLocaleDateString('zh-CN')}
                      </p>
                    </div>
                    <button onClick={() => handleDeleteDevice(d.id)}
                      className="text-zinc-600 hover:text-red-400 transition-colors px-2 py-1 text-xs">
                      删除
                    </button>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>
        </Tabs>
      </div>

      {/* MCP Dialog */}
      <Dialog open={mcpOpen} onOpenChange={setMcpOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-white">添加 MCP 服务</DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <div className="grid grid-cols-2 gap-3">
              <Field label="ID">
                <Input value={newMcp.id} onChange={e => setNewMcp(m => ({ ...m, id: e.target.value }))}
                  placeholder="my-mcp-server" className={inp} />
              </Field>
              <Field label="Transport">
                <Select value={newMcp.transport} onValueChange={v => setNewMcp(m => ({ ...m, transport: v as MCPServer['transport'] }))}>
                  <SelectTrigger className="bg-zinc-800 border-zinc-700 text-white h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="bg-zinc-800 border-zinc-700 text-white">
                    <SelectItem value="stdio">stdio</SelectItem>
                    <SelectItem value="sse">sse</SelectItem>
                    <SelectItem value="streamable">streamable</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </div>
            {newMcp.transport === 'stdio' ? (
              <>
                <Field label="Command">
                  <Input value={newMcp.command} onChange={e => setNewMcp(m => ({ ...m, command: e.target.value }))}
                    placeholder="npx / python / ..." className={inp} />
                </Field>
                <Field label="Args（空格分隔）">
                  <Input value={mcpArgsStr} onChange={e => setMcpArgsStr(e.target.value)}
                    placeholder="-y @modelcontextprotocol/server-filesystem /path" className={inp} />
                </Field>
              </>
            ) : (
              <Field label="Endpoint">
                <Input value={newMcp.endpoint} onChange={e => setNewMcp(m => ({ ...m, endpoint: e.target.value }))}
                  placeholder="https://..." className={inp} />
              </Field>
            )}
            <Field label="工具白名单（逗号分隔，留空=全部）">
              <Input value={mcpToolsStr} onChange={e => setMcpToolsStr(e.target.value)}
                placeholder="read_file, write_file" className={inp} />
            </Field>
            <Field label="超时 (ms)">
              <Input type="number" min={0} value={newMcp.timeout_ms}
                onChange={e => setNewMcp(m => ({ ...m, timeout_ms: +e.target.value }))} className={inp} />
            </Field>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setMcpOpen(false)}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">取消</Button>
            <Button onClick={handleAddMcp} disabled={!newMcp.id.trim() || (newMcp.transport === 'stdio' ? !newMcp.command.trim() : !newMcp.endpoint.trim())}
              className="bg-violet-600 hover:bg-violet-500 text-white">添加</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Device Dialog */}
      <Dialog open={devOpen} onOpenChange={setDevOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-white">绑定设备</DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <Field label="Device ID">
              <Input value={newDevId} onChange={e => setNewDevId(e.target.value)}
                placeholder="esp32-abc123" className={inp} autoFocus />
            </Field>
            <Field label="设备名称（可选）">
              <Input value={newDevName} onChange={e => setNewDevName(e.target.value)}
                placeholder="客厅音箱" className={inp} />
            </Field>
            {devErr && <p className="text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-3 py-2">{devErr}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDevOpen(false)}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">取消</Button>
            <Button onClick={handleAddDevice} disabled={devAdding || !newDevId.trim()}
              className="bg-violet-600 hover:bg-violet-500 text-white">
              {devAdding ? '绑定中...' : '绑定'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
