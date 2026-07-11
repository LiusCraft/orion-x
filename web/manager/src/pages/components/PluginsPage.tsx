import { useState } from "react";
import {
	Puzzle,
	Plus,
	Globe,
	Trash2,
	Edit2,
	CheckCircle2,
	Lock,
	Zap,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
	DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const MARKET_PLUGINS = [
	{
		id: "p1",
		name: "天气查询",
		icon: "🌤️",
		desc: "实时获取全球任意城市的天气信息，包括温度、湿度、风速和预报。",
		tags: ["工具", "免费"],
		installed: true,
	},
	{
		id: "p2",
		name: "汇率换算",
		icon: "💱",
		desc: "实时汇率查询和货币换算，支持 180+ 种货币，数据来自 ExchangeRate-API。",
		tags: ["金融", "免费"],
		installed: false,
	},
	{
		id: "p3",
		name: "Wikipedia 搜索",
		icon: "📖",
		desc: "搜索 Wikipedia 百科词条，返回摘要和相关链接，支持多语言。",
		tags: ["知识", "免费"],
		installed: true,
	},
	{
		id: "p4",
		name: "Arxiv 论文",
		icon: "📄",
		desc: "搜索 Arxiv 学术论文，返回标题、摘要、作者和链接。",
		tags: ["学术", "免费"],
		installed: false,
	},
];

interface MyPlugin {
	id: string;
	name: string;
	url: string;
	method: string;
	authType: string;
	desc: string;
	createdAt: string;
}

const MOCK_MY_PLUGINS: MyPlugin[] = [
	{
		id: "mp1",
		name: "公司内部 API",
		url: "https://api.internal.com/v1/query",
		method: "POST",
		authType: "Bearer Token",
		desc: "查询公司内部 CRM 数据",
		createdAt: "2025-06-10",
	},
	{
		id: "mp2",
		name: "产品库存查询",
		url: "https://erp.company.com/api/stock",
		method: "GET",
		authType: "API Key",
		desc: "实时查询产品库存信息",
		createdAt: "2025-05-28",
	},
];

const METHODS = ["GET", "POST", "PUT", "DELETE"];
const AUTH_TYPES = ["无鉴权", "Bearer Token", "API Key", "Basic Auth"];

export default function PluginsPage() {
	const [installedIds, setInstalledIds] = useState<Set<string>>(
		new Set(["p1", "p3"]),
	);
	const [myPlugins, setMyPlugins] = useState<MyPlugin[]>(MOCK_MY_PLUGINS);
	const [addOpen, setAddOpen] = useState(false);
	const [form, setForm] = useState({
		name: "",
		url: "",
		method: "POST",
		authType: "无鉴权",
		authValue: "",
		desc: "",
	});

	const handleAdd = () => {
		if (!form.name.trim() || !form.url.trim()) return;
		const plugin: MyPlugin = {
			id: `mp_${Date.now()}`,
			name: form.name.trim(),
			url: form.url.trim(),
			method: form.method,
			authType: form.authType,
			desc: form.desc.trim(),
			createdAt: new Date().toISOString().slice(0, 10),
		};
		setMyPlugins((prev) => [plugin, ...prev]);
		setForm({
			name: "",
			url: "",
			method: "POST",
			authType: "无鉴权",
			authValue: "",
			desc: "",
		});
		setAddOpen(false);
	};

	return (
		<div className="min-h-full">
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-lg font-semibold text-white">插件管理</h1>
						<p className="text-sm text-zinc-500 mt-0.5">
							插件本质是工具，由智能体在对话中自动调用
						</p>
					</div>
					<Button
						onClick={() => setAddOpen(true)}
						className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
					>
						<Plus className="w-4 h-4" />
						添加 HTTP 插件
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
							我的插件 ({myPlugins.length})
						</TabsTrigger>
					</TabsList>

					<TabsContent value="market">
						<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
							{MARKET_PLUGINS.map((plugin) => {
								const isInstalled = installedIds.has(plugin.id);
								return (
									<div
										key={plugin.id}
										className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all"
									>
										<div className="text-2xl mb-3">{plugin.icon}</div>
										<p className="font-medium text-sm text-white mb-1">
											{plugin.name}
										</p>
										<p className="text-xs text-zinc-500 leading-relaxed mb-3 line-clamp-2">
											{plugin.desc}
										</p>
										<div className="flex flex-wrap gap-1 mb-4">
											{plugin.tags.map((t) => (
												<span
													key={t}
													className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50"
												>
													{t}
												</span>
											))}
										</div>
										<Button
											size="sm"
											onClick={() =>
												setInstalledIds((prev) => {
													const n = new Set(prev);
													if (n.has(plugin.id)) n.delete(plugin.id);
													else n.add(plugin.id);
													return n;
												})
											}
											className={`h-7 w-full text-xs ${
												isInstalled
													? "border-zinc-700 text-zinc-400 hover:text-red-400 hover:border-red-400/30 hover:bg-red-400/8"
													: "bg-violet-600 hover:bg-violet-500 text-white"
											}`}
											variant={isInstalled ? "outline" : "default"}
										>
											{isInstalled ? (
												<span className="flex items-center gap-1.5">
													<CheckCircle2 className="w-3 h-3" />
													已安装
												</span>
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
						{myPlugins.length === 0 ? (
							<div className="flex flex-col items-center py-20">
								<div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
									<Puzzle className="w-6 h-6 text-zinc-600" />
								</div>
								<p className="text-zinc-400 text-sm">还没有自定义插件</p>
								<p className="text-zinc-600 text-xs mt-1 mb-4">
									通过 HTTP API 接入你自己的工具
								</p>
								<Button
									onClick={() => setAddOpen(true)}
									className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5"
								>
									<Plus className="w-3.5 h-3.5" />
									添加 HTTP 插件
								</Button>
							</div>
						) : (
							<div className="space-y-3">
								{myPlugins.map((p) => (
									<div
										key={p.id}
										className="bg-zinc-900 border border-zinc-800 rounded-xl px-5 py-4 flex items-center gap-4 hover:border-zinc-700 transition-all group"
									>
										<div className="w-9 h-9 rounded-xl bg-zinc-800 border border-zinc-700 flex items-center justify-center shrink-0">
											<Globe
												className="w-4 h-4 text-zinc-400"
												strokeWidth={1.5}
											/>
										</div>
										<div className="flex-1 min-w-0">
											<div className="flex items-center gap-2 mb-0.5">
												<p className="font-medium text-sm text-white">
													{p.name}
												</p>
												<span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 border border-zinc-700/50 text-zinc-400 font-mono">
													{p.method}
												</span>
												<span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 border border-zinc-700/50 text-zinc-500 flex items-center gap-1">
													<Lock className="w-2.5 h-2.5" />
													{p.authType}
												</span>
											</div>
											<p className="text-xs text-zinc-500 font-mono truncate">
												{p.url}
											</p>
										</div>
										<div className="flex gap-1.5 opacity-0 group-hover:opacity-100 transition-opacity">
											<button className="text-zinc-500 hover:text-zinc-300 p-1.5 rounded hover:bg-zinc-800 cursor-pointer transition-colors">
												<Edit2 className="w-3.5 h-3.5" />
											</button>
											<button
												onClick={() =>
													setMyPlugins((prev) =>
														prev.filter((x) => x.id !== p.id),
													)
												}
												className="text-zinc-500 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 cursor-pointer transition-colors"
											>
												<Trash2 className="w-3.5 h-3.5" />
											</button>
										</div>
										<span className="text-[11px] text-zinc-600 shrink-0">
											{p.createdAt}
										</span>
									</div>
								))}
							</div>
						)}
					</TabsContent>
				</Tabs>
			</div>

			{/* Add HTTP Plugin Dialog */}
			<Dialog open={addOpen} onOpenChange={setAddOpen}>
				<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg">
					<DialogHeader>
						<DialogTitle className="text-white flex items-center gap-2">
							<Zap className="w-4 h-4 text-violet-400" />
							添加 HTTP API 插件
						</DialogTitle>
					</DialogHeader>
					<div className="space-y-4 py-2">
						<div className="grid grid-cols-3 gap-3">
							<div className="col-span-2 space-y-1.5">
								<Label className="text-xs text-zinc-400 uppercase tracking-wide">
									名称
								</Label>
								<Input
									value={form.name}
									onChange={(e) =>
										setForm((f) => ({ ...f, name: e.target.value }))
									}
									placeholder="插件名称"
									className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm focus-visible:ring-violet-500"
								/>
							</div>
							<div className="space-y-1.5">
								<Label className="text-xs text-zinc-400 uppercase tracking-wide">
									Method
								</Label>
								<select
									value={form.method}
									onChange={(e) =>
										setForm((f) => ({ ...f, method: e.target.value }))
									}
									className="w-full h-9 rounded-md bg-zinc-800 border border-zinc-700 text-white text-sm px-2.5 focus:outline-none focus:ring-1 focus:ring-violet-500 cursor-pointer"
								>
									{METHODS.map((m) => (
										<option key={m} value={m}>
											{m}
										</option>
									))}
								</select>
							</div>
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								API URL
							</Label>
							<Input
								value={form.url}
								onChange={(e) =>
									setForm((f) => ({ ...f, url: e.target.value }))
								}
								placeholder="https://api.example.com/v1/endpoint"
								className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm font-mono focus-visible:ring-violet-500"
							/>
						</div>
						<div className="grid grid-cols-2 gap-3">
							<div className="space-y-1.5">
								<Label className="text-xs text-zinc-400 uppercase tracking-wide">
									鉴权方式
								</Label>
								<select
									value={form.authType}
									onChange={(e) =>
										setForm((f) => ({ ...f, authType: e.target.value }))
									}
									className="w-full h-9 rounded-md bg-zinc-800 border border-zinc-700 text-white text-sm px-2.5 focus:outline-none focus:ring-1 focus:ring-violet-500 cursor-pointer"
								>
									{AUTH_TYPES.map((a) => (
										<option key={a} value={a}>
											{a}
										</option>
									))}
								</select>
							</div>
							{form.authType !== "无鉴权" && (
								<div className="space-y-1.5">
									<Label className="text-xs text-zinc-400 uppercase tracking-wide">
										{form.authType === "Bearer Token"
											? "Token"
											: form.authType === "API Key"
												? "API Key"
												: "密码"}
									</Label>
									<Input
										type="password"
										value={form.authValue}
										onChange={(e) =>
											setForm((f) => ({ ...f, authValue: e.target.value }))
										}
										placeholder="••••••••"
										className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm focus-visible:ring-violet-500"
									/>
								</div>
							)}
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								描述（告诉智能体何时使用）
							</Label>
							<Input
								value={form.desc}
								onChange={(e) =>
									setForm((f) => ({ ...f, desc: e.target.value }))
								}
								placeholder="例：查询公司内部 CRM 数据时调用此接口"
								className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 text-sm focus-visible:ring-violet-500"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setAddOpen(false)}
							className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
						>
							取消
						</Button>
						<Button
							onClick={handleAdd}
							disabled={!form.name.trim() || !form.url.trim()}
							className="bg-violet-600 hover:bg-violet-500 text-white"
						>
							添加插件
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
