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
	Pencil,
	Loader2,
	Terminal,
	ChevronDown,
	ChevronRight,
	Play,
	CheckCircle2,
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
import { mcpApi, type MCPMarketEntry, type MCPServer } from "@/lib/api";

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

export default function McpPage() {
	const [market, setMarket] = useState<MCPMarketEntry[]>([]);
	const [servers, setServers] = useState<MCPServer[]>([]);
	const [loading, setLoading] = useState(true);

	const [addOpen, setAddOpen] = useState(false);
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
	const [selectedTool, setSelectedTool] = useState<string | null>(null);
	const [callArgs, setCallArgs] = useState("{}");
	const [callResult, setCallResult] = useState<{
		success: boolean;
		message?: string;
		is_error?: boolean;
		output?: string;
	} | null>(null);
	const [calling, setCalling] = useState(false);
	const [form, setForm] = useState({
		name: "",
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
	});

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
			if (keys[i].trim()) {
				rec[keys[i].trim()] = vals[i].trim();
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

	const handleRemoveServer = async (serverId: string) => {
		await mcpApi.servers.remove(serverId);
		setServers((prev) => prev.filter((s) => s.id !== serverId));
	};

	const handleEdit = (s: MCPServer) => {
		setEditTarget(s);
		setForm({
			name: s.name,
			transport: s.transport,
			command: s.command || "",
			args: (s.args || []).join(" "),
			endpoint: s.endpoint || "",
			toolList: (s.tool_name_list || []).join(", "),
			timeoutMs: s.timeout_ms,
			cwd: s.cwd || "",
			envKeys: s.env ? Object.keys(s.env) : [""],
			envVals: s.env ? Object.values(s.env) : [""],
			headerKeys: s.headers ? Object.keys(s.headers) : [""],
			headerVals: s.headers ? Object.values(s.headers) : [""],
		});
		setAddOpen(true);
	};

	const handleSave = async () => {
		if (!form.name.trim()) return;
		const payload = {
			name: form.name.trim(),
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
		resetForm();
	};

	const getConnParams = () => {
		const p: {
			transport: string;
			command?: string;
			args?: string[];
			env?: Record<string, string>;
			cwd?: string;
			endpoint?: string;
			headers?: Record<string, string>;
			timeout_ms?: number;
		} = {
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
		return p;
	};

	const handleListTools = async () => {
		setToolsLoading(true);
		setToolsOpen(true);
		setSelectedTool(null);
		setCallResult(null);
		try {
			const { data } = await mcpApi.listTools(getConnParams());
			if (data.success && data.tools) {
				setToolsList(data.tools);
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
			let parsed: Record<string, unknown> = {};
			try {
				parsed = JSON.parse(callArgs);
			} catch {
				setCallResult({
					success: false,
					message: "参数格式错误，请输入合法的 JSON",
				});
				setCalling(false);
				return;
			}
			const { data } = await mcpApi.callTool({
				...getConnParams(),
				tool_name: selectedTool!,
				arguments: parsed,
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
			const payload: {
				transport: string;
				command?: string;
				args?: string[];
				env?: Record<string, string>;
				cwd?: string;
				endpoint?: string;
				headers?: Record<string, string>;
				timeout_ms?: number;
			} = {
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
			const { data } = await mcpApi.testConnection(payload);
			setTestResult(data);
		} catch (err: unknown) {
			const msg = err instanceof Error ? err.message : "请求失败";
			setTestResult({ success: false, message: msg });
		} finally {
			setTesting(false);
		}
	};

	const resetForm = () => {
		setForm({
			name: "",
			transport: "streamable",
			command: "",
			args: "",
			endpoint: "",
			toolList: "",
			timeoutMs: 30000,
			cwd: "",
			envKeys: [""],
			envVals: [""],
			headerKeys: [""],
			headerVals: [""],
		});
		setEditTarget(null);
		setTestResult(null);
		setAddOpen(false);
		setToolsList([]);
		setToolsOpen(false);
		setSelectedTool(null);
		setCallResult(null);
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
						onClick={() => setAddOpen(true)}
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
									const Icon = pickIcon(mcp.name);
									return (
										<div
											key={mcp.id}
											className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all"
										>
											<div className="flex items-start gap-3 mb-3">
												<div className="w-9 h-9 rounded-xl bg-violet-400/10 flex items-center justify-center shrink-0">
													<Icon
														className="w-4 h-4 text-violet-400"
														strokeWidth={1.5}
													/>
												</div>
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
									const Icon = pickIcon(s.name);
									return (
										<div
											key={s.id}
											className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group"
										>
											<div className="flex items-center gap-3 mb-3">
												<div className="w-9 h-9 rounded-xl bg-violet-400/10 flex items-center justify-center shrink-0">
													<Icon
														className="w-4 h-4 text-violet-400"
														strokeWidth={1.5}
													/>
												</div>
												<div className="flex-1 min-w-0">
													<p className="font-medium text-sm text-white truncate">
														{s.name}
													</p>
													<p className="text-[11px] text-zinc-600 font-mono mt-0.5">
														{s.transport}
													</p>
												</div>
											</div>
											<p className="text-xs text-zinc-500 leading-relaxed mb-3 line-clamp-2">
												{s.description || s.endpoint}
											</p>
											<div className="flex items-center justify-between">
												{s.market_id && (
													<span className="text-[10px] text-zinc-600">
														来自市场
													</span>
												)}
												<div className="flex gap-1 ml-auto">
													<button
														onClick={() => handleEdit(s)}
														className="text-zinc-500 hover:text-zinc-300 p-1.5 rounded hover:bg-zinc-800 transition-colors cursor-pointer opacity-0 group-hover:opacity-100"
													>
														<Pencil className="w-3.5 h-3.5" />
													</button>
													<button
														onClick={() => handleRemoveServer(s.id)}
														className="text-zinc-500 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 transition-colors cursor-pointer opacity-0 group-hover:opacity-100"
													>
														<Trash2 className="w-3.5 h-3.5" />
													</button>
												</div>
											</div>
										</div>
									);
								})}
							</div>
						)}
					</TabsContent>
				</Tabs>
			</div>

			<Dialog
				open={addOpen}
				onOpenChange={(open) => {
					if (!open) resetForm();
				}}
			>
				<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg max-h-[80vh] overflow-y-auto">
					<DialogHeader>
						<DialogTitle className="text-white flex items-center gap-2">
							<Cpu className="w-4 h-4 text-violet-400" />
							{editTarget ? "编辑 MCP 服务器" : "添加自定义 MCP 服务器"}
						</DialogTitle>
					</DialogHeader>
					<div className="space-y-4 py-2">
						<Field label="名称">
							<Input
								value={form.name}
								onChange={(e) =>
									setForm((f) => ({ ...f, name: e.target.value }))
								}
								placeholder="my-mcp-server"
								className={inp}
							/>
						</Field>
						<Field label="传输协议">
							<div className="flex gap-1.5">
								{(["streamable", "sse"] as const).map((t) => (
									<button
										key={t}
										type="button"
										onClick={() => setForm((f) => ({ ...f, transport: t }))}
										className={`flex-1 h-9 rounded-md text-sm font-mono border transition-colors cursor-pointer ${
											form.transport === t
												? "bg-violet-600 border-violet-500 text-white"
												: "bg-zinc-800 border-zinc-700 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-300"
										}`}
									>
										{t}
									</button>
								))}
							</div>
						</Field>
						{form.transport === "stdio" ? (
							<>
								<Field label="命令">
									<Input
										value={form.command}
										onChange={(e) =>
											setForm((f) => ({ ...f, command: e.target.value }))
										}
										placeholder="npx / python / uvx"
										className={inp}
									/>
								</Field>
								<Field label="参数（空格分隔）">
									<Input
										value={form.args}
										onChange={(e) =>
											setForm((f) => ({ ...f, args: e.target.value }))
										}
										placeholder="-y @modelcontextprotocol/server-filesystem /path"
										className={inp}
									/>
								</Field>
								<Field label="工作目录（可选）">
									<Input
										value={form.cwd}
										onChange={(e) =>
											setForm((f) => ({ ...f, cwd: e.target.value }))
										}
										placeholder="/data/mcp"
										className={inp}
									/>
								</Field>
								<div className="space-y-2">
									<div className="flex items-center justify-between">
										<Label className="text-xs text-zinc-400 uppercase tracking-wide">
											环境变量
										</Label>
										<button
											onClick={addEnvRow}
											className="text-[11px] text-violet-400 hover:text-violet-300 cursor-pointer"
										>
											+ 添加
										</button>
									</div>
									{form.envKeys.map((_, i) => (
										<div key={i} className="flex gap-2">
											<Input
												value={form.envKeys[i]}
												onChange={(e) =>
													updateEnv(i, e.target.value, form.envVals[i])
												}
												placeholder="KEY"
												className={`${inp} flex-1 font-mono text-xs`}
											/>
											<Input
												value={form.envVals[i]}
												onChange={(e) =>
													updateEnv(i, form.envKeys[i], e.target.value)
												}
												placeholder="value"
												className={`${inp} flex-1 font-mono text-xs`}
											/>
										</div>
									))}
								</div>
							</>
						) : (
							<>
								<Field label="Endpoint">
									<Input
										value={form.endpoint}
										onChange={(e) =>
											setForm((f) => ({ ...f, endpoint: e.target.value }))
										}
										placeholder="https://api.example.com/mcp"
										className={inp}
									/>
								</Field>
								<div className="space-y-2">
									<div className="flex items-center justify-between">
										<Label className="text-xs text-zinc-400 uppercase tracking-wide">
											HTTP Headers
										</Label>
										<button
											onClick={addHeaderRow}
											className="text-[11px] text-violet-400 hover:text-violet-300 cursor-pointer"
										>
											+ 添加
										</button>
									</div>
									{form.headerKeys.map((_, i) => (
										<div key={i} className="flex gap-2">
											<Input
												value={form.headerKeys[i]}
												onChange={(e) =>
													updateHeader(i, e.target.value, form.headerVals[i])
												}
												placeholder="Header-Name"
												className={`${inp} flex-1 font-mono text-xs`}
											/>
											<Input
												value={form.headerVals[i]}
												onChange={(e) =>
													updateHeader(i, form.headerKeys[i], e.target.value)
												}
												placeholder="value"
												className={`${inp} flex-1 font-mono text-xs`}
											/>
										</div>
									))}
								</div>
							</>
						)}
						<Field label="工具白名单（逗号分隔，留空=全部）">
							<Input
								value={form.toolList}
								onChange={(e) =>
									setForm((f) => ({ ...f, toolList: e.target.value }))
								}
								placeholder="read_file, write_file"
								className={inp}
							/>
						</Field>
						<Field label="超时 (ms)">
							<Input
								type="number"
								min={0}
								value={form.timeoutMs}
								onChange={(e) =>
									setForm((f) => ({ ...f, timeoutMs: +e.target.value }))
								}
								className={inp}
							/>
						</Field>
					</div>

					{/* ── Tool Testing Section ── */}
					{testResult?.success && (
						<div className="border-t border-zinc-800 pt-4 space-y-3">
							<button
								type="button"
								onClick={() => {
									if (toolsOpen) {
										setToolsOpen(false);
									} else {
										handleListTools();
									}
								}}
								className="flex items-center gap-2 text-xs text-zinc-400 hover:text-zinc-200 transition-colors cursor-pointer w-full"
							>
								<Terminal className="w-3.5 h-3.5" strokeWidth={1.5} />
								<span className="font-medium">体验工具</span>
								{toolsLoading ? (
									<Loader2 className="w-3 h-3 animate-spin ml-auto" />
								) : toolsOpen ? (
									<ChevronDown
										className="w-3.5 h-3.5 ml-auto"
										strokeWidth={1.5}
									/>
								) : (
									<ChevronRight
										className="w-3.5 h-3.5 ml-auto"
										strokeWidth={1.5}
									/>
								)}
							</button>

							{toolsOpen && !toolsLoading && toolsList.length > 0 && (
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
													{tool.input_schema?.properties ? (
														<div className="text-[10px] text-zinc-500 space-y-0.5">
															<span className="text-zinc-400">参数:</span>
															{Object.entries(
																tool.input_schema.properties as Record<
																	string,
																	unknown
																>,
															).map(([k, v]) => (
																<div key={k} className="flex gap-2">
																	<span className="font-mono text-zinc-300">
																		{k}
																	</span>
																	<span className="text-zinc-600">
																		{(v as Record<string, string>)
																			.description ?? ""}
																	</span>
																</div>
															))}
														</div>
													) : null}
													<textarea
														value={callArgs}
														onChange={(e) => setCallArgs(e.target.value)}
														className="w-full h-20 bg-zinc-900 border border-zinc-700 rounded text-[11px] font-mono text-zinc-300 p-2 resize-none focus:outline-none focus:ring-1 focus:ring-violet-500 placeholder:text-zinc-600"
														placeholder='{"key": "value"}'
													/>
													<div className="flex items-start gap-2">
														<Button
															size="sm"
															onClick={handleCallTool}
															disabled={calling}
															className="bg-violet-600 hover:bg-violet-500 text-white h-7 text-[11px] px-3 gap-1"
														>
															{calling ? (
																<Loader2 className="w-3 h-3 animate-spin" />
															) : (
																<Play className="w-3 h-3" />
															)}
															调用
														</Button>
														{callResult && (
															<div
																className={`flex-1 text-[11px] whitespace-pre-wrap break-all ${
																	callResult.success
																		? callResult.is_error
																			? "text-amber-400"
																			: "text-emerald-400"
																		: "text-red-400"
																}`}
															>
																{callResult.output || callResult.message}
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

					<DialogFooter className="flex items-center gap-3">
						{testResult && (
							<span
								className={`text-xs flex-1 ${testResult.success ? "text-emerald-400" : "text-red-400"}`}
							>
								{testResult.message}
							</span>
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
							onClick={() => resetForm()}
							className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
						>
							取消
						</Button>
						<Button
							onClick={handleSave}
							disabled={!form.name.trim()}
							className="bg-violet-600 hover:bg-violet-500 text-white"
						>
							{editTarget ? "保存" : "添加"}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}

function Field({
	label,
	children,
}: {
	label: string;
	children: React.ReactNode;
}) {
	return (
		<div className="space-y-1.5">
			<Label className="text-xs text-zinc-400 uppercase tracking-wide">
				{label}
			</Label>
			{children}
		</div>
	);
}

const inp =
	"bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500 h-9";
