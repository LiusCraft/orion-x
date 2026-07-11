import { useEffect, useState } from "react";
import {
	Cpu,
	Plus,
	Globe,
	Database,
	Shield,
	HardDrive,
	GitBranch,
	Link,
	Brain,
	Trash2,
	Loader2,
	RefreshCw,
	Terminal,
	Play,
	CheckCircle2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
	Sheet,
	SheetContent,
	SheetHeader,
	SheetTitle,
} from "@/components/ui/sheet";
import { mcpApi, type MCPMarketEntry, type MCPServer } from "@/lib/api";
import {
	McpServerDetail,
	ToolInputs,
	ToolResult,
} from "@/pages/components/McpServerDetail";

function LogoIcon({
	name,
	icon,
	size = "md",
}: {
	name?: string;
	icon?: string;
	size?: "sm" | "md";
}) {
	const dims = size === "sm" ? "w-7 h-7" : "w-9 h-9";
	const iconDims = size === "sm" ? "w-3.5 h-3.5" : "w-4 h-4";

	if (icon) {
		return (
			<img
				src={icon}
				alt=""
				className={`${dims} rounded-xl shrink-0 object-contain bg-zinc-800/60`}
				onError={(e) => {
					(e.target as HTMLImageElement).style.display = "none";
				}}
			/>
		);
	}

	const IconComp = pickIcon(name);
	return (
		<div
			className={`${dims} rounded-xl bg-violet-400/10 flex items-center justify-center shrink-0`}
		>
			<IconComp className={`${iconDims} text-violet-400`} strokeWidth={1.5} />
		</div>
	);
}

function pickIcon(name?: string) {
	const n = (name || "").toLowerCase();
	if (n.includes("search") || n.includes("web") || n.includes("brave"))
		return Globe;
	if (
		n.includes("database") ||
		n.includes("db") ||
		n.includes("sql") ||
		n.includes("postgres")
	)
		return Database;
	if (n.includes("code") || n.includes("exec")) return Shield;
	if (n.includes("file") || n.includes("storage")) return HardDrive;
	if (n.includes("github") || n.includes("git")) return GitBranch;
	if (n.includes("fetch") || n.includes("link")) return Link;
	if (n.includes("memory") || n.includes("brain")) return Brain;
	return Globe;
}

type DrawerMode = "new" | "edit";

const initialForm = {
	name: "",
	description: "",
	icon: "",
	tags: "",
	transport: "streamable" as string,
	command: "",
	args: "",
	endpoint: "",
	toolList: "",
	timeoutMs: 30000,
	cwd: "",
	envKeys: [""] as string[],
	envVals: [""] as string[],
	headerKeys: [""] as string[],
	headerVals: [""] as string[],
};

