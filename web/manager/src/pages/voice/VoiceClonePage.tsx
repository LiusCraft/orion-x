import { useState } from "react";
import { Wand2, Upload, Mic, CheckCircle2, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type CloneStatus = "idle" | "uploading" | "processing" | "done" | "error";

const TIPS = [
  "录制环境应安静，无回声和背景音",
  "说话应自然清晰，语速适中",
  "音频时长建议 30 秒至 3 分钟",
  "支持 WAV、MP3、M4A 格式，最大 20MB",
];

export default function VoiceClonePage() {
  const [step, setStep] = useState(1);
  const [dragging, setDragging] = useState(false);
  const [fileName, setFileName] = useState("");
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [cloneStatus, setCloneStatus] = useState<CloneStatus>("idle");
  const [progress, setProgress] = useState(0);

  const handleFileSelect = () => {
    setFileName("my_voice_sample.wav");
    setStep(2);
  };

  const handleStartClone = () => {
    if (!name.trim()) return;
    setCloneStatus("processing");
    setStep(3);
    let p = 0;
    const timer = setInterval(() => {
      p += Math.random() * 15;
      if (p >= 100) {
        p = 100;
        clearInterval(timer);
        setCloneStatus("done");
      }
      setProgress(Math.min(p, 100));
    }, 400);
  };

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

        {/* Step 1: Upload */}
        {step === 1 && (
          <div className="space-y-4">
            <div
              onDragOver={(e) => {
                e.preventDefault();
                setDragging(true);
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={(e) => {
                e.preventDefault();
                setDragging(false);
                handleFileSelect();
              }}
              onClick={handleFileSelect}
              className={`border-2 border-dashed rounded-xl p-12 text-center cursor-pointer transition-all ${
                dragging
                  ? "border-violet-500 bg-violet-600/5"
                  : "border-zinc-700 hover:border-violet-500/50 hover:bg-zinc-900"
              }`}
            >
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
            </div>

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
              <div>
                <p className="text-sm text-white">{fileName}</p>
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

            <div className="flex gap-3 pt-2">
              <Button
                variant="outline"
                onClick={() => setStep(1)}
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
              >
                上一步
              </Button>
              <Button
                onClick={handleStartClone}
                disabled={!name.trim()}
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
            {cloneStatus === "processing" && (
              <>
                <div className="w-16 h-16 rounded-2xl bg-violet-600/10 flex items-center justify-center mx-auto mb-5">
                  <Loader2 className="w-8 h-8 text-violet-400 animate-spin" />
                </div>
                <p className="text-white font-medium mb-1">正在复刻中...</p>
                <p className="text-xs text-zinc-500 mb-6">
                  正在分析音频特征，提取声线模型，预计需要 1-3 分钟
                </p>
                <div className="w-full bg-zinc-800 rounded-full h-1.5 mb-2">
                  <div
                    className="bg-violet-600 h-1.5 rounded-full transition-all duration-300"
                    style={{ width: `${progress}%` }}
                  />
                </div>
                <p className="text-xs text-zinc-600">{Math.round(progress)}%</p>
              </>
            )}
            {cloneStatus === "done" && (
              <>
                <div className="w-16 h-16 rounded-2xl bg-emerald-500/10 flex items-center justify-center mx-auto mb-5">
                  <CheckCircle2 className="w-8 h-8 text-emerald-400" />
                </div>
                <p className="text-white font-medium mb-1">复刻成功！</p>
                <p className="text-xs text-zinc-500 mb-6">
                  音色「{name}」已创建，可在已有音色中找到
                </p>
                <div className="flex gap-3 justify-center">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setStep(1);
                      setCloneStatus("idle");
                      setProgress(0);
                      setName("");
                      setDesc("");
                      setFileName("");
                    }}
                    className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
                  >
                    再复刻一个
                  </Button>
                  <Button className="bg-violet-600 hover:bg-violet-500 text-white">
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
