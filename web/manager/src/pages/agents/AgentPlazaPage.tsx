import { useState } from 'react'
import { Search, Bot, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const CATEGORIES = ['全部', '对话助手', '工具专家', '创意创作', '知识问答', '生活服务', '医疗健康']

const TEMPLATES = [
  {
    id: '1',
    name: '通用对话助手',
    icon: '🤖',
    color: 'from-violet-500 to-purple-600',
    tags: ['对话助手', '官方'],
    desc: '具备多轮对话能力的通用 AI 助手，支持工具调用和长期记忆，适合各类日常问答场景。',
    uses: 2841,
    category: '对话助手',
  },
  {
    id: '2',
    name: '代码智能体',
    icon: '💻',
    color: 'from-cyan-500 to-blue-600',
    tags: ['工具专家', '编程'],
    desc: '专注于代码生成、调试、重构的智能体，内置代码执行工具，支持 20+ 编程语言。',
    uses: 1932,
    category: '工具专家',
  },
  {
    id: '3',
    name: '内容创作助手',
    icon: '✍️',
    color: 'from-rose-500 to-pink-600',
    tags: ['创意创作'],
    desc: '专业的文案、剧本、故事创作助手，具备高情感表达能力，支持多种文体风格切换。',
    uses: 1487,
    category: '创意创作',
  },
  {
    id: '4',
    name: '知识库问答',
    icon: '📚',
    color: 'from-amber-500 to-orange-600',
    tags: ['知识问答', 'RAG'],
    desc: '基于向量知识库的问答智能体，支持上传文档后精准检索，适合企业内部知识管理。',
    uses: 1103,
    category: '知识问答',
  },
  {
    id: '5',
    name: '旅行规划师',
    icon: '✈️',
    color: 'from-emerald-500 to-teal-600',
    tags: ['生活服务', '工具专家'],
    desc: '能够搜索机票、酒店、景点并生成完整行程规划的旅行 AI，内置地图和搜索工具。',
    uses: 876,
    category: '生活服务',
  },
  {
    id: '6',
    name: '健康问诊助手',
    icon: '🏥',
    color: 'from-green-500 to-emerald-600',
    tags: ['医疗健康'],
    desc: '提供健康咨询、症状分析、用药建议的 AI 助手，仅供参考，不替代专业医疗意见。',
    uses: 654,
    category: '医疗健康',
  },
  {
    id: '7',
    name: '数据分析师',
    icon: '📊',
    color: 'from-blue-500 to-indigo-600',
    tags: ['工具专家', '数据'],
    desc: '上传 CSV/Excel 后自动分析数据、生成图表和洞察报告，支持 SQL 查询和 Python 分析。',
    uses: 598,
    category: '工具专家',
  },
  {
    id: '8',
    name: '语音播报员',
    icon: '🎙️',
    color: 'from-fuchsia-500 to-violet-600',
    tags: ['对话助手', '语音'],
    desc: '专为语音交互优化的播报型智能体，回答简洁流畅，适合 IoT 设备和语音终端场景。',
    uses: 421,
    category: '对话助手',
  },
]

export default function AgentPlazaPage() {
  const [query, setQuery] = useState('')
  const [activeCategory, setActiveCategory] = useState('全部')

  const filtered = TEMPLATES.filter((t) => {
    const matchCategory = activeCategory === '全部' || t.category === activeCategory
    const matchQuery = !query || t.name.includes(query) || t.desc.includes(query)
    return matchCategory && matchQuery
  })

  return (
    <div className="min-h-full">
      {/* Header */}
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">智能体广场</h1>
            <p className="text-sm text-zinc-500 mt-0.5">选择模板快速创建，或从零构建你的专属智能体</p>
          </div>
          <Button className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20">
            <Sparkles className="w-3.5 h-3.5" />
            从零创建
          </Button>
        </div>

        {/* Search */}
        <div className="relative mt-4 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索智能体模板..."
            className="pl-9 bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600 h-9 text-sm focus-visible:ring-violet-500 focus-visible:border-violet-500"
          />
        </div>

        {/* Category filter */}
        <div className="flex gap-1.5 mt-3 flex-wrap">
          {CATEGORIES.map((cat) => (
            <button
              key={cat}
              onClick={() => setActiveCategory(cat)}
              className={`px-3 py-1 rounded-full text-xs font-medium transition-all duration-150 cursor-pointer ${
                activeCategory === cat
                  ? 'bg-violet-600 text-white'
                  : 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200'
              }`}
            >
              {cat}
            </button>
          ))}
        </div>
      </div>

      {/* Grid */}
      <div className="px-8 py-6">
        {filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Bot className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">没有找到匹配的模板</p>
            <p className="text-zinc-600 text-xs mt-1">试试其他关键词或分类</p>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {filtered.map((tpl) => (
              <div
                key={tpl.id}
                className="group bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 hover:bg-zinc-800/50 transition-all duration-200"
              >
                {/* Icon */}
                <div className={`w-10 h-10 rounded-xl bg-gradient-to-br ${tpl.color} flex items-center justify-center text-xl mb-3 shadow-lg`}>
                  {tpl.icon}
                </div>

                {/* Title + tags */}
                <div className="flex items-start justify-between mb-1.5">
                  <p className="font-medium text-sm text-white leading-snug">{tpl.name}</p>
                </div>
                <div className="flex flex-wrap gap-1 mb-2.5">
                  {tpl.tags.map((tag) => (
                    <span key={tag} className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/50">
                      {tag}
                    </span>
                  ))}
                </div>

                {/* Desc */}
                <p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">{tpl.desc}</p>

                {/* Footer */}
                <div className="flex items-center justify-between">
                  <span className="text-[11px] text-zinc-600">{tpl.uses.toLocaleString()} 次使用</span>
                  <div className="flex gap-1.5">
                    <button className="text-xs text-zinc-500 hover:text-zinc-300 px-2 py-1 rounded hover:bg-zinc-800 transition-colors cursor-pointer">
                      预览
                    </button>
                    <Button size="sm" className="h-7 px-3 text-xs bg-violet-600 hover:bg-violet-500 text-white">
                      基于此创建
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
