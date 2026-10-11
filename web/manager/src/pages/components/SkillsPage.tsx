import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Zap, Plus, CheckCircle2, Star } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const MARKET_SKILLS = [
	{
		id: "s1",
		nameKey: "skills.market.sentiment.name",
		icon: "💬",
		tags: [
			{ key: "skills.tags.nlp" },
			{ key: "skills.tags.official", official: true },
		],
		descKey: "skills.market.sentiment.desc",
		star: 4.8,
		installed: true,
	},
	{
		id: "s2",
		nameKey: "skills.market.summary.name",
		icon: "📝",
		tags: [
			{ key: "skills.tags.nlp" },
			{ key: "skills.tags.official", official: true },
		],
		descKey: "skills.market.summary.desc",
		star: 4.7,
		installed: true,
	},
	{
		id: "s3",
		nameKey: "skills.market.entities.name",
		icon: "🏷️",
		tags: [
			{ key: "skills.tags.nlp" },
			{ key: "skills.tags.official", official: true },
		],
		descKey: "skills.market.entities.desc",
		star: 4.6,
		installed: false,
	},
	{
		id: "s4",
		nameKey: "skills.market.keywords.name",
		icon: "🔑",
		tags: [{ key: "skills.tags.nlp" }],
		descKey: "skills.market.keywords.desc",
		star: 4.5,
		installed: false,
	},
	{
		id: "s5",
		nameKey: "skills.market.codeReview.name",
		icon: "🔍",
		tags: [
			{ key: "skills.tags.programming" },
			{ key: "skills.tags.official", official: true },
		],
		descKey: "skills.market.codeReview.desc",
		star: 4.9,
		installed: false,
	},
	{
		id: "s6",
		nameKey: "skills.market.imageCaption.name",
		icon: "🖼️",
		tags: [{ key: "skills.tags.multimodal" }],
		descKey: "skills.market.imageCaption.desc",
		star: 4.4,
		installed: false,
	},
	{
		id: "s7",
		nameKey: "skills.market.translation.name",
		icon: "🌐",
		tags: [
			{ key: "skills.tags.language" },
			{ key: "skills.tags.official", official: true },
		],
		descKey: "skills.market.translation.desc",
		star: 4.8,
		installed: false,
	},
	{
		id: "s8",
		nameKey: "skills.market.sqlGen.name",
		icon: "🗄️",
		tags: [
			{ key: "skills.tags.database" },
			{ key: "skills.tags.programming" },
		],
		descKey: "skills.market.sqlGen.desc",
		star: 4.7,
		installed: false,
	},
];

export default function SkillsPage() {
	const { t } = useTranslation(["components", "common"]);
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
						<h1 className="text-lg font-semibold text-white">
							{t("skills.title")}
						</h1>
						<p className="text-sm text-zinc-500 mt-0.5">
							{t("skills.subtitle")}
						</p>
					</div>
					<Button
						variant="outline"
						className="h-9 px-4 text-sm border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1.5"
					>
						<Plus className="w-4 h-4" />
						{t("skills.custom")}
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
							{t("market")}
						</TabsTrigger>
						<TabsTrigger
							value="mine"
							className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
						>
							{t("skills.tab.installed", { total: installed.size })}
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
											{t(sk.nameKey)}
										</p>
										<div className="flex flex-wrap gap-1 mb-2">
											{sk.tags.map((tag) => (
												<span
													key={tag.key}
													className={`text-[10px] px-1.5 py-0.5 rounded border ${tag.official ? "bg-violet-600/15 text-violet-400 border-violet-500/20" : "bg-zinc-800 text-zinc-500 border-zinc-700/50"}`}
												>
													{t(tag.key)}
												</span>
											))}
										</div>
										<p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">
											{t(sk.descKey)}
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
													{t("installed")}
												</>
											) : (
												t("install")
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
								<p className="text-zinc-400 text-sm">
									{t("skills.mineEmpty")}
								</p>
								<p className="text-zinc-600 text-xs mt-1">
									{t("skills.mineEmptyHint")}
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
												{t("installed")}
											</span>
										</div>
										<p className="font-medium text-sm text-white mb-1">
											{t(sk.nameKey)}
										</p>
										<p className="text-xs text-zinc-500 leading-relaxed mb-4 line-clamp-2">
											{t(sk.descKey)}
										</p>
										<div className="flex gap-1.5">
											<button className="flex-1 text-xs text-zinc-500 hover:text-zinc-300 py-1.5 rounded hover:bg-zinc-800 transition-colors cursor-pointer border border-zinc-800 hover:border-zinc-700">
												{t("skills.configure")}
											</button>
											<button
												onClick={() => toggle(sk.id)}
												className="flex-1 text-xs text-zinc-500 hover:text-red-400 py-1.5 rounded hover:bg-red-400/10 transition-colors cursor-pointer border border-zinc-800 hover:border-red-400/30"
											>
												{t("skills.uninstall")}
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
