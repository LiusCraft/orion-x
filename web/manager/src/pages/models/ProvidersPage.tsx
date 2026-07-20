import { useState, useEffect } from "react";
import {
  Building2,
  Plus,
  Trash2,
  Edit2,
  Eye,
  EyeOff,
  CheckCircle2,
  Shield,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SimpleSelect } from "@/components/ui/select";
import { providerApi, type Provider, type ProviderSlug } from "@/lib/api";
import { useAuthStore } from "@/lib/store";

const CATEGORY_LABEL: Record<string, string> = {
  llm: "LLM 大语言模型",
  asr: "ASR 语音识别",
  tts: "TTS 语音合成",
  embedding: "Embedding 向量",
};

export default function ProvidersPage() {
  const isAdmin = useAuthStore((state) => state.isAdmin);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [slugOptions, setSlugOptions] = useState<ProviderSlug[]>([]);
  const [loading, setLoading] = useState(true);
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Provider | null>(null);
  const [showKey, setShowKey] = useState(false);
  const [form, setForm] = useState({
    name: "",
    slug: "",
    base_url: "",
    api_key: "",
  });
  const [saving, setSaving] = useState(false);

  const load = () => {
    setLoading(true);
    Promise.all([providerApi.list(), providerApi.slugs()])
      .then(([pr, sr]) => {
        setProviders(pr.data);
        setSlugOptions(sr.data);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    load();
  }, []);

  const handleSlugChange = (slug: string) => {
    const opt = slugOptions.find((s) => s.slug === slug);
    setForm((f) => ({
      ...f,
      slug,
      name: f.name || opt?.name || "",
      base_url: f.base_url || opt?.base_url || "",
    }));
  };

  const openAdd = () => {
    setForm({ name: "", slug: "", base_url: "", api_key: "" });
    setEditTarget(null);
    setShowKey(false);
    setAddOpen(true);
  };

  const openEdit = (p: Provider) => {
    setForm({ name: p.name, slug: p.slug, base_url: p.base_url, api_key: "" });
    setEditTarget(p);
    setShowKey(false);
    setAddOpen(true);
  };

  const handleSave = async () => {
    if (!form.name.trim() || !form.slug.trim() || !form.base_url.trim()) return;
    setSaving(true);
    try {
      if (editTarget) {
        await providerApi.update(editTarget.id, {
          name: form.name,
          base_url: form.base_url,
          ...(form.api_key ? { api_key: form.api_key } : {}),
        });
      } else {
        await providerApi.create({
          name: form.name,
          slug: form.slug,
          base_url: form.base_url,
          api_key: form.api_key,
        });
      }
      setAddOpen(false);
      load();
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    await providerApi.remove(id);
    setProviders((prev) => prev.filter((p) => p.id !== id));
  };

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">厂商管理</h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              管理 AI 服务厂商的接入配置和 API Key
            </p>
          </div>
          <Button
            onClick={openAdd}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            添加厂商
          </Button>
        </div>
      </div>

      <div className="px-8 py-6">
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <div className="w-6 h-6 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
          </div>
        ) : providers.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Building2 className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">还没有厂商</p>
            <p className="text-zinc-600 text-xs mt-1 mb-4">
              添加厂商后即可在该厂商下创建模型
            </p>
            <Button
              onClick={openAdd}
              className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5"
            >
              <Plus className="w-3.5 h-3.5" />
              添加厂商
            </Button>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {providers.map((p) => (
              <div
                key={p.id}
                className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group"
              >
                <div className="flex items-start justify-between mb-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <p className="font-medium text-sm text-white">{p.name}</p>
                      {p.is_system && (
                        <span className="flex items-center gap-0.5 text-[10px] px-1.5 py-0.5 rounded border bg-violet-600/15 text-violet-400 border-violet-500/20">
                          <Shield className="w-2.5 h-2.5" />
                          官方
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-1.5 mt-1">
                      {(() => {
                        const [cat, vendor] = p.slug.split("/");
                        const catColor: Record<string, string> = {
                          llm: "bg-violet-600/15 text-violet-400 border-violet-500/20",
                          tts: "bg-pink-400/10 text-pink-400 border-pink-400/20",
                          asr: "bg-sky-400/10 text-sky-400 border-sky-400/20",
                          embedding:
                            "bg-amber-400/10 text-amber-400 border-amber-400/20",
                        };
                        return (
                          <>
                            <span
                              className={`text-[10px] px-1.5 py-0.5 rounded border font-mono uppercase ${catColor[cat] ?? "bg-zinc-700/40 text-zinc-400 border-zinc-600/30"}`}
                            >
                              {cat}
                            </span>
                            {vendor && (
                              <span className="text-[10px] px-1.5 py-0.5 rounded border bg-zinc-800 text-zinc-400 border-zinc-700 font-mono">
                                {vendor}
                              </span>
                            )}
                          </>
                        );
                      })()}
                    </div>
                  </div>
                  {!p.is_system && (
                    <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onClick={() => openEdit(p)}
                        className="text-zinc-500 hover:text-zinc-300 p-1.5 rounded hover:bg-zinc-800 cursor-pointer transition-colors"
                      >
                        <Edit2 className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => handleDelete(p.id)}
                        className="text-zinc-500 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 cursor-pointer transition-colors"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  )}
                  {p.is_system && isAdmin && (
                    <button
                      onClick={() => openEdit(p)}
                      className="opacity-0 group-hover:opacity-100 transition-opacity text-zinc-500 hover:text-zinc-300 p-1.5 rounded hover:bg-zinc-800 cursor-pointer"
                    >
                      <Edit2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>

                <div className="bg-zinc-800/60 rounded-lg px-3 py-2">
                  <p className="text-[10px] text-zinc-600 mb-0.5">Base URL</p>
                  <p className="text-xs text-zinc-400 font-mono truncate">
                    {p.base_url}
                  </p>
                </div>

                <div className="flex items-center justify-between mt-3">
                  <span className="flex items-center gap-1 text-[11px] text-emerald-400">
                    <CheckCircle2 className="w-3 h-3" />
                    已配置
                  </span>
                  <span className="text-[11px] text-zinc-600 font-mono">
                    {p.created_at.slice(0, 10)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Building2 className="w-4 h-4 text-violet-400" />
              {editTarget ? "编辑厂商" : "添加厂商"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                Slug
              </Label>
              {editTarget ? (
                <Input
                  value={form.slug}
                  disabled
                  className="text-sm font-mono disabled:opacity-40"
                />
              ) : (
                <SimpleSelect
                  value={form.slug}
                  onValueChange={handleSlugChange}
                  className="font-mono"
                  placeholder="选择厂商类型"
                  options={slugOptions.map((option) => ({
                    value: option.slug,
                    label: `${option.name} (${option.slug})`,
                    group: CATEGORY_LABEL[option.category] ?? option.category,
                  }))}
                />
              )}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  名称
                </Label>
                <Input
                  value={form.name}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, name: e.target.value }))
                  }
                  placeholder="Anthropic"
                  className="text-sm "
                />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  Base URL
                </Label>
                <Input
                  value={form.base_url}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, base_url: e.target.value }))
                  }
                  placeholder="https://api.anthropic.com"
                  className="text-sm font-mono "
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                API Key
                {editTarget && (
                  <span className="ml-1 text-zinc-600 normal-case">
                    (留空不修改)
                  </span>
                )}
              </Label>
              <div className="relative">
                <Input
                  type={showKey ? "text" : "password"}
                  value={form.api_key}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, api_key: e.target.value }))
                  }
                  placeholder="sk-••••••••"
                  className="text-sm font-mono pr-10"
                />
                <button
                  onClick={() => setShowKey((v) => !v)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300 cursor-pointer transition-colors"
                >
                  {showKey ? (
                    <EyeOff className="w-4 h-4" />
                  ) : (
                    <Eye className="w-4 h-4" />
                  )}
                </button>
              </div>
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
              onClick={handleSave}
              disabled={
                saving ||
                !form.name.trim() ||
                !form.slug.trim() ||
                !form.base_url.trim()
              }
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
