import { useEffect, useState } from 'react'
import { Play, Pause, Plus, Music, Search, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { voiceApi, languageApi, type ModelVoice } from '@/lib/api'
import { Tooltip } from '@/components/ui/tooltip'

const GENDERS = ['全部', '女声', '男声', '中性']
const GENDER_MAP: Record<string, string> = {
  '女声': 'female',
  '男声': 'male',
  '中性': 'neutral',
}

function genderLabel(g: string | undefined): string {
  if (g === 'female') return '女声'
  if (g === 'male') return '男声'
  if (g === 'neutral') return '中性'
  return ''
}

const VOICE_COLORS = [
  'from-pink-500 to-rose-600',
  'from-blue-500 to-indigo-600',
  'from-violet-500 to-purple-600',
  'from-slate-500 to-zinc-600',
  'from-emerald-500 to-teal-600',
  'from-fuchsia-500 to-pink-600',
  'from-amber-500 to-orange-600',
  'from-cyan-500 to-blue-600',
]

export default function VoicePlazaPage() {
  const [voices, setVoices] = useState<ModelVoice[]>([])
  const [loading, setLoading] = useState(true)
  const [playing, setPlaying] = useState<string | null>(null)
  const [emotionPopup, setEmotionPopup] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [gender, setGender] = useState('全部')
  const [lang, setLang] = useState('全部')
  const [tag, setTag] = useState('全部')
  const [emotion, setEmotion] = useState('全部')
  const [langNames, setLangNames] = useState<Record<string, string>>({})

  useEffect(() => {
    setLoading(true)
    Promise.all([
      voiceApi.listSystem(),
      languageApi.list(),
    ]).then(([voicesRes, langRes]) => {
      setVoices(voicesRes.data)
      const map: Record<string, string> = {}
      for (const l of langRes.data) {
        map[l.code] = l.name
      }
      setLangNames(map)
    }).finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!emotionPopup) return
    const handle = (e: MouseEvent) => {
      const el = e.target as HTMLElement
      if (!el.closest('[data-popup]')) setEmotionPopup(null)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [emotionPopup])

  const allLangs = [...new Set(voices.flatMap((v) => v.langs ?? []))].sort()
  const allTags = [...new Set(voices.flatMap((v) => v.tags ?? []))].sort()
  const allEmotions = [...new Set(voices.flatMap((v) => Object.keys(v.emotions ?? {})))].sort()

  const filtered = voices.filter((v) => {
    const matchGender = gender === '全部' || GENDER_MAP[gender] === v.gender
    const matchLang = lang === '全部' || (v.langs ?? []).includes(lang)
    const matchTag = tag === '全部' || (v.tags ?? []).includes(tag)
    const matchEmotion = emotion === '全部' || Object.keys(v.emotions ?? {}).includes(emotion)
    const matchQuery =
      !query || v.name.includes(query) || (v.description ?? '').includes(query)
    return matchGender && matchLang && matchTag && matchEmotion && matchQuery
  })

  const togglePlay = (id: string) => {
    setPlaying((prev) => (prev === id ? null : id))
  }
  const toggleEmotionPopup = (id: string) => {
    setEmotionPopup((prev) => (prev === id ? null : id))
    setPlaying(null)
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
        <div className="relative mt-4">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索音色..."
            className="pl-9 w-48 bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600 h-9 text-sm focus-visible:ring-violet-500"
          />
        </div>
        <div className="mt-4 space-y-3">
          <div className="flex items-center gap-2 text-sm">
            <span className="text-zinc-400 w-10 shrink-0">性别</span>
            <div className="flex gap-1.5 flex-wrap">
              {GENDERS.map((g) => (
                <button key={g} onClick={() => setGender(g)} className={`px-2.5 py-1 rounded text-xs font-medium transition-all cursor-pointer ${gender === g ? 'bg-violet-600/20 text-violet-400 border border-violet-500/30' : 'bg-zinc-800/50 text-zinc-500 border border-transparent hover:text-zinc-300 hover:border-zinc-700'}`}>{g}</button>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2 text-sm">
            <span className="text-zinc-400 w-10 shrink-0">语言</span>
            <div className="flex gap-1.5 flex-wrap">
              {['全部', ...allLangs].map((l) => (
                <button key={l} onClick={() => setLang(l)} className={`px-2.5 py-1 rounded text-xs font-medium transition-all cursor-pointer ${lang === l ? 'bg-violet-600/20 text-violet-400 border border-violet-500/30' : 'bg-zinc-800/50 text-zinc-500 border border-transparent hover:text-zinc-300 hover:border-zinc-700'}`}>{langNames[l] || l}</button>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2 text-sm">
            <span className="text-zinc-400 w-10 shrink-0">标签</span>
            <div className="flex gap-1.5 flex-wrap">
              {['全部', ...allTags].map((t) => (
                <button key={t} onClick={() => setTag(t)} className={`px-2.5 py-1 rounded text-xs font-medium transition-all cursor-pointer ${tag === t ? 'bg-violet-600/20 text-violet-400 border border-violet-500/30' : 'bg-zinc-800/50 text-zinc-500 border border-transparent hover:text-zinc-300 hover:border-zinc-700'}`}>{t}</button>
              ))}
            </div>
          </div>
          {allEmotions.length > 0 && (
            <div className="flex items-center gap-2 text-sm">
              <span className="text-zinc-400 w-10 shrink-0">情绪</span>
              <div className="flex gap-1.5 flex-wrap">
                {['全部', ...allEmotions].map((e) => (
                  <button key={e} onClick={() => setEmotion(e)} className={`px-2.5 py-1 rounded text-xs font-medium transition-all cursor-pointer ${emotion === e ? 'bg-violet-600/20 text-violet-400 border border-violet-500/30' : 'bg-zinc-800/50 text-zinc-500 border border-transparent hover:text-zinc-300 hover:border-zinc-700'}`}>{e}</button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="px-8 py-6">
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <Loader2 className="w-6 h-6 text-zinc-500 animate-spin" />
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Music className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">没有匹配的音色</p>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {filtered.map((voice, idx) => {
              const isPlaying = playing === voice.id || (playing && playing.startsWith(voice.id + ':'))
              const color = VOICE_COLORS[idx % VOICE_COLORS.length]
              const tags = voice.tags ?? []
              const langs = voice.langs ?? []
              return (
                <div key={voice.id} className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group">
                  <div className="flex items-center gap-3 mb-3">
                    {voice.avatar_url ? (
                      <img src={voice.avatar_url} alt={voice.name} className="w-12 h-12 rounded-xl object-cover shadow-lg" />
                    ) : (
                      <div className={`w-12 h-12 rounded-xl bg-gradient-to-br ${color} flex items-center justify-center text-2xl font-bold text-white shadow-lg`}>
                        {voice.name[0]}
                      </div>
                    )}
                    <div>
                      <p className="font-semibold text-sm text-white">{voice.name}</p>
                      <div className="flex items-center gap-1.5 mt-0.5">
                        {voice.gender && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">{genderLabel(voice.gender)}</span>
                        )}
                        {tags.slice(0, 2).map((t) => (
                          <span key={t} className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">{t}</span>
                        ))}
                        {langs.length > 0 && (
                          <Tooltip content={langs.map((l) => langNames[l] || l).join(', ')}>
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">
                              {langNames[langs[0]] || langs[0]}{langs.length > 1 ? ` +${langs.length - 1}` : ''}
                            </span>
                          </Tooltip>
                        )}
                        {voice.emotions && (
                          <Tooltip content={Object.keys(voice.emotions).join(', ')}>
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-900/30 text-amber-500 border border-amber-800/40">
                              {Object.keys(voice.emotions).length}情绪
                            </span>
                          </Tooltip>
                        )}
                      </div>
                    </div>
                  </div>

                  <p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">{voice.description}</p>

                  <div className="flex gap-2 relative">
                    <button data-popup="trigger"
                      onClick={(e) => {
                        e.stopPropagation()
                        if (voice.emotions && Object.keys(voice.emotions).length > 0) {
                          toggleEmotionPopup(voice.id)
                        } else {
                          togglePlay(voice.id)
                        }
                      }}
                      className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs border transition-all cursor-pointer ${
                        isPlaying
                          ? 'bg-violet-600/20 border-violet-500/30 text-violet-400'
                          : 'bg-zinc-800 border-zinc-700 text-zinc-400 hover:border-zinc-600 hover:text-zinc-200'
                      }`}
                    >
                      {isPlaying ? <Pause className="w-3 h-3" /> : <Play className="w-3 h-3" />}
                      {isPlaying ? '停止' : '试听'}
                    </button>
                    {emotionPopup === voice.id && voice.emotions && (
                      <div data-popup="menu" onMouseLeave={() => setEmotionPopup(null)} className="absolute bottom-full left-0 mb-2 p-2 rounded-lg bg-zinc-800 border border-zinc-700 shadow-xl z-50 min-w-[140px]">
                        <p className="text-[10px] text-zinc-500 mb-1.5 px-1">选择情绪试听</p>
                        <button
                          onClick={(e) => {
                            e.stopPropagation()
                            setPlaying(playing === voice.id ? null : voice.id)
                          }}
                          disabled={!voice.preview_url}
                          className={`flex items-center gap-2 w-full px-2 py-1.5 rounded text-xs transition-all cursor-pointer ${
                            playing === voice.id ? 'bg-violet-600/20 text-violet-400' : 'text-zinc-300 hover:bg-zinc-700'
                          } ${!voice.preview_url ? 'opacity-40 cursor-not-allowed' : ''}`}
                        >
                          {voice.preview_url ? (
                            playing === voice.id ? <Pause className="w-3 h-3 shrink-0" /> : <Play className="w-3 h-3 shrink-0" />
                          ) : (
                            <Music className="w-3 h-3 shrink-0 text-zinc-600" />
                          )}
                          默认
                        </button>
                        <div className="h-px bg-zinc-700 my-1" />
                        {Object.entries(voice.emotions).map(([key, val]) => {
                          const emotionId = `${voice.id}:${key}`
                          const isEmotionPlaying = playing === emotionId
                          const previewUrl = val && typeof val === 'object' && 'preview_url' in val ? (val as Record<string, unknown>).preview_url : undefined
                          return (
                            <button
                              key={key}
                              onClick={(e) => {
                                e.stopPropagation()
                                if (previewUrl) {
                                  setPlaying(isEmotionPlaying ? null : emotionId)
                                }
                              }}
                              disabled={!previewUrl}
                              className={`flex items-center gap-2 w-full px-2 py-1.5 rounded text-xs transition-all cursor-pointer ${
                                isEmotionPlaying
                                  ? 'bg-amber-600/20 text-amber-400'
                                  : 'text-zinc-300 hover:bg-zinc-700'
                              } ${!previewUrl ? 'opacity-40 cursor-not-allowed' : ''}`}
                            >
                              {previewUrl ? <Play className="w-3 h-3 shrink-0" /> : <Music className="w-3 h-3 shrink-0 text-zinc-600" />}
                              {key}
                            </button>
                          )
                        })}
                      </div>
                    )}
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
