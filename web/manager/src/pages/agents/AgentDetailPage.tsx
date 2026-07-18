import { useEffect, useState, useRef, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
	voicebotApi,
	deviceApi,
	languageApi,
	availableResourcesApi,
	modelApi,
	mcpApi,
	knowledgeApi,
	type Device,
	type Language,
	type AvailableResources,
	type AIModel,
	type MCPServer,
	type VoicebotMCPServer,
	type KnowledgeBase,
	type KnowledgeDocument,
} from "@/lib/api";
import {
	McpServerIcon,
	McpServerDetail,
} from "@/pages/components/McpServerDetail";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
	Sheet,
	SheetContent,
	SheetHeader,
	SheetTitle,
} from "@/components/ui/sheet";
import {
	Search,
	CheckCircle2,
	BookOpen,
	Trash2,
	Upload,
	Link2,
	ChevronDown,
	ChevronRight,
	RotateCw,
	FileText,
	Clock,
	AlertCircle,
	Loader2,
	Brain,
} from "lucide-react";
import { Switch } from "@/components/ui/switch";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
} from "@/components/ui/select";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
	DialogFooter,
} from "@/components/ui/dialog";
import { ChevronLeft, Plus, Play, Send } from "lucide-react";
import QuickChat from "./QuickChat";

interface BotConfig {
	language: string;
	asr: {
		model_id: string;
		vad_mode: string;
		vad_threshold: number;
		vad_min_silence_ms: number;
		vad_speech_pad_ms: number;
	};
	tts: {
		model_id: string;
		voice_id: string;
		volume: number;
		rate: number;
		pitch: number;
	};
	llm: { model_id: string; soul_prompt: string; rules_prompt: string };
	audio: { sample_rate: number };
	memory: {
		memory_char_limit: number;
		user_char_limit: number;
		review_enabled: boolean;
	};
}

const DC: BotConfig = {
	language: "zh",
	asr: {
		model_id: "",
		vad_mode: "auto",
		vad_threshold: 0.5,
		vad_min_silence_ms: 500,
		vad_speech_pad_ms: 300,
	},
	tts: { model_id: "", voice_id: "", volume: 50, rate: 1.0, pitch: 1.0 },
	llm: { model_id: "", soul_prompt: "", rules_prompt: "" },
	audio: { sample_rate: 16000 },
	memory: {
		memory_char_limit: 2200,
		user_char_limit: 1375,
		review_enabled: true,
	},
};

const EMOTIONS = ["happy", "sad", "angry", "calm", "excited"] as const;
const EMOTION_LABELS: Record<string, string> = {
	happy: "开心",
	sad: "悲伤",
	angry: "生气",
	calm: "平静",
	excited: "兴奋",
};

const VAD_MODES = [
	{ value: "realtime", label: "实时模式", desc: "持续监听，需要 AEC 支持" },
	{ value: "auto", label: "自动模式", desc: "VAD 检测语音边界，自动停止" },
	{ value: "manual", label: "手动模式", desc: "客户端发送开始/停止信号" },
];

