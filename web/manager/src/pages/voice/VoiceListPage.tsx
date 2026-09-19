import { useEffect, useRef, useState } from "react";
import { Mic2, Play, Pause, Trash2, Plus, Loader2, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useNavigate } from "react-router-dom";
import { voiceApi, type ModelVoice } from "@/lib/api";

const GRADIENTS = [
  "from-pink-500 to-rose-600",
  "from-blue-500 to-indigo-600",
  "from-violet-500 to-purple-600",
  "from-emerald-500 to-teal-600",
  "from-amber-500 to-orange-600",
  "from-fuchsia-500 to-pink-600",
];

const GENDER_LABELS: Record<string, string> = {
  female: "女声",
  male: "男声",
  neutral: "中性",
};

function gradientFor(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i += 1) {
    hash = (hash * 31 + id.charCodeAt(i)) % GRADIENTS.length;
  }
  return GRADIENTS[hash];
}

function errorMessage(err: unknown, fallback: string): string {
  return (
    (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
    fallback
  );
}

export default function VoiceListPage() {
  const navigate = useNavigate();
  const audioRef = useRef<HTMLAudioElement | null>(null);

  const [myVoices, setMyVoices] = useState<ModelVoice[]>([]);
  const [systemVoices, setSystemVoices] = useState<ModelVoice[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [playing, setPlaying] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    Promise.all([voiceApi.listSystem(), voiceApi.listMine()])
      .then(([systemRes, mineRes]) => {
        setSystemVoices(systemRes.data);
        setMyVoices(mineRes.data);
      })
      .catch((err) => setError(errorMessage(err, "加载音色失败")))
      .finally(() => setLoading(false));
  }, []);

  const togglePlay = (voice: ModelVoice) => {
    const audio = audioRef.current;
    if (!audio || !voice.preview_url) return;
    if (playing === voice.id) {
      audio.pause();
      setPlaying(null);
      return;
    }
    audio.src = voice.preview_url;
    void audio
      .play()
      .then(() => setPlaying(voice.id))
      .catch(() => setPlaying(null));
  };

  const handleDelete = async (voice: ModelVoice) => {
    if (!window.confirm(`删除音色「${voice.name}」？参考音频也会一并删除。`)) {
      return;
    }
    setDeletingId(voice.id);
    setError("");
    try {
      await voiceApi.remove(voice.model_id, voice.id);
      setMyVoices((prev) => prev.filter((v) => v.id !== voice.id));
    } catch (err) {
      setError(errorMessage(err, "删除失败，请重试"));
    } finally {
      setDeletingId(null);
    }
  };

  const VoiceCard = ({ voice }: { voice: ModelVoice }) => {
    const style = voice.tags?.[0];
    const gender = voice.gender ? GENDER_LABELS[voice.gender] : undefined;
    const canPreview = Boolean(voice.preview_url);
    return (
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group">
        <div className="flex items-center gap-3 mb-3">
          <div
            className={`w-12 h-12 rounded-xl bg-gradient-to-br ${gradientFor(voice.id)} flex items-center justify-center text-xl font-bold text-white shadow-lg`}
          >
            {voice.name[0]}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <p className="font-semibold text-sm text-white truncate">
                {voice.name}
              </p>
              {voice.is_cloned && (
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-600/15 text-violet-400 border border-violet-500/20 shrink-0">
                  复刻
                </span>
              )}
            </div>
            <div className="flex items-center gap-1.5 mt-0.5">
              {gender && (
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">
                  {gender}
                </span>
              )}
              {style && (
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50">
                  {style}
                </span>
              )}
            </div>
          </div>
          {!voice.is_system && (
            <button
              onClick={() => handleDelete(voice)}
              disabled={deletingId === voice.id}
              title="删除音色"
              className="opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 transition-all cursor-pointer disabled:opacity-50"
            >
              {deletingId === voice.id ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <Trash2 className="w-3.5 h-3.5" />
              )}
            </button>
          )}
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => togglePlay(voice)}
            disabled={!canPreview}
            title={canPreview ? "试听" : "该音色暂无试听音频"}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs border transition-all ${
              !canPreview
                ? "bg-zinc-900 border-zinc-800 text-zinc-700 cursor-not-allowed"
                : playing === voice.id
                  ? "bg-violet-600/20 border-violet-500/30 text-violet-400 cursor-pointer"
                  : "bg-zinc-800 border-zinc-700 text-zinc-400 hover:border-zinc-600 hover:text-zinc-200 cursor-pointer"
            }`}
          >
            {playing === voice.id ? (
              <Pause className="w-3 h-3" />
            ) : (
              <Play className="w-3 h-3" />
            )}
            {playing === voice.id ? "停止" : "试听"}
          </button>
          <p className="ml-auto text-[11px] text-zinc-600 self-center">
            {voice.created_at?.slice(0, 10)}
          </p>
        </div>
      </div>
    );
  };

  return (
    <div className="min-h-full">
      <audio ref={audioRef} onEnded={() => setPlaying(null)} className="hidden" />
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">已有音色</h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              系统音色和你复刻的自定义音色
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => navigate("/voice/plaza")}
              className="h-9 px-4 text-sm border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1.5"
            >
              <Plus className="w-4 h-4" />
              从广场添加
            </Button>
            <Button
              onClick={() => navigate("/voice/clone")}
              className="h-9 px-4 text-sm bg-violet-600 hover:bg-violet-500 text-white gap-1.5 shadow-md shadow-violet-600/20"
            >
              <Mic2 className="w-4 h-4" />
              复刻新音色
            </Button>
          </div>
        </div>
      </div>

      <div className="px-8 py-6 space-y-8">
        {error && (
          <div className="flex items-start gap-2 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3">
            <AlertCircle className="w-4 h-4 text-red-400 mt-0.5 shrink-0" />
            <p className="text-xs text-red-300">{error}</p>
          </div>
        )}

        {loading ? (
          <div className="flex items-center justify-center py-16 text-zinc-600 gap-2">
            <Loader2 className="w-4 h-4 animate-spin" />
            <span className="text-sm">加载中...</span>
          </div>
        ) : (
          <>
            <div>
              <h2 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4">
                我的复刻音色（{myVoices.length}）
              </h2>
              {myVoices.length === 0 ? (
                <div className="bg-zinc-900 border border-zinc-800 border-dashed rounded-xl p-8 text-center">
                  <Mic2
                    className="w-8 h-8 text-zinc-700 mx-auto mb-3"
                    strokeWidth={1.5}
                  />
                  <p className="text-zinc-500 text-sm">还没有复刻的音色</p>
                  <button
                    onClick={() => navigate("/voice/clone")}
                    className="text-xs text-violet-400 hover:text-violet-300 mt-1.5 cursor-pointer"
                  >
                    去复刻 →
                  </button>
                </div>
              ) : (
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                  {myVoices.map((v) => (
                    <VoiceCard key={v.id} voice={v} />
                  ))}
                </div>
              )}
            </div>

            <div>
              <h2 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4">
                系统音色（{systemVoices.length}）
              </h2>
              {systemVoices.length === 0 ? (
                <div className="bg-zinc-900 border border-zinc-800 border-dashed rounded-xl p-8 text-center">
                  <p className="text-zinc-500 text-sm">还没有添加系统音色</p>
                  <button
                    onClick={() => navigate("/voice/plaza")}
                    className="text-xs text-violet-400 hover:text-violet-300 mt-1.5 cursor-pointer"
                  >
                    去广场选择 →
                  </button>
                </div>
              ) : (
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                  {systemVoices.map((v) => (
                    <VoiceCard key={v.id} voice={v} />
                  ))}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
