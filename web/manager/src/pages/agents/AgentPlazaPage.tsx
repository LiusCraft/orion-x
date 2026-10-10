import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Search, Bot, Sparkles, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
	agentTemplateApi,
	voicebotApi,
	type AgentTemplate,
} from "@/lib/api";
import { useDocumentTitle } from "@/lib/title";

const ALL_CATEGORIES = "all";

export default function AgentPlazaPage() {
	const { t } = useTranslation("agentsList");
	useDocumentTitle(t("plaza.title"));

	const [templates, setTemplates] = useState<AgentTemplate[]>([]);
	const [loading, setLoading] = useState(true);
	const [query, setQuery] = useState("");
	const [activeCategory, setActiveCategory] = useState(ALL_CATEGORIES);
	const [using, setUsing] = useState<string | null>(null);
	const [error, setError] = useState("");
	const navigate = useNavigate();

	useEffect(() => {
		setLoading(true);
		agentTemplateApi
			.listSystem()
			.then((res) => setTemplates(res.data))
			.finally(() => setLoading(false));
	}, []);

	const categories = [
		ALL_CATEGORIES,
		...new Set(templates.map((tpl) => tpl.category).filter(Boolean)),
	];

	const filtered = templates.filter((tpl) => {
		const matchCategory =
			activeCategory === ALL_CATEGORIES || tpl.category === activeCategory;
		const matchQuery =
			!query.trim() ||
			tpl.name.toLowerCase().includes(query.trim().toLowerCase()) ||
			(tpl.description ?? "")
				.toLowerCase()
				.includes(query.trim().toLowerCase());
		return matchCategory && matchQuery;
	});

	const handleUseTemplate = async (tpl: AgentTemplate) => {
		setUsing(tpl.id);
		setError("");
		try {
			const { data } = await agentTemplateApi.use(tpl.id);
			// 用模板的名称和配置创建 voicebot
			const configJSON = JSON.stringify(data.config);
			const res = await voicebotApi.create(data.name, configJSON);
			navigate(`/agents/${res.data.id}`);
		} catch (err) {
			setUsing(null);
			const detail = (err as {
				response?: { data?: { error?: string } };
			})?.response?.data?.error;
			setError(detail ?? (err instanceof Error ? err.message : t("plaza.createFailed")));
		}
	};

	return (
		<div className="min-h-full">
			{/* Header */}
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-lg font-semibold text-white">{t("plaza.title")}</h1>
						<p className="text-sm text-zinc-500 mt-0.5">
							{t("plaza.subtitle")}
						</p>
					</div>
					<Button
						onClick={() => {
							navigate("/agents");
						}}
						className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
					>
						<Sparkles className="w-3.5 h-3.5" />
						{t("plaza.createFromScratch")}
					</Button>
				</div>

				{/* Search */}
				<div className="relative mt-4 max-w-md">
					<Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
					<Input
						value={query}
						onChange={(e) => setQuery(e.target.value)}
						placeholder={t("plaza.searchPlaceholder")}
						className="pl-9 h-9 text-sm"
					/>
				</div>

				{/* Category filter */}
				{categories.length > 1 && (
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
								{cat === ALL_CATEGORIES ? t("plaza.categoryAll") : cat}
							</button>
						))}
					</div>
				)}
			</div>

			{/* Grid */}
			<div className="px-8 py-6">
				{error && <p className="text-xs text-red-400 mb-3">{error}</p>}
				{loading ? (
					<div className="flex items-center justify-center py-20 text-center">
						<Loader2 className="w-6 h-6 text-zinc-500 animate-spin" />
					</div>
				) : filtered.length === 0 ? (
					<div className="flex flex-col items-center justify-center py-20 text-center">
						<div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
							<Bot className="w-6 h-6 text-zinc-600" />
						</div>
						<p className="text-zinc-400 text-sm">{t("plaza.emptyTitle")}</p>
						<p className="text-zinc-600 text-xs mt-1">{t("plaza.emptyHint")}</p>
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
										{t("plaza.useCount", {
											value: tpl.use_count.toLocaleString(),
										})}
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
												t("plaza.createFromTemplate")
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
