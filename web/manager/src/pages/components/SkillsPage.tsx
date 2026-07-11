import { useState } from "react";
import { Zap, Plus, CheckCircle2, Star } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const MARKET_SKILLS = [
	{
		id: "s1",
		name: "情绪分析",
		icon: "💬",
		tags: ["NLP", "官方"],
		desc: "分析用户输入的情感倾向，返回情绪类型和置信度，用于对话情感感知。",
		star: 4.8,
		installed: true,
	},
	{
		id: "s2",
		name: "文本摘要",
		icon: "📝",
		tags: ["NLP", "官方"],
		desc: "将长文本压缩为简洁摘要，支持中英双语，可配置摘要长度比例。",
		star: 4.7,
		installed: true,
	},
	{
		id: "s3",
		name: "实体识别",
		icon: "🏷️",
		tags: ["NLP", "官方"],
		desc: "从文本中提取人名、地点、组织、时间等实体信息，返回结构化数据。",
		star: 4.6,
		installed: false,
	},
	{
		id: "s4",
		name: "关键词提取",
		icon: "🔑",
		tags: ["NLP"],
		desc: "从文档中自动提取关键词和主题词，支持 TF-IDF 和 TextRank 算法。",
		star: 4.5,
		installed: false,
	},
	{
		id: "s5",
		name: "代码审查",
		icon: "🔍",
		tags: ["编程", "官方"],
		desc: "对提交的代码进行安全、性能和最佳实践检查，输出问题列表和建议。",
		star: 4.9,
		installed: false,
	},
	{
		id: "s6",
		name: "图像描述",
		icon: "🖼️",
		tags: ["多模态"],
		desc: "为图片生成自然语言描述，支持场景、物体、文字识别。",
		star: 4.4,
		installed: false,
	},
	{
		id: "s7",
		name: "翻译专家",
		icon: "🌐",
		tags: ["语言", "官方"],
		desc: "高质量多语言翻译，支持 50+ 种语言，保留格式和术语一致性。",
		star: 4.8,
		installed: false,
	},
	{
		id: "s8",
		name: "SQL 生成",
		icon: "🗄️",
		tags: ["数据库", "编程"],
		desc: "根据自然语言描述自动生成 SQL 查询，支持多种数据库方言。",
		star: 4.7,
		installed: false,
	},
];

export default function SkillsPage() {
	const [installed, setInstalled] = useState<Set<string>>(
		new Set(["s1", "s2"]),
	);

	const toggle = (id: string) => {
		setInstalled((prev) => {
			const n = new Set(prev);
			if (n.has(id)) n.delete(id);
			else n.add(id);
			return n;
		});
	};

	return (
		<div className="min-h-full">
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-lg font-semibold text-white">SKILL 管理</h1>
						<p className="text-sm text-zinc-500 mt-0.5">
							预置能力模块，可复用在多个智能体中
						</p>
					</div>
					<Button
						variant="outline"
						className="h-9 px-4 text-sm border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1.5"
					>
						<Plus className="w-4 h-4" />
						自定义 SKILL
					</Button>
				</div>
			</div>

			<div className="px-8 py-6">
				<Tabs defaultValue="market">
					<TabsList className="bg-zinc-900 border border-zinc-800 h-9 p-0.5 mb-6">
						<TabsTrigger
							value="market"
							className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
						>
							市场
						</TabsTrigger>
						<TabsTrigger
							value="mine"
							className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
						>
							已有 ({installed.size})
						</TabsTrigger>
					</TabsList>

					<TabsContent value="market">
						<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
							{MARKET_SKILLS.map((sk) => {
								const isInstalled = installed.has(sk.id);
								return (
									<div
										key={sk.id}
										className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all"
									>
										<div className="flex items-start justify-between mb-3">
											<span className="text-2xl">{sk.icon}</span>
											<span className="flex items-center gap-0.5 text-[11px] text-amber-400">
												<Star className="w-3 h-3 fill-amber-400" />
												{sk.star}
											</span>
										</div>
										<p className="font-medium text-sm text-white mb-1">
											{sk.name}
										</p>
										<div className="flex flex-wrap gap-1 mb-2">
											{sk.tags.map((t) => (
												<span
													key={t}
													className={`text-[10px] px-1.5 py-0.5 rounded border ${t === "官方" ? "bg-violet-600/15 text-violet-400 border-violet-500/20" : "bg-zinc-800 text-zinc-500 border-zinc-700/50"}`}
												>
													{t}
												</span>
											))}
										</div>
										<p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">
											{sk.desc}
										</p>
										<Button
											size="sm"
											onClick={() => toggle(sk.id)}
											className={`h-7 w-full text-xs ${
												isInstalled
													? "border-zinc-700 text-zinc-400 hover:text-red-400 hover:border-red-400/30 hover:bg-red-400/8"
													: "bg-violet-600 hover:bg-violet-500 text-white"
											}`}
											variant={isInstalled ? "outline" : "default"}
										>
											{isInstalled ? (
												<>
													<CheckCircle2 className="w-3 h-3 mr-1" />
													已安装
												</>
											) : (
												"安装"
											)}
										</Button>
									</div>
								);
							})}
						</div>
					</TabsContent>

					<TabsContent value="mine">
						{installed.size === 0 ? (
							<div className="flex flex-col items-center py-20">
								<div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
									<Zap className="w-6 h-6 text-zinc-600" />
								</div>
								<p className="text-zinc-400 text-sm">还没有安装任何 SKILL</p>
								<p className="text-zinc-600 text-xs mt-1">
									前往市场选择需要的能力模块
								</p>
							</div>
						) : (
							<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
								{MARKET_SKILLS.filter((s) => installed.has(s.id)).map((sk) => (
									<div
										key={sk.id}
										className="bg-zinc-900 border border-zinc-800 rounded-xl p-5"
									>
										<div className="flex items-start justify-between mb-3">
											<span className="text-2xl">{sk.icon}</span>
											<span className="flex items-center gap-1 text-[11px] text-emerald-400">
												<CheckCircle2 className="w-3 h-3" />
												已安装
											</span>
										</div>
										<p className="font-medium text-sm text-white mb-1">
											{sk.name}
										</p>
										<p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">
											{sk.desc}
										</p>
										<div className="flex gap-1.5">
											<button className="flex-1 text-xs text-zinc-500 hover:text-zinc-300 py-1.5 rounded hover:bg-zinc-800 transition-colors cursor-pointer border border-zinc-800 hover:border-zinc-700">
												配置
											</button>
											<button
												onClick={() => toggle(sk.id)}
												className="flex-1 text-xs text-zinc-500 hover:text-red-400 py-1.5 rounded hover:bg-red-400/10 transition-colors cursor-pointer border border-zinc-800 hover:border-red-400/30"
											>
												卸载
											</button>
										</div>
									</div>
								))}
							</div>
						)}
					</TabsContent>
				</Tabs>
			</div>
		</div>
	);
}
