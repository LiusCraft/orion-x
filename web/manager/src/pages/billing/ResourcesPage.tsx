import { CreditCard, Globe, Music, Package } from 'lucide-react'

const RESOURCES = [
  {
    id: 'r1',
    name: 'Web Search MCP',
    category: 'MCP',
    icon: Globe,
    iconColor: 'text-blue-400',
    iconBg: 'bg-blue-400/10',
    billingModel: '按次计费',
    unitPrice: '¥ 0.002 / 次',
    thisMonth: 12840,
    unit: '次',
    cost: 25.68,
    status: 'active',
  },
  {
    id: 'r2',
    name: 'Code Executor MCP',
    category: 'MCP',
    icon: Package,
    iconColor: 'text-violet-400',
    iconBg: 'bg-violet-400/10',
    billingModel: '按时计费',
    unitPrice: '¥ 0.05 / 小时',
    thisMonth: 48.5,
    unit: '小时',
    cost: 2.43,
    status: 'active',
  },
  {
    id: 'r3',
    name: '音色 · 晓晓',
    category: '音色',
    icon: Music,
    iconColor: 'text-pink-400',
    iconBg: 'bg-pink-400/10',
    billingModel: '按次计费',
    unitPrice: '¥ 0.0008 / 字符',
    thisMonth: 428000,
    unit: '字符',
    cost: 342.4,
    status: 'active',
  },
  {
    id: 'r4',
    name: '音色 · 云扬',
    category: '音色',
    icon: Music,
    iconColor: 'text-blue-400',
    iconBg: 'bg-blue-400/10',
    billingModel: '按次计费',
    unitPrice: '¥ 0.0008 / 字符',
    thisMonth: 186000,
    unit: '字符',
    cost: 148.8,
    status: 'active',
  },
  {
    id: 'r5',
    name: 'File Storage MCP',
    category: 'MCP',
    icon: Package,
    iconColor: 'text-amber-400',
    iconBg: 'bg-amber-400/10',
    billingModel: '包月',
    unitPrice: '¥ 9.9 / 月',
    thisMonth: 1,
    unit: '月',
    cost: 9.9,
    status: 'active',
  },
]

const CATEGORY_BADGE: Record<string, string> = {
  'MCP': 'bg-violet-600/15 text-violet-400 border-violet-500/20',
  '音色': 'bg-pink-400/10 text-pink-400 border-pink-400/20',
}

function formatUsage(value: number, unit: string) {
  if (unit === '次' && value >= 10000) return `${(value / 10000).toFixed(1)}w 次`
  if (unit === '字符' && value >= 10000) return `${(value / 10000).toFixed(1)}w 字符`
  if (unit === '小时') return `${value.toFixed(1)} 小时`
  return `${value} ${unit}`
}

export default function ResourcesPage() {
  const total = RESOURCES.reduce((s, r) => s + r.cost, 0)

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">其他资源计费</h1>
            <p className="text-sm text-zinc-500 mt-0.5">MCP 服务、音色等按使用量计费的资源</p>
          </div>
          <div className="text-right">
            <p className="text-xs text-zinc-500">本月合计</p>
            <p className="text-xl font-semibold text-amber-400 font-mono">¥ {total.toFixed(2)}</p>
          </div>
        </div>
      </div>

      <div className="px-8 py-6">
        <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-zinc-800">
                {['资源名称', '分类', '计费模式', '单价', '本月用量', '本月费用'].map((h) => (
                  <th key={h} className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {RESOURCES.map((res, i) => {
                const Icon = res.icon
                return (
                  <tr key={res.id} className={`border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors ${i === RESOURCES.length - 1 ? 'border-0' : ''}`}>
                    <td className="px-4 py-3.5">
                      <div className="flex items-center gap-2.5">
                        <div className={`w-7 h-7 rounded-lg ${res.iconBg} flex items-center justify-center shrink-0`}>
                          <Icon className={`w-3.5 h-3.5 ${res.iconColor}`} strokeWidth={1.5} />
                        </div>
                        <span className="text-sm text-zinc-200">{res.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <span className={`text-[11px] px-1.5 py-0.5 rounded border ${CATEGORY_BADGE[res.category] ?? 'bg-zinc-800 text-zinc-400 border-zinc-700'}`}>
                        {res.category}
                      </span>
                    </td>
                    <td className="px-4 py-3.5">
                      <span className={`text-xs px-2 py-0.5 rounded ${
                        res.billingModel === '按次计费' ? 'bg-zinc-800 text-zinc-400' :
                        res.billingModel === '按时计费' ? 'bg-blue-400/10 text-blue-400' :
                        'bg-emerald-400/10 text-emerald-400'
                      }`}>
                        {res.billingModel}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 text-xs text-zinc-400 font-mono">{res.unitPrice}</td>
                    <td className="px-4 py-3.5 text-sm text-zinc-300 font-mono">{formatUsage(res.thisMonth, res.unit)}</td>
                    <td className="px-4 py-3.5 text-sm text-amber-400 font-mono">¥ {res.cost.toFixed(2)}</td>
                  </tr>
                )
              })}
              <tr className="border-t border-zinc-700 bg-zinc-800/30">
                <td colSpan={4} className="px-4 py-3 text-xs font-semibold text-zinc-400">合计</td>
                <td />
                <td className="px-4 py-3 text-sm font-semibold text-amber-300 font-mono">¥ {total.toFixed(2)}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div className="mt-4 bg-zinc-900/50 border border-zinc-800 rounded-xl p-4">
          <p className="text-xs text-zinc-500 leading-relaxed">
            <span className="text-zinc-400 font-medium">计费说明：</span>
            按次计费在每次调用后计入；按时计费以实际运行时长计算；包月服务在当月开始时一次性扣除。所有费用均为估算，实际账单以月底结算为准。
          </p>
        </div>
      </div>
    </div>
  )
}
