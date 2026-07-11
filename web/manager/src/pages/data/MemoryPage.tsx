import { useState } from 'react'
import { Brain, Trash2, Search } from 'lucide-react'
import { Input } from '@/components/ui/input'

const IMPORTANCE_MAP = {
  high: { label: '高', cls: 'bg-red-400/10 text-red-400 border-red-400/20' },
  mid: { label: '中', cls: 'bg-amber-400/10 text-amber-400 border-amber-400/20' },
  low: { label: '低', cls: 'bg-zinc-700/50 text-zinc-500 border-zinc-700' },
} as const

const MOCK_MEMORIES = [
  { id: 'm1', agent: '客服助手', summary: '用户偏好：张先生倾向于简洁直接的回答，不喜欢冗长解释', importance: 'high' as const, createdAt: '2025-07-04 14:23' },
  { id: 'm2', agent: '客服助手', summary: '历史订单：用户 U8821 曾于 2025-06-15 投诉过物流延误问题', importance: 'high' as const, createdAt: '2025-07-03 09:10' },
  { id: 'm3', agent: '数据分析师', summary: '报表周期：每周一上午生成上周销售汇总报告，发送至 admin@company.com', importance: 'mid' as const, createdAt: '2025-07-02 16:45' },
  { id: 'm4', agent: '内容运营', summary: '品牌调性：文章风格要求专业严肃，避免使用网络流行语', importance: 'mid' as const, createdAt: '2025-07-01 11:30' },
  { id: 'm5', agent: '语音播报员', summary: '用户设备：小爱音箱 Pro，语速偏快，音量通常在 60%', importance: 'low' as const, createdAt: '2025-06-30 08:15' },
  { id: 'm6', agent: '客服助手', summary: '常见问题：退款流程通常需要 3-5 个工作日处理', importance: 'low' as const, createdAt: '2025-06-28 17:00' },
]

export default function MemoryPage() {
  const [memories, setMemories] = useState(MOCK_MEMORIES)
  const [query, setQuery] = useState('')

  const filtered = memories.filter(
    (m) => !query || m.agent.includes(query) || m.summary.includes(query)
  )

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">记忆库</h1>
            <p className="text-sm text-zinc-500 mt-0.5">智能体从对话中积累的长期记忆条目</p>
          </div>
          <div className="flex items-center gap-2 text-sm text-zinc-500">
            <Brain className="w-4 h-4 text-zinc-600" />
            共 {memories.length} 条记忆
          </div>
        </div>
        <div className="relative mt-4 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索记忆内容或来源智能体..."
            className="pl-9 bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600 h-9 text-sm focus-visible:ring-violet-500"
          />
        </div>
      </div>

      <div className="px-8 py-6">
        {filtered.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Brain className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">暂无记忆条目</p>
            <p className="text-zinc-600 text-xs mt-1">当智能体开启记忆功能后，对话中的关键信息将自动存储</p>
          </div>
        ) : (
          <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-zinc-800">
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-28">来源智能体</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">内容摘要</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-20">重要程度</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-36">创建时间</th>
                  <th className="px-4 py-3 w-16"></th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((mem, i) => {
                  const { label, cls } = IMPORTANCE_MAP[mem.importance]
                  return (
                    <tr key={mem.id} className={`border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors group ${i === filtered.length - 1 ? 'border-0' : ''}`}>
                      <td className="px-4 py-3.5">
                        <span className="text-xs text-violet-400 bg-violet-600/10 border border-violet-500/20 px-2 py-0.5 rounded">
                          {mem.agent}
                        </span>
                      </td>
                      <td className="px-4 py-3.5 text-sm text-zinc-300 leading-relaxed">{mem.summary}</td>
                      <td className="px-4 py-3.5">
                        <span className={`text-[11px] px-2 py-0.5 rounded border ${cls}`}>{label}</span>
                      </td>
                      <td className="px-4 py-3.5 text-xs text-zinc-600 font-mono">{mem.createdAt}</td>
                      <td className="px-4 py-3.5">
                        <button
                          onClick={() => setMemories((prev) => prev.filter((m) => m.id !== mem.id))}
                          className="opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-red-400 transition-all p-1 rounded hover:bg-red-400/10 cursor-pointer"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
