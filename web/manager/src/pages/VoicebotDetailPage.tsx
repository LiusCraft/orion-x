import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { voicebotApi, deviceApi, type Voicebot, type Device } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'

export default function VoicebotDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [bot, setBot] = useState<Voicebot | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)

  const [editName, setEditName] = useState('')
  const [editConfig, setEditConfig] = useState('')
  const [configError, setConfigError] = useState('')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  const [addOpen, setAddOpen] = useState(false)
  const [newDeviceId, setNewDeviceId] = useState('')
  const [newDeviceName, setNewDeviceName] = useState('')
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState('')

  const fetchAll = async () => {
    if (!id) return
    try {
      const [botRes, devRes] = await Promise.all([
        voicebotApi.get(id),
        deviceApi.list(id),
      ])
      setBot(botRes.data)
      setEditName(botRes.data.name)
      setEditConfig(JSON.stringify(JSON.parse(botRes.data.config_json), null, 2))
      setDevices(devRes.data)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchAll() }, [id])

  const handleSaveConfig = async () => {
    if (!bot) return
    setConfigError('')
    try { JSON.parse(editConfig) } catch { setConfigError('JSON 格式错误'); return }
    setSaving(true)
    try {
      await voicebotApi.update(bot.id, editName, editConfig)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      setConfigError(msg ?? '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleAddDevice = async () => {
    if (!bot || !newDeviceId.trim()) return
    setAddError('')
    setAdding(true)
    try {
      await deviceApi.create(bot.id, newDeviceId.trim(), newDeviceName.trim())
      setAddOpen(false)
      setNewDeviceId('')
      setNewDeviceName('')
      const { data } = await deviceApi.list(bot.id)
      setDevices(data)
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      setAddError(msg ?? '添加失败')
    } finally {
      setAdding(false)
    }
  }

  const handleDeleteDevice = async (deviceId: string) => {
    if (!bot || !confirm(`确认删除设备 ${deviceId}？`)) return
    await deviceApi.remove(bot.id, deviceId)
    setDevices((prev) => prev.filter((d) => d.id !== deviceId))
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-zinc-950 flex items-center justify-center">
        <div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
      </div>
    )
  }

  if (!bot) return <div className="min-h-screen bg-zinc-950 flex items-center justify-center text-zinc-500 text-sm">未找到</div>

  return (
    <div className="min-h-screen bg-zinc-950 text-white">
      {/* Topbar */}
      <header className="border-b border-zinc-800 px-6 h-14 flex items-center gap-3">
        <button
          onClick={() => navigate('/voicebots')}
          className="text-zinc-500 hover:text-zinc-300 transition-colors"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <div className="h-4 w-px bg-zinc-800" />
        <span className="text-sm font-medium">{bot.name}</span>
        <span className="text-xs text-zinc-600 font-mono">{bot.id}</span>
      </header>

      <main className="max-w-3xl mx-auto px-6 py-8">
        <Tabs defaultValue="config">
          <TabsList className="bg-zinc-900 border border-zinc-800 mb-6">
            <TabsTrigger value="config" className="data-[state=active]:bg-zinc-700 data-[state=active]:text-white text-zinc-400 text-sm">
              配置
            </TabsTrigger>
            <TabsTrigger value="devices" className="data-[state=active]:bg-zinc-700 data-[state=active]:text-white text-zinc-400 text-sm">
              设备 {devices.length > 0 && <span className="ml-1.5 bg-zinc-600 text-zinc-300 text-xs rounded-full px-1.5 py-0.5">{devices.length}</span>}
            </TabsTrigger>
          </TabsList>

          {/* 配置 Tab */}
          <TabsContent value="config" className="space-y-5">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400 uppercase tracking-wide font-medium">名称</label>
              <Input
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="bg-zinc-900 border-zinc-800 text-white focus-visible:ring-violet-500 h-10"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400 uppercase tracking-wide font-medium">配置 JSON</label>
              <Textarea
                value={editConfig}
                onChange={(e) => { setEditConfig(e.target.value); setConfigError('') }}
                className="bg-zinc-900 border-zinc-800 text-green-400 font-mono text-xs min-h-[420px] focus-visible:ring-violet-500 resize-none"
                spellCheck={false}
              />
              {configError && (
                <p className="text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-3 py-2">
                  {configError}
                </p>
              )}
            </div>
            <Button
              onClick={handleSaveConfig}
              disabled={saving}
              className={saved
                ? 'bg-emerald-600 hover:bg-emerald-600 text-white'
                : 'bg-violet-600 hover:bg-violet-500 text-white'}
            >
              {saving ? '保存中...' : saved ? '已保存 ✓' : '保存配置'}
            </Button>
          </TabsContent>

          {/* 设备 Tab */}
          <TabsContent value="devices">
            <div className="flex justify-end mb-4">
              <Button
                onClick={() => setAddOpen(true)}
                className="bg-violet-600 hover:bg-violet-500 text-white text-sm h-9"
              >
                + 添加设备
              </Button>
            </div>
            {devices.length === 0 ? (
              <div className="text-center py-16 border border-dashed border-zinc-800 rounded-xl">
                <p className="text-zinc-500 text-sm">暂无绑定设备</p>
                <p className="text-zinc-600 text-xs mt-1">点击右上角添加 Device ID</p>
              </div>
            ) : (
              <div className="space-y-2">
                {devices.map((d) => (
                  <div
                    key={d.id}
                    className="flex items-center justify-between bg-zinc-900 border border-zinc-800 rounded-xl px-4 py-3"
                  >
                    <div>
                      <p className="text-sm font-mono text-white">{d.id}</p>
                      <p className="text-xs text-zinc-500 mt-0.5">
                        {d.name && <span className="mr-2">{d.name}</span>}
                        {new Date(d.created_at).toLocaleDateString('zh-CN')}
                      </p>
                    </div>
                    <button
                      onClick={() => handleDeleteDevice(d.id)}
                      className="text-xs text-zinc-600 hover:text-red-400 transition-colors px-2 py-1"
                    >
                      删除
                    </button>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>
        </Tabs>
      </main>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white">
          <DialogHeader>
            <DialogTitle className="text-white">添加设备</DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400 uppercase tracking-wide font-medium">Device ID</label>
              <Input
                value={newDeviceId}
                onChange={(e) => setNewDeviceId(e.target.value)}
                placeholder="esp32-abc123"
                className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500 focus-visible:ring-violet-500"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400 uppercase tracking-wide font-medium">设备名称（可选）</label>
              <Input
                value={newDeviceName}
                onChange={(e) => setNewDeviceName(e.target.value)}
                placeholder="客厅音箱"
                className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500 focus-visible:ring-violet-500"
              />
            </div>
            {addError && (
              <p className="text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-3 py-2">
                {addError}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">
              取消
            </Button>
            <Button onClick={handleAddDevice} disabled={adding}
              className="bg-violet-600 hover:bg-violet-500 text-white">
              {adding ? '添加中...' : '添加'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
