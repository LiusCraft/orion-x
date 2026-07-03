import { useState } from 'react'
import { BarChart3, Coins, TrendingUp, Zap } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const TIME_RANGES = ['本月', '上月', '最近 3 月']

const OVERVIEW = [
  { label: '本月总 Token', value: '142.8M', sub: 'Input: 98.2M / Output: 44.6M', icon: Zap, color: 'text-violet-400', bg: 'bg-violet-400/10' },
  { label: '估算费用', value: '¥ 1,284.50', sub: '较上月 +18.3%', icon: Coins, color: 'text-amber-400', bg: 'bg-amber-400/10' },
  { label: '日均调用', value: '11,432', sub: '次/天', icon: BarChart3, color: 'text-blue-400', bg: 'bg-blue-400/10' },
  { label: '增长率', value: '+18.3%', sub: '环比上月', icon: TrendingUp, color: 'text-emerald-400', bg: 'bg-emerald-400/10' },
]

const MODEL_USAGE: Record<string, { model: string; inputTokens: number; outputTokens: number; calls: number; cost: number }[]> = {
  '文本': [
    { model: 'claude-sonnet-4-6', inputTokens: 68200000, outputTokens: 31400000, calls: 12840, cost: 892.4 },
    { model: 'claude-opus-4-8', inputTokens: 18400000, outputTokens: 7800000, calls: 3201, cost: 312.6 },
    { model: 'claude-haiku-4-5', inputTokens: 11600000, outputTokens: 5400000, calls: 28450, cost: 42.8 },
  ],
  '视觉': [
    { model: 'claude-sonnet-4-6 (视觉)', inputTokens: 4200000, outputTokens: 890000, calls: 1240, cost: 18.4 },
  ],
  '全模态': [],
  '语音': [
    { model: 'paraformer-realtime-v2', inputTokens: 0, outputTokens: 0, calls: 6720, cost: 6.72 },
    { model: 'cosyvoice-v2', inputTokens: 0, outputTokens: 0, calls: 9840, cost: 9.84 },
  ],
  '向量': [
    { model: 'text-embedding-3-small', inputTokens: 45200000, outputTokens: 0, calls: 45200, cost: 1.81 },
  ],
}

function formatTokens(n: number) {
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)}B`
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)}K`
  return String(n)
}

export default function UsagePage() {
  const [range, setRange] = useState('本月')

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">模型用量</h1>
            <p className="text-sm text-zinc-500 mt-0.5">各模型 Token 消耗和费用明细</p>
          </div>
          <div className="flex gap-1 bg-zinc-900 border border-zinc-800 rounded-lg p-0.5">
            {TIME_RANGES.map((r) => (
              <button key={r} onClick={() => setRange(r)} className={`px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer ${range === r ? 'bg-zinc-800 text-white' : 'text-zinc-500 hover:text-zinc-300'}`}>{r}</button>
            ))}
          </div>
        </div>
      </div>

      <div className="px-8 py-6 space-y-6">
        {/* Overview */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          {OVERVIEW.map(({ label, value, sub, icon: Icon, color, bg }) => (
            <div key={label} className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
              <div className="flex items-center justify-between mb-3">
                <p className="text-xs text-zinc-500">{label}</p>
                <div className={`w-8 h-8 rounded-lg ${bg} flex items-center justify-center`}>
                  <Icon className={`w-4 h-4 ${color}`} strokeWidth={1.5} />
                </div>
              </div>
              <p className="text-xl font-semibold text-white mb-0.5">{value}</p>
              <p className="text-[11px] text-zinc-600">{sub}</p>
            </div>
          ))}
        </div>

        {/* By type */}
        <Tabs defaultValue="文本">
          <TabsList className="bg-zinc-900 border border-zinc-800 h-9 p-0.5 mb-5">
            {Object.keys(MODEL_USAGE).map((t) => (
              <TabsTrigger key={t} value={t} className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4">{t}</TabsTrigger>
            ))}
          </TabsList>

          {Object.entries(MODEL_USAGE).map(([type, rows]) => (
            <TabsContent key={type} value={type}>
              {rows.length === 0 ? (
                <div className="bg-zinc-900 border border-zinc-800 rounded-xl p-10 text-center">
                  <p className="text-zinc-500 text-sm">本期无{type}模型用量</p>
                </div>
              ) : (
                <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-zinc-800">
                        {['模型', '调用次数', '输入 Token', '输出 Token', '总 Token', '估算费用'].map((h) => (
                          <th key={h} className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">{h}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((row, i) => (
                        <tr key={row.model} className={`border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors ${i === rows.length - 1 ? 'border-0' : ''}`}>
                          <td className="px-4 py-3.5 text-sm text-white font-mono">{row.model}</td>
                          <td className="px-4 py-3.5 text-sm text-zinc-300 font-mono">{row.calls.toLocaleString()}</td>
                          <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{row.inputTokens > 0 ? formatTokens(row.inputTokens) : '—'}</td>
                          <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{row.outputTokens > 0 ? formatTokens(row.outputTokens) : '—'}</td>
                          <td className="px-4 py-3.5 text-sm text-violet-400 font-mono">{formatTokens(row.inputTokens + row.outputTokens)}</td>
                          <td className="px-4 py-3.5 text-sm text-amber-400 font-mono">¥ {row.cost.toFixed(2)}</td>
                        </tr>
                      ))}
                      <tr className="border-t border-zinc-700 bg-zinc-800/30">
                        <td className="px-4 py-3 text-xs font-semibold text-zinc-400">合计</td>
                        <td className="px-4 py-3 text-xs font-mono text-zinc-300">{rows.reduce((s, r) => s + r.calls, 0).toLocaleString()}</td>
                        <td colSpan={2} />
                        <td className="px-4 py-3 text-xs font-mono text-violet-300">{formatTokens(rows.reduce((s, r) => s + r.inputTokens + r.outputTokens, 0))}</td>
                        <td className="px-4 py-3 text-xs font-mono text-amber-300">¥ {rows.reduce((s, r) => s + r.cost, 0).toFixed(2)}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              )}
            </TabsContent>
          ))}
        </Tabs>
      </div>
    </div>
  )
}