export default function McpPage() {
	const [market, setMarket] = useState<MCPMarketEntry[]>([]);
	const [servers, setServers] = useState<MCPServer[]>([]);
	const [loading, setLoading] = useState(true);

	const [drawerOpen, setDrawerOpen] = useState(false);
	const [drawerMode, setDrawerMode] = useState<DrawerMode>("new");
	// The server being edited — null for "new" mode
	const [editTarget, setEditTarget] = useState<MCPServer | null>(null);
	const [testing, setTesting] = useState(false);
	const [testResult, setTestResult] = useState<{
		success: boolean;
		message: string;
	} | null>(null);
	const [toolsList, setToolsList] = useState<
		{
			name: string;
			description: string;
			input_schema?: Record<string, unknown>;
		}[]
	>([]);
	const [toolsLoading, setToolsLoading] = useState(false);
	const [toolsOpen, setToolsOpen] = useState(false);
	const [testedEndpoint, setTestedEndpoint] = useState("");
	const [testedHeaders, setTestedHeaders] = useState("");
	const [selectedTool, setSelectedTool] = useState<string | null>(null);
	const [toolArgs, setToolArgs] = useState<
		Record<string, Record<string, unknown>>
	>({});
	const [callResult, setCallResult] = useState<{
		success: boolean;
		message?: string;
		is_error?: boolean;
		output?: string;
	} | null>(null);
	const [calling, setCalling] = useState(false);
	const [form, setForm] = useState({ ...initialForm });
	const [selectedMarket, setSelectedMarket] = useState<MCPMarketEntry | null>(
		null,
	);
	const [descPreview, setDescPreview] = useState(true);

	const isOfficial = editTarget?.market_id != null;

	const addEnvRow = () =>
		setForm((f) => ({
			...f,
			envKeys: [...f.envKeys, ""],
			envVals: [...f.envVals, ""],
		}));
	const addHeaderRow = () =>
		setForm((f) => ({
			...f,
			headerKeys: [...f.headerKeys, ""],
			headerVals: [...f.headerVals, ""],
		}));

	const updateEnv = (i: number, key: string, val: string) => {
		const k = [...form.envKeys];
		k[i] = key;
		const v = [...form.envVals];
		v[i] = val;
		setForm((f) => ({ ...f, envKeys: k, envVals: v }));
	};
	const updateHeader = (i: number, key: string, val: string) => {
		const k = [...form.headerKeys];
		k[i] = key;
		const v = [...form.headerVals];
		v[i] = val;
		setForm((f) => ({ ...f, headerKeys: k, headerVals: v }));
	};

	const entriesToRecord = (
		keys: string[],
		vals: string[],
	): Record<string, string> | undefined => {
		const rec: Record<string, string> = {};
		let has = false;
		for (let i = 0; i < keys.length; i++) {
			const k = keys[i].trim();
			const v = vals[i].trim();
			if (k && v) {
				rec[k] = v;
				has = true;
			}
		}
		return has ? rec : undefined;
	};

	useEffect(() => {
		Promise.all([mcpApi.market.list(), mcpApi.servers.list()]).then(
			([mk, sv]) => {
				setMarket(mk.data);
				setServers(sv.data);
				setLoading(false);
			},
		);
	}, []);

	const removeServer = async (serverId: string) => {
		await mcpApi.servers.remove(serverId);
		setServers((prev) => prev.filter((s) => s.id !== serverId));
		setDrawerOpen(false);
	};

	const openDrawer = (server: MCPServer | null, mode: DrawerMode) => {
		setDrawerMode(mode);
		setEditTarget(server);
		if (server) {
			const marketEntry = server.market_id
				? market.find((m) => m.id === server.market_id)
				: undefined;
			const marketHdrs = marketEntry?.config?.headers as
				| Record<string, string>
				| undefined;
			const hKeys = server.headers
				? Object.keys(server.headers)
				: marketHdrs
					? Object.keys(marketHdrs)
					: [""];
			const hVals = server.headers
				? Object.values(server.headers)
				: marketHdrs
					? new Array(Object.keys(marketHdrs).length).fill("")
					: [""];

			setForm({
				name: server.name,
				description: server.description || "",
				icon: server.icon || "",
				tags: (server.tags || []).join(", "),
				transport: server.transport,
				command: server.command || "",
				args: (server.args || []).join(" "),
				endpoint: server.endpoint || "",
				toolList: (server.tool_name_list || []).join(", "),
				timeoutMs: server.timeout_ms,
				cwd: server.cwd || "",
				envKeys: server.env ? Object.keys(server.env) : [""],
				envVals: server.env ? Object.values(server.env) : [""],
				headerKeys: hKeys,
				headerVals: hVals,
			});
		} else {
			setForm({ ...initialForm });
			setTestedEndpoint("");
			setTestedHeaders("");
		}
		// Default to preview if description has content
		setDescPreview(!!server?.description);
		setTestResult(null);
		setToolsList([]);
		setToolsOpen(false);
		setSelectedTool(null);
		setToolArgs({});
		setCallResult(null);
		setDrawerOpen(true);
	};

	const handleSave = async () => {
		if (!form.name.trim()) return;
		const payload = {
			name: form.name.trim(),
			description: form.description.trim(),
			icon: form.icon.trim(),
			tags: form.tags.trim()
				? form.tags
						.split(",")
						.map((s) => s.trim())
						.filter(Boolean)
				: [],
			transport: form.transport,
			command: form.command.trim(),
			args: form.args.trim() ? form.args.split(/\s+/) : [],
			env: entriesToRecord(form.envKeys, form.envVals) ?? {},
			cwd: form.cwd.trim(),
			endpoint: form.endpoint.trim(),
			headers: entriesToRecord(form.headerKeys, form.headerVals) ?? {},
			tool_name_list: form.toolList.trim()
				? form.toolList
						.split(",")
						.map((s) => s.trim())
						.filter(Boolean)
				: [],
			timeout_ms: form.timeoutMs,
		};
		if (editTarget) {
			const { data } = await mcpApi.servers.update(editTarget.id, payload);
			setServers((prev) =>
				prev.map((s) => (s.id === editTarget.id ? data : s)),
			);
		} else {
			const { data } = await mcpApi.servers.create(payload);
			setServers((prev) => [...prev, data]);
		}
		setDrawerOpen(false);
	};

	const getConnParams = () => {
		const p: Parameters<typeof mcpApi.listTools>[0] = {
			transport: form.transport,
			timeout_ms: Math.min(form.timeoutMs, 15000),
		};
		if (form.transport === "stdio") {
			p.command = form.command.trim() || undefined;
			p.args = form.args.trim() ? form.args.split(/\s+/) : undefined;
			p.env =
				(entriesToRecord(form.envKeys, form.envVals) as
					| Record<string, string>
					| undefined) ?? undefined;
			p.cwd = form.cwd.trim() || undefined;
		} else {
			p.endpoint = form.endpoint.trim() || undefined;
			p.headers =
				(entriesToRecord(form.headerKeys, form.headerVals) as
					| Record<string, string>
					| undefined) ?? undefined;
		}
		if (isOfficial && editTarget?.market_id) {
			p.market_id = editTarget.market_id;
		}
		return p;
	};

	const handleListTools = async () => {
		setToolsLoading(true);
		setToolsOpen(true);
		setSelectedTool(null);
		setCallResult(null);
		setToolArgs({});
		try {
			const { data } = await mcpApi.listTools(getConnParams());
			if (data.success && data.tools) {
				setToolsList(data.tools);
				// Init args for each tool from schema defaults
				const init: Record<string, Record<string, unknown>> = {};
				for (const t of data.tools) {
					if (t.input_schema?.properties) {
						const props = t.input_schema.properties as Record<
							string,
							{ type?: string; default?: unknown }
						>;
						const vals: Record<string, unknown> = {};
						for (const [k, v] of Object.entries(props)) {
							if (v.default !== undefined) {
								vals[k] = v.default;
							} else {
								// Set type-appropriate empty value
								vals[k] =
									v.type === "boolean"
										? false
										: v.type === "number" || v.type === "integer"
											? 0
											: "";
							}
						}
						init[t.name] = vals;
					} else {
						init[t.name] = {};
					}
				}
				setToolArgs(init);
			} else {
				setToolsList([]);
			}
		} catch {
			setToolsList([]);
		} finally {
			setToolsLoading(false);
		}
	};

	const handleCallTool = async () => {
		setCalling(true);
		setCallResult(null);
		try {
			const args = toolArgs[selectedTool!] ?? {};
			// Filter out empty values
			const cleaned: Record<string, unknown> = {};
			for (const [k, v] of Object.entries(args)) {
				if (v !== "" && v !== undefined) cleaned[k] = v;
			}
			const { data } = await mcpApi.callTool({
				...getConnParams(),
				tool_name: selectedTool!,
				arguments: cleaned,
			});
			setCallResult(data);
		} catch (err: unknown) {
			const msg = err instanceof Error ? err.message : "请求失败";
			setCallResult({ success: false, message: msg });
		} finally {
			setCalling(false);
		}
	};

	const handleTestConnection = async () => {
		setTesting(true);
		setTestResult(null);
		try {
			const payload: Parameters<typeof mcpApi.testConnection>[0] = {
				transport: form.transport,
				timeout_ms: Math.min(form.timeoutMs, 15000),
			};
			if (form.transport === "stdio") {
				payload.command = form.command.trim() || undefined;
				payload.args = form.args.trim() ? form.args.split(/\s+/) : undefined;
				payload.env = entriesToRecord(form.envKeys, form.envVals) ?? undefined;
				payload.cwd = form.cwd.trim() || undefined;
			} else {
				payload.endpoint = form.endpoint.trim() || undefined;
				payload.headers =
					entriesToRecord(form.headerKeys, form.headerVals) ?? undefined;
			}
			if (isOfficial && editTarget?.market_id) {
				payload.market_id = editTarget.market_id;
			}
			const { data } = await mcpApi.testConnection(payload);
			setTestResult(data);
			if (data.success) {
				setTestedEndpoint(form.endpoint);
				setTestedHeaders(
					JSON.stringify(form.headerKeys.concat(form.headerVals)),
				);
				handleListTools();
			}
		} catch (err: unknown) {
			const msg = err instanceof Error ? err.message : "请求失败";
			setTestResult({ success: false, message: msg });
		} finally {
			setTesting(false);
		}
	};

	const handleClose = () => {
		setDrawerOpen(false);
	};

	if (loading)
		return (
			<div className="flex items-center justify-center py-32">
				<div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
			</div>
		);

	return (
		<div className="min-h-full">
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-lg font-semibold text-white">MCP 管理</h1>
						<p className="text-sm text-zinc-500 mt-0.5">
							Model Context Protocol 服务管理，安装后可被智能体引用
						</p>
					</div>
					<Button
						onClick={() => openDrawer(null, "new")}
						className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
					>
						<Plus className="w-4 h-4" />
						自定义 MCP
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
							已开通 ({servers.length})
						</TabsTrigger>
					</TabsList>

					<TabsContent value="market">
						<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
							{market.length === 0 ? (
								<div className="col-span-full flex flex-col items-center py-20">
									<div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
										<Cpu className="w-6 h-6 text-zinc-600" />
									</div>
									<p className="text-zinc-400 text-sm">暂无官方 MCP</p>
									<p className="text-zinc-600 text-xs mt-1">敬请期待</p>
								</div>
							) : (
								market.map((mcp) => {
									const installed = servers.some((s) => s.market_id === mcp.id);
									return (
										<div
											key={mcp.id}
											onClick={() => setSelectedMarket(mcp)}
											className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all cursor-pointer"
										>
											<div className="flex items-start gap-3 mb-3">
												<LogoIcon name={mcp.name} icon={mcp.icon} />
												<div className="flex-1 min-w-0">
													<p className="font-medium text-sm text-white truncate">
														{mcp.name}
													</p>
													<p className="text-[11px] text-zinc-600 mt-0.5">
														{mcp.price || mcp.billing}
													</p>
												</div>
												{mcp.provider === "官方" && (
													<span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-600/20 text-violet-400 border border-violet-500/20 shrink-0">
														官方
													</span>
												)}
											</div>
											<p className="text-xs text-zinc-500 leading-relaxed mb-3 line-clamp-2">
												{mcp.description}
											</p>
											<div className="flex flex-wrap gap-1 mb-4">
												{(mcp.tags || [])
													.filter((t) => t !== mcp.provider)
													.map((tag) => (
														<span
															key={tag}
															className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50"
														>
															{tag}
														</span>
													))}
											</div>
											<Button
												size="sm"
												disabled={installed}
												onClick={async () => {
													try {
														const { data } = await mcpApi.servers.create({
															market_id: mcp.id,
														});
														setServers((prev) => [...prev, data]);
													} catch {
														// ignore
													}
												}}
												className={`h-7 w-full text-xs ${installed ? "border-zinc-700 text-zinc-400 cursor-default" : "bg-violet-600 hover:bg-violet-500 text-white"}`}
												variant={installed ? "outline" : "default"}
											>
												{installed ? (
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
								})
							)}
						</div>
					</TabsContent>

					<TabsContent value="mine">
						{servers.length === 0 ? (
							<div className="flex flex-col items-center py-20">
								<div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
									<Cpu className="w-6 h-6 text-zinc-600" />
								</div>
								<p className="text-zinc-400 text-sm">还没有开通任何 MCP</p>
								<p className="text-zinc-600 text-xs mt-1">
									从市场安装或添加自定义 MCP
								</p>
							</div>
						) : (
							<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
								{servers.map((s) => {
									return (
										<div
											key={s.id}
											onClick={() => openDrawer(s, "edit")}
											className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group cursor-pointer relative"
										>
											<button
												onClick={(e) => {
													e.stopPropagation();
													removeServer(s.id);
												}}
												className="absolute top-3 right-3 text-zinc-500 hover:text-red-400 p-1 rounded hover:bg-red-400/10 transition-colors cursor-pointer"
												title="删除"
											>
												<Trash2 className="w-3.5 h-3.5" />
											</button>
											<div className="flex items-center gap-3 mb-3">
												<LogoIcon name={s.name} icon={s.icon} />
												<div className="flex-1 min-w-0">
													<p className="font-medium text-sm text-white truncate pr-6">
														{s.name}
													</p>
													<p className="text-[11px] text-zinc-600 font-mono mt-0.5">
														{s.transport}
													</p>
												</div>
											</div>
											<p className="text-xs text-zinc-500 leading-relaxed line-clamp-2">
												{s.description || s.endpoint}
											</p>
											{s.tags && s.tags.length > 0 && (
												<div className="flex flex-wrap gap-1">
													{s.tags.map((tag) => (
														<span
															key={tag}
															className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50"
														>
															{tag}
														</span>
													))}
												</div>
											)}
											{s.market_id && (
												<span className="text-[10px] text-zinc-600">
													来自市场
												</span>
											)}
										</div>
									);
								})}
							</div>
						)}
					</TabsContent>
				</Tabs>
			</div>

			{/* ── Market detail preview ── */}
			<Sheet
				open={!!selectedMarket}
				onOpenChange={(v) => !v && setSelectedMarket(null)}
			>
				<SheetContent className="flex flex-col gap-0" style={{ padding: 0 }}>
					<SheetHeader className="px-6 pt-6 pb-4 border-b border-zinc-800">
						<div className="flex items-center gap-3">
							<LogoIcon
								name={selectedMarket?.name}
								icon={selectedMarket?.icon}
							/>
							<div>
								<SheetTitle>{selectedMarket?.name}</SheetTitle>
								{selectedMarket?.provider && (
									<p className="text-[11px] text-zinc-500 mt-0.5">
										{selectedMarket.provider}
										{selectedMarket.price ? ` · ${selectedMarket.price}` : ""}
									</p>
								)}
							</div>
						</div>
					</SheetHeader>

					{selectedMarket && (
						<McpServerDetail
							server={
								{
									id: selectedMarket.id,
									name: selectedMarket.name,
									description: selectedMarket.description || "",
									icon: selectedMarket.icon || "",
									tags: selectedMarket.tags || [],
									transport:
										(selectedMarket.config
											?.transport as MCPServer["transport"]) || "streamable",
									command: (selectedMarket.config?.command as string) || "",
									args: (selectedMarket.config?.args as string[]) || [],
									env:
										(selectedMarket.config?.env as Record<string, string>) ||
										{},
									market_id: selectedMarket.id,
									cwd: (selectedMarket.config?.cwd as string) || "",
									endpoint: (selectedMarket.config?.endpoint as string) || "",
									headers:
										(selectedMarket.config?.headers as Record<
											string,
											string
										>) || {},
									timeout_ms:
										(selectedMarket.config?.timeout_ms as number) || 30000,
									created_at: selectedMarket.created_at,
								} as MCPServer
							}
							mode="view"
						/>
					)}

					<div className="border-t border-zinc-800 px-6 py-4">
						<Button
							size="sm"
							disabled={
								selectedMarket
									? servers.some((s) => s.market_id === selectedMarket.id)
									: false
							}
							onClick={async () => {
								if (!selectedMarket) return;
								try {
									const { data } = await mcpApi.servers.create({
										market_id: selectedMarket.id,
									});
									setServers((prev) => [...prev, data]);
									setSelectedMarket(null);
								} catch {
									// ignore
								}
							}}
							className={`h-8 w-full text-xs ${selectedMarket && servers.some((s) => s.market_id === selectedMarket.id) ? "border-zinc-700 text-zinc-400 cursor-default" : "bg-violet-600 hover:bg-violet-500 text-white"}`}
							variant={
								selectedMarket &&
								servers.some((s) => s.market_id === selectedMarket.id)
									? "outline"
									: "default"
							}
						>
							{selectedMarket &&
							servers.some((s) => s.market_id === selectedMarket.id) ? (
								<>
									<CheckCircle2 className="w-3.5 h-3.5 mr-1" />
									已安装
								</>
							) : (
								"安装"
							)}
						</Button>
					</div>
				</SheetContent>
			</Sheet>

			{/* ── Right‑side drawer: editor ── */}
			<Sheet open={drawerOpen} onOpenChange={setDrawerOpen}>
				<SheetContent className="flex flex-col gap-0" style={{ padding: 0 }}>
					<SheetHeader className="px-6 pt-6 pb-4 border-b border-zinc-800">
						<div className="flex items-center gap-3">
							<LogoIcon name={editTarget?.name} icon={editTarget?.icon} />{" "}
							<div>
								<SheetTitle>
									{drawerMode === "new"
										? "添加自定义 MCP 服务器"
										: isOfficial
											? editTarget?.name
											: "编辑 MCP 服务器"}
								</SheetTitle>
								{isOfficial && (
									<p className="text-[11px] text-zinc-500 mt-0.5">
										官方 MCP · 部分配置由系统管理
									</p>
								)}
							</div>
						</div>
					</SheetHeader>

					<div className="flex-1 overflow-y-auto">
						<McpServerDetail
							server={editTarget ?? ({ name: "" } as MCPServer)}
							mode="edit"
							form={form}
							onFormChange={(key, val) =>
								setForm((f) => ({ ...f, [key]: val }))
							}
							isOfficial={isOfficial}
							toolsList={toolsList}
							descPreview={descPreview}
							onDescPreviewChange={setDescPreview}
							addEnvRow={addEnvRow}
							updateEnv={updateEnv}
							addHeaderRow={addHeaderRow}
							updateHeader={updateHeader}
						/>

						{/* ── Tool testing section ── */}
						{testResult?.success && (
							<div className="border-t border-zinc-800 px-6 pt-4 pb-4 space-y-3">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2 text-xs text-zinc-400">
										<Terminal className="w-3.5 h-3.5" strokeWidth={1.5} />
										<span className="font-medium">体验工具</span>
										{toolsLoading && (
											<Loader2 className="w-3 h-3 animate-spin" />
										)}
									</div>
									<button
										type="button"
										onClick={handleListTools}
										className="text-[11px] text-violet-400 hover:text-violet-300 transition-colors flex items-center gap-1"
										title="刷新工具列表"
									>
										<RefreshCw
											className={`w-3 h-3 ${toolsLoading ? "animate-spin" : ""}`}
										/>
										刷新
									</button>
								</div>

								{!toolsLoading && toolsList.length > 0 && (
									<div className="space-y-2 max-h-80 overflow-y-auto">
										{toolsList.map((tool) => (
											<div
												key={tool.name}
												className="bg-zinc-800/50 border border-zinc-700/50 rounded-lg overflow-hidden"
											>
												<button
													type="button"
													onClick={() =>
														setSelectedTool(
															selectedTool === tool.name ? null : tool.name,
														)
													}
													className="flex items-center gap-2 w-full px-3 py-2 text-left cursor-pointer hover:bg-zinc-700/50 transition-colors"
												>
													<span className="text-xs font-mono text-violet-400 shrink-0">
														{selectedTool === tool.name ? "▾" : "▸"}
													</span>
													<span className="text-xs text-white font-medium">
														{tool.name}
													</span>
													{tool.description && (
														<span className="text-[10px] text-zinc-500 truncate ml-2">
															{tool.description}
														</span>
													)}
												</button>

												{selectedTool === tool.name && (
													<div className="border-t border-zinc-700/50 px-3 py-2 space-y-2">
														<ToolInputs
															schema={tool.input_schema}
															values={toolArgs[tool.name] ?? {}}
															onChange={(key, val) =>
																setToolArgs((prev) => ({
																	...prev,
																	[tool.name]: {
																		...prev[tool.name],
																		[key]: val,
																	},
																}))
															}
														/>
														<div className="flex items-start gap-2">
															<Button
																size="sm"
																onClick={handleCallTool}
																disabled={calling}
																className="bg-violet-600 hover:bg-violet-500 text-white h-7 text-[11px] px-3 gap-1 shrink-0"
															>
																{calling ? (
																	<Loader2 className="w-3 h-3 animate-spin" />
																) : (
																	<Play className="w-3 h-3" />
																)}
																调用
															</Button>
															{callResult && (
																<div className="flex-1 min-w-0">
																	<ToolResult result={callResult} />
																</div>
															)}
														</div>
													</div>
												)}
											</div>
										))}
									</div>
								)}

								{toolsOpen && !toolsLoading && toolsList.length === 0 && (
									<p className="text-xs text-zinc-600">未获取到工具列表</p>
								)}
							</div>
						)}
					</div>

					{/* ── Footer actions ── */}
					<div className="border-t border-zinc-800 px-6 py-4 space-y-3">
						{testResult && (
							<div
								className={`text-xs ${testResult.success ? "text-emerald-400" : "text-red-400"}`}
							>
								{testResult.message}
							</div>
						)}
						<div className="flex items-center gap-2">
							{editTarget && (
								<Button
									variant="outline"
									size="sm"
									onClick={() => removeServer(editTarget.id)}
									className="border-zinc-700 text-red-400 hover:bg-red-400/10 hover:text-red-300 h-8 text-xs gap-1.5"
								>
									<Trash2 className="w-3.5 h-3.5" />
									删除
								</Button>
							)}
							<Button
								size="sm"
								onClick={handleTestConnection}
								disabled={testing || !form.name.trim()}
								className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1.5"
								variant="outline"
							>
								{testing ? (
									<Loader2 className="w-3.5 h-3.5 animate-spin" />
								) : (
									<Cpu className="w-3.5 h-3.5" />
								)}
								测试连接
							</Button>
							<Button
								variant="outline"
								onClick={handleClose}
								className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
							>
								取消
							</Button>
							<Button
								onClick={handleSave}
								disabled={
									!form.name.trim() ||
									form.endpoint !== testedEndpoint ||
									JSON.stringify(form.headerKeys.concat(form.headerVals)) !==
										testedHeaders
								}
								className="bg-violet-600 hover:bg-violet-500 text-white ml-auto"
							>
								{drawerMode === "new" ? "添加" : "保存"}
							</Button>
						</div>
					</div>
				</SheetContent>
			</Sheet>
		</div>
	);
}
