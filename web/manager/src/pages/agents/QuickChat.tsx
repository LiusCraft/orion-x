import { useEffect, useRef, useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Send, Square } from "lucide-react";
import { deviceApi } from "@/lib/api";

interface ChatMsg {
	role: "user" | "assistant";
	text: string;
	sub?: string;
	id: number;
}

const WS_URL = "ws://localhost:8080/ws";

export default function QuickChat({
	agentId,
	vadMode,
}: {
	agentId: string;
	vadMode: string;
}) {
	const [msgs, setMsgs] = useState<ChatMsg[]>([]);
	const [text, setText] = useState("");
	const [connected, setConnected] = useState(false);
	const [connecting, setConnecting] = useState(false);
	const [sessionId, setSessionId] = useState("");

	const wsRef = useRef<WebSocket | null>(null);
	const msgIdRef = useRef(0);
	const chatRef = useRef<HTMLDivElement>(null);
	const audioCtxRef = useRef<AudioContext | null>(null);
	const gainRef = useRef<GainNode | null>(null);
	const nextPlayRef = useRef(0);
	const streamRef = useRef<MediaStream | null>(null);
	const procRef = useRef<ScriptProcessorNode | null>(null);

	const isAutoMode = vadMode !== "manual";

	const scrollToBottom = () => {
		if (chatRef.current)
			chatRef.current.scrollTop = chatRef.current.scrollHeight;
	};

	const addMsg = useCallback(
		(role: "user" | "assistant", text: string, sub = "") => {
			setMsgs((p) => {
				const id = ++msgIdRef.current;
				return [...p, { role, text, sub, id }];
			});
			setTimeout(scrollToBottom, 50);
		},
		[],
	);

	const send = useCallback((msg: Record<string, unknown>) => {
		if (wsRef.current?.readyState === WebSocket.OPEN) {
			wsRef.current.send(JSON.stringify(msg));
		}
	}, []);

	const stopMic = useCallback(() => {
		procRef.current?.disconnect();
		procRef.current = null;
		streamRef.current?.getTracks().forEach((t) => t.stop());
		streamRef.current = null;
		audioCtxRef.current?.close();
		audioCtxRef.current = null;
	}, []);

	const startMic = useCallback(async () => {
		if (streamRef.current) return;
		try {
			const stream = await navigator.mediaDevices.getUserMedia({
				audio: {
					sampleRate: 16000,
					channelCount: 1,
					echoCancellation: true,
					noiseSuppression: true,
				},
			});
			streamRef.current = stream;
			const ctx = new AudioContext({ sampleRate: 16000 });
			audioCtxRef.current = ctx;
			const src = ctx.createMediaStreamSource(stream);
			const proc = ctx.createScriptProcessor(4096, 1, 1);
			procRef.current = proc;
			proc.onaudioprocess = ({ inputBuffer }) => {
				if (wsRef.current?.readyState !== WebSocket.OPEN) return;
				const f32 = inputBuffer.getChannelData(0);
				const i16 = new Int16Array(f32.length);
				for (let i = 0; i < f32.length; i++) {
					const s = Math.max(-1, Math.min(1, f32[i]));
					i16[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
				}
				wsRef.current.send(i16.buffer);
			};
			src.connect(proc);
			proc.connect(ctx.destination);
		} catch {
			/* mic denied */
		}
	}, []);

	const connect = useCallback(async () => {
		if (wsRef.current) wsRef.current.close();
		stopMic();
		setConnecting(true);

		try {
			await deviceApi.list(agentId).then((res) => {
				if (!res.data.some((d) => d.id === agentId)) {
					return deviceApi.create(agentId, agentId, "quick-experience");
				}
			});
		} catch {
			/* proceed */
		}

		const params = new URLSearchParams({
			"protocol-version": "1",
			"device-id": agentId,
			"client-id": crypto.randomUUID(),
		});
		const ws = new WebSocket(`${WS_URL}?${params}`);
		ws.binaryType = "arraybuffer";
		wsRef.current = ws;

		ws.onopen = () => {
			ws.send(
				JSON.stringify({
					type: "hello",
					device_id: agentId,
					mode: isAutoMode ? "auto" : "manual",
					audio_params: {
						format: "pcm",
						sample_rate: 16000,
						channels: 1,
						bits_per_sample: 16,
					},
				}),
			);
			if (isAutoMode) startMic();
		};

		ws.onmessage = ({ data }) => {
			if (typeof data !== "string") {
				if (!audioCtxRef.current) {
					const ctx = new AudioContext({ sampleRate: 16000 });
					audioCtxRef.current = ctx;
					gainRef.current = ctx.createGain();
					gainRef.current.connect(ctx.destination);
					nextPlayRef.current = 0;
				}
				const i16 = new Int16Array(data);
				if (i16.length === 0) return;
				const f32 = new Float32Array(i16.length);
				for (let i = 0; i < i16.length; i++) f32[i] = i16[i] / 32768.0;
				const ctx = audioCtxRef.current;
				const buf = ctx.createBuffer(1, f32.length, 16000);
				buf.copyToChannel(f32, 0);
				const src = ctx.createBufferSource();
				src.buffer = buf;
				src.connect(gainRef.current ?? ctx.destination);
				const now = ctx.currentTime;
				const start = Math.max(now, nextPlayRef.current);
				src.start(start);
				nextPlayRef.current = start + buf.duration;
				return;
			}

			let msg: Record<string, unknown>;
			try {
				msg = JSON.parse(data as string);
			} catch {
				return;
			}

			const type = msg.type as string;
			if (type === "hello") {
				setSessionId((msg.session_id as string) || "");
				setConnected(true);
				setConnecting(false);
				addMsg(
					"assistant",
					isAutoMode ? "已连接，开始对话吧" : "已连接，按住麦克风按钮说话",
					"system",
				);
			} else if (type === "stt" && msg.text) {
				addMsg("user", msg.text as string, "语音识别");
			} else if (
				type === "tts" &&
				msg.text &&
				(msg.state as string) !== "sentence_end"
			) {
				addMsg("assistant", msg.text as string, "TTS");
			}
		};

		ws.onerror = () => {
			setConnecting(false);
			addMsg("assistant", "连接失败", "error");
		};
		ws.onclose = () => {
			stopMic();
			setConnected(false);
			setConnecting(false);
			audioCtxRef.current?.close();
			audioCtxRef.current = null;
		};
	}, [agentId, addMsg, stopMic, startMic, isAutoMode]);

	const disconnect = useCallback(() => {
		send({ type: "abort" });
		stopMic();
		wsRef.current?.close();
		wsRef.current = null;
		setConnected(false);
		setSessionId("");
	}, [send, stopMic]);

	const handleSendText = useCallback(() => {
		const t = text.trim();
		if (!t || !wsRef.current) return;
		send({ type: "listen", state: "detect", text: t });
		addMsg("user", t, "文本");
		setText("");
	}, [text, send, addMsg]);

	const pttRef = useRef(false);
	const handlePTTDown = useCallback(() => {
		if (pttRef.current || !wsRef.current) return;
		pttRef.current = true;
		send({ type: "listen", state: "start" });
		startMic();
	}, [send, startMic]);

	const handlePTTUp = useCallback(() => {
		if (!pttRef.current) return;
		pttRef.current = false;
		send({ type: "listen", state: "stop" });
		stopMic();
	}, [send, stopMic]);

	// Cleanup on unmount
	useEffect(() => {
		return () => {
			stopMic();
			wsRef.current?.close();
		};
	}, [stopMic]);

	return (
		<div className="h-full flex flex-col">
			{/* Header */}
			<div className="flex items-center justify-between px-4 py-2.5 border-b border-zinc-800">
				<div className="flex items-center gap-2">
					<span className="text-sm font-medium text-white">快速体验</span>
					<span
						className={`text-xs ${connected ? "text-emerald-400" : connecting ? "text-yellow-400" : "text-zinc-500"}`}
					>
						● {connected ? "已连接" : connecting ? "连接中..." : ""}
					</span>
					{sessionId && (
						<span className="text-[10px] text-zinc-600 font-mono truncate max-w-[120px]">
							{sessionId.slice(0, 12)}
						</span>
					)}
				</div>
				{connected && (
					<Button
						size="sm"
						variant="outline"
						onClick={disconnect}
						className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 h-7 px-3 text-xs"
					>
						断开
					</Button>
				)}
			</div>

			{/* Chat area */}
			<div ref={chatRef} className="flex-1 overflow-y-auto p-3 space-y-2">
				{!connected && !connecting ? (
					<div className="h-full flex flex-col items-center justify-center gap-4">
						<p className="text-zinc-500 text-sm">连接 WebSocket 开始体验</p>
						<Button
							onClick={connect}
							className="bg-violet-600 hover:bg-violet-500 text-white h-10 px-6 text-sm"
						>
							立即体验
						</Button>
					</div>
				) : null}
				{msgs.map((m) => (
					<div
						key={m.id}
						className={`flex ${m.role === "user" ? "justify-end" : "justify-start"}`}
					>
						<div
							className={`max-w-[80%] px-3 py-2 rounded-xl text-sm leading-relaxed
              ${
								m.role === "user"
									? "bg-violet-600/20 border border-violet-500/30 text-white"
									: m.sub === "error"
										? "bg-red-900/30 border border-red-800/40 text-red-300"
										: m.sub === "system"
											? "bg-zinc-800/50 text-zinc-400 text-xs text-center w-full max-w-full"
											: "bg-zinc-800 border border-zinc-700 text-zinc-200"
							}`}
						>
							<p>{m.text}</p>
							{m.sub && m.sub !== "system" && (
								<p
									className={`text-[10px] mt-1 ${m.role === "user" ? "text-violet-300/60" : "text-zinc-500"}`}
								>
									{m.sub}
								</p>
							)}
						</div>
					</div>
				))}
			</div>

			{/* Controls */}
			<div className="border-t border-zinc-800 px-3 py-2.5 flex items-center gap-2">
				<Input
					value={text}
					onChange={(e) => setText(e.target.value)}
					onKeyDown={(e) => e.key === "Enter" && handleSendText()}
					placeholder={
						connected
							? isAutoMode
								? "输入文本或直接说话..."
								: "输入文本或按住按钮说话..."
							: "请先连接"
					}
					disabled={!connected}
					className="bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 focus-visible:ring-violet-500 h-8 text-sm flex-1"
				/>
				<Button
					size="sm"
					disabled={!connected || !text.trim()}
					onClick={handleSendText}
					className="bg-violet-600 hover:bg-violet-500 text-white h-8 w-8 p-0 shrink-0"
				>
					<Send className="w-3.5 h-3.5" />
				</Button>

				{!isAutoMode && (
					<button
						disabled={!connected}
						onMouseDown={handlePTTDown}
						onMouseUp={handlePTTUp}
						onMouseLeave={handlePTTUp}
						onTouchStart={(e) => {
							e.preventDefault();
							handlePTTDown();
						}}
						onTouchEnd={(e) => {
							e.preventDefault();
							handlePTTUp();
						}}
						className={`h-8 px-3 text-xs rounded-lg font-medium select-none shrink-0 border transition-colors
              ${
								pttRef.current
									? "bg-red-500/20 border-red-500/40 text-red-400"
									: connected
										? "bg-zinc-800 border-zinc-700 text-zinc-300 hover:bg-zinc-700 active:bg-red-500/10 active:border-red-500/30"
										: "bg-zinc-800/50 border-zinc-800 text-zinc-600 cursor-not-allowed"
							}`}
					>
						{pttRef.current ? "说话中..." : "按住说话"}
					</button>
				)}

				<Button
					size="sm"
					variant="outline"
					disabled={!connected}
					onClick={() => send({ type: "abort" })}
					className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 h-8 w-8 p-0 shrink-0"
				>
					<Square className="w-3.5 h-3.5" />
				</Button>
			</div>
		</div>
	);
}
