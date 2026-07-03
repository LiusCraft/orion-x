import { useState } from 'react'
import { Cpu, Plus, ExternalLink, CheckCircle2, Clock, Globe, Shield, Database, Search as SearchIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'

const MARKET_MCPS = [
  {
    id: 'm1',
    name: 'Web Search',
    provider: '官方',
    icon: Globe,
    iconColor: 'text-blue-400',
    iconBg: 'bg-blue-400/10',
    desc: '实时网络搜索工具，支持 Google、Bing 等主流搜索引擎，返回摘要和链接。',
    tags: ['搜索', '官方'],
    billing: '按次计费',
    price: '¥0.002/次',
    activated: true,
  },
  {
    id: 'm2',
    name: 'Database Query',
    provider: '官方',
    icon: Database,
    iconColor: 'text-emerald-400',
    iconBg: 'bg-emerald-400/10',
    desc: '连接 PostgreSQL、MySQL 数据库，支持安全的 SQL 查询执行。',
    tags: ['数据库', '官方'],
    billing: '按次计费',
    price: '¥0.001/次',
    activated: false,
  },
  {
    id: 'm3',
    name: 'Code Executor',
    provider: '官方',
    icon: Shield,
    iconColor: 'text-violet-400',
    iconBg: 'bg-violet-400/10',
    desc: '安全的代码执行沙箱，支持 Python、JavaScript 等语言，带超时和资源限制。',
    tags: ['代码', '沙箱', '官方'],
    billing: '按时计费',
    price: '¥0.05/小时',
    activated: true,
  },
  {
    id: 'm4',
    name: 'File Storage',
    provider: '官方',
    icon: Database,
    iconColor: 'text-amber-400',
    iconBg: 'bg-amber-400/10',
    desc: '云端文件存储和检索，支持文档、图片、音视频文件的读写操作。',
    tags: ['存储', '官方'],
    billing: '包月',
    price: '¥9.9/月',
    activated: false,
  },
  {
    id: 'm5',
    name: '钉钉 MCP',
    provider: '第三方',
    icon: Globe,
    iconColor: 'text-cyan-400',
    iconBg: 'bg-cyan-400/10',
    desc: '集成钉钉消息、日程、任务等功能，让 AI 可以发送消息、创建日程。',
    tags: ['钉钉', '协作', '第三方'],
    billing: '免费',
    price: '免费',
    activated: false,
  },
  {
    id: 'm6',
    name: 'Notion MCP',
    provider: '第三方',
    icon: Globe,
    iconColor: 'text-zinc-300',
    iconBg: 'bg-zinc-700/50',
    desc: '读写 Notion 页面、数据库，将 AI 生成内容直接存入 Notion 工作区。',
    tags: ['笔记', '知识库', '第三方'],
    billing: '免费',
    price: '免费',
    activated: false,
  },
]

const ACTIVATED = MARKET_MCPS.filter((m) => m.activated)

export default function McpPage() {
  const [activated, setActivated] = useState<Set<string>>(new Set(ACTIVATED.map((m) => m.id)))

  const toggle = (id: string) => {
    setActivated((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">MCP 管理</h1>
            <p className="text-sm text-zinc-500 mt-0.5">Model Context Protocol 服务，为智能体提供外部工具能力</p>
          </div>
          <Button variant="outline" className="h-9 px-4 text-sm border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1.5">
            <Plus className="w-4 h-4" />
            自定义 MCP
          </Button>
        </div>
      </div>

      <div className="px-8 py-6">
        <Tabs defaultValue="market">
          <TabsList className="bg-zinc-900 border border-zinc-800 h-9 p-0.5 mb-6">
            <TabsTrigger value="market" className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4">
              市场
            </TabsTrigger>
            <TabsTrigger value="activated" className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4">
              已开通 ({activated.size})
            </TabsTrigger>
          </TabsList>

          <TabsContent value="market">
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {MARKET_MCPS.map((mcp) => {
                const Icon = mcp.icon
                const isActivated = activated.has(mcp.id)
                return (
                  <div key={mcp.id} className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all duration-200">
                    <div className="flex items-start gap-3 mb-3">
                      <div className={`w-9 h-9 rounded-xl ${mcp.iconBg} flex items-center justify-center shrink-0`}>
                        <Icon className={`w-4 h-4 ${mcp.iconColor}`} strokeWidth={1.5} />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <p className="font-medium text-sm text-white truncate">{mcp.name}</p>
                          {mcp.provider === '官方' && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-600/20 text-violet-400 border border-violet-500/20 shrink-0">官方</span>
                          )}
                        </div>
                        <p className="text-[11px] text-zinc-600 mt-0.5">{mcp.price}</p>
                      </div>
                    </div>

                    <p className="text-xs text-zinc-500 leading-relaxed mb-3 line-clamp-2">{mcp.desc}</p>

                    <div className="flex flex-wrap gap-1 mb-4">
                      {mcp.tags.filter((t) => t !== mcp.provider).map((tag) => (
                        <span key={tag} className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">
                          {tag}
                        </span>
                      ))}
                    </div>

                    <div className="flex items-center justify-between">
                      {isActivated ? (
                        <span className="flex items-center gap-1 text-[11px] text-emerald-400">
                          <CheckCircle2 className="w-3 h-3" />
                          已开通
                        </span>
                      ) : (
                        <span className="text-[11px] text-zinc-600">{mcp.billing}</span>
                      )}
                      <Button
                        size="sm"
                        onClick={() => toggle(mcp.id)}
                        className={`h-7 px-3 text-xs ${
                          isActivated
                            ? 'border-zinc-700 text-zinc-400 hover:bg-zinc-800 hover:text-red-400 hover:border-red-400/30'
                            : 'bg-violet-600 hover:bg-violet-500 text-white'
                        }`}
                        variant={isActivated ? 'outline' : 'default'}
                      >
                        {isActivated ? '关闭' : '开通'}
                      </Button>
                    </div>
                  </div>
                )
              })}
            </div>
          </TabsContent>

          <TabsContent value="activated">
            {activated.size === 0 ? (
              <div className="flex flex-col items-center py-20">
                <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
                  <Cpu className="w-6 h-6 text-zinc-600" />
                </div>
                <p className="text-zinc-400 text-sm">还没有开通任何 MCP</p>
                <p className="text-zinc-600 text-xs mt-1">前往市场开通需要的 MCP 服务</p>
              </div>
            ) : (
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {MARKET_MCPS.filter((m) => activated.has(m.id)).map((mcp) => {
                  const Icon = mcp.icon
                  return (
                    <div key={mcp.id} className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
                      <div className="flex items-center gap-3 mb-3">
                        <div className={`w-9 h-9 rounded-xl ${mcp.iconBg} flex items-center justify-center`}>
                          <Icon className={`w-4 h-4 ${mcp.iconColor}`} strokeWidth={1.5} />
                        </div>
                        <div>
                          <p className="font-medium text-sm text-white">{mcp.name}</p>
                          <span className="flex items-center gap-1 text-[11px] text-emerald-400 mt-0.5">
                            <CheckCircle2 className="w-3 h-3" />
                            运行中
                          </span>
                        </div>
                      </div>
                      <div className="flex items-center justify-between mt-2">
                        <span className="text-[11px] text-zinc-600">{mcp.price}</span>
                        <div className="flex gap-1.5">
                          <button className="text-xs text-zinc-500 hover:text-zinc-300 px-2 py-1 rounded hover:bg-zinc-800 transition-colors cursor-pointer">
                            配置
                          </button>
                          <button
                            onClick={() => toggle(mcp.id)}
                            className="text-xs text-zinc-500 hover:text-red-400 px-2 py-1 rounded hover:bg-red-400/10 transition-colors cursor-pointer"
                          >
                            关闭
                          </button>
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
