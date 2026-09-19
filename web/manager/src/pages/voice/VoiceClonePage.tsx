import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Wand2,
  Upload,
  Mic,
  CheckCircle2,
  Loader2,
  AlertCircle,
  FileAudio,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select";
import {
  assetsApi,
  languageApi,
  voiceCloneApi,
  type Asset,
  type Language,
  type VoiceCloneModel,
} from "@/lib/api";

type CloneStatus = "idle" | "uploading" | "cloning" | "done";

const TIPS = [
  "录制环境应安静，无回声和背景音",
  "说话应自然清晰，语速适中",
  "音频时长建议 30 秒至 3 分钟",
  "支持 WAV、MP3、M4A、FLAC 格式，最大 20MB",
];

const ALLOWED_EXTS = [".wav", ".mp3", ".m4a", ".flac"];
const MAX_SAMPLE_SIZE = 20 * 1024 * 1024;

function errorMessage(err: unknown, fallback: string): string {
  return (
    (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
    fallback
  );
}

function formatSize(bytes: number): string {
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)}MB`;
  if (bytes >= 1 << 10) return `${Math.round(bytes / (1 << 10))}KB`;
  return `${bytes}B`;
}

export default function VoiceClonePage() {
  const navigate = useNavigate();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [step, setStep] = useState(1);
  const [dragging, setDragging] = useState(false);

  const [file, setFile] = useState<File | null>(null);
  const [asset, setAsset] = useState<Asset | null>(null);
  const [uploadPercent, setUploadPercent] = useState(0);

  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [modelId, setModelId] = useState("");
  const [langs, setLangs] = useState<string[]>([]);

  const [models, setModels] = useState<VoiceCloneModel[]>([]);
  const [languages, setLanguages] = useState<Language[]>([]);

  const [status, setStatus] = useState<CloneStatus>("idle");
  const [error, setError] = useState("");
  const [createdName, setCreatedName] = useState("");

  useEffect(() => {
    voiceCloneApi
      .models()
      .then(({ data }) => {
        setModels(data);
        const first = data.find((m) => m.configured) ?? data[0];
        if (first) setModelId(first.id);
      })
      .catch(() => setModels([]));
    languageApi
      .list()
      .then(({ data }) => setLanguages(data))
      .catch(() => setLanguages([]));
  }, []);

  const selectedModel = useMemo(
    () => models.find((m) => m.id === modelId) ?? null,
    [models, modelId],
  );

  const upload = async (picked: File) => {
    setStatus("uploading");
    setUploadPercent(0);
    setError("");
    try {
      const { data } = await assetsApi.upload(
        picked,
        "voice_sample",
        setUploadPercent,
      );
      setAsset(data);
      setStep(2);
    } catch (err) {
      setError(errorMessage(err, "参考音频上传失败，请重试"));
    } finally {
      setStatus("idle");
    }
  };

  const handleFileSelect = (picked: File | null | undefined) => {
    if (!picked) return;
    const lower = picked.name.toLowerCase();
    if (!ALLOWED_EXTS.some((ext) => lower.endsWith(ext))) {
      setError(`仅支持 ${ALLOWED_EXTS.join(" / ")} 格式的音频`);
      return;
    }
    if (picked.size > MAX_SAMPLE_SIZE) {
      setError("参考音频不能超过 20MB");
      return;
    }
    setFile(picked);
    setAsset(null);
    void upload(picked);
  };

  const startClone = async () => {
    if (!asset || !modelId || !name.trim()) return;
    setStep(3);
    setStatus("cloning");
    setError("");
    try {
      const { data } = await voiceCloneApi.clone(modelId, {
        name: name.trim(),
        description: desc.trim() || undefined,
        source_asset_id: asset.id,
        langs: langs.length ? langs : undefined,
      });
      setCreatedName(data.name);
      setStatus("done");
    } catch (err) {
      setStatus("idle");
      setError(errorMessage(err, "复刻失败，请重试"));
      setStep(2);
    }
  };

  const reset = () => {
    setStep(1);
    setFile(null);
    setAsset(null);
    setUploadPercent(0);
    setName("");
    setDesc("");
    setLangs([]);
    setError("");
    setCreatedName("");
    setStatus("idle");
  };

  const toggleLang = (code: string) => {
    setLangs((prev) =>
      prev.includes(code) ? prev.filter((c) => c !== code) : [...prev, code],
    );
  };

  	const canClone = Boolean(
  		asset && modelId && name.trim() && selectedModel?.configured,
  	);

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div>
          <h1 className="text-lg font-semibold text-white">语音复刻</h1>
          <p className="text-sm text-zinc-500 mt-0.5">
            上传参考音频，克隆专属音色
          </p>
        </div>
      </div>

      <div className="px-8 py-8 max-w-2xl">
        {/* Steps */}
        <div className="flex items-center gap-2 mb-8">
          {["上传音频", "填写信息", "开始复刻"].map((label, i) => {
            const idx = i + 1;
            const isDone = step > idx;
            const isActive = step === idx;
            return (
              <div key={label} className="flex items-center gap-2">
                <div
                  className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-semibold transition-all ${
                    isDone
                      ? "bg-emerald-500 text-white"
                      : isActive
                        ? "bg-violet-600 text-white"
                        : "bg-zinc-800 text-zinc-500"
                  }`}
                >
                  {isDone ? <CheckCircle2 className="w-4 h-4" /> : idx}
                </div>
                <span
                  className={`text-xs ${isActive ? "text-white font-medium" : isDone ? "text-emerald-400" : "text-zinc-600"}`}
                >
                  {label}
                </span>
                {i < 2 && <div className="w-12 h-px bg-zinc-800 mx-1" />}
              </div>
            );
          })}
        </div>

        {error && (
          <div className="mb-5 flex items-start gap-2 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3">
            <AlertCircle className="w-4 h-4 text-red-400 mt-0.5 shrink-0" />
            <p className="text-xs text-red-300">{error}</p>
          </div>
        )}

        {/* Step 1: Upload */}
        {step === 1 && (
          <div className="space-y-4">
            <input
              ref={fileInputRef}
              type="file"
              accept={ALLOWED_EXTS.join(",")}
              className="hidden"
              onChange={(e) => {
                handleFileSelect(e.target.files?.[0]);
                e.target.value = "";
              }}
            />
            <div
              onDragOver={(e) => {
                e.preventDefault();
                setDragging(true);
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={(e) => {
                e.preventDefault();
                setDragging(false);
                handleFileSelect(e.dataTransfer.files?.[0]);
              }}
              onClick={() => fileInputRef.current?.click()}
              className={`border-2 border-dashed rounded-xl p-12 text-center cursor-pointer transition-all ${
                dragging
                  ? "border-violet-500 bg-violet-600/5"
                  : "border-zinc-700 hover:border-violet-500/50 hover:bg-zinc-900"
              }`}
            >
              {status === "uploading" ? (
                <>
                  <div className="w-14 h-14 rounded-2xl bg-violet-600/10 flex items-center justify-center mx-auto mb-4">
                    <Loader2 className="w-7 h-7 text-violet-400 animate-spin" />
                  </div>
                  <p className="text-sm text-zinc-300 font-medium mb-1">
                    正在上传 {file?.name}
                  </p>
                  <div className="w-56 mx-auto mt-4">
                    <div className="w-full bg-zinc-800 rounded-full h-1.5 mb-2">
                      <div
                        className="bg-violet-600 h-1.5 rounded-full transition-all duration-200"
                        style={{ width: `${uploadPercent}%` }}
                      />
                    </div>
                    <p className="text-xs text-zinc-500">{uploadPercent}%</p>
                  </div>
                </>
              ) : (
                <>
                  <div className="w-14 h-14 rounded-2xl bg-zinc-800 flex items-center justify-center mx-auto mb-4">
                    <Mic className="w-7 h-7 text-zinc-500" strokeWidth={1.5} />
                  </div>
                  <p className="text-sm text-zinc-300 font-medium mb-1">
                    拖拽音频文件到此处
                  </p>
                  <p className="text-xs text-zinc-500 mb-4">或点击选择文件</p>
                  <div className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-zinc-800 border border-zinc-700 text-xs text-zinc-400">
                    <Upload className="w-3.5 h-3.5" />
                    选择音频文件
                  </div>
                </>
              )}
            </div>

            {asset && status !== "uploading" && (
              <div className="bg-zinc-900 border border-zinc-800 rounded-xl px-4 py-3 flex items-center gap-3">
                <div className="w-8 h-8 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                  <FileAudio className="w-4 h-4 text-emerald-400" />
                </div>
                <div className="min-w-0">
                  <p className="text-sm text-white truncate">{asset.name}</p>
                  <p className="text-xs text-zinc-500">
                    {formatSize(asset.size)} · 已上传
                  </p>
                </div>
                <button
                  onClick={() => setStep(2)}
                  className="ml-auto text-xs text-violet-400 hover:text-violet-300 cursor-pointer transition-colors"
                >
                  下一步 →
                </button>
              </div>
            )}

            <div className="bg-zinc-900 border border-zinc-800 rounded-xl p-4">
              <p className="text-xs font-semibold text-zinc-400 mb-3">
                录制建议
              </p>
              <ul className="space-y-2">
                {TIPS.map((tip) => (
                  <li
                    key={tip}
                    className="flex items-start gap-2 text-xs text-zinc-500"
                  >
                    <span className="w-1 h-1 rounded-full bg-violet-500 mt-1.5 shrink-0" />
                    {tip}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        )}

        {/* Step 2: Form */}
        {step === 2 && (
          <div className="space-y-5">
            <div className="bg-zinc-900 border border-zinc-800 rounded-xl px-4 py-3 flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                <CheckCircle2 className="w-4 h-4 text-emerald-400" />
              </div>
              <div className="min-w-0">
                <p className="text-sm text-white truncate">
                  {asset?.name ?? file?.name}
                </p>
                <p className="text-xs text-zinc-500">已上传，准备就绪</p>
              </div>
              <button
                onClick={() => setStep(1)}
                className="ml-auto text-xs text-zinc-500 hover:text-zinc-300 cursor-pointer transition-colors"
              >
                重新上传
              </button>
            </div>

            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                音色名称 <span className="text-red-400">*</span>
              </Label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="给你的音色取个名字"
                autoFocus
              />
            </div>

            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                描述（可选）
              </Label>
              <Input
                value={desc}
                onChange={(e) => setDesc(e.target.value)}
                placeholder="例：温柔女声，适合客服场景"
              />
            </div>

            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                复刻模型 <span className="text-red-400">*</span>
              </Label>
              {models.length === 0 ? (
                <p className="text-xs text-amber-400/90">
                  没有可用的复刻模型：请在「模型与供应商」中添加支持 voice_clone
                  的 TTS 模型，并配置供应商 API Key。
                </p>
              ) : (
                <Select
                  value={modelId}
                  onValueChange={(v) => setModelId(v ?? "")}
                >
                  <SelectTrigger>
                    <span className="text-left flex-1 truncate">
                      {selectedModel ? (
                        <>
                          {selectedModel.name}
                          <span className="text-zinc-500 ml-2 text-xs">
                            {selectedModel.provider_name}
                          </span>
                          {!selectedModel.configured && (
                            <span className="text-amber-400 ml-2 text-xs">
                              未配置 API Key
                            </span>
                          )}
                        </>
                      ) : (
                        <span className="text-zinc-500">选择复刻模型</span>
                      )}
                    </span>
                  </SelectTrigger>
                  <SelectContent>
                    {models.map((m) => (
                      <SelectItem
                        key={m.id}
                        value={m.id}
                        disabled={!m.configured}
                      >
                        <span>{m.name}</span>
                        <span className="text-zinc-500 ml-2 text-xs">
                          {m.provider_name}
                          {!m.configured && " · 未配置 API Key"}
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>

            {languages.length > 0 && (
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  语言（可选）
                </Label>
                <div className="flex flex-wrap gap-2">
                  {languages.map((l) => {
                    const active = langs.includes(l.code);
                    return (
                      <button
                        key={l.code}
                        type="button"
                        onClick={() => toggleLang(l.code)}
                        className={`px-2.5 py-1 rounded-lg text-xs border transition-all cursor-pointer ${
                          active
                            ? "bg-violet-600/20 border-violet-500/40 text-violet-300"
                            : "bg-zinc-900 border-zinc-700 text-zinc-400 hover:border-zinc-600 hover:text-zinc-200"
                        }`}
                      >
                        {l.name}
                      </button>
                    );
                  })}
                </div>
                <p className="text-[11px] text-zinc-600">
                  用于提示厂商参考音频的语言，可多选
                </p>
              </div>
            )}

            <div className="flex gap-3 pt-2">
              <Button
                variant="outline"
                onClick={() => setStep(1)}
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
              >
                上一步
              </Button>
              <Button
                onClick={startClone}
                disabled={!canClone}
                className="flex-1 bg-violet-600 hover:bg-violet-500 text-white gap-1.5 shadow-md shadow-violet-600/20"
              >
                <Wand2 className="w-4 h-4" />
                开始复刻
              </Button>
            </div>
          </div>
        )}

        {/* Step 3: Processing */}
        {step === 3 && (
          <div className="bg-zinc-900 border border-zinc-800 rounded-xl p-8 text-center">
            {status === "cloning" && (
              <>
                <div className="w-16 h-16 rounded-2xl bg-violet-600/10 flex items-center justify-center mx-auto mb-5">
                  <Loader2 className="w-8 h-8 text-violet-400 animate-spin" />
                </div>
                <p className="text-white font-medium mb-1">正在复刻中...</p>
                <p className="text-xs text-zinc-500">
                  正在向厂商提交参考音频并创建音色，请稍候
                </p>
              </>
            )}
            {status === "done" && (
              <>
                <div className="w-16 h-16 rounded-2xl bg-emerald-500/10 flex items-center justify-center mx-auto mb-5">
                  <CheckCircle2 className="w-8 h-8 text-emerald-400" />
                </div>
                <p className="text-white font-medium mb-1">复刻成功！</p>
                <p className="text-xs text-zinc-500 mb-6">
                  音色「{createdName}」已创建，可在已有音色中找到并绑定到智能体
                </p>
                <div className="flex gap-3 justify-center">
                  <Button
                    variant="outline"
                    onClick={reset}
                    className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1.5"
                  >
                    <RefreshCw className="w-4 h-4" />
                    再复刻一个
                  </Button>
                  <Button
                    onClick={() => navigate("/voice")}
                    className="bg-violet-600 hover:bg-violet-500 text-white"
                  >
                    查看已有音色
                  </Button>
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
