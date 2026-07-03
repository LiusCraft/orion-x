import { useState } from 'react'
import { Activity, TrendingUp, TrendingDown, AlertTriangle, Clock, Zap } from 'lucide-react'

const TIME_RANGES = ['今日', '7 天', '30 天']

const MOCK_MODELS = [
  { name: 'claude-sonnet-4-6', type: '文本', calls: 12840, failed: 42, avgDuration: '1.24s', firstPacket: '0.38s', contentErrors: 3, rateLimitErrors: 12, rpm: 248, tpm: 186400 },
  { name: 'claude-opus-4-8', type: '文本', calls: 3201, failed: 8, avgDuration: '2.81s', firstPacket: '0.72s', contentErrors: 0, rateLimitErrors: 4, rpm: 62, tpm: 95200 },
  { name: 'claude-haiku-4-5', type: '文本', calls: 28450, failed: 105, avgDuration: '0.64s', firstPacket: '0.18s', contentErrors: 8, rateLimitErrors: 45, rpm: 542, tpm: 312000 },
  { name: 'Whisper-v3', type: '语音', calls: 6720, failed: 22, avgDuration: '0.89s', firstPacket: '0.21s', contentErrors: 0, rateLimitErrors: 6, rpm: 128, tpm: 0 },
  { name: 'CosyVoice-2', type: '语音', calls: 9840, failed: 18, avgDuration: '0.45s', firstPacket: '0.12s', contentErrors: 0, rateLimitErrors: 2, rpm: 190, tpm: 0 },
  { name: 'text-embedding-v3', type: '向量', calls: 45200, failed: 92, avgDuration: '0.12s', firstPacket: '—', contentErrors: 0, rateLimitErrors: 21, rpm: 870, tpm: 2840000 },
]

const OVERVIEW = [
  { label: '总调用次数', value: '106,251', icon: Zap, change: '+12.4%', up: true, color: 'text-violet-400', bg: 'bg-violet-400/10' },
  { label: '总体失败率', value: '0.27%', icon: AlertTriangle, change: '-0.05%', up: false, color: 'text-amber-400', bg: 'bg-amber-400/10' },
  { label: '平均响应时长', value: '0.69s', icon: Clock, change: '-0.08s', up: false, color: 'text-blue-400', bg: 'bg-blue-400/10' },
  { label: '平均首包时长', value: '0.32s', icon: Activity, change: '-0.02s', up: false, color: 'text-emerald-400', bg: 'bg-emerald-400/10' },
]

function FailRate({ calls, failed }: { calls: number; failed: number }) {
  const rate = calls === 0 ? 0 : (failed / calls) * 100
  const cls = rate > 1 ? 'text-red-400' : rate > 0.3 ? 'text-amber-400' : 'text-emerald-400'
  return <span className={cls}>{rate.toFixed(2)}%</span>
}

export default function ModelMonitorPage() {
  const [range, setRange] = useState('今日')

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">模型监控</h1>
            <p className="text-sm text-zinc-500 mt-0.5">实时查看各模型调用质量与性能指标</p>
          </div>
          <div className="flex gap-1 bg-zinc-900 border border-zinc-800 rounded-lg p-0.5">
            {TIME_RANGES.map((r) => (
              <button
                key={r}
                onClick={() => setRange(r)}
                className={`px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer ${
                  range === r ? 'bg-zinc-800 text-white' : 'text-zinc-500 hover:text-zinc-300'
                }`}
              >
                {r}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="px-8 py-6 space-y-6">
        {/* Overview cards */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          {OVERVIEW.map(({ label, value, icon: Icon, change, up, color, bg }) => (
            <div key={label} className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
              <div className="flex items-center justify-between mb-3">
                <p className="text-xs text-zinc-500">{label}</p>
                <div className={`w-8 h-8 rounded-lg ${bg} flex items-center justify-center`}>
                  <Icon className={`w-4 h-4 ${color}`} strokeWidth={1.5} />
                </div>
              </div>
              <p className="text-2xl font-semibold text-white mb-1">{value}</p>
              <span className={`flex items-center gap-0.5 text-xs ${up ? 'text-emerald-400' : 'text-zinc-500'}`}>
                {up ? <TrendingUp className="w-3 h-3" /> : <TrendingDown className="w-3 h-3" />}
                {change} 较上期
              </span>
            </div>
          ))}
        </div>

        {/* Table */}
        <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
          <div className="px-5 py-3.5 border-b border-zinc-800">
            <h2 className="text-sm font-medium text-white">各模型详情</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[900px]">
              <thead>
                <tr className="border-b border-zinc-800">
                  {['模型名称', '类型', '调用次数', '失败次数', '失败率', '均时长', '首包时长', '安全错误', '限流错误', 'RPM', 'TPM'].map((h) => (
                    <th key={h} className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider whitespace-nowrap">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {MOCK_MODELS.map((m, i) => (
                  <tr key={m.name} className={`border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors ${i === MOCK_MODELS.length - 1 ? 'border-0' : ''}`}>
                    <td className="px-4 py-3.5">
                      <span className="text-sm text-white font-mono">{m.name}</span>
                    </td>
                    <td className="px-4 py-3.5">
                      <span className={`text-[11px] px-1.5 py-0.5 rounded border ${
                        m.type === '文本' ? 'bg-violet-600/15 text-violet-400 border-violet-500/20'
                        : m.type === '语音' ? 'bg-blue-400/10 text-blue-400 border-blue-400/20'
                        : 'bg-emerald-400/10 text-emerald-400 border-emerald-400/20'
                      }`}>
                        {m.type}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 text-sm text-zinc-300 font-mono">{m.calls.toLocaleString()}</td>
                    <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{m.failed}</td>
                    <td className="px-4 py-3.5 text-sm font-mono"><FailRate calls={m.calls} failed={m.failed} /></td>
                    <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{m.avgDuration}</td>
                    <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{m.firstPacket}</td>
                    <td className="px-4 py-3.5 text-sm font-mono">
                      <span className={m.contentErrors > 0 ? 'text-red-400' : 'text-zinc-600'}>{m.contentErrors}</span>
                    </td>
                    <td className="px-4 py-3.5 text-sm font-mono">
                      <span className={m.rateLimitErrors > 10 ? 'text-amber-400' : 'text-zinc-500'}>{m.rateLimitErrors}</span>
                    </td>
                    <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{m.rpm}</td>
                    <td className="px-4 py-3.5 text-sm text-zinc-400 font-mono">{m.tpm > 0 ? m.tpm.toLocaleString() : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  )
}
