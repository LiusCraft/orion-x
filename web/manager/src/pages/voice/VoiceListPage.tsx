import { useState } from 'react'
import { Mic2, Play, Pause, Trash2, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useNavigate } from 'react-router-dom'

interface Voice {
  id: string
  name: string
  gender: string
  style: string
  source: 'system' | 'custom'
  color: string
  addedAt: string
}

const MOCK_VOICES: Voice[] = [
  { id: 'v1', name: '晓晓', gender: '女声', style: '温柔', source: 'system', color: 'from-pink-500 to-rose-600', addedAt: '2025-06-01' },
  { id: 'v2', name: '云扬', gender: '男声', style: '专业', source: 'system', color: 'from-blue-500 to-indigo-600', addedAt: '2025-06-01' },
  { id: 'v3', name: '我的声音', gender: '女声', style: '温柔', source: 'custom', color: 'from-violet-500 to-purple-600', addedAt: '2025-07-02' },
  { id: 'v4', name: '客服音色', gender: '女声', style: '活泼', source: 'custom', color: 'from-fuchsia-500 to-pink-600', addedAt: '2025-07-03' },
]

export default function VoiceListPage() {
  const [voices, setVoices] = useState<Voice[]>(MOCK_VOICES)
  const [playing, setPlaying] = useState<string | null>(null)
  const navigate = useNavigate()

  const systemVoices = voices.filter((v) => v.source === 'system')
  const customVoices = voices.filter((v) => v.source === 'custom')

  const VoiceCard = ({ voice }: { voice: Voice }) => (
    <div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group">
      <div className="flex items-center gap-3 mb-3">
        <div className={`w-12 h-12 rounded-xl bg-gradient-to-br ${voice.color} flex items-center justify-center text-xl font-bold text-white shadow-lg`}>
          {voice.name[0]}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <p className="font-semibold text-sm text-white">{voice.name}</p>
            {voice.source === 'custom' && (
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-600/15 text-violet-400 border border-violet-500/20">复刻</span>
            )}
          </div>
          <div className="flex items-center gap-1.5 mt-0.5">
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">{voice.gender}</span>
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">{voice.style}</span>
          </div>
        </div>
        {voice.source === 'custom' && (
          <button
            onClick={() => setVoices((prev) => prev.filter((v) => v.id !== voice.id))}
            className="opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 transition-all cursor-pointer"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
      <div className="flex gap-2">
        <button
          onClick={() => setPlaying((p) => (p === voice.id ? null : voice.id))}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs border transition-all cursor-pointer ${
            playing === voice.id
              ? 'bg-violet-600/20 border-violet-500/30 text-violet-400'
              : 'bg-zinc-800 border-zinc-700 text-zinc-400 hover:border-zinc-600 hover:text-zinc-200'
          }`}
        >
          {playing === voice.id ? <Pause className="w-3 h-3" /> : <Play className="w-3 h-3" />}
          {playing === voice.id ? '停止' : '试听'}
        </button>
        <p className="ml-auto text-[11px] text-zinc-600 self-center">{voice.addedAt}</p>
      </div>
    </div>
  )

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">已有音色</h1>
            <p className="text-sm text-zinc-500 mt-0.5">系统音色和你复刻的自定义音色</p>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => navigate('/voice/plaza')}
              className="h-9 px-4 text-sm border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1.5"
            >
              <Plus className="w-4 h-4" />
              从广场添加
            </Button>
            <Button
              onClick={() => navigate('/voice/clone')}
              className="h-9 px-4 text-sm bg-violet-600 hover:bg-violet-500 text-white gap-1.5 shadow-md shadow-violet-600/20"
            >
              <Mic2 className="w-4 h-4" />
              复刻新音色
            </Button>
          </div>
        </div>
      </div>

      <div className="px-8 py-6 space-y-8">
        <div>
          <h2 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4">我的复刻音色（{customVoices.length}）</h2>
          {customVoices.length === 0 ? (
            <div className="bg-zinc-900 border border-zinc-800 border-dashed rounded-xl p-8 text-center">
              <Mic2 className="w-8 h-8 text-zinc-700 mx-auto mb-3" strokeWidth={1.5} />
              <p className="text-zinc-500 text-sm">还没有复刻的音色</p>
              <button onClick={() => navigate('/voice/clone')} className="text-xs text-violet-400 hover:text-violet-300 mt-1.5 cursor-pointer">
                去复刻 →
              </button>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {customVoices.map((v) => <VoiceCard key={v.id} voice={v} />)}
            </div>
          )}
        </div>

        <div>
          <h2 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4">已添加的系统音色（{systemVoices.length}）</h2>
          {systemVoices.length === 0 ? (
            <div className="bg-zinc-900 border border-zinc-800 border-dashed rounded-xl p-8 text-center">
              <p className="text-zinc-500 text-sm">还没有添加系统音色</p>
              <button onClick={() => navigate('/voice/plaza')} className="text-xs text-violet-400 hover:text-violet-300 mt-1.5 cursor-pointer">
                去广场选择 →
              </button>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {systemVoices.map((v) => <VoiceCard key={v.id} voice={v} />)}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