function parseCfg(json: string): BotConfig {
	try {
		const c = JSON.parse(json);
		if (c.asr?.model_id || c.llm?.model_id) {
			return {
				language: c.language || DC.language,
				asr: { ...DC.asr, ...c.asr },
				tts: { ...DC.tts, ...c.tts },
				llm: { ...DC.llm, ...c.llm },
				audio: { ...DC.audio, ...c.audio },
				memory: { ...DC.memory, ...c.memory },
			};
		}
		return {
			language: DC.language,
			asr: {
				model_id: "",
				vad_mode: c.audio?.in_pipe?.enable_vad === false ? "manual" : "auto",
				vad_threshold: c.audio?.in_pipe?.vad_threshold ?? DC.asr.vad_threshold,
				vad_min_silence_ms:
					c.audio?.in_pipe?.vad_min_silence_ms ?? DC.asr.vad_min_silence_ms,
				vad_speech_pad_ms:
					c.audio?.in_pipe?.vad_speech_pad_ms ?? DC.asr.vad_speech_pad_ms,
			},
			tts: {
				model_id: "",
				voice_id: c.provider?.tts?.aliyun?.voice ?? DC.tts.voice_id,
				volume: c.provider?.tts?.aliyun?.volume ?? DC.tts.volume,
				rate: c.provider?.tts?.aliyun?.rate ?? DC.tts.rate,
				pitch: c.provider?.tts?.aliyun?.pitch ?? DC.tts.pitch,
			},
			llm: { model_id: "", soul_prompt: "", rules_prompt: "" },
			audio: {
				sample_rate: c.audio?.in_pipe?.sample_rate ?? DC.audio.sample_rate,
			},
			memory: { ...DC.memory, ...c.memory },
		};
	} catch {
		return structuredClone(DC);
	}
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

export default function AgentDetailPage() {
	const { id } = useParams<{ id: string }>();
	const navigate = useNavigate();

	const [name, setName] = useState("");
	const [cfg, setCfg] = useState<BotConfig>(structuredClone(DC));
	const [devices, setDevices] = useState<Device[]>([]);
	const [loading, setLoading] = useState(true);
	const [saving, setSaving] = useState(false);
	const [saveStatus, setSaveStatus] = useState<"idle" | "ok" | "err">("idle");
	const [saveErr, setSaveErr] = useState("");

	const [languages, setLanguages] = useState<Language[]>([]);
	const [resources, setResources] = useState<AvailableResources | null>(null);
	const [llmModels, setLlmModels] = useState<AIModel[]>([]);
	const [voiceNameFilter, setVoiceNameFilter] = useState("");
	const [voiceGenderFilter, setVoiceGenderFilter] = useState("");
	const [voiceTagFilter, setVoiceTagFilter] = useState("");

	// MCP servers & bindings
	const [mcpServers, setMcpServers] = useState<MCPServer[]>([]);
	const [mcpBindings, setMcpBindings] = useState<VoicebotMCPServer[]>([]);
	const [devOpen, setDevOpen] = useState(false);
	const [newDevId, setNewDevId] = useState("");
	const [newDevName, setNewDevName] = useState("");
	const [deviceSearch, setDeviceSearch] = useState("");
	const [mcpSearch, setMcpSearch] = useState("");
	const [mcpFilter, setMcpFilter] = useState<"all" | "bound" | "unbound">(
		"all",
	);
	const [mcpPage, setMcpPage] = useState(1);
	const [mcpPageSize, setMcpPageSize] = useState(12);
	const [detailMcp, setDetailMcp] = useState<MCPServer | null>(null);
	const [devAdding, setDevAdding] = useState(false);
	const [devErr, setDevErr] = useState("");
	const [channelDeviceID, setChannelDeviceID] = useState<string | null>(null);
	const [telegramToken, setTelegramToken] = useState("");
	const [telegramSaving, setTelegramSaving] = useState(false);
	const [telegramErr, setTelegramErr] = useState("");

	// Knowledge base state
	const [kbs, setKbs] = useState<KnowledgeBase[]>([]);
	const [loadingKBs, setLoadingKBs] = useState(false);
	const [expandedKB, setExpandedKB] = useState<string | null>(null);
	const [docs, setDocs] = useState<KnowledgeDocument[]>([]);
	const [loadingDocs, setLoadingDocs] = useState(false);
	const [uploadOpen, setUploadOpen] = useState(false);

	const [uploadKBId, setUploadKBId] = useState("");
	const [urlOpen, setUrlOpen] = useState(false);
	const [urlKBId, setUrlKBId] = useState("");
	const [urlInput, setUrlInput] = useState("");
	const [fileDragging, setFileDragging] = useState(false);
	const [kbSearchText, setKbSearchText] = useState("");
	const [kbSearchResults, setKbSearchResults] = useState<
		Array<{
			chunk_id: string;
			content: string;
			score: number;
			document_name: string;
		}>
	>([]);
	const [kbSearchKBId, setKbSearchKBId] = useState("");
	const [kbSearching, setKbSearching] = useState(false);
	const fileRef = useRef<HTMLInputElement>(null);

	// Bind KB dialog
	const [bindKBOpen, setBindKBOpen] = useState(false);
	const [bindKBList, setBindKBList] = useState<KnowledgeBase[]>([]);
	const [bindKBSearch, setBindKBSearch] = useState("");
	const [bindKBLoading, setBindKBLoading] = useState(false);
	const [bindKBBinding, setBindKBBinding] = useState<Set<string>>(new Set());

	const containerRef = useRef<HTMLDivElement>(null);
	const [splitPct, setSplitPct] = useState(50);
	const dragging = useRef(false);

	useEffect(() => {
		if (!id) return;
		(async () => {
			const [b, d, langs, llm, mcpSv, mcpBd] = await Promise.all([
				voicebotApi.get(id),
				deviceApi.list(id),
				languageApi.list(),
				modelApi.list(),
				mcpApi.servers.list({
					page: 1,
					page_size: 9999,
				}) /* ponytail: fetches all for binding visibility, paginated client-side */,
				mcpApi.bindings.list(id),
			]);
			const parsed = parseCfg(b.data.config_json);
			setName(b.data.name);
			setCfg(parsed);
			setDevices(d.data);
			setLanguages(langs.data);
			setLlmModels(llm.data);
			setMcpServers(mcpSv.data.data);
			setMcpBindings(mcpBd.data.filter((m) => m.bound));

			const res = await availableResourcesApi.list(
				parsed.language || undefined,
			);
			setResources(res.data);

			setLoading(false);
		})();
	}, [id]);

	// Re-fetch resources when language changes (skip first load)
	useEffect(() => {
		if (id && resources) {
			availableResourcesApi
				.list(cfg.language || undefined)
				.then((r) => setResources(r.data));
		}
	}, [cfg.language]);

	const setAsr = (p: Partial<BotConfig["asr"]>) =>
		setCfg((c) => ({ ...c, asr: { ...c.asr, ...p } }));
	const setTts = (p: Partial<BotConfig["tts"]>) =>
		setCfg((c) => ({ ...c, tts: { ...c.tts, ...p } }));
	const setLlm = (p: Partial<BotConfig["llm"]>) =>
		setCfg((c) => ({ ...c, llm: { ...c.llm, ...p } }));
	const setMem = (p: Partial<BotConfig["memory"]>) =>
		setCfg((c) => ({ ...c, memory: { ...c.memory, ...p } }));

	const onResizeStart = useCallback(
		(e: React.MouseEvent) => {
			e.preventDefault();
			dragging.current = true;
			const startX = e.clientX;
			const startPct = splitPct;
			const onMove = (ev: MouseEvent) => {
				if (!dragging.current || !containerRef.current) return;
				const rect = containerRef.current.getBoundingClientRect();
				let pct = startPct + ((ev.clientX - startX) / rect.width) * 100;
				pct = Math.max(30, Math.min(70, pct));
				setSplitPct(pct);
			};
			const onUp = () => {
				dragging.current = false;
				document.removeEventListener("mousemove", onMove);
				document.removeEventListener("mouseup", onUp);
			};
			document.addEventListener("mousemove", onMove);
			document.addEventListener("mouseup", onUp);
		},
		[splitPct],
	);

	const asrModels = resources?.asr ?? [];
	const llmTypes = new Set(["text", "vision", "multimodal"]);

	const allVoiceTags = [
		...new Set((resources?.voices ?? []).flatMap((v) => v.tags || [])),
	].sort();
	const filteredVoices = (resources?.voices ?? []).filter((v) => {
		if (
			voiceNameFilter &&
			!v.name.toLowerCase().includes(voiceNameFilter.toLowerCase())
		)
			return false;
		if (voiceGenderFilter && v.gender !== voiceGenderFilter) return false;
		if (voiceTagFilter && !(v.tags || []).includes(voiceTagFilter))
			return false;
		return true;
	});

	const handleSave = async () => {
		if (!id) return;
		setSaving(true);
		setSaveStatus("idle");
		setSaveErr("");
		try {
			await voicebotApi.update(id, name, JSON.stringify(cfg));
			setSaveStatus("ok");
			setTimeout(() => setSaveStatus("idle"), 2000);
		} catch (e: unknown) {
			setSaveErr(
				(e as { response?: { data?: { error?: string } } })?.response?.data
					?.error ?? "保存失败",
			);
			setSaveStatus("err");
		} finally {
			setSaving(false);
		}
	};

	const boundIds = new Set(mcpBindings.filter((b) => b.bound).map((b) => b.id));

	const allFiltered = mcpServers.filter((s) => {
		if (mcpFilter === "bound" && !boundIds.has(s.id)) return false;
		if (mcpFilter === "unbound" && boundIds.has(s.id)) return false;
		if (mcpSearch && !s.name.toLowerCase().includes(mcpSearch.toLowerCase()))
			return false;
		return true;
	});

	const mcpTotalPages = Math.max(
		1,
		Math.ceil(allFiltered.length / mcpPageSize),
	);
	const safePage = Math.min(mcpPage, mcpTotalPages);
	const filteredMcps = allFiltered.slice(
		(safePage - 1) * mcpPageSize,
		safePage * mcpPageSize,
	);

	const handleUnbindMcp = async (serverId: string) => {
		if (!id) return;
		await mcpApi.bindings.unbind(id, serverId);
		setMcpBindings((prev) => prev.filter((b) => b.id !== serverId));
	};

	const handleAddMcpBinding = async (serverId: string) => {
		if (!id) return;
		await mcpApi.bindings.bind(id, serverId);
		const { data } = await mcpApi.bindings.list(id);
		setMcpBindings(data.filter((m) => m.bound));
	};

	const handleAddDevice = async () => {
		if (!id || !newDevId.trim()) return;
		setDevAdding(true);
		setDevErr("");
		try {
			await deviceApi.create(id, newDevId.trim(), newDevName.trim());
			const { data } = await deviceApi.list(id);
			setDevices(data);
			setDevOpen(false);
			setNewDevId("");
			setNewDevName("");
		} catch (e: unknown) {
			setDevErr(
				(e as { response?: { data?: { error?: string } } })?.response?.data
					?.error ?? "添加失败",
			);
		} finally {
			setDevAdding(false);
		}
	};

	const handleDeleteDevice = async (devId: string) => {
		if (!id || !confirm(`确认删除设备 ${devId}？`)) return;
		await deviceApi.remove(id, devId);
		setDevices((prev) => prev.filter((d) => d.id !== devId));
	};

	const filteredDevices = devices.filter((device) => {
		const query = deviceSearch.trim().toLowerCase();
		return (
			query === "" ||
			device.id.toLowerCase().includes(query) ||
			device.name.toLowerCase().includes(query)
		);
	});

	const updateDevice = (device: Device) => {
		setDevices((prev) => prev.map((d) => (d.id === device.id ? device : d)));
	};

	const openChannelSettings = (deviceID: string) => {
		setChannelDeviceID((current) => (current === deviceID ? null : deviceID));
		setTelegramToken("");
		setTelegramErr("");
	};

	const handleSaveTelegram = async (deviceID: string) => {
		if (!id || !telegramToken.trim()) return;
		setTelegramSaving(true);
		setTelegramErr("");
		try {
			const { data } = await deviceApi.setTelegram(
				id,
				deviceID,
				telegramToken.trim(),
			);
			updateDevice(data);
			setTelegramToken("");
		} catch (e: unknown) {
			setTelegramErr(
				(e as { response?: { data?: { error?: string } } })?.response?.data
					?.error ?? "保存 Telegram 配置失败",
			);
		} finally {
			setTelegramSaving(false);
		}
	};

	const handleClearTelegram = async (deviceID: string) => {
		if (!id || !confirm("确认断开此设备的 Telegram Bot？")) return;
		setTelegramSaving(true);
		setTelegramErr("");
		try {
			const { data } = await deviceApi.clearTelegram(id, deviceID);
			updateDevice(data);
		} catch (e: unknown) {
			setTelegramErr(
				(e as { response?: { data?: { error?: string } } })?.response?.data
					?.error ?? "断开 Telegram Bot 失败",
			);
		} finally {
			setTelegramSaving(false);
		}
	};

	// ── Knowledge base handlers ──
	const loadKBs = async () => {
		if (!id) return;
		setLoadingKBs(true);
		try {
			const { data } = await knowledgeApi.listBoundKBs(id);
			setKbs(data.knowledge_bases || []);
		} catch {
			setKbs([]);
		} finally {
			setLoadingKBs(false);
		}
	};

	const loadDocs = async (kbId: string) => {
		setLoadingDocs(true);
		try {
			const { data } = await knowledgeApi.listDocs(kbId);
			setDocs(data.documents || []);
		} catch {
			setDocs([]);
		} finally {
			setLoadingDocs(false);
		}
	};

	const toggleKB = async (kbId: string) => {
		if (expandedKB === kbId) {
			setExpandedKB(null);
			setDocs([]);
			return;
		}
		setExpandedKB(kbId);
		loadDocs(kbId);
	};

	useEffect(() => {
		if (!expandedKB) return;
		const hasProcessing = docs.some(
			(d) => d.status !== "ready" && d.status !== "error",
		);
		if (!hasProcessing) return;
		const timer = setInterval(() => loadDocs(expandedKB), 3000);
		return () => clearInterval(timer);
	}, [expandedKB, docs]);

	const handleDeleteKB = async (kbId: string) => {
		if (!confirm("确认解绑该知识库？")) return;
		try {
			await knowledgeApi.unbindKB(id!, kbId);
			setKbs((prev) => prev.filter((k) => k.id !== kbId));
			if (expandedKB === kbId) {
				setExpandedKB(null);
				setDocs([]);
			}
		} catch {
			/* silently fail */
		}
	};

	const handleBindKB = async () => {
		setBindKBOpen(true);
		setBindKBLoading(true);
		try {
			const { data } = await knowledgeApi.listAllKBs();
			const boundIds = new Set(kbs.map((k) => k.id));
			setBindKBList(
				(data.knowledge_bases || []).filter((k) => !boundIds.has(k.id)),
			);
		} catch {
			setBindKBList([]);
		} finally {
			setBindKBLoading(false);
		}
	};

	const doBindKB = async (kbId: string) => {
		if (!id) return;
		setBindKBBinding((p) => new Set(p).add(kbId));
		try {
			await knowledgeApi.bindKB(id, kbId);
			setBindKBList((prev) => prev.filter((k) => k.id !== kbId));
			loadKBs();
		} catch {
			/* silently fail */
		} finally {
			setBindKBBinding((p) => {
				const n = new Set(p);
				n.delete(kbId);
				return n;
			});
		}
	};

	const handleUpload = async () => {
		const file = fileRef.current?.files?.[0];
		if (!file || !uploadKBId) return;
		try {
			await knowledgeApi.uploadDoc(uploadKBId, file);
			setUploadOpen(false);
			loadDocs(uploadKBId);
			if (fileRef.current) fileRef.current.value = "";
		} catch {
			/* silently fail */
		}
	};

	const handleImportURL = async () => {
		if (!urlInput.trim() || !urlKBId) return;
		try {
			await knowledgeApi.ingestURL(urlKBId, urlInput);
			setUrlOpen(false);
			setUrlInput("");
			loadDocs(urlKBId);
		} catch {
			/* silently fail */
		}
	};

	const handleDeleteDoc = async (docId: string) => {
		if (!confirm("确认删除该文档？")) return;
		try {
			await knowledgeApi.deleteDoc(docId);
			setDocs((prev) => prev.filter((d) => d.id !== docId));
		} catch {
			/* silently fail */
		}
	};

	const handleRetryDoc = async (docId: string, kbId: string) => {
		try {
			await knowledgeApi.retryDoc(docId);
			loadDocs(kbId);
		} catch {
			/* silently fail */
		}
	};

	const handleKBSearch = async (kbId: string) => {
		if (!kbSearchText.trim()) return;
		setKbSearchKBId(kbId);
		setKbSearching(true);
		try {
			const { data } = await knowledgeApi.searchKB(kbId, kbSearchText, 5);
			setKbSearchResults(Array.isArray(data) ? data : []);
		} catch {
			setKbSearchResults([]);
		} finally {
			setKbSearching(false);
		}
	};

	const DOC_STATUS_MAP: Record<
		string,
		{ label: string; icon: React.ElementType; cls: string }
	> = {
		ready: { label: "已索引", icon: CheckCircle2, cls: "text-emerald-400" },
		pending: { label: "等待中", icon: Clock, cls: "text-zinc-400" },
		parsing: {
			label: "解析中",
			icon: Loader2,
			cls: "text-amber-400 animate-spin",
		},
		chunking: {
			label: "分块中",
			icon: Loader2,
			cls: "text-amber-400 animate-spin",
		},
		embedding: {
			label: "向量化",
			icon: Loader2,
			cls: "text-amber-400 animate-spin",
		},
		storing: {
			label: "存储中",
			icon: Loader2,
			cls: "text-amber-400 animate-spin",
		},
		error: { label: "失败", icon: AlertCircle, cls: "text-red-400" },
	};

	const asr = cfg.asr;
	const tts = cfg.tts;
	const llm = cfg.llm;
	const mem = cfg.memory;

	if (loading)
		return (
			<div className="flex items-center justify-center py-32">
				<div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
			</div>
		);

	return (
		<div className="min-h-full">
			{/* Header */}
			<div className="border-b border-zinc-800/80 px-8 py-4 flex items-center gap-3">
				<button
					onClick={() => navigate("/agents")}
					className="text-zinc-500 hover:text-zinc-300 transition-colors"
				>
					<ChevronLeft className="w-4 h-4" />
				</button>
				<div className="h-4 w-px bg-zinc-800" />
				<input
					value={name}
					onChange={(e) => setName(e.target.value)}
					className="bg-transparent text-white font-medium text-sm focus:outline-none border-b border-transparent focus:border-zinc-600 py-0.5 min-w-0 max-w-xs"
				/>
				<span className="text-xs text-zinc-600 font-mono hidden sm:block">
					{id}
				</span>
				<div className="ml-auto flex items-center gap-2">
					{saveStatus === "err" && (
						<span className="text-xs text-red-400">{saveErr}</span>
					)}
					<Button
						onClick={handleSave}
						disabled={saving}
						className={
							saveStatus === "ok"
								? "bg-emerald-600 hover:bg-emerald-600 text-white h-8 px-4 text-sm"
								: "bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-sm"
						}
					>
						{saving ? "保存中..." : saveStatus === "ok" ? "已保存 ✓" : "保存"}
					</Button>
				</div>
			</div>

			<div ref={containerRef} className="flex h-[calc(100vh-61px)]">
				<div
					className="overflow-y-auto px-8 py-6 min-w-0"
					style={{ width: `${splitPct}%` }}
				>
					<Tabs
						defaultValue="basic"
						onValueChange={(v) => {
							if (v === "data") {
								loadKBs();
							}
						}}
					>
						<TabsList className="bg-zinc-900 border border-zinc-800 mb-0 h-auto flex-wrap gap-0.5 p-1">
							{(
								["基本", "语音识别", "语音合成", "数据", "MCP", "设备"] as const
							).map((t, i) => (
								<TabsTrigger
									key={t}
									value={["basic", "asr", "tts", "data", "mcp", "devices"][i]}
									className="data-[state=active]:bg-zinc-700 data-[state=active]:text-white text-zinc-400 text-sm h-8"
								>
									{t}
								</TabsTrigger>
							))}
						</TabsList>

						{/* ── 基本 ── */}
						<TabsContent value="basic" className="space-y-5 pt-1">
							<Field label="智能体名称">
								<Input
									value={name}
									onChange={(e) => setName(e.target.value)}
									className={inp}
								/>
							</Field>
							<Field label="身份设定 (SOUL)">
								<textarea
									value={llm.soul_prompt}
									onChange={(e) => setLlm({ soul_prompt: e.target.value })}
									placeholder="智能体的身份、人格、交流风格... 留空则使用默认"
									className="bg-zinc-800 border border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500 rounded-lg px-3 py-2 text-sm w-full resize-none h-20"
								/>
							</Field>
							<Field label="行为规则 (RULES)">
								<textarea
									value={llm.rules_prompt}
									onChange={(e) => setLlm({ rules_prompt: e.target.value })}
									placeholder="工具使用规则、对话约束... 留空则使用默认"
									className="bg-zinc-800 border border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500 rounded-lg px-3 py-2 text-sm w-full resize-none h-20"
								/>
							</Field>
							<div className="flex flex-wrap gap-4">
								<div className="shrink-0">
									<Field label="语言">
										<Select
											value={cfg.language}
											onValueChange={(v) =>
												setCfg((c) => ({ ...c, language: v ?? DC.language }))
											}
										>
											<SelectTrigger>
												<span className="text-left flex-1 truncate">
													{languages.find((l) => l.code === cfg.language)
														?.name || cfg.language}
												</span>
											</SelectTrigger>
											<SelectContent className="max-h-60">
												{languages.map((l) => (
													<SelectItem key={l.code} value={l.code}>
														{l.name} ({l.code})
													</SelectItem>
												))}
											</SelectContent>
										</Select>
									</Field>
								</div>
								<div className="shrink-0">
									<Field label="聊天模型">
										<Select
											value={llm.model_id}
											onValueChange={(v) => setLlm({ model_id: v ?? "" })}
										>
											<SelectTrigger>
												<span className="text-left flex-1 truncate">
													{llmModels
														.filter((m) => llmTypes.has(m.type))
														.find((m) => m.id === llm.model_id)?.name || (
														<span className="text-zinc-500">选择模型</span>
													)}
												</span>
											</SelectTrigger>
											<SelectContent>
												{llmModels
													.filter((m) => llmTypes.has(m.type))
													.map((m) => (
														<SelectItem key={m.id} value={m.id}>
															<span>{m.name}</span>
															<span className="text-zinc-500 ml-2 text-xs">
																{m.provider?.name}
															</span>
														</SelectItem>
													))}
												{llmModels.filter((m) => llmTypes.has(m.type))
													.length === 0 && (
													<div className="px-2 py-3 text-xs text-zinc-500 text-center">
														暂无可用的聊天模型
													</div>
												)}
											</SelectContent>
										</Select>
									</Field>
								</div>
							</div>
						</TabsContent>

						{/* ── ASR ── */}
						<TabsContent value="asr" className="space-y-5 pt-1">
							<Field label="语音识别模型">
								<Select
									value={asr.model_id}
									onValueChange={(v) => setAsr({ model_id: v ?? "" })}
								>
									<SelectTrigger>
										<span className="text-left flex-1 truncate">
											{asrModels.find((m) => m.id === asr.model_id)?.name || (
												<span className="text-zinc-500">选择语音识别模型</span>
											)}
										</span>
									</SelectTrigger>
									<SelectContent>
										{asrModels.map((m) => (
											<SelectItem key={m.id} value={m.id}>
												{m.name}
											</SelectItem>
										))}
										{asrModels.length === 0 && (
											<div className="px-2 py-3 text-xs text-zinc-500 text-center">
												暂无可用的语音识别模型
											</div>
										)}
									</SelectContent>
								</Select>
							</Field>

							<div className="border-t border-zinc-800 pt-4">
								<Label className="text-xs text-zinc-400 uppercase tracking-wide mb-3 block">
									监听模式
								</Label>
								<div className="space-y-2">
									{VAD_MODES.map((mode) => (
										<label
											key={mode.value}
											className={`flex items-start gap-3 p-3 rounded-xl border cursor-pointer transition-colors
                      ${asr.vad_mode === mode.value ? "border-violet-500 bg-violet-500/10" : "border-zinc-800 bg-zinc-900 hover:border-zinc-700"}`}
											onClick={() => setAsr({ vad_mode: mode.value })}
										>
											<div
												className={`w-4 h-4 mt-0.5 rounded-full border-2 shrink-0 flex items-center justify-center
                      ${asr.vad_mode === mode.value ? "border-violet-500" : "border-zinc-600"}`}
											>
												{asr.vad_mode === mode.value && (
													<div className="w-2 h-2 rounded-full bg-violet-500" />
												)}
											</div>
											<div>
												<p className="text-sm text-white font-medium">
													{mode.label}
												</p>
												<p className="text-xs text-zinc-500 mt-0.5">
													{mode.desc}
												</p>
											</div>
										</label>
									))}
								</div>
							</div>

							{asr.vad_mode !== "manual" && (
								<div className="border-t border-zinc-800 pt-4 space-y-4">
									<Field
										label={`活动检测阈值 (${asr.vad_threshold.toFixed(2)})`}
									>
										<input
											type="range"
											min={0}
											max={1}
											step={0.05}
											value={asr.vad_threshold}
											onChange={(e) =>
												setAsr({ vad_threshold: +e.target.value })
											}
											className="w-full accent-violet-500"
										/>
										<div className="flex justify-between text-[10px] text-zinc-600 -mt-1">
											<span>0 (低灵敏度)</span>
											<span>1 (高灵敏度)</span>
										</div>
									</Field>
									{asr.vad_mode === "auto" && (
										<div className="flex flex-wrap gap-4">
											<Field label="最小静音时长 (ms)">
												<Input
													type="number"
													min={0}
													value={asr.vad_min_silence_ms}
													onChange={(e) =>
														setAsr({ vad_min_silence_ms: +e.target.value })
													}
													className={inp}
												/>
											</Field>
											<Field label="语音填充 (ms)">
												<Input
													type="number"
													min={0}
													value={asr.vad_speech_pad_ms}
													onChange={(e) =>
														setAsr({ vad_speech_pad_ms: +e.target.value })
													}
													className={inp}
												/>
											</Field>
										</div>
									)}
								</div>
							)}
						</TabsContent>

						{/* ── TTS ── */}
						<TabsContent value="tts" className="space-y-5 pt-1">
							<div>
								<div className="flex items-center justify-between mb-3">
									<Label className="text-xs text-zinc-400 uppercase tracking-wide">
										音色列表
										<span className="text-zinc-600 ml-1 font-normal normal-case">
											({resources?.voices.length ?? 0})
										</span>
									</Label>
									{tts.voice_id &&
										resources?.voices?.some((v) => v.id === tts.voice_id) && (
											<span className="text-[11px] text-violet-400">
												当前选中:{" "}
												{
													resources?.voices.find((v) => v.id === tts.voice_id)
														?.name
												}
											</span>
										)}
								</div>

								{resources?.voices && resources.voices.length > 0 && (
									<>
										<div className="flex gap-2 mb-3">
											<input
												type="text"
												value={voiceNameFilter}
												onChange={(e) => setVoiceNameFilter(e.target.value)}
												placeholder="搜索音色..."
												className="bg-zinc-800 border border-zinc-700 text-white placeholder:text-zinc-500 rounded-lg px-2.5 py-1.5 text-xs flex-1 min-w-0 focus:outline-none focus:border-violet-500 transition-colors"
											/>
											<select
												value={voiceGenderFilter}
												onChange={(e) => setVoiceGenderFilter(e.target.value)}
												className="bg-zinc-800 border border-zinc-700 text-white rounded-lg px-2 py-1.5 text-xs focus:outline-none focus:border-violet-500"
											>
												<option value="">全部性别</option>
												<option value="female">女声</option>
												<option value="male">男声</option>
												<option value="neutral">中性</option>
											</select>
											<select
												value={voiceTagFilter}
												onChange={(e) => setVoiceTagFilter(e.target.value)}
												className="bg-zinc-800 border border-zinc-700 text-white rounded-lg px-2 py-1.5 text-xs focus:outline-none focus:border-violet-500"
											>
												<option value="">全部标签</option>
												{allVoiceTags.map((t) => (
													<option key={t} value={t}>
														{t}
													</option>
												))}
											</select>
										</div>

										{filteredVoices.length === 0 ? (
											<div className="text-center py-10 border border-dashed border-zinc-800 rounded-xl">
												<p className="text-zinc-500 text-sm">没有匹配的音色</p>
											</div>
										) : (
											<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
												{filteredVoices.map((v) => {
													const isSelected = tts.voice_id === v.id;
													const emoMap = v.emotions as Record<
														string,
														unknown
													> | null;
													return (
														<div
															key={v.id}
															className={`relative rounded-xl border p-4 cursor-pointer transition-all
                            ${isSelected ? "border-violet-500 bg-violet-500/10 shadow-sm shadow-violet-500/20" : "border-zinc-800 bg-zinc-900 hover:border-zinc-700"}`}
															onClick={() => setTts({ voice_id: v.id })}
														>
															<div className="flex items-start justify-between">
																<div className="min-w-0">
																	<p className="text-sm font-medium text-white truncate">
																		{v.name}
																	</p>
																</div>
																{v.gender && (
																	<span
																		className={`text-[10px] px-1.5 py-0.5 rounded shrink-0
                                ${
																	v.gender === "female"
																		? "bg-pink-500/20 text-pink-300"
																		: v.gender === "male"
																			? "bg-blue-500/20 text-blue-300"
																			: "bg-zinc-700 text-zinc-400"
																}`}
																	>
																		{v.gender === "female"
																			? "女声"
																			: v.gender === "male"
																				? "男声"
																				: "中性"}
																	</span>
																)}
															</div>
															<div className="flex flex-wrap gap-1 mt-2">
																{EMOTIONS.map((em) => {
																	const supported = emoMap
																		? em in emoMap
																		: false;
																	return (
																		<span
																			key={em}
																			className={`text-[10px] px-1.5 py-0.5 rounded border
                                    ${
																			supported
																				? "bg-zinc-800 border-zinc-700 text-zinc-300"
																				: "bg-transparent border-zinc-800 text-zinc-700"
																		}`}
																		>
																			{EMOTION_LABELS[em]}
																		</span>
																	);
																})}
															</div>
															{v.preview_url && (
																<button
																	onClick={(e) => {
																		e.stopPropagation();
																		new Audio(v.preview_url).play();
																	}}
																	className="mt-2 text-[11px] text-violet-400 hover:text-violet-300 flex items-center gap-1"
																>
																	<Play className="w-3 h-3" />
																	试听
																</button>
															)}
															{isSelected && (
																<div className="absolute bottom-2 right-2 w-5 h-5 rounded-full bg-violet-600 flex items-center justify-center">
																	<svg
																		className="w-3 h-3 text-white"
																		fill="none"
																		viewBox="0 0 24 24"
																		stroke="currentColor"
																	>
																		<path
																			strokeLinecap="round"
																			strokeLinejoin="round"
																			strokeWidth={3}
																			d="M5 13l4 4L19 7"
																		/>
																	</svg>
																</div>
															)}
														</div>
													);
												})}
											</div>
										)}
									</>
								)}

								{(!resources?.voices || resources.voices.length === 0) && (
									<div className="text-center py-10 border border-dashed border-zinc-800 rounded-xl">
										<p className="text-zinc-500 text-sm">
											当前语言下没有可用的音色
										</p>
									</div>
								)}
							</div>

							{tts.voice_id && (
								<div className="border-t border-zinc-800 pt-4 space-y-4">
									<Label className="text-xs text-zinc-400 uppercase tracking-wide block">
										合成参数
									</Label>
									<div className="grid grid-cols-3 gap-4">
										<Field label={`语速 (${tts.rate.toFixed(1)})`}>
											<input
												type="range"
												min={0.5}
												max={2}
												step={0.1}
												value={tts.rate}
												onChange={(e) => setTts({ rate: +e.target.value })}
												className="w-full accent-violet-500"
											/>
										</Field>
										<Field label={`音量 (${tts.volume})`}>
											<input
												type="range"
												min={0}
												max={100}
												value={tts.volume}
												onChange={(e) => setTts({ volume: +e.target.value })}
												className="w-full accent-violet-500"
											/>
										</Field>
										<Field label={`语调 (${tts.pitch.toFixed(1)})`}>
											<input
												type="range"
												min={0.5}
												max={2}
												step={0.1}
												value={tts.pitch}
												onChange={(e) => setTts({ pitch: +e.target.value })}
												className="w-full accent-violet-500"
											/>
										</Field>
									</div>
								</div>
							)}
						</TabsContent>

						{/* ── 数据（记忆 & 知识库） ── */}
						<TabsContent value="data" className="space-y-3 pt-1">
							<div className="flex items-center gap-2 pb-3 border-b border-zinc-800">
								<Brain className="w-4 h-4 text-violet-400" />
								<span className="text-sm text-white font-medium">记忆</span>
							</div>
							<div className="grid grid-cols-2 gap-4">
								<Field label="记忆字符上限">
									<Input
										type="number"
										min={0}
										value={mem.memory_char_limit}
										onChange={(e) =>
											setMem({ memory_char_limit: +e.target.value })
										}
										className={inp}
									/>
								</Field>
								<Field label="用户画像字符上限">
									<Input
										type="number"
										min={0}
										value={mem.user_char_limit}
										onChange={(e) =>
											setMem({ user_char_limit: +e.target.value })
										}
										className={inp}
									/>
								</Field>
								<Field label="后台自省">
									<div className="flex items-center gap-2 h-9">
										<Switch
											checked={mem.review_enabled}
											onCheckedChange={(v) => setMem({ review_enabled: v })}
										/>
										<span className="text-xs text-zinc-500">
											{mem.review_enabled ? "每轮对话后自动提取记忆" : "关闭"}
										</span>
									</div>
								</Field>
							</div>
							<div className="border-t border-zinc-800 pt-3">
								<button
									onClick={() => navigate("/data/memory")}
									className="text-xs text-violet-400 hover:text-violet-300 flex items-center gap-1"
								>
									查看记忆库条目 →
								</button>
							</div>

							{/* ── 知识库 ── */}
							<div className="space-y-3">
								<div className="flex items-center gap-2 pb-3 border-b border-zinc-800">
									<BookOpen className="w-4 h-4 text-violet-400" />
									<span className="text-sm text-white font-medium">知识库</span>
								</div>
								<div className="flex items-center justify-between">
									<span className="text-xs text-zinc-500">
										{loadingKBs ? "加载中…" : `${kbs.length} 个知识库`}
									</span>
									<Button
										size="sm"
										onClick={handleBindKB}
										className="bg-violet-600 hover:bg-violet-500 text-white h-7 px-3 text-xs gap-1"
									>
										<BookOpen className="w-3 h-3" />
										绑定知识库
									</Button>
								</div>

								{loadingKBs ? (
									<div className="flex justify-center py-8">
										<div className="w-4 h-4 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
									</div>
								) : kbs.length === 0 ? (
									<div className="text-center py-10 border border-dashed border-zinc-800 rounded-xl">
										<p className="text-zinc-500 text-sm">暂无绑定知识库</p>
										<p className="text-zinc-600 text-xs mt-1">
											前往知识库页面绑定，让智能体具备特定领域知识
										</p>
									</div>
								) : (
									<div className="space-y-1">
										{kbs.map((kb) => {
											const kbExpanded = expandedKB === kb.id;
											return (
												<div
													key={kb.id}
													className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden"
												>
													<button
														onClick={() => toggleKB(kb.id)}
														className="w-full flex items-center gap-2 px-4 py-2.5 hover:bg-zinc-800/30 transition-colors text-left cursor-pointer"
													>
														{kbExpanded ? (
															<ChevronDown className="w-3.5 h-3.5 text-zinc-600 shrink-0" />
														) : (
															<ChevronRight className="w-3.5 h-3.5 text-zinc-600 shrink-0" />
														)}
														<BookOpen className="w-3.5 h-3.5 text-violet-400 shrink-0" />
														<span className="text-sm text-zinc-300 flex-1 truncate">
															{kb.name}
														</span>
														{kb.description && (
															<span className="text-xs text-zinc-600 truncate max-w-32">
																{kb.description}
															</span>
														)}
														<span className="text-xs text-zinc-600 font-mono">
															{kb.embedding_model_id || "未配置"}
														</span>
														<button
															onClick={(e) => {
																e.stopPropagation();
																handleDeleteKB(kb.id);
															}}
															className="shrink-0 text-zinc-600 hover:text-amber-400 transition-all p-1 rounded hover:bg-amber-400/10 cursor-pointer"
															title="解绑"
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
																	onClick={() => {
																		setUploadKBId(kb.id);
																		setUploadOpen(true);
																	}}
																	className="bg-zinc-800 hover:bg-zinc-700 text-zinc-300 h-7 px-3 text-xs gap-1 border border-zinc-700"
																>
																	<Upload className="w-3 h-3" />
																	上传文件
																</Button>
																<Button
																	size="sm"
																	onClick={() => {
																		setUrlKBId(kb.id);
																		setUrlOpen(true);
																	}}
																	className="bg-zinc-800 hover:bg-zinc-700 text-zinc-300 h-7 px-3 text-xs gap-1 border border-zinc-700"
																>
																	<Link2 className="w-3 h-3" />
																	导入 URL
																</Button>
																<span className="ml-auto text-xs text-zinc-600">
																	{docs.length} 个文档
																</span>
															</div>

															{/* KB Search */}
															<div className="flex items-center gap-2 px-4 py-2 border-b border-zinc-800/30 bg-zinc-800/20">
																<Search className="w-3 h-3 text-zinc-500 shrink-0" />
																<Input
																	value={kbSearchText}
																	onChange={(e) =>
																		setKbSearchText(e.target.value)
																	}
																	placeholder="搜索知识库内容..."
																	className="flex-1 bg-zinc-900 border-zinc-700 text-white placeholder:text-zinc-600 h-7 text-xs focus-visible:ring-violet-500"
																	onKeyDown={(e) => {
																		if (e.key === "Enter")
																			handleKBSearch(kb.id);
																	}}
																/>
																<Button
																	size="sm"
																	onClick={() => handleKBSearch(kb.id)}
																	disabled={kbSearching || !kbSearchText.trim()}
																	className="bg-violet-600 hover:bg-violet-500 text-white h-7 px-3 text-xs gap-1"
																>
																	{kbSearching ? (
																		<Loader2 className="w-3 h-3 animate-spin" />
																	) : (
																		<Search className="w-3 h-3" />
																	)}
																	检索
																</Button>
															</div>

															{/* KB Search Results */}
															{kbSearchKBId === kb.id &&
																kbSearchResults.length > 0 && (
																	<div className="border-b border-zinc-800/30 bg-zinc-800/10">
																		{kbSearchResults.map((r, i) => (
																			<div
																				key={i}
																				className="px-4 py-2.5 border-b border-zinc-800/10 last:border-0"
																			>
																				<div className="flex items-center gap-2 mb-1">
																					<FileText className="w-3 h-3 text-violet-400 shrink-0" />
																					<span className="text-xs text-zinc-400 truncate">
																						{r.document_name}
																					</span>
																					<span className="text-[10px] text-zinc-600 bg-zinc-800 px-1.5 py-0.5 rounded ml-auto shrink-0">
																						得分 {(r.score * 100).toFixed(0)}%
																					</span>
																				</div>
																				<p className="text-sm text-zinc-300 leading-relaxed">
																					{r.content.slice(0, 300)}
																					{r.content.length > 300 && "…"}
																				</p>
																			</div>
																		))}
																		<div className="px-4 py-1.5 flex justify-end">
																			<button
																				onClick={() => {
																					setKbSearchResults([]);
																					setKbSearchKBId("");
																				}}
																				className="text-xs text-zinc-500 hover:text-zinc-300 cursor-pointer"
																			>
																				清除结果
																			</button>
																		</div>
																	</div>
																)}

															{/* Docs list */}
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
																		const status =
																			DOC_STATUS_MAP[doc.status] ||
																			DOC_STATUS_MAP.error;
																		const StatusIcon = status.icon;
																		return (
																			<div
																				key={doc.id}
																				className={`flex items-center gap-3 px-4 py-2.5 hover:bg-zinc-800/10 transition-colors group ${
																					i < docs.length - 1
																						? "border-b border-zinc-800/20"
																						: ""
																				}`}
																			>
																				<FileText className="w-3.5 h-3.5 text-zinc-500 shrink-0" />
																				<div className="flex-1 min-w-0">
																					<div className="flex items-center gap-2">
																						<span className="text-sm text-zinc-300 truncate">
																							{doc.name}
																						</span>
																						{doc.source === "url" && (
																							<Link2 className="w-3 h-3 text-zinc-600 shrink-0" />
																						)}
																					</div>
																					{doc.status === "error" &&
																						doc.error_message && (
																							<p className="text-[11px] text-red-400/80 truncate mt-0.5">
																								{doc.error_message}
																							</p>
																						)}
																				</div>
																				<span className="text-xs text-zinc-600 font-mono shrink-0">
																					{doc.source}
																				</span>
																				<span
																					className={`flex items-center gap-1 text-xs ${status.cls} shrink-0`}
																				>
																					<StatusIcon
																						className={`w-3 h-3 ${
																							doc.status !== "ready" &&
																							doc.status !== "error"
																								? "animate-spin"
																								: ""
																						}`}
																					/>
																					{status.label}
																				</span>
																				<span className="text-xs text-zinc-600 shrink-0 w-16 text-right">
																					{doc.status === "ready"
																						? `${doc.chunk_count} 块`
																						: doc.char_count
																							? `${(doc.char_count / 1000).toFixed(1)} KB`
																							: "—"}
																				</span>
																				<button
																					onClick={() =>
																						handleDeleteDoc(doc.id)
																					}
																					className="shrink-0 opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-red-400 transition-all p-1 rounded hover:bg-red-400/10 cursor-pointer"
																				>
																					<Trash2 className="w-3.5 h-3.5" />
																				</button>
																				{doc.status === "error" && (
																					<button
																						onClick={() =>
																							handleRetryDoc(doc.id, kb.id)
																						}
																						className="shrink-0 opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-amber-400 transition-all p-1 rounded hover:bg-amber-400/10 cursor-pointer"
																					>
																						<RotateCw className="w-3.5 h-3.5" />
																					</button>
																				)}
																			</div>
																		);
																	})}
																</div>
															)}
														</div>
													)}
												</div>
											);
										})}
									</div>
								)}
							</div>
						</TabsContent>

						{/* ── MCP ── */}
						<TabsContent value="mcp" className="space-y-4 pt-1">
							<div className="flex gap-2">
								<div className="relative flex-1">
									<Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-500" />
									<input
										type="text"
										value={mcpSearch}
										onChange={(e) => setMcpSearch(e.target.value)}
										placeholder="搜索 MCP..."
										className="bg-zinc-800 border border-zinc-700 text-white placeholder:text-zinc-500 rounded-lg pl-8 pr-2.5 py-1.5 text-xs w-full focus:outline-none focus:border-violet-500 transition-colors"
									/>
								</div>
								<div className="flex gap-1">
									{(["all", "bound", "unbound"] as const).map((f) => (
										<button
											key={f}
											type="button"
											onClick={() => setMcpFilter(f)}
											className={`text-[11px] px-2.5 py-1.5 rounded-lg transition-colors ${
												mcpFilter === f
													? "bg-violet-600 text-white"
													: "bg-zinc-800 text-zinc-400 hover:text-zinc-300"
											}`}
										>
											{f === "all"
												? "全部"
												: f === "bound"
													? "已绑定"
													: "未绑定"}
										</button>
									))}
								</div>
							</div>

							{filteredMcps.length === 0 ? (
								<div className="text-center py-14 border border-dashed border-zinc-800 rounded-xl">
									<p className="text-zinc-500 text-sm">
										{mcpServers.length === 0
											? "暂无可用 MCP"
											: mcpSearch
												? "没有匹配的 MCP"
												: "没有匹配的 MCP"}
									</p>
									{mcpServers.length === 0 && (
										<p className="text-zinc-600 text-xs mt-1">
											请先在 MCP 管理页面开通 MCP 服务器
										</p>
									)}
								</div>
							) : (
								<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
									{filteredMcps.map((s) => {
										const bound = boundIds.has(s.id);
										return (
											<div
												key={s.id}
												onClick={() =>
													bound
														? handleUnbindMcp(s.id)
														: handleAddMcpBinding(s.id)
												}
												className={`relative rounded-xl border p-4 cursor-pointer transition-all ${
													bound
														? "border-violet-500 bg-violet-500/10 shadow-sm shadow-violet-500/20"
														: "border-zinc-800 bg-zinc-900 hover:border-zinc-700"
												}`}
											>
												{bound && (
													<CheckCircle2 className="absolute top-3 right-3 w-4 h-4 text-violet-400" />
												)}
												<div className="flex items-start gap-3 mb-2">
													<McpServerIcon
														name={s.name}
														icon={s.icon}
														size="sm"
													/>
													<div className="min-w-0 flex-1">
														<p className="text-sm font-medium text-white truncate">
															{s.name}
														</p>
														<p className="text-[11px] text-zinc-500 font-mono mt-0.5">
															{s.transport}
														</p>
													</div>
												</div>
												<p className="text-xs text-zinc-500 line-clamp-2 leading-relaxed mb-2">
													{s.description || s.endpoint}
												</p>
												{s.tags && s.tags.length > 0 && (
													<div className="flex flex-wrap gap-1 mb-2">
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
												{bound && (
													<span className="text-[10px] text-violet-400/60">
														已绑定
													</span>
												)}
												<button
													type="button"
													onClick={(e) => {
														e.stopPropagation();
														setDetailMcp(s);
													}}
													className="absolute bottom-3 right-3 text-[11px] text-zinc-500 hover:text-zinc-300 transition-colors"
												>
													查看详情
												</button>
											</div>
										);
									})}
								</div>
							)}
							{allFiltered.length > mcpPageSize && (
								<div className="flex items-center justify-between mt-4">
									<span className="text-xs text-zinc-500">
										共 {allFiltered.length} 条
									</span>
									<div className="flex items-center gap-2">
										<select
											value={mcpPageSize}
											onChange={(e) => {
												setMcpPageSize(Number(e.target.value));
												setMcpPage(1);
											}}
											className="h-7 text-xs bg-zinc-800 border border-zinc-700 rounded px-2 text-zinc-300 focus:outline-none focus:ring-1 focus:ring-violet-500 cursor-pointer"
										>
											{[12, 24, 48].map((n) => (
												<option key={n} value={n}>
													{n} 条/页
												</option>
											))}
										</select>
										<div className="flex items-center gap-1">
											<button
												onClick={() => setMcpPage((p) => Math.max(1, p - 1))}
												disabled={safePage <= 1}
												className="h-7 px-2 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
											>
												上一页
											</button>
											<span className="text-xs text-zinc-400 px-1">
												{safePage} / {mcpTotalPages}
											</span>
											<button
												onClick={() =>
													setMcpPage((p) => Math.min(mcpTotalPages, p + 1))
												}
												disabled={safePage >= mcpTotalPages}
												className="h-7 px-2 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
											>
												下一页
											</button>
										</div>
									</div>
								</div>
							)}
							<div className="border-t border-zinc-800 pt-3">
								<button
									onClick={() => navigate("/components/mcp")}
									className="text-xs text-violet-400 hover:text-violet-300"
								>
									管理 MCP 服务器 →
								</button>
							</div>
						</TabsContent>

						{/* ── 设备 ── */}

						{/* ── 设备 ── */}
						<TabsContent value="devices" className="space-y-4 pt-1">
							<div className="flex items-center justify-between gap-3">
								<div className="relative w-full max-w-xs">
									<Search className="absolute left-2.5 top-1/2 w-3.5 h-3.5 -translate-y-1/2 text-zinc-500 pointer-events-none" />
									<Input
										value={deviceSearch}
										onChange={(e) => setDeviceSearch(e.target.value)}
										placeholder="搜索设备 ID 或名称"
										className={`${inp} pl-8`}
									/>
								</div>
								<Button
									onClick={() => setDevOpen(true)}
									className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-3 text-sm gap-1.5"
								>
									<Plus className="w-3.5 h-3.5" />
									绑定设备
								</Button>
							</div>
							{devices.length === 0 ? (
								<div className="text-center py-14 border border-dashed border-zinc-800 rounded-xl">
									<p className="text-zinc-500 text-sm">暂无绑定设备</p>
									<p className="text-zinc-600 text-xs mt-1">
										填写设备 ID 即可绑定硬件
									</p>
								</div>
							) : filteredDevices.length === 0 ? (
								<div className="text-center py-14 border border-dashed border-zinc-800 rounded-xl">
									<p className="text-zinc-500 text-sm">未找到匹配的设备</p>
									<p className="text-zinc-600 text-xs mt-1">尝试设备 ID 或名称</p>
								</div>
							) : (
								<div className="space-y-2">
									{filteredDevices.map((d) => (
										<div key={d.id} className="bg-zinc-900 border border-zinc-800 rounded-lg">
											<div className="flex items-center justify-between px-4 py-3">
												<div>
												<p className="text-sm font-mono text-white">{d.id}</p>
												<p className="text-xs text-zinc-500 mt-0.5">
													{d.name && <span className="mr-2">{d.name}</span>}
													{new Date(d.created_at).toLocaleDateString("zh-CN")}
												</p>
											</div>
												<div className="flex items-center gap-2">
													<button
														onClick={() => openChannelSettings(d.id)}
														className="inline-flex items-center gap-1.5 text-xs text-zinc-400 hover:text-white transition-colors px-2 py-1"
													>
														<Send className="w-3.5 h-3.5" />
														Channels
													</button>
												<button
													onClick={() =>
														navigate(
															`/data/memory?device_id=${encodeURIComponent(d.id)}`,
														)
													}
													className="text-xs text-violet-400 hover:text-violet-300 transition-colors"
												>
													查看记忆
												</button>
												<button
													onClick={() => handleDeleteDevice(d.id)}
													className="text-zinc-600 hover:text-red-400 transition-colors px-2 py-1 text-xs"
												>
													删除
												</button>
												</div>
											</div>
											{channelDeviceID === d.id && (
												<div className="border-t border-zinc-800 px-4 py-3 space-y-3">
													<div className="flex items-center justify-between gap-3">
														<div className="flex items-center gap-2 min-w-0">
															<Send className="w-4 h-4 text-sky-400 shrink-0" />
															<div>
																<p className="text-sm text-zinc-200">Telegram Bot</p>
																<p className="text-xs text-zinc-500 mt-0.5">
																	{d.telegram.enabled
																		? `已连接 ${d.telegram.token_hint ?? ""}`
																		: "未连接"}
																</p>
															</div>
														</div>
														{d.telegram.enabled && (
															<Button
																variant="outline"
																onClick={() => handleClearTelegram(d.id)}
																disabled={telegramSaving}
																className="h-8 border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-red-300"
															>
																断开
															</Button>
														)}
													</div>
													<div className="flex gap-2">
														<Input
															type="password"
															value={telegramToken}
															onChange={(e) => setTelegramToken(e.target.value)}
															placeholder={d.telegram.enabled ? "输入新 Bot Token 以替换" : "Bot Token"}
															className={inp}
														/>
														<Button
															onClick={() => handleSaveTelegram(d.id)}
															disabled={telegramSaving || !telegramToken.trim()}
															className="h-9 bg-violet-600 hover:bg-violet-500 text-white shrink-0"
														>
															{telegramSaving ? "保存中..." : "保存"}
														</Button>
													</div>
													{telegramErr && <p className="text-xs text-red-400">{telegramErr}</p>}
												</div>
											)}
										</div>
									))}
								</div>
							)}
						</TabsContent>
					</Tabs>
				</div>
				<div
					className="w-[5px] shrink-0 cursor-col-resize bg-zinc-800 hover:bg-violet-500/50 transition-colors relative z-10"
					onMouseDown={onResizeStart}
				/>
				<div
					className="flex flex-col min-w-0"
					style={{ width: `${100 - splitPct}%` }}
				>
					<QuickChat agentId={id!} vadMode={cfg.asr.vad_mode} />
				</div>
			</div>

			{/* Device Dialog */}
			<Dialog open={devOpen} onOpenChange={setDevOpen}>
				<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
					<DialogHeader>
						<DialogTitle className="text-white">绑定设备</DialogTitle>
					</DialogHeader>
					<div className="space-y-3 py-2">
						<Field label="Device ID">
							<Input
								value={newDevId}
								onChange={(e) => setNewDevId(e.target.value)}
								placeholder="esp32-abc123"
								className={inp}
								autoFocus
							/>
						</Field>
						<Field label="设备名称（可选）">
							<Input
								value={newDevName}
								onChange={(e) => setNewDevName(e.target.value)}
								placeholder="客厅音箱"
								className={inp}
							/>
						</Field>
						{devErr && (
							<p className="text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-3 py-2">
								{devErr}
							</p>
						)}
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setDevOpen(false)}
							className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
						>
							取消
						</Button>
						<Button
							onClick={handleAddDevice}
							disabled={devAdding || !newDevId.trim()}
							className="bg-violet-600 hover:bg-violet-500 text-white"
						>
							{devAdding ? "绑定中..." : "绑定"}
						</Button>
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
							onDragOver={(e) => {
								e.preventDefault();
								setFileDragging(true);
							}}
							onDragLeave={() => setFileDragging(false)}
							onDrop={(e) => {
								e.preventDefault();
								setFileDragging(false);
							}}
							className={`border-2 border-dashed rounded-xl p-10 text-center transition-all cursor-pointer ${fileDragging ? "border-violet-500 bg-violet-600/5" : "border-zinc-700 hover:border-zinc-600"}`}
							onClick={() => fileRef.current?.click()}
						>
							<Upload
								className="w-8 h-8 text-zinc-600 mx-auto mb-3"
								strokeWidth={1.5}
							/>
							<p className="text-sm text-zinc-400 mb-1">
								拖拽文件到此处，或点击选择
							</p>
							<p className="text-xs text-zinc-600 mt-3">
								支持 TXT、Markdown、代码文件等
							</p>
						</div>
						<input
							ref={fileRef}
							type="file"
							className="hidden"
							onChange={handleUpload}
						/>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setUploadOpen(false)}
							className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
						>
							关闭
						</Button>
						<Button
							onClick={handleUpload}
							disabled={!fileRef.current?.files?.length}
							className="bg-violet-600 hover:bg-violet-500 text-white"
						>
							上传
						</Button>
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
						<label className="text-xs text-zinc-400 mb-1.5 block">
							网页地址
						</label>
						<Input
							value={urlInput}
							onChange={(e) => setUrlInput(e.target.value)}
							placeholder="https://example.com/doc.html"
							className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 h-9 text-sm"
							onKeyDown={(e) => {
								if (e.key === "Enter") handleImportURL();
							}}
						/>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setUrlOpen(false)}
							className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
						>
							取消
						</Button>
						<Button
							onClick={handleImportURL}
							disabled={!urlInput.trim()}
							className="bg-violet-600 hover:bg-violet-500 text-white"
						>
							导入
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* Bind KB Dialog */}
			<Dialog open={bindKBOpen} onOpenChange={setBindKBOpen}>
				<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg max-h-[70vh] flex flex-col">
					<DialogHeader>
						<DialogTitle className="text-white flex items-center gap-2">
							<BookOpen className="w-4 h-4 text-violet-400" />
							绑定知识库
						</DialogTitle>
					</DialogHeader>
					<div className="relative">
						<Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-500" />
						<Input
							value={bindKBSearch}
							onChange={(e) => setBindKBSearch(e.target.value)}
							placeholder="模糊搜索知识库..."
							className="pl-8 bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500 h-8 text-sm"
						/>
					</div>
					<div className="overflow-y-auto flex-1 -mx-1 px-1 space-y-1 mt-2 max-h-[380px]">
						{bindKBLoading ? (
							<div className="flex justify-center py-8">
								<div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
							</div>
						) : bindKBList.filter(
								(k) =>
									!bindKBSearch ||
									k.name.toLowerCase().includes(bindKBSearch.toLowerCase()),
							).length === 0 ? (
							<div className="text-center py-8 text-xs text-zinc-500">
								{bindKBList.length === 0
									? "暂无可用知识库，请先创建"
									: "没有匹配的知识库"}
							</div>
						) : (
							bindKBList
								.filter(
									(k) =>
										!bindKBSearch ||
										k.name.toLowerCase().includes(bindKBSearch.toLowerCase()),
								)
								.map((k) => {
									const binding = bindKBBinding.has(k.id);
									return (
										<div
											key={k.id}
											className="flex items-center gap-3 px-3 py-2.5 bg-zinc-800/50 border border-zinc-800 rounded-lg hover:border-zinc-700 transition-colors"
										>
											<BookOpen className="w-4 h-4 text-violet-400 shrink-0" />
											<div className="flex-1 min-w-0">
												<p className="text-sm text-zinc-300 truncate">
													{k.name}
												</p>
												{k.description && (
													<p className="text-xs text-zinc-500 truncate mt-0.5">
														{k.description}
													</p>
												)}
											</div>
											<span className="text-[10px] text-zinc-600 font-mono shrink-0">
												{k.embedding_model_id || "未配置"}
											</span>
											<Button
												size="sm"
												onClick={() => doBindKB(k.id)}
												disabled={binding}
												className="bg-violet-600 hover:bg-violet-500 text-white h-7 px-3 text-xs shrink-0 disabled:opacity-50"
											>
												{binding ? "绑定中..." : "绑定"}
											</Button>
										</div>
									);
								})
						)}
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setBindKBOpen(false)}
							className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white mt-2"
						>
							关闭
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* MCP 详情抽屉 */}
			<Sheet open={!!detailMcp} onOpenChange={(o) => !o && setDetailMcp(null)}>
				<SheetContent className="flex flex-col gap-0" style={{ padding: 0 }}>
					{detailMcp && (
						<>
							<SheetHeader className="px-6 pt-6 pb-4 border-b border-zinc-800">
								<div className="flex items-center gap-3">
									<McpServerIcon name={detailMcp.name} icon={detailMcp.icon} />
									<div>
										<SheetTitle>{detailMcp.name}</SheetTitle>
										<p className="text-[11px] text-zinc-500 font-mono mt-0.5">
											{detailMcp.transport}
										</p>
									</div>
								</div>
							</SheetHeader>

							<McpServerDetail server={detailMcp} mode="view" />
						</>
					)}
				</SheetContent>
			</Sheet>
		</div>
	);
}
