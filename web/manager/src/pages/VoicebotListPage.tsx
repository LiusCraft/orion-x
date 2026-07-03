import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { voicebotApi, type Voicebot } from '@/lib/api'
import { useAuthStore } from '@/lib/store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'

export default function VoicebotListPage() {
  const [bots, setBots] = useState<Voicebot[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const { username, logout } = useAuthStore()
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
      await voicebotApi.create(newName.trim())
      setCreateOpen(false)
      setNewName('')
      await fetchBots()
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="min-h-screen bg-zinc-950 text-white">
      {/* Topbar */}
      <header className="border-b border-zinc-800 px-6 h-14 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="w-7 h-7 rounded-lg bg-violet-600 flex items-center justify-center">
            <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
            </svg>
          </div>
          <span className="font-semibold text-sm">Orion-X</span>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-zinc-500">{username}</span>
          <button
            onClick={logout}
            className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors"
          >
            退出
          </button>
        </div>
      </header>

      <main className="max-w-5xl mx-auto px-6 py-10">
        {/* Header row */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-xl font-semibold">VoiceBot</h1>
            <p className="text-sm text-zinc-500 mt-0.5">管理你的语音机器人和绑定设备</p>
          </div>
          <Button
            onClick={() => setCreateOpen(true)}
            className="bg-violet-600 hover:bg-violet-500 text-white text-sm h-9 px-4"
          >
            + 新建
          </Button>
        </div>

        {/* List */}
        {loading ? (
          <div className="flex items-center gap-2 text-zinc-500 text-sm py-20 justify-center">
            <div className="w-4 h-4 border-2 border-zinc-600 border-t-violet-500 rounded-full animate-spin" />
            加载中...
          </div>
        ) : bots.length === 0 ? (
          <div className="text-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mx-auto mb-4">
              <svg className="w-6 h-6 text-zinc-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4v16m8-8H4" />
              </svg>
            </div>
            <p className="text-zinc-400 text-sm">还没有 VoiceBot</p>
            <p className="text-zinc-600 text-xs mt-1">点击右上角新建一个</p>
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {bots.map((bot) => (
              <button
                key={bot.id}
                onClick={() => navigate(`/voicebots/${bot.id}`)}
                className="text-left bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-600 hover:bg-zinc-800/60 transition-all group"
              >
                <div className="flex items-start justify-between mb-3">
                  <div className="w-9 h-9 rounded-xl bg-violet-600/20 border border-violet-500/30 flex items-center justify-center">
                    <svg className="w-4 h-4 text-violet-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M8.25 3v1.5M4.5 8.25H3m18 0h-1.5M4.5 12H3m18 0h-1.5m-15 3.75H3m18 0h-1.5M8.25 19.5V21M12 3v1.5m0 15V21m3.75-18v1.5m0 15V21m-9-1.5h10.5a2.25 2.25 0 002.25-2.25V6.75a2.25 2.25 0 00-2.25-2.25H6.75A2.25 2.25 0 004.5 6.75v10.5a2.25 2.25 0 002.25 2.25zm.75-12h9v9h-9v-9z" />
                    </svg>
                  </div>
                  <svg className="w-4 h-4 text-zinc-600 group-hover:text-zinc-400 transition-colors mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                  </svg>
                </div>
                <p className="font-medium text-sm text-white">{bot.name}</p>
                <p className="text-xs text-zinc-500 mt-1 font-mono">{bot.id.slice(0, 8)}...</p>
                <p className="text-xs text-zinc-600 mt-3">
                  {new Date(bot.created_at).toLocaleDateString('zh-CN')}
                </p>
              </button>
            ))}
          </div>
        )}
      </main>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white">
          <DialogHeader>
            <DialogTitle className="text-white">新建 VoiceBot</DialogTitle>
          </DialogHeader>
          <div className="space-y-1.5 py-2">
            <label className="text-xs text-zinc-400 uppercase tracking-wide font-medium">名称</label>
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="我的助手"
              className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500 focus-visible:ring-violet-500"
              onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">
              取消
            </Button>
            <Button onClick={handleCreate} disabled={creating}
              className="bg-violet-600 hover:bg-violet-500 text-white">
              {creating ? '创建中...' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
