import { useState } from 'react'
import { BookOpen, Trash2, Upload, FileText, FileSpreadsheet, File, Search, CheckCircle2, Clock, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'

type DocStatus = 'indexed' | 'processing' | 'failed'

const STATUS_MAP: Record<DocStatus, { label: string; icon: React.ElementType; cls: string }> = {
  indexed: { label: '已索引', icon: CheckCircle2, cls: 'text-emerald-400' },
  processing: { label: '处理中', icon: Clock, cls: 'text-amber-400' },
  failed: { label: '失败', icon: AlertCircle, cls: 'text-red-400' },
}

interface KnowledgeDoc {
  id: string
  name: string
  type: string
  size: string
  status: DocStatus
  chunks: number
  uploadedAt: string
}

const MOCK_DOCS: KnowledgeDoc[] = [
  { id: 'd1', name: '产品手册 v2.1.pdf', type: 'PDF', size: '2.4 MB', status: 'indexed', chunks: 128, uploadedAt: '2025-07-01' },
  { id: 'd2', name: '常见问题 FAQ.docx', type: 'Word', size: '456 KB', status: 'indexed', chunks: 42, uploadedAt: '2025-06-28' },
  { id: 'd3', name: '销售数据 Q2.xlsx', type: 'Excel', size: '1.1 MB', status: 'processing', chunks: 0, uploadedAt: '2025-07-04' },
  { id: 'd4', name: '技术规范文档.pdf', type: 'PDF', size: '5.8 MB', status: 'indexed', chunks: 320, uploadedAt: '2025-06-20' },
  { id: 'd5', name: '用户访谈记录.txt', type: 'TXT', size: '89 KB', status: 'failed', chunks: 0, uploadedAt: '2025-07-03' },
]

function FileIcon({ type }: { type: string }) {
  if (type === 'PDF') return <FileText className="w-4 h-4 text-red-400" strokeWidth={1.5} />
  if (type === 'Excel') return <FileSpreadsheet className="w-4 h-4 text-emerald-400" strokeWidth={1.5} />
  if (type === 'Word') return <FileText className="w-4 h-4 text-blue-400" strokeWidth={1.5} />
  return <File className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />
}

export default function KnowledgePage() {
  const [docs, setDocs] = useState<KnowledgeDoc[]>(MOCK_DOCS)
  const [query, setQuery] = useState('')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [dragging, setDragging] = useState(false)

  const filtered = docs.filter((d) => !query || d.name.toLowerCase().includes(query.toLowerCase()))
  const totalChunks = docs.filter((d) => d.status === 'indexed').reduce((s, d) => s + d.chunks, 0)

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">知识库</h1>
            <p className="text-sm text-zinc-500 mt-0.5">上传文档，让智能体具备特定领域知识</p>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-xs text-zinc-600 bg-zinc-900 border border-zinc-800 px-2.5 py-1.5 rounded-lg">
              {totalChunks} 个向量块已索引
            </span>
            <Button
              onClick={() => setUploadOpen(true)}
              className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
            >
              <Upload className="w-4 h-4" />
              上传文档
            </Button>
          </div>
        </div>
        <div className="relative mt-4 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索文档..."
            className="pl-9 bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600 h-9 text-sm focus-visible:ring-violet-500"
          />
        </div>
      </div>

      <div className="px-8 py-6">
        {filtered.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <BookOpen className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">知识库为空</p>
            <p className="text-zinc-600 text-xs mt-1 mb-4">上传 PDF、Word、TXT 等文档</p>
            <Button onClick={() => setUploadOpen(true)} className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5">
              <Upload className="w-3.5 h-3.5" />上传文档
            </Button>
          </div>
        ) : (
          <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-zinc-800">
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">文档名称</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-16">类型</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-20">大小</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-24">状态</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-20">向量块</th>
                  <th className="text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider w-28">上传时间</th>
                  <th className="px-4 py-3 w-16"></th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((doc, i) => {
                  const { label, icon: StatusIcon, cls } = STATUS_MAP[doc.status]
                  return (
                    <tr key={doc.id} className={`border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors group ${i === filtered.length - 1 ? 'border-0' : ''}`}>
                      <td className="px-4 py-3.5">
                        <div className="flex items-center gap-2.5">
                          <div className="w-7 h-7 rounded-lg bg-zinc-800 border border-zinc-700/50 flex items-center justify-center shrink-0">
                            <FileIcon type={doc.type} />
                          </div>
                          <span className="text-sm text-zinc-200">{doc.name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3.5 text-xs text-zinc-500 font-mono">{doc.type}</td>
                      <td className="px-4 py-3.5 text-xs text-zinc-500">{doc.size}</td>
                      <td className="px-4 py-3.5">
                        <span className={`flex items-center gap-1 text-xs ${cls}`}>
                          <StatusIcon className="w-3 h-3" />
                          {label}
                        </span>
                      </td>
                      <td className="px-4 py-3.5 text-xs text-zinc-500">
                        {doc.status === 'indexed' ? doc.chunks : '—'}
                      </td>
                      <td className="px-4 py-3.5 text-xs text-zinc-600 font-mono">{doc.uploadedAt}</td>
                      <td className="px-4 py-3.5">
                        <button
                          onClick={() => setDocs((prev) => prev.filter((d) => d.id !== doc.id))}
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

      {/* Upload Dialog */}
      <Dialog open={uploadOpen} onOpenChange={setUploadOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Upload className="w-4 h-4 text-violet-400" />
              上传文档
            </DialogTitle>
          </DialogHeader>
          <div className="py-2">
            <div
              onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
              onDragLeave={() => setDragging(false)}
              onDrop={(e) => { e.preventDefault(); setDragging(false) }}
              className={`border-2 border-dashed rounded-xl p-10 text-center transition-all ${
                dragging ? 'border-violet-500 bg-violet-600/5' : 'border-zinc-700 hover:border-zinc-600'
              }`}
            >
              <Upload className="w-8 h-8 text-zinc-600 mx-auto mb-3" strokeWidth={1.5} />
              <p className="text-sm text-zinc-400 mb-1">拖拽文件到此处，或</p>
              <button className="text-sm text-violet-400 hover:text-violet-300 underline underline-offset-2 cursor-pointer">
                点击选择文件
              </button>
              <p className="text-xs text-zinc-600 mt-3">支持 PDF、Word、TXT、Excel，单文件最大 50MB</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setUploadOpen(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
