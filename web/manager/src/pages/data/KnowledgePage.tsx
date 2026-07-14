import { useEffect, useState, useRef, useCallback } from "react"
import {
	Brain, Trash2, Upload, Search, ChevronRight, ChevronDown, RotateCw,
	BookOpen, FileText, Link2, CheckCircle2, Clock, AlertCircle, Loader2, Plus
} from "lucide-react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { modelApi, knowledgeApi, voicebotApi, type KnowledgeBase, type KnowledgeDocument, type AIModel } from "@/lib/api"

const STATUS_MAP: Record<string, { label: string; icon: React.ElementType; cls: string }> = {
	ready: { label: "已索引", icon: CheckCircle2, cls: "text-emerald-400" },
	pending: { label: "等待中", icon: Clock, cls: "text-zinc-400" },
	parsing: { label: "解析中", icon: Loader2, cls: "text-amber-400 animate-spin" },
	chunking: { label: "分块中", icon: Loader2, cls: "text-amber-400 animate-spin" },
	embedding: { label: "向量化", icon: Loader2, cls: "text-amber-400 animate-spin" },
	storing: { label: "存储中", icon: Loader2, cls: "text-amber-400 animate-spin" },
	error: { label: "失败", icon: AlertCircle, cls: "text-red-400" },
}

function shortId(id: string) {
	return id.length > 8 ? id.slice(0, 8) + "…" : id
}

function formatSize(chars: number) {
	if (chars >= 1_000_000) return `${(chars / 1_000_000).toFixed(1)} MB`
	if (chars >= 1_000) return `${(chars / 1_000).toFixed(1)} KB`
	return `${chars} B`
}

