import { useState, useEffect } from "react";
import {
  Layers,
  Plus,
  Trash2,
  Edit2,
  CheckCircle2,
  Eye,
  EyeOff,
  Shield,
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
import { SimpleSelect } from "@/components/ui/select";
import {
  modelApi,
  providerApi,
  type AIModel,
  type ModelType,
  type Provider,
} from "@/lib/api";

const TYPE_BADGE: Record<string, string> = {
  text: "bg-violet-600/15 text-violet-400 border-violet-500/20",
  vision: "bg-blue-400/10 text-blue-400 border-blue-400/20",
  speech: "bg-emerald-400/10 text-emerald-400 border-emerald-400/20",
  multimodal: "bg-fuchsia-400/10 text-fuchsia-400 border-fuchsia-400/20",
  embedding: "bg-amber-400/10 text-amber-400 border-amber-400/20",
};

const TYPE_LABEL: Record<string, string> = {
  text: "文本",
  vision: "视觉",
  speech: "语音",
  multimodal: "全模态",
  embedding: "向量",
};

export default function MyModelsPage() {
  const [models, setModels] = useState<AIModel[]>([]);
  const [modelTypes, setModelTypes] = useState<ModelType[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);
  const [addOpen, setAddOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editModel, setEditModel] = useState<AIModel | null>(null);
  const [showKey, setShowKey] = useState(false);
  const [activeType, setActiveType] = useState<ModelType | "all">("all");
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<{
    provider_id: string;
    name: string;
    type: ModelType;
    base_url: string;
    model_id: string;
  }>({ provider_id: "", name: "", type: "text", base_url: "", model_id: "" });

  const load = () => {
    setLoading(true);
    Promise.all([modelApi.list(), providerApi.list(), modelApi.types()])
      .then(([mr, pr, tr]) => {
        setModels(mr.data);
        setProviders(pr.data);
        setModelTypes(tr.data);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    load();
  }, []);

  const openAdd = (type?: ModelType) => {
    setForm({
      provider_id: "",
      name: "",
      type: type ?? ((activeType === "all" ? "text" : activeType) as ModelType),
      base_url: "",
      model_id: "",
    });
    setShowKey(false);
    setAddOpen(true);
  };

  const openEdit = (model: AIModel) => {
    setEditModel(model);
    setForm({
      provider_id: model.provider_id,
      name: model.name,
      type: model.type,
      base_url: model.base_url ?? "",
      model_id: model.model_id,
    });
    setEditOpen(true);
  };

  const handleAdd = async () => {
    if (!form.name.trim() || !form.model_id.trim() || !form.provider_id) return;
    setSaving(true);
    try {
      await modelApi.create({
        provider_id: form.provider_id,
        name: form.name,
        type: form.type,
        base_url: form.base_url || undefined,
        model_id: form.model_id,
      });
      setAddOpen(false);
      load();
    } finally {
      setSaving(false);
    }
  };

  const handleEdit = async () => {
    if (!editModel || !form.name.trim() || !form.model_id.trim()) return;
    setSaving(true);
    try {
      await modelApi.update(editModel.id, {
        name: form.name,
        base_url: form.base_url || undefined,
        model_id: form.model_id,
      });
      setEditOpen(false);
      setEditModel(null);
      load();
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    await modelApi.remove(id);
    setModels((prev) => prev.filter((m) => m.id !== id));
  };

  const filtered =
    activeType === "all" ? models : models.filter((m) => m.type === activeType);

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">我的模型</h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              添加和管理自托管或第三方 AI 模型
            </p>
          </div>
          <Button
            onClick={() => openAdd()}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            添加模型
          </Button>
        </div>
      </div>

      <div className="px-8 py-6">
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <div className="w-6 h-6 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
          </div>
        ) : (
          <Tabs
            value={activeType}
            onValueChange={(v) => setActiveType(v as ModelType | "all")}
          >
            <TabsList className="bg-zinc-900 border border-zinc-800 h-9 p-0.5 mb-6 gap-0">
              <TabsTrigger
                value="all"
                className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
              >
                全部
                <span className="ml-1.5 text-[10px] text-zinc-600">
                  ({models.length})
                </span>
              </TabsTrigger>
              {modelTypes.map((value) => (
                <TabsTrigger
                  key={value}
                  value={value}
                  className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
                >
                  {TYPE_LABEL[value] ?? value}
                  <span className="ml-1.5 text-[10px] text-zinc-600">
                    ({models.filter((m) => m.type === value).length})
                  </span>
                </TabsTrigger>
              ))}
            </TabsList>

            {["all", ...modelTypes].map((value) => (
              <TabsContent key={value} value={value}>
                {filtered.length === 0 ? (
                  <div className="flex flex-col items-center py-20">
                    <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
                      <Layers className="w-6 h-6 text-zinc-600" />
                    </div>
                    <p className="text-zinc-400 text-sm">
                      还没有{TYPE_LABEL[value] ?? ""}模型
                    </p>
                    <p className="text-zinc-600 text-xs mt-1 mb-4">
                      添加兼容 OpenAI 接口的{TYPE_LABEL[value] ?? ""}模型
                    </p>
                    <Button
                      onClick={() =>
                        openAdd(
                          value === "all" ? undefined : (value as ModelType),
                        )
                      }
                      className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5"
                    >
                      <Plus className="w-3.5 h-3.5" />
                      添加{TYPE_LABEL[value] ?? ""}模型
                    </Button>
                  </div>
                ) : (
                  <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                    {filtered.map((model) => (
                      <div
                        key={model.id}
                        className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group"
                      >
                        <div className="flex items-start justify-between mb-3">
                          <div>
                            <div className="flex items-center gap-2">
                              <p className="font-medium text-sm text-white">
                                {model.name}
                              </p>
                              {model.is_system && (
                                <span className="flex items-center gap-0.5 text-[10px] px-1 py-0.5 rounded border bg-violet-600/15 text-violet-400 border-violet-500/20">
                                  <Shield className="w-2.5 h-2.5" />
                                </span>
                              )}
                            </div>
                            <div className="flex items-center gap-2 mt-1">
                              <span
                                className={`text-[10px] px-1.5 py-0.5 rounded border ${TYPE_BADGE[model.type]}`}
                              >
                                {TYPE_LABEL[model.type]}
                              </span>
                              {model.type === "speech" &&
                                (() => {
                                  const slug = model.provider?.slug ?? "";
                                  if (slug.startsWith("tts/"))
                                    return (
                                      <span className="text-[10px] px-1.5 py-0.5 rounded border bg-pink-400/10 text-pink-400 border-pink-400/20">
                                        TTS
                                      </span>
                                    );
                                  if (slug.startsWith("asr/"))
                                    return (
                                      <span className="text-[10px] px-1.5 py-0.5 rounded border bg-sky-400/10 text-sky-400 border-sky-400/20">
                                        ASR
                                      </span>
                                    );
                                  return null;
                                })()}
                              <span className="text-[11px] text-zinc-500">
                                {model.provider?.name}
                              </span>
                            </div>
                          </div>
                          {!model.is_system && (
                            <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                              <button
                                onClick={() => openEdit(model)}
                                className="text-zinc-500 hover:text-zinc-300 p-1.5 rounded hover:bg-zinc-800 cursor-pointer transition-colors"
                              >
                                <Edit2 className="w-3.5 h-3.5" />
                              </button>
                              <button
                                onClick={() => handleDelete(model.id)}
                                className="text-zinc-500 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 cursor-pointer transition-colors"
                              >
                                <Trash2 className="w-3.5 h-3.5" />
                              </button>
                            </div>
                          )}
                        </div>

                        <div className="space-y-1.5">
                          <div className="bg-zinc-800/60 rounded-lg px-3 py-2">
                            <p className="text-[10px] text-zinc-600 mb-0.5">
                              Model ID
                            </p>
                            <p className="text-xs text-zinc-300 font-mono truncate">
                              {model.model_id}
                            </p>
                          </div>
                          <div className="bg-zinc-800/60 rounded-lg px-3 py-2">
                            <p className="text-[10px] text-zinc-600 mb-0.5">
                              Base URL
                            </p>
                            <p className="text-xs text-zinc-500 font-mono truncate">
                              {model.base_url ||
                                model.provider?.base_url ||
                                "—"}
                            </p>
                          </div>
                        </div>

                        <div className="flex items-center justify-between mt-3">
                          <span className="flex items-center gap-1 text-[11px] text-emerald-400">
                            <CheckCircle2 className="w-3 h-3" />
                            可用
                          </span>
                          <span className="text-[11px] text-zinc-600 font-mono">
                            {model.created_at.slice(0, 10)}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </TabsContent>
            ))}
          </Tabs>
        )}
      </div>

      {/* 添加模型 Dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Layers className="w-4 h-4 text-violet-400" />
              添加模型
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
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
                  placeholder="模型别名"
                  className="text-sm "
                />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  类型
                </Label>
                <SimpleSelect
                  value={form.type}
                  onValueChange={(value) => {
                    const t = value as ModelType;
                    setForm((f) => ({ ...f, type: t, provider_id: "" }));
                  }}
                  options={modelTypes.map((value) => ({
                    value,
                    label: `${TYPE_LABEL[value] ?? value}模型`,
                  }))}
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                厂商
              </Label>
              <SimpleSelect
                value={form.provider_id}
                onValueChange={(providerID) =>
                  setForm((f) => ({ ...f, provider_id: providerID }))
                }
                placeholder="选择厂商"
                options={providers
                  .filter((p) => {
                    const isVoice =
                      p.slug.startsWith("tts/") || p.slug.startsWith("asr/");
                    return form.type === "speech" ? isVoice : !isVoice;
                  })
                  .map((provider) => ({
                    value: provider.id,
                    label: `${provider.name} · ${provider.slug}`,
                  }))}
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                Model ID
              </Label>
              <Input
                value={form.model_id}
                onChange={(e) =>
                  setForm((f) => ({ ...f, model_id: e.target.value }))
                }
                placeholder="claude-sonnet-4-6"
                className="text-sm font-mono "
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                Base URL{" "}
                <span className="text-zinc-600 normal-case ml-1">
                  （留空用厂商默认）
                </span>
              </Label>
              <Input
                value={form.base_url}
                onChange={(e) =>
                  setForm((f) => ({ ...f, base_url: e.target.value }))
                }
                placeholder={
                  providers.find((p) => p.id === form.provider_id)?.base_url ??
                  "https://..."
                }
                className="text-sm font-mono "
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                API Key{" "}
                <span className="text-zinc-600 normal-case ml-1">
                  （留空用厂商 Key）
                </span>
              </Label>
              <div className="relative">
                <Input
                  type={showKey ? "text" : "password"}
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
              onClick={handleAdd}
              disabled={
                saving ||
                !form.name.trim() ||
                !form.model_id.trim() ||
                !form.provider_id
              }
              className="bg-violet-600 hover:bg-violet-500 text-white"
            >
              添加模型
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑模型 Dialog */}
      <Dialog
        open={editOpen}
        onOpenChange={(v) => {
          if (!v) {
            setEditOpen(false);
            setEditModel(null);
          }
        }}
      >
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Edit2 className="w-4 h-4 text-violet-400" />
              编辑模型
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                名称
              </Label>
              <Input
                value={form.name}
                onChange={(e) =>
                  setForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="模型别名"
                className="text-sm "
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                Model ID
              </Label>
              <Input
                value={form.model_id}
                onChange={(e) =>
                  setForm((f) => ({ ...f, model_id: e.target.value }))
                }
                placeholder="claude-sonnet-4-6"
                className="text-sm font-mono "
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                Base URL{" "}
                <span className="text-zinc-600 normal-case ml-1">
                  （留空用厂商默认）
                </span>
              </Label>
              <Input
                value={form.base_url}
                onChange={(e) =>
                  setForm((f) => ({ ...f, base_url: e.target.value }))
                }
                placeholder={editModel?.provider?.base_url ?? "https://..."}
                className="text-sm font-mono "
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                厂商
              </Label>
              <p className="text-sm text-zinc-300 px-3 py-2 bg-zinc-800/60 rounded-md">
                {providers.find((p) => p.id === form.provider_id)?.name ??
                  form.provider_id}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                类型
              </Label>
              <p className="text-sm text-zinc-300 px-3 py-2 bg-zinc-800/60 rounded-md">
                {TYPE_LABEL[form.type] ?? form.type}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setEditOpen(false);
                setEditModel(null);
              }}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
            >
              取消
            </Button>
            <Button
              onClick={handleEdit}
              disabled={saving || !form.name.trim() || !form.model_id.trim()}
              className="bg-violet-600 hover:bg-violet-500 text-white"
            >
              保存修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
