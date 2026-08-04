import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Search, Bot, Sparkles, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
	agentTemplateApi,
	voicebotApi,
	type AgentTemplate,
} from "@/lib/api";

export default function AgentPlazaPage() {
	const [templates, setTemplates] = useState<AgentTemplate[]>([]);
	const [loading, setLoading] = useState(true);
	const [query, setQuery] = useState("");
	const [activeCategory, setActiveCategory] = useState("全部");
	const [using, setUsing] = useState<string | null>(null);
	const navigate = useNavigate();

	useEffect(() => {
		setLoading(true);
		agentTemplateApi
			.listSystem()
			.then((res) => setTemplates(res.data))
			.finally(() => setLoading(false));
	}, []);

	const categories = [
		"全部",
		...new Set(templates.map((t) => t.category).filter(Boolean)),
	];

	const filtered = templates.filter((t) => {
		const matchCategory =
			activeCategory === "全部" || t.category === activeCategory;
		const matchQuery =
			!query.trim() ||
			t.name.toLowerCase().includes(query.trim().toLowerCase()) ||
			(t.description ?? "")
				.toLowerCase()
				.includes(query.trim().toLowerCase());
		return matchCategory && matchQuery;
	});

	const handleUseTemplate = async (tpl: AgentTemplate) => {
		setUsing(tpl.id);
		try {
			const { data } = await agentTemplateApi.use(tpl.id);
			// 用模板的名称和配置创建 voicebot
			const configJSON = JSON.stringify(data.config);
			const res = await voicebotApi.create(data.name, configJSON);
			navigate(`/agents/${res.data.id}`);
		} catch {
			setUsing(null);
		}
	};

	// 将现有硬编码模板转换为 API 格式，作为 fallback
	const hasData = templates.length > 0;

	return (
		<div className="min-h-full">
			{/* Header */}
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-lg font-semibold text-white">智能体广场</h1>
						<p className="text-sm text-zinc-500 mt-0.5">
							选择模板快速创建，或从零构建你的专属智能体
						</p>
					</div>
					<Button
						onClick={() => {
							setUsing("_new");
							navigate("/agents");
						}}
						className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
					>
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
						className="pl-9 h-9 text-sm"
					/>
				</div>

				{/* Category filter */}
				{hasData && (
					<div className="flex gap-1.5 mt-3 flex-wrap">
						{categories.map((cat) => (
							<button
								key={cat}
								onClick={() => setActiveCategory(cat)}
								className={`px-3 py-1 rounded-full text-xs font-medium transition-all duration-150 cursor-pointer ${
									activeCategory === cat
										? "bg-violet-600 text-white"
										: "bg-zinc-800 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200"
								}`}
							>
								{cat}
							</button>
						))}
					</div>
				)}
			</div>

			{/* Grid */}
			<div className="px-8 py-6">
				{loading ? (
					<div className="flex items-center justify-center py-20 text-center">
						<Loader2 className="w-6 h-6 text-zinc-500 animate-spin" />
					</div>
				) : filtered.length === 0 ? (
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
								{/* Icon + Title row */}
								<div className="flex items-start gap-3 mb-3">
									<div
										className={`w-10 h-10 rounded-xl bg-gradient-to-br ${tpl.color || "from-violet-500 to-purple-600"} flex items-center justify-center text-xl shrink-0 shadow-lg`}
									>
										{tpl.icon || "🤖"}
									</div>
									<div className="flex-1 min-w-0">
										<p className="font-medium text-sm text-white leading-snug truncate">
											{tpl.name}
										</p>
										{(tpl.tags && tpl.tags.length > 0) && (
											<div className="flex flex-wrap gap-1 mt-1.5">
												{tpl.tags.map((tag) => (
													<span
														key={tag}
														className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/50"
													>
														{tag}
													</span>
												))}
											</div>
										)}
									</div>
								</div>

								{/* Desc */}
								<p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">
									{tpl.description || ""}
								</p>

								{/* Footer */}
								<div className="flex items-center justify-between">
									<span className="text-[11px] text-zinc-600">
										{tpl.use_count.toLocaleString()} 次使用
									</span>
									<div className="flex gap-1.5">
										<Button
											size="sm"
											disabled={using === tpl.id}
											onClick={() => handleUseTemplate(tpl)}
											className="h-7 px-3 text-xs bg-violet-600 hover:bg-violet-500 text-white"
										>
											{using === tpl.id ? (
												<Loader2 className="w-3 h-3 animate-spin" />
											) : (
												"基于此创建"
											)}
										</Button>
									</div>
								</div>
							</div>
						))}
					</div>
				)}
			</div>
		</div>
	);
}
