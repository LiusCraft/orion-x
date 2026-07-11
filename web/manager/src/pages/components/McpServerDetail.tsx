import { useState } from "react";
import {
	Globe,
	Database,
	Shield,
	HardDrive,
	GitBranch,
	Link,
	Brain,
	Cpu,
	EyeOff,
	Asterisk,
	Copy,
	Check,
	AlertCircle,
	CheckCircle2,
	X as XIcon,
	type LucideIcon,
} from "lucide-react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import type { MCPServer } from "@/lib/api";

export const inp =
	"bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500 h-9";

function pickIcon(name?: string): LucideIcon {
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
	return Cpu;
}

export function McpServerIcon({
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

export function Field({
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

import { marked } from "marked";

marked.use({
	renderer: {
		code({ text, lang }) {
			const langClass = lang ? `language-${lang}` : "";
			return `<pre class="text-[11px] bg-zinc-800/80 rounded-lg px-3 py-2 my-2 overflow-x-auto"><code class="${langClass} text-zinc-200">${text}</code></pre>`;
		},
		codespan({ text }) {
			return `<code class="text-violet-300 bg-zinc-800/80 px-1.5 py-0.5 rounded text-[11px] font-mono">${text}</code>`;
		},
		listitem(text) {
			return `<li class="ml-4 list-disc text-zinc-300">${text}</li>`;
		},
		list(token) {
			const tag = (token as { ordered?: boolean }).ordered ? "ol" : "ul";
			const text = (token as { text?: string }).text ?? "";
			return `<${tag} class="space-y-0.5 my-1.5">${text}</${tag}>`;
		},
		heading({ text, depth }) {
			const sizes = ["text-base", "text-sm", "text-xs"];
			const size = sizes[Math.min(depth - 1, sizes.length - 1)];
			return `<h${depth} class="${size} text-white font-semibold mt-3 mb-1.5">${text}</h${depth}>`;
		},
		hr() {
			return `<hr class="border-zinc-700 my-3" />`;
		},
		paragraph({ text }) {
			return `<p class="text-xs text-zinc-300 leading-relaxed mb-1.5">${text}</p>`;
		},
		strong({ text }) {
			return `<strong class="text-white font-semibold">${text}</strong>`;
		},
		em({ text }) {
			return `<em class="text-zinc-200 italic">${text}</em>`;
		},
		blockquote({ text }) {
			return `<blockquote class="border-l-2 border-violet-500/40 pl-3 py-0.5 my-2 text-xs text-zinc-400 italic">${text}</blockquote>`;
		},
		table({ header, rows }) {
			const thead = `<thead class="text-xs text-zinc-300">${header}</thead>`;
			const tbody = `<tbody class="text-xs text-zinc-400">${rows}</tbody>`;
			return `<table class="w-full border-collapse my-2">${thead}${tbody}</table>`;
		},
		tablerow({ text }) {
			return `<tr class="border-b border-zinc-800">${text}</tr>`;
		},
		tablecell({ text, align, header }) {
			const tag = header ? "th" : "td";
			const alignClass = align ? `text-${align}` : "";
			return `<${tag} class="px-2 py-1 ${alignClass}">${text}</${tag}>`;
		},
	},
});

export function SimpleMarkdown({ text }: { text: string | string[] }) {
	if (!text) return null;
	const html = marked.parse(text as string, { breaks: true }) as string;
	return <span dangerouslySetInnerHTML={{ __html: html }} />;
}

export function TagInput({
	tags,
	onChange,
}: {
	tags: string[];
	onChange: (tags: string[]) => void;
}) {
	const [input, setInput] = useState("");
	const addTag = (tag: string) => {
		const t = tag.trim();
		if (t && !tags.includes(t)) onChange([...tags, t]);
		setInput("");
	};
	const removeTag = (tag: string) => onChange(tags.filter((t) => t !== tag));
	const handleKey = (e: React.KeyboardEvent) => {
		if (e.key === "Enter" || e.key === ",") {
			e.preventDefault();
			addTag(input);
		}
		if (e.key === "Backspace" && !input && tags.length > 0)
			removeTag(tags[tags.length - 1]);
	};
	return (
		<div className="flex flex-wrap gap-1.5 p-2 rounded-lg bg-zinc-800 border border-zinc-700 focus-within:ring-1 focus-within:ring-violet-500">
			{tags.map((tag) => (
				<span
					key={tag}
					className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-violet-600 text-white border border-violet-400/30 font-medium"
				>
					{tag}
					<button
						type="button"
						onClick={() => removeTag(tag)}
						className="hover:text-white transition-colors cursor-pointer"
					>
						<XIcon className="w-2.5 h-2.5" />
					</button>
				</span>
			))}
			<input
				value={input}
				onChange={(e) => setInput(e.target.value)}
				onKeyDown={handleKey}
				placeholder={
					tags.length === 0 ? "输入标签后按 Enter 或逗号添加" : "添加标签..."
				}
				className="flex-1 min-w-[80px] bg-transparent border-none outline-none text-xs text-white placeholder:text-zinc-500"
			/>
		</div>
	);
}

export function ToolAllowlist({
	tools,
	selected,
	onChange,
}: {
	tools: { name: string; description: string }[];
	selected: string[];
	onChange: (selected: string[]) => void;
}) {
	const allSelected = selected.length === 0;
	const toggle = (name: string) =>
		onChange(
			selected.includes(name)
				? selected.filter((s) => s !== name)
				: [...selected, name],
		);
	return (
		<div className="space-y-2">
			<div className="flex items-center justify-between">
				<span className="text-[11px] text-zinc-500">
					{allSelected
						? "已选择全部工具"
						: `已选择 ${selected.length}/${tools.length} 个工具`}
				</span>
				<button
					type="button"
					onClick={() => onChange(allSelected ? tools.map((t) => t.name) : [])}
					className="text-[11px] text-violet-400 hover:text-violet-300 transition-colors"
				>
					{allSelected ? "取消全选" : "全选"}
				</button>
			</div>
			<div className="grid grid-cols-2 gap-1.5 max-h-48 overflow-y-auto bg-zinc-800/50 rounded-lg p-2">
				{tools.map((tool) => {
					const on = allSelected || selected.includes(tool.name);
					return (
						<label
							key={tool.name}
							className={`flex items-start gap-2 p-1.5 rounded cursor-pointer transition-colors ${on ? "bg-violet-500/10" : "hover:bg-zinc-800"}`}
						>
							<input
								type="checkbox"
								checked={on}
								onChange={() => toggle(tool.name)}
								className="accent-violet-500 w-3.5 h-3.5 mt-0.5 shrink-0"
							/>
							<div className="min-w-0">
								<p className="text-[11px] font-mono text-zinc-200 truncate">
									{tool.name}
								</p>
								{tool.description && (
									<p className="text-[10px] text-zinc-500 truncate leading-relaxed">
										{tool.description}
									</p>
								)}
							</div>
						</label>
					);
				})}
			</div>
		</div>
	);
}

export function ToolInputs({
	schema,
	values,
	onChange,
}: {
	schema?: Record<string, unknown>;
	values: Record<string, unknown>;
	onChange: (key: string, val: unknown) => void;
}) {
	const properties = schema?.properties as
		| Record<string, { type?: string; description?: string; enum?: string[] }>
		| undefined;
	if (!properties || Object.keys(properties).length === 0)
		return <div className="text-[11px] text-zinc-500 px-1">无参数</div>;
	const required = new Set<string>(
		(schema?.required as string[] | undefined) ?? [],
	);
	return (
		<div className="border-t border-zinc-700/50 px-3 py-2.5 space-y-2.5">
			{Object.entries(properties).map(([key, prop]) => {
				const isReq = required.has(key);
				const type = prop.type || "string";
				const desc = prop.description;
				const val = values[key];
				return (
					<div key={key}>
						<div className="flex items-center gap-1 mb-1">
							<span className="text-[11px] font-mono text-zinc-300">{key}</span>
							<span className="text-[10px] text-zinc-600">{type}</span>
							{isReq && <Asterisk className="w-2.5 h-2.5 text-red-400" />}
						</div>
						{desc && (
							<p className="text-[10px] text-zinc-500 mb-1.5 leading-relaxed">
								{desc}
							</p>
						)}
						{type === "boolean" ? (
							<label className="flex items-center gap-2 cursor-pointer">
								<input
									type="checkbox"
									checked={!!val}
									onChange={(e) => onChange(key, e.target.checked)}
									className="accent-violet-500 w-3.5 h-3.5"
								/>
								<span className="text-xs text-zinc-400">
									{val ? "true" : "false"}
								</span>
							</label>
						) : type === "integer" || type === "number" ? (
							<input
								type="number"
								value={val as string | number}
								onChange={(e) =>
									onChange(
										key,
										type === "integer"
											? parseInt(e.target.value, 10) || 0
											: parseFloat(e.target.value) || 0,
									)
								}
								className="w-full h-8 bg-zinc-900 border border-zinc-700 rounded text-[11px] font-mono text-zinc-300 px-2.5 focus:outline-none focus:ring-1 focus:ring-violet-500"
								placeholder={type === "integer" ? "0" : "0.0"}
							/>
						) : prop.enum ? (
							<select
								value={val as string}
								onChange={(e) => onChange(key, e.target.value)}
								className="w-full h-8 bg-zinc-900 border border-zinc-700 rounded text-[11px] font-mono text-zinc-300 px-2.5 focus:outline-none focus:ring-1 focus:ring-violet-500 appearance-none"
							>
								{prop.enum.map((opt) => (
									<option key={opt} value={opt}>
										{opt}
									</option>
								))}
							</select>
						) : (
							<input
								value={val as string}
								onChange={(e) => onChange(key, e.target.value)}
								className="w-full h-8 bg-zinc-900 border border-zinc-700 rounded text-[11px] font-mono text-zinc-300 px-2.5 focus:outline-none focus:ring-1 focus:ring-violet-500 placeholder:text-zinc-600"
								placeholder={type}
							/>
						)}
					</div>
				);
			})}
		</div>
	);
}

export function ToolResult({
	result,
}: {
	result: {
		success: boolean;
		message?: string;
		is_error?: boolean;
		output?: string;
	};
}) {
	const [copied, setCopied] = useState(false);
	const text = result.output || result.message || "";
	const isJson =
		result.success && !result.is_error && /^\s*[[{]/.test(text.trim());
	let parsed: unknown = null;
	let formatted = text;
	let parseError = false;
	if (isJson) {
		try {
			parsed = JSON.parse(text);
			formatted = JSON.stringify(parsed, null, 2);
		} catch {
			parseError = true;
		}
	}
	const handleCopy = async () => {
		try {
			await navigator.clipboard.writeText(formatted);
			setCopied(true);
			setTimeout(() => setCopied(false), 2000);
		} catch {
			/* ignore */
		}
	};
	const statusColor = result.is_error
		? "text-amber-400 border-amber-500/30 bg-amber-400/5"
		: result.success
			? "text-emerald-400 border-emerald-500/30 bg-emerald-400/5"
			: "text-red-400 border-red-500/30 bg-red-400/5";
	const icon = result.is_error ? (
		<AlertCircle className="w-3.5 h-3.5 shrink-0" />
	) : result.success ? (
		<CheckCircle2 className="w-3.5 h-3.5 shrink-0" />
	) : (
		<AlertCircle className="w-3.5 h-3.5 shrink-0" />
	);
	return (
		<div
			className={`text-[11px] rounded-lg border ${statusColor} overflow-hidden`}
		>
			<div className="flex items-center gap-1.5 px-2.5 py-1.5 border-b border-inherit opacity-80">
				{icon}
				<span className="font-medium">
					{result.is_error
						? "执行出错"
						: result.success
							? "执行成功"
							: "请求失败"}
				</span>
				<button
					onClick={handleCopy}
					className="ml-auto p-0.5 rounded hover:bg-black/20 transition-colors"
					title="复制结果"
				>
					{copied ? (
						<Check className="w-3 h-3" />
					) : (
						<Copy className="w-3 h-3" />
					)}
				</button>
			</div>
			<pre className="px-2.5 py-2 overflow-x-auto leading-relaxed whitespace-pre-wrap break-all max-h-48">
				{parsed && !parseError ? <SyntaxJson data={parsed} /> : formatted}
			</pre>
		</div>
	);
}

function SyntaxJson({ data }: { data: unknown }) {
	if (data === null || data === undefined)
		return <span className="text-zinc-500">null</span>;
	if (typeof data === "string")
		return <span className="text-emerald-400">"{data}"</span>;
	if (typeof data === "number")
		return <span className="text-amber-400">{String(data)}</span>;
	if (typeof data === "boolean")
		return <span className="text-sky-400">{data ? "true" : "false"}</span>;
	if (Array.isArray(data))
		return (
			<>
				<span className="text-zinc-500">[</span>
				<div className="pl-4 border-l border-zinc-700/30 ml-1">
					{data.map((item, i) => (
						<div key={i}>
							<SyntaxJson data={item} />
							{i < data.length - 1 && <span className="text-zinc-600">,</span>}
						</div>
					))}
				</div>
				<span className="text-zinc-500">]</span>
			</>
		);
	if (typeof data === "object") {
		const entries = Object.entries(data as Record<string, unknown>);
		if (entries.length === 0) return <span className="text-zinc-500">{}</span>;
		return (
			<>
				<span className="text-zinc-500">{"{"}</span>
				<div className="pl-4 border-l border-zinc-700/30 ml-1">
					{entries.map(([k, v], i) => (
						<div key={k} className="break-all">
							<span className="text-violet-400">"{k}"</span>
							<span className="text-zinc-600">: </span>
							<SyntaxJson data={v} />
							{i < entries.length - 1 && (
								<span className="text-zinc-600">,</span>
							)}
						</div>
					))}
				</div>
				<span className="text-zinc-500">{"}"}</span>
			</>
		);
	}
	return <span>{String(data)}</span>;
}

/**
 * MCP 服务器详情/编辑组件
 * - mode="view": 只读展示
 * - mode="edit": 可编辑表单（通过 form/onFormChange 受控）
 */
export function McpServerDetail({
	server,
	mode = "view",
	form,
	onFormChange,
	isOfficial,
	toolsList,
	descPreview,
	onDescPreviewChange,
	addEnvRow,
	updateEnv,
	addHeaderRow,
	updateHeader,
	headerMeta,
	headerValues,
	onHeaderValueChange,
}: {
	server: MCPServer;
	mode?: "view" | "edit";
	form?: Record<string, string | number | string[]>;
	onFormChange?: (key: string, value: string | number) => void;
	isOfficial?: boolean;
	toolsList?: { name: string; description: string }[];
	descPreview?: boolean;
	onDescPreviewChange?: (v: boolean) => void;
	addEnvRow?: () => void;
	updateEnv?: (i: number, key: string, val: string) => void;
	addHeaderRow?: () => void;
	updateHeader?: (i: number, key: string, val: string) => void;
	headerMeta?: Record<
		string,
		{
			kind: string;
			label?: string;
			description?: string;
			placeholder?: string;
			default?: string;
			value?: string;
		}
	>;
	headerValues?: Record<string, string>;
	onHeaderValueChange?: (key: string, value: string) => void;
}) {
	if (mode === "view") {
		return (
			<div className="flex-1 overflow-y-auto px-6 py-4 space-y-5">
				{server.description && (
					<div>
						<p className="text-xs text-zinc-400 uppercase tracking-wide mb-1.5">
							描述
						</p>
						<div className="text-sm text-zinc-300 leading-relaxed">
							<SimpleMarkdown text={server.description} />
						</div>
					</div>
				)}
				{server.market_id ? null : server.transport === "stdio" ? (
					<div>
						<p className="text-xs text-zinc-400 uppercase tracking-wide mb-1.5">
							命令
						</p>
						<code className="block text-sm text-zinc-200 bg-zinc-800/60 rounded-lg px-3 py-2 font-mono break-all">
							{server.command}
							{server.args?.length ? ` ${server.args.join(" ")}` : ""}
						</code>
						{server.cwd && (
							<p className="text-[11px] text-zinc-600 mt-1">
								工作目录: {server.cwd}
							</p>
						)}
					</div>
				) : (
					<div>
						<p className="text-xs text-zinc-400 uppercase tracking-wide mb-1.5">
							Endpoint
						</p>
						<code className="block text-sm text-zinc-200 bg-zinc-800/60 rounded-lg px-3 py-2 font-mono break-all">
							{server.endpoint}
						</code>
					</div>
				)}
				{server.tags && server.tags.length > 0 && (
					<div>
						<p className="text-xs text-zinc-400 uppercase tracking-wide mb-1.5">
							标签
						</p>
						<div className="flex flex-wrap gap-1.5">
							{server.tags.map((t) => (
								<span
									key={t}
									className="text-[11px] px-2 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/50 font-mono"
								>
									{t}
								</span>
							))}
						</div>
					</div>
				)}
				{server.tool_name_list && server.tool_name_list.length > 0 && (
					<div>
						<p className="text-xs text-zinc-400 uppercase tracking-wide mb-1.5">
							工具白名单
						</p>
						<div className="flex flex-wrap gap-1.5">
							{server.tool_name_list.map((t) => (
								<span
									key={t}
									className="text-[11px] px-2 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/50 font-mono"
								>
									{t}
								</span>
							))}
						</div>
					</div>
				)}

				{headerMeta && (
					<div className="border-t border-zinc-800 pt-4 space-y-4">
						<p className="text-xs text-zinc-400 uppercase tracking-wide">
							连接配置
						</p>
						{Object.entries(headerMeta)
							.filter(([_, meta]) => meta.kind !== "auto")
							.map(([key, meta]) => (
								<div key={key} className="space-y-1.5">
									<Label className="text-xs text-zinc-400">
										{meta.label || key}
										{meta.kind === "required" && (
											<span className="text-red-400 ml-1">*</span>
										)}
									</Label>
									{meta.description && (
										<p className="text-[10px] text-zinc-500">
											{meta.description}
										</p>
									)}
									<Input
										value={headerValues?.[key] || ""}
										onChange={(e) => onHeaderValueChange?.(key, e.target.value)}
										placeholder={
											meta.placeholder || `输入 ${meta.label || key}`
										}
										className={inp}
									/>
								</div>
							))}
						{Object.entries(headerMeta).filter(
							([_, meta]) => meta.kind === "auto",
						).length > 0 && (
							<div className="space-y-2 bg-zinc-800/30 rounded-lg px-3 py-2.5">
								<p className="text-[10px] text-zinc-500 uppercase tracking-wide">
									自动配置（无需填写）
								</p>
								{Object.entries(headerMeta)
									.filter(([_, meta]) => meta.kind === "auto")
									.map(([key, meta]) => (
										<div key={key} className="flex items-center gap-2 text-xs">
											<span className="font-mono text-zinc-500">{key}</span>
											<span className="text-zinc-600">:</span>
											<span className="text-zinc-400">
												{meta.value || meta.default || "-"}
											</span>
										</div>
									))}
							</div>
						)}
					</div>
				)}
			</div>
		);
	}

	// mode="edit" — render form fields (Sheet/footer/buttons managed by parent)
	if (!form || !onFormChange) return null;
	const f = form as Record<string, string>;
	const official = !!isOfficial;
	const arr = (k: string): string[] =>
		((form as Record<string, unknown>)[k] as string[]) || [""];

	return (
		<div className="flex-1 overflow-y-auto px-6 py-4 space-y-4">
			<Field label="名称">
				<Input
					value={f.name}
					onChange={(e) => onFormChange("name", e.target.value)}
					placeholder="my-mcp-server"
					className={inp}
					disabled={official}
				/>
			</Field>

			<div className="space-y-1.5">
				<div className="flex items-center justify-between">
					<Label className="text-xs text-zinc-400 uppercase tracking-wide">
						描述
					</Label>
					{!official && (
						<div className="flex items-center gap-1">
							<button
								type="button"
								onClick={() => onDescPreviewChange?.(false)}
								className={`text-[11px] px-2 py-0.5 rounded transition-colors ${!descPreview ? "bg-violet-600 text-white" : "text-zinc-500 hover:text-zinc-300"}`}
							>
								编辑
							</button>
							<button
								type="button"
								onClick={() => onDescPreviewChange?.(true)}
								className={`text-[11px] px-2 py-0.5 rounded transition-colors ${descPreview ? "bg-violet-600 text-white" : "text-zinc-500 hover:text-zinc-300"}`}
							>
								预览
							</button>
						</div>
					)}
				</div>
				{official || (descPreview && f.description) ? (
					<div className="min-h-[100px] bg-zinc-800/60 rounded-lg px-3 py-2.5">
						<div className="text-xs text-zinc-300 leading-relaxed">
							<SimpleMarkdown text={f.description} />
						</div>
					</div>
				) : (
					<Textarea
						value={f.description}
						onChange={(e) => onFormChange("description", e.target.value)}
						placeholder="支持 Markdown 语法\n例如：**粗体**、`代码`\n- 列表项"
						className="min-h-[100px] bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500 text-sm"
						disabled={official}
					/>
				)}
			</div>

			<Field label="图标 URL">
				<div className="flex gap-2 items-center">
					{f.icon ? (
						<img
							src={f.icon}
							alt=""
							className="w-7 h-7 rounded-lg shrink-0 object-contain"
							onError={(e) => {
								(e.target as HTMLImageElement).style.display = "none";
							}}
						/>
					) : null}
					{official ? (
						<div className="flex-1 text-xs text-zinc-500 bg-zinc-800/60 rounded-lg px-3 py-2 truncate">
							{f.icon || "无"}
						</div>
					) : (
						<Input
							value={f.icon}
							onChange={(e) => onFormChange("icon", e.target.value)}
							placeholder="https://example.com/logo.png"
							className={inp}
						/>
					)}
				</div>
			</Field>

			<Field label="标签">
				{official ? (
					<div className="flex flex-wrap gap-1.5">
						{f.tags
							? f.tags
									.split(",")
									.map((t: string) => t.trim())
									.filter(Boolean)
									.map((t: string) => (
										<span
											key={t}
											className="text-[11px] px-2 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/50 font-mono"
										>
											{t}
										</span>
									))
							: null}
					</div>
				) : (
					<TagInput
						tags={
							f.tags
								? f.tags
										.split(",")
										.map((t) => t.trim())
										.filter(Boolean)
								: []
						}
						onChange={(tags) => onFormChange("tags", tags.join(", "))}
					/>
				)}
			</Field>

			<Field label="传输协议">
				{official ? (
					<div className="text-sm text-zinc-300 bg-zinc-800/60 rounded-lg px-3 py-2 font-mono">
						{f.transport}
					</div>
				) : (
					<div className="flex gap-1.5">
						{(["streamable", "sse"] as const).map((t) => (
							<button
								key={t}
								type="button"
								onClick={() => onFormChange("transport", t)}
								className={`flex-1 h-9 rounded-md text-sm font-mono border transition-colors cursor-pointer ${f.transport === t ? "bg-violet-600 border-violet-500 text-white" : "bg-zinc-800 border-zinc-700 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-300"}`}
							>
								{t}
							</button>
						))}
					</div>
				)}
			</Field>

			{f.transport === "stdio" ? (
				<>
					<Field label="命令">
						<Input
							value={f.command}
							onChange={(e) => onFormChange("command", e.target.value)}
							placeholder="npx / python / uvx"
							className={inp}
							disabled={official}
						/>
					</Field>
					<Field label="参数（空格分隔）">
						<Input
							value={f.args}
							onChange={(e) => onFormChange("args", e.target.value)}
							placeholder="-y @modelcontextprotocol/server-filesystem /path"
							className={inp}
							disabled={official}
						/>
					</Field>
					<Field label="工作目录（可选）">
						<Input
							value={f.cwd}
							onChange={(e) => onFormChange("cwd", e.target.value)}
							placeholder="/data/mcp"
							className={inp}
							disabled={official}
						/>
					</Field>
					{/* Env vars */}
					<div className="space-y-2">
						<div className="flex items-center justify-between">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								环境变量
							</Label>
							{!official && addEnvRow && (
								<button
									onClick={addEnvRow}
									className="text-[11px] text-violet-400 hover:text-violet-300 cursor-pointer"
								>
									+ 添加
								</button>
							)}
						</div>
						{arr("envKeys").map((_: string, i: number) => (
							<div key={i} className="flex gap-2">
								<Input
									value={arr("envKeys")[i]}
									onChange={(e) =>
										updateEnv?.(i, e.target.value, arr("envVals")[i])
									}
									placeholder="KEY"
									className={`${inp} flex-1 font-mono text-xs`}
									disabled={official}
								/>
								<Input
									value={arr("envVals")[i]}
									onChange={(e) =>
										updateEnv?.(i, arr("envKeys")[i], e.target.value)
									}
									placeholder="value"
									className={`${inp} flex-1 font-mono text-xs`}
									disabled={official}
								/>
							</div>
						))}
					</div>
				</>
			) : (
				<Field label="Endpoint">
					<div className="relative">
						<Input
							value={f.endpoint}
							onChange={(e) => onFormChange("endpoint", e.target.value)}
							placeholder="https://api.example.com/mcp"
							className={`${inp} ${official ? "blur-sm select-none" : ""}`}
							disabled={official}
							readOnly={official}
						/>
						{official && (
							<div className="absolute inset-0 flex items-center justify-center pointer-events-none">
								<span className="flex items-center gap-1.5 text-[11px] text-zinc-500 bg-zinc-800/80 px-2 py-1 rounded">
									<EyeOff className="w-3 h-3" />
									系统配置 · 不可见
								</span>
							</div>
						)}
					</div>
				</Field>
			)}

			{/* HTTP Headers */}
			<div className="space-y-2">
				<div className="flex items-center justify-between">
					<Label className="text-xs text-zinc-400 uppercase tracking-wide">
						HTTP Headers
					</Label>
					{!official && addHeaderRow && (
						<button
							onClick={addHeaderRow}
							className="text-[11px] text-violet-400 hover:text-violet-300 cursor-pointer"
						>
							+ 添加
						</button>
					)}
				</div>
				{arr("headerKeys").map((_: string, i: number) => (
					<div key={i} className="flex gap-2">
						{official ? (
							<div className="flex-1 text-xs font-mono text-zinc-400 bg-zinc-800/60 rounded-lg px-3 py-2 truncate">
								{arr("headerKeys")[i]}
							</div>
						) : (
							<Input
								value={arr("headerKeys")[i]}
								onChange={(e) =>
									updateHeader?.(i, e.target.value, arr("headerVals")[i])
								}
								placeholder="Header-Name"
								className={`${inp} flex-1 font-mono text-xs`}
							/>
						)}
						<Input
							value={arr("headerVals")[i]}
							onChange={(e) =>
								updateHeader?.(i, arr("headerKeys")[i], e.target.value)
							}
							placeholder="value"
							className={`${inp} flex-1 font-mono text-xs`}
						/>
					</div>
				))}
			</div>

			<Field label="工具白名单（留空=全部可用）">
				{toolsList && toolsList.length > 0 ? (
					<ToolAllowlist
						tools={toolsList}
						selected={
							f.toolList
								? f.toolList
										.split(",")
										.map((t) => t.trim())
										.filter(Boolean)
								: []
						}
						onChange={(sel) => onFormChange("toolList", sel.join(", "))}
					/>
				) : f.toolList?.trim() ? (
					<div className="flex flex-wrap gap-1.5">
						{f.toolList
							.split(",")
							.map((t: string) => t.trim())
							.filter(Boolean)
							.map((t: string) => (
								<span
									key={t}
									className="text-[11px] px-2 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/50 font-mono"
								>
									{t}
								</span>
							))}
					</div>
				) : (
					<p className="text-xs text-zinc-500">请先测试连接获取工具列表</p>
				)}
			</Field>

			<Field label="超时 (ms)">
				<Input
					type="number"
					min={0}
					value={f.timeoutMs}
					onChange={(e) => onFormChange("timeoutMs", +e.target.value)}
					className={inp}
				/>
			</Field>
		</div>
	);
}
