import { useState } from 'react'
import { Play, Pause, Plus, Music, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const GENDERS = ['全部', '女声', '男声', '中性']
const STYLES_FILTER = ['全部', '温柔', '活泼', '专业', '低沉', '甜美', '播报']

const VOICES = [
  { id: 'v1', name: '晓晓', gender: '女声', style: '温柔', lang: '普通话', provider: '官方', color: 'from-pink-500 to-rose-600', desc: '声线温柔细腻，适合情感类对话和故事讲述' },
  { id: 'v2', name: '云扬', gender: '男声', style: '专业', lang: '普通话', provider: '官方', color: 'from-blue-500 to-indigo-600', desc: '声音浑厚稳重，适合新闻播报和商务场景' },
  { id: 'v3', name: '晓伊', gender: '女声', style: '活泼', lang: '普通话', provider: '官方', color: 'from-violet-500 to-purple-600', desc: '充满活力的年轻女声，适合客服和互动娱乐' },
  { id: 'v4', name: '云健', gender: '男声', style: '低沉', lang: '普通话', provider: '官方', color: 'from-slate-500 to-zinc-600', desc: '低沉磁性男声，适合高端品牌和导航播报' },
  { id: 'v5', name: 'Aria', gender: '女声', style: '专业', lang: 'English', provider: '官方', color: 'from-emerald-500 to-teal-600', desc: 'Professional female voice for English content' },
  { id: 'v6', name: '晓萱', gender: '女声', style: '甜美', lang: '普通话', provider: '官方', color: 'from-fuchsia-500 to-pink-600', desc: '甜美可爱的声线，适合动漫配音和儿童应用' },
  { id: 'v7', name: '云希', gender: '男声', style: '播报', lang: '普通话', provider: '官方', color: 'from-amber-500 to-orange-600', desc: '标准普通话播报音色，适合资讯和教育内容' },
  { id: 'v8', name: '晓双', gender: '中性', style: '专业', lang: '普通话', provider: '官方', color: 'from-cyan-500 to-blue-600', desc: '中性专业音色，适合科技产品和 IoT 设备' },
]

export default function VoicePlazaPage() {
  const [playing, setPlaying] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [gender, setGender] = useState('全部')
  const [style, setStyle] = useState('全部')

  const filtered = VOICES.filter((v) => {
    const matchGender = gender === '全部' || v.gender === gender
    const matchStyle = style === '全部' || v.style === style
    const matchQuery = !query || v.name.includes(query) || v.desc.includes(query)
    return matchGender && matchStyle && matchQuery
  })

  const togglePlay = (id: string) => {
    setPlaying((prev) => (prev === id ? null : id))
  }

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">音色广场</h1>
            <p className="text-sm text-zinc-500 mt-0.5">官方上架音色，试听后选用到你的智能体</p>
          </div>
        </div>
        <div className="flex items-center gap-3 mt-4 flex-wrap">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索音色..."
              className="pl-9 w-48 bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600 h-9 text-sm focus-visible:ring-violet-500"
            />
          </div>
          <div className="flex gap-1">
            {GENDERS.map((g) => (
              <button key={g} onClick={() => setGender(g)} className={`px-3 py-1 rounded-full text-xs font-medium transition-all cursor-pointer ${gender === g ? 'bg-violet-600 text-white' : 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200'}`}>{g}</button>
            ))}
          </div>
          <div className="flex gap-1">
            {STYLES_FILTER.map((s) => (
              <button key={s} onClick={() => setStyle(s)} className={`px-3 py-1 rounded-full text-xs font-medium transition-all cursor-pointer ${style === s ? 'bg-zinc-700 text-white' : 'bg-zinc-900 border border-zinc-800 text-zinc-500 hover:text-zinc-300'}`}>{s}</button>
            ))}
          </div>
        </div>
      </div>

      <div className="px-8 py-6">
        {filtered.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Music className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">没有匹配的音色</p>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {filtered.map((voice) => {
              const isPlaying = playing === voice.id
              return (
                <div key={voice.id} className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group">
                  <div className="flex items-center gap-3 mb-3">
                    <div className={`w-12 h-12 rounded-xl bg-gradient-to-br ${voice.color} flex items-center justify-center text-2xl font-bold text-white shadow-lg`}>
                      {voice.name[0]}
                    </div>
                    <div>
                      <p className="font-semibold text-sm text-white">{voice.name}</p>
                      <div className="flex items-center gap-1.5 mt-0.5">
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">{voice.gender}</span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">{voice.style}</span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">{voice.lang}</span>
                      </div>
                    </div>
                  </div>

                  <p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">{voice.desc}</p>

                  <div className="flex gap-2">
                    <button
                      onClick={() => togglePlay(voice.id)}
                      className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs border transition-all cursor-pointer ${
                        isPlaying
                          ? 'bg-violet-600/20 border-violet-500/30 text-violet-400'
                          : 'bg-zinc-800 border-zinc-700 text-zinc-400 hover:border-zinc-600 hover:text-zinc-200'
                      }`}
                    >
                      {isPlaying ? <Pause className="w-3 h-3" /> : <Play className="w-3 h-3" />}
                      {isPlaying ? '停止' : '试听'}
                    </button>
                    <Button size="sm" className="flex-1 h-7 text-xs bg-violet-600/15 hover:bg-violet-600 text-violet-400 hover:text-white border border-violet-500/20 hover:border-violet-500 transition-all">
                      <Plus className="w-3 h-3 mr-1" />使用
                    </Button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