export default function KnowledgePage() {
	const [kbs, setKbs] = useState<KnowledgeBase[]>([])
	const [loadingKBs, setLoadingKBs] = useState(true)

	const [expandedKB, setExpandedKB] = useState<string | null>(null)
	const [docs, setDocs] = useState<KnowledgeDocument[]>([])
	const [loadingDocs, setLoadingDocs] = useState(false)

	const [searchText, setSearchText] = useState("")
	const [deleting, setDeleting] = useState<Set<string>>(new Set())

	// Create KB dialog
	const [createKBOpen, setCreateKBOpen] = useState(false)
	const [kbName, setKbName] = useState("")
	const [kbDesc, setKbDesc] = useState("")
	const [kbModelId, setKbModelId] = useState("")
	const [embModels, setEmbModels] = useState<AIModel[]>([])

	// Upload
	const [uploadOpen, setUploadOpen] = useState(false)
	const [uploadKBId, setUploadKBId] = useState("")
	const fileRef = useRef<HTMLInputElement>(null)
	const [fileDragging, setFileDragging] = useState(false)

	// URL import
	const [urlOpen, setUrlOpen] = useState(false)
	const [urlKBId, setUrlKBId] = useState("")
	const [urlInput, setUrlInput] = useState("")

	// Search within a KB
	const [kbSearchText, setKbSearchText] = useState("")
	const [kbSearchResults, setKbSearchResults] = useState<Array<{ chunk_id: string; content: string; score: number; document_name: string }>>([])
	const [kbSearchKBId, setKbSearchKBId] = useState("")
	const [kbSearching, setKbSearching] = useState(false)

	// ── Load all KBs ──
	const loadAllKBs = useCallback(async () => {
		setLoadingKBs(true)
		try {
			const { data } = await knowledgeApi.listAllKBs()
			setKbs(data.knowledge_bases || [])
		} catch {
			setKbs([])
		} finally {
			setLoadingKBs(false)
		}
	}, [])

	useEffect(() => { loadAllKBs() }, [loadAllKBs])

	// ── Load docs for expanded KB ──
	const loadDocs = useCallback(async (kbId: string) => {
		setLoadingDocs(true)
		try {
			const { data } = await knowledgeApi.listDocs(kbId)
			setDocs(data.documents || [])
		} catch {
			setDocs([])
		} finally {
			setLoadingDocs(false)
		}
	}, [])

	// Poll status for processing docs
	useEffect(() => {
		if (!expandedKB) return
		const hasProcessing = docs.some(d => d.status !== "ready" && d.status !== "error")
		if (!hasProcessing) return
		const timer = setInterval(() => loadDocs(expandedKB), 3000)
		return () => clearInterval(timer)
	}, [expandedKB, docs, loadDocs])

	// ── Expand/Collapse KBs ──
	const toggleKB = async (id: string) => {
		if (expandedKB === id) { setExpandedKB(null); setDocs([]); return }
		setExpandedKB(id)
		loadDocs(id)
	}

	// ── Create KB ──
	const openCreateKB = useCallback(async () => {
		setKbName(""); setKbDesc(""); setKbModelId("")
		setCreateKBOpen(true)
		try {
			const { data } = await modelApi.list("embedding")
			setEmbModels(Array.isArray(data) ? data : [])
		} catch {
			setEmbModels([])
		}
	}, [])

	const handleCreateKB = async () => {
		if (!kbName.trim() || !kbModelId) return
		try {
			const { data } = await voicebotApi.list()
			const bots = Array.isArray(data) ? data : []
			if (bots.length > 0) {
				await knowledgeApi.createKB(bots[0].id, { name: kbName, description: kbDesc, embedding_model_id: kbModelId })
				setCreateKBOpen(false)
				setKbName("")
				setKbDesc("")
				loadAllKBs()
			} else {
				alert("请先创建至少一个智能体")
			}
		} catch {
			/* silently fail */
		}
	}


	// ── Delete KB ──
	const handleDeleteKB = async (kbId: string) => {
		if (!confirm("确认删除该知识库及其所有文档？")) return
		setDeleting(p => new Set(p).add(kbId))
		try {
			await knowledgeApi.deleteKB(kbId)
			setKbs(prev => prev.filter(k => k.id !== kbId))
			if (expandedKB === kbId) { setExpandedKB(null); setDocs([]) }
		} catch {
			/* silently fail */
		} finally {
			setDeleting(p => { const n = new Set(p); n.delete(kbId); return n })
		}
	}

	// ── Upload ──
	const handleUpload = async () => {
		const file = fileRef.current?.files?.[0]
		if (!file || !uploadKBId) return
		try {
			await knowledgeApi.uploadDoc(uploadKBId, file)
			setUploadOpen(false)
			loadDocs(uploadKBId)
			if (fileRef.current) fileRef.current.value = ""
		} catch {
			/* silently fail */
		}
	}

	// ── Import URL ──
	const handleImportURL = async () => {
		if (!urlInput.trim() || !urlKBId) return
		try {
			await knowledgeApi.ingestURL(urlKBId, urlInput)
			setUrlOpen(false)
			setUrlInput("")
			loadDocs(urlKBId)
		} catch {
			/* silently fail */
		}
	}

	// ── Delete doc ──
	const handleDeleteDoc = async (docId: string) => {
		if (!confirm("确认删除该文档？")) return
		setDeleting(p => new Set(p).add(docId))
		try {
			await knowledgeApi.deleteDoc(docId)
			setDocs(prev => prev.filter(d => d.id !== docId))
		} catch {
			/* silently fail */
		} finally {
			setDeleting(p => { const n = new Set(p); n.delete(docId); return n })
		}
	}

	// ── Retry ──
	const handleRetryDoc = async (docId: string, kbId: string) => {
		setDeleting(p => new Set(p).add(docId))
		try {
			await knowledgeApi.retryDoc(docId)
			loadDocs(kbId)
		} catch {
			/* silently fail */
		} finally {
			setDeleting(p => { const n = new Set(p); n.delete(docId); return n })
		}
	}

	// ── Search within KB ──
	const handleKBSearch = async (kbId: string) => {
		if (!kbSearchText.trim()) return
		setKbSearchKBId(kbId)
		setKbSearching(true)
		try {
			const { data } = await knowledgeApi.searchKB(kbId, kbSearchText, 5)
			setKbSearchResults(Array.isArray(data) ? data : [])
		} catch {
			setKbSearchResults([])
		} finally {
			setKbSearching(false)
		}
	}

	const filteredKBs = kbs.filter(
		k => !searchText || k.name.toLowerCase().includes(searchText.toLowerCase())
	)

	return (
		<div className="min-h-full">
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-lg font-semibold text-white">知识库</h1>
						<p className="text-sm text-zinc-500 mt-0.5">独立的知识库，可绑定到任意智能体</p>
					</div>
					<div className="flex items-center gap-2">
						<Button
							size="sm"
							onClick={openCreateKB}
							className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-3 text-sm gap-1.5"
						>
							<Plus className="w-3.5 h-3.5" />
							新建知识库
						</Button>
						<span className="text-sm text-zinc-500 shrink-0 ml-2">
							<BookOpen className="w-4 h-4 text-zinc-600 inline mr-1" />
							{kbs.length} 个知识库
						</span>
					</div>
				</div>
				<div className="relative mt-4 max-w-md">
					<Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500 pointer-events-none" />
					<Input
						value={searchText}
						onChange={e => setSearchText(e.target.value)}
						placeholder="搜索知识库..."
						className="pl-9 bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600 h-9 text-sm focus-visible:ring-violet-500"
					/>
				</div>
			</div>

			<div className="px-8 py-6 space-y-4">
				{loadingKBs && (
					<div className="flex justify-center py-20">
						<div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
					</div>
				)}

				{!loadingKBs && filteredKBs.length === 0 && (
					<div className="flex flex-col items-center py-20">
						<div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
							<BookOpen className="w-6 h-6 text-zinc-600" />
						</div>
						<p className="text-zinc-400 text-sm">
							{searchText ? "没有匹配的知识库" : "暂无知识库"}
						</p>
						<p className="text-zinc-600 text-xs mt-1">创建知识库后即可上传文档</p>
					</div>
				)}

				{!loadingKBs && filteredKBs.length > 0 && (
					<div className="space-y-1">
						{filteredKBs.map(kb => {
							const kbExpanded = expandedKB === kb.id
							return (
								<div key={kb.id} className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden">
									<button
										onClick={() => toggleKB(kb.id)}
										className="w-full flex items-center gap-2.5 px-4 py-3 hover:bg-zinc-800/30 transition-colors text-left cursor-pointer"
									>
										{kbExpanded ? <ChevronDown className="w-3.5 h-3.5 text-zinc-600 shrink-0" /> : <ChevronRight className="w-3.5 h-3.5 text-zinc-600 shrink-0" />}
										<BookOpen className="w-3.5 h-3.5 text-violet-400 shrink-0" />
										<span className="text-sm text-zinc-300 flex-1 truncate">{kb.name}</span>
										{kb.description && <span className="text-xs text-zinc-600 truncate max-w-40">{kb.description}</span>}
										<span className="text-xs text-zinc-600 font-mono">{kb.embedding_model_id || "未配置"}</span>
										<button
											onClick={(e) => { e.stopPropagation(); handleDeleteKB(kb.id) }}
											disabled={deleting.has(kb.id)}
											className="shrink-0 text-zinc-600 hover:text-red-400 transition-all p-1 rounded hover:bg-red-400/10 disabled:opacity-40 cursor-pointer -mr-1"
										>
											<Trash2 className="w-3.5 h-3.5" />
										</button>
									</button>

									{kbExpanded && (
										<div className="border-t border-zinc-800">
											{/* Doc actions */}
											<div className="flex items-center gap-2 px-4 py-2 border-b border-zinc-800/30">
												<Button
													size="sm"
													onClick={() => { setUploadKBId(kb.id); setUploadOpen(true) }}
													className="bg-zinc-800 hover:bg-zinc-700 text-zinc-300 h-7 px-3 text-xs gap-1 border border-zinc-700"
												>
													<Upload className="w-3 h-3" />
													上传文件
												</Button>
												<Button
													size="sm"
													onClick={() => { setUrlKBId(kb.id); setUrlOpen(true) }}
													className="bg-zinc-800 hover:bg-zinc-700 text-zinc-300 h-7 px-3 text-xs gap-1 border border-zinc-700"
												>
													<Link2 className="w-3 h-3" />
													导入 URL
												</Button>
												<span className="ml-auto text-xs text-zinc-600">{docs.length} 个文档</span>
											</div>

											{/* KB Search */}
											<div className="flex items-center gap-2 px-4 py-2 border-b border-zinc-800/30 bg-zinc-800/20">
												<Search className="w-3 h-3 text-zinc-500 shrink-0" />
												<Input
													value={kbSearchText}
													onChange={e => setKbSearchText(e.target.value)}
													placeholder="搜索知识库内容..."
													className="flex-1 bg-zinc-900 border-zinc-700 text-white placeholder:text-zinc-600 h-7 text-xs focus-visible:ring-violet-500"
													onKeyDown={e => { if (e.key === "Enter") handleKBSearch(kb.id) }}
												/>
												<Button
													size="sm"
													onClick={() => handleKBSearch(kb.id)}
													disabled={kbSearching || !kbSearchText.trim()}
													className="bg-violet-600 hover:bg-violet-500 text-white h-7 px-3 text-xs gap-1"
												>
													{kbSearching ? <Loader2 className="w-3 h-3 animate-spin" /> : <Search className="w-3 h-3" />}
													检索
												</Button>
											</div>

											{/* KB Search Results */}
											{kbSearchKBId === kb.id && kbSearchResults.length > 0 && (
												<div className="border-b border-zinc-800/30 bg-zinc-800/10">
													{kbSearchResults.map((r, i) => (
														<div key={i} className="px-4 py-2.5 border-b border-zinc-800/10 last:border-0">
															<div className="flex items-center gap-2 mb-1.5">
																<FileText className="w-3 h-3 text-violet-400 shrink-0" />
																<span className="text-xs text-zinc-400 truncate">{r.document_name}</span>
																<span className="text-[10px] text-zinc-600 bg-zinc-800 px-1.5 py-0.5 rounded ml-auto shrink-0">得分 {(r.score * 100).toFixed(0)}%</span>
															</div>
															<p className="text-sm text-zinc-300 leading-relaxed">{r.content.slice(0, 300)}{r.content.length > 300 ? "…" : ""}</p>
														</div>
													))}
													<div className="px-4 py-1.5 flex justify-end">
														<button onClick={() => { setKbSearchResults([]); setKbSearchKBId("") }} className="text-xs text-zinc-500 hover:text-zinc-300 cursor-pointer">清除结果</button>
													</div>
												</div>
											)}

											{loadingDocs ? (
												<div className="flex justify-center py-8">
													<div className="w-4 h-4 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
												</div>
											) : docs.length === 0 ? (
												<div className="px-4 py-8 text-center text-xs text-zinc-600">
													暂无文档，上传文件或导入 URL 开始
												</div>
											) : (
												<div>
													{docs.map((doc, i) => {
														const status = STATUS_MAP[doc.status] || STATUS_MAP.error
														const StatusIcon = status.icon
														return (
															<div key={doc.id} className={`flex items-center gap-3 px-4 py-2.5 hover:bg-zinc-800/10 transition-colors group ${i < docs.length - 1 ? "border-b border-zinc-800/20" : ""}`}>
																<FileText className="w-3.5 h-3.5 text-zinc-500 shrink-0" />
																<div className="flex-1 min-w-0">
																	<div className="flex items-center gap-2">
																		<span className="text-sm text-zinc-300 truncate">{doc.name}</span>
																		{doc.source === "url" && <Link2 className="w-3 h-3 text-zinc-600 shrink-0" />}
																	</div>
																	{doc.status === "error" && doc.error_message && (
																		<p className="text-[11px] text-red-400/80 truncate mt-0.5">{doc.error_message}</p>
																	)}
																</div>
																<span className="text-xs text-zinc-600 font-mono shrink-0">{doc.source}</span>
																<span className={`flex items-center gap-1 text-xs ${status.cls} shrink-0`}>
																	<StatusIcon className={`w-3 h-3 ${doc.status !== "ready" && doc.status !== "error" ? "animate-spin" : ""}`} />
																	{status.label}
																</span>
																<span className="text-xs text-zinc-600 shrink-0 w-16 text-right">
																	{doc.status === "ready" ? `${doc.chunk_count} 块` : formatSize(doc.char_count) || "—"}
																</span>
																<button
																	onClick={() => handleDeleteDoc(doc.id)}
																	disabled={deleting.has(doc.id)}
																	className="shrink-0 opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-red-400 transition-all p-1 rounded hover:bg-red-400/10 disabled:opacity-40 cursor-pointer -mr-1"
																>
																	<Trash2 className="w-3.5 h-3.5" />
																</button>
																{doc.status === "error" && (
																	<button
																		onClick={() => handleRetryDoc(doc.id, kb.id)}
																		disabled={deleting.has(doc.id)}
																		className="shrink-0 opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-amber-400 transition-all p-1 rounded hover:bg-amber-400/10 disabled:opacity-40 cursor-pointer"
																	>
																		<RotateCw className="w-3.5 h-3.5" />
																	</button>
																)}
															</div>
														)
													})}
												</div>
											)}
										</div>
									)}
								</div>
							)
						})}
					</div>
				)}
			</div>

			{/* Create KB Dialog */}
			<Dialog open={createKBOpen} onOpenChange={setCreateKBOpen}>
				<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
					<DialogHeader>
						<DialogTitle className="text-white flex items-center gap-2">
							<BookOpen className="w-4 h-4 text-violet-400" />
							新建知识库
						</DialogTitle>
					</DialogHeader>
					<div className="space-y-4 py-2">
						<div>
							<label className="text-xs text-zinc-400 mb-1.5 block">名称</label>
							<Input
								value={kbName}
								onChange={e => setKbName(e.target.value)}
								placeholder="例如：产品手册"
								className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 h-9 text-sm"
								onKeyDown={e => { if (e.key === "Enter") handleCreateKB() }}
							/>
						</div>
						<div>
							<label className="text-xs text-zinc-400 mb-1.5 block">向量模型 <span className="text-red-400">*</span></label>
							<select
								value={kbModelId}
								onChange={e => setKbModelId(e.target.value)}
								className="w-full bg-zinc-800 border border-zinc-700 text-white text-sm rounded-md h-9 px-3 focus:outline-none focus:border-violet-500"
							>
								<option value="">请选择 Embedding 模型...</option>
								{embModels.map(m => (
									<option key={m.id} value={m.id}>{m.name} ({m.model_id})</option>
								))}
							</select>
							{embModels.length === 0 && (
								<p className="text-xs text-zinc-600 mt-1">暂无可用模型，请先在「模型管理」中添加 type=embedding 的模型</p>
							)}
						</div>
						<div>
							<label className="text-xs text-zinc-400 mb-1.5 block">描述（可选）</label>
							<Input
								value={kbDesc}
								onChange={e => setKbDesc(e.target.value)}
								placeholder="知识库用途说明"
								className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 h-9 text-sm"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setCreateKBOpen(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">取消</Button>
						<Button onClick={handleCreateKB} disabled={!kbName.trim() || !kbModelId} className="bg-violet-600 hover:bg-violet-500 text-white">创建</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

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
							onDragOver={(e) => { e.preventDefault(); setFileDragging(true) }}
							onDragLeave={() => setFileDragging(false)}
							onDrop={(e) => { e.preventDefault(); setFileDragging(false) }}
							className={`border-2 border-dashed rounded-xl p-10 text-center transition-all cursor-pointer ${fileDragging ? "border-violet-500 bg-violet-600/5" : "border-zinc-700 hover:border-zinc-600"}`}
							onClick={() => fileRef.current?.click()}
						>
							<Upload className="w-8 h-8 text-zinc-600 mx-auto mb-3" strokeWidth={1.5} />
							<p className="text-sm text-zinc-400 mb-1">拖拽文件到此处，或点击选择</p>
							<p className="text-xs text-zinc-600 mt-3">支持 TXT、Markdown、代码文件等</p>
						</div>
						<input ref={fileRef} type="file" className="hidden" onChange={handleUpload} />
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setUploadOpen(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">关闭</Button>
						<Button onClick={handleUpload} disabled={!fileRef.current?.files?.length} className="bg-violet-600 hover:bg-violet-500 text-white">上传</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* URL Import Dialog */}
			<Dialog open={urlOpen} onOpenChange={setUrlOpen}>
				<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
					<DialogHeader>
						<DialogTitle className="text-white flex items-center gap-2">
							<Link2 className="w-4 h-4 text-violet-400" />
							导入 URL
						</DialogTitle>
					</DialogHeader>
					<div className="py-2">
						<label className="text-xs text-zinc-400 mb-1.5 block">网页地址</label>
						<Input
							value={urlInput}
							onChange={e => setUrlInput(e.target.value)}
							placeholder="https://example.com/doc.html"
							className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 h-9 text-sm"
							onKeyDown={e => { if (e.key === "Enter") handleImportURL() }}
						/>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setUrlOpen(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white">取消</Button>
						<Button onClick={handleImportURL} disabled={!urlInput.trim()} className="bg-violet-600 hover:bg-violet-500 text-white">导入</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	)
}
