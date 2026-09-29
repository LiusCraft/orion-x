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
  type BillingPrice,
  type ModelType,
  type Provider,
} from "@/lib/api";
import {
  loadPlatformPrices,
  modelCategory,
  modelPriceLines,
  type ModelPriceLine,
} from "@/lib/modelPrices";
import { useDocumentTitle } from "@/lib/title";
import { useAuthStore } from "@/lib/store";
import {
  ScopeFilter,
  ViewToggle,
  type ResourceScope,
  type ViewMode,
} from "@/pages/models/shared";

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

/** 官方模型的标记，和厂商管理页保持一致：图标 + 文字。 */
function OfficialBadge() {
  return (
    <span className="flex items-center gap-0.5 text-[10px] px-1.5 py-0.5 rounded border bg-violet-600/15 text-violet-400 border-violet-500/20 shrink-0">
      <Shield className="w-2.5 h-2.5" />
      官方
    </span>
  );
}

function TypeBadge({ type }: { type: ModelType }) {
  return (
    <span
      className={`text-[10px] px-1.5 py-0.5 rounded border shrink-0 ${TYPE_BADGE[type]}`}
    >
      {TYPE_LABEL[type]}
    </span>
  );
}

/** 语音类模型再标一层 ASR / TTS，价格口径和维度不同。 */
function VoiceCategoryBadge({ model }: { model: AIModel }) {
  const category = modelCategory(model);
  if (category === "tts")
    return (
      <span className="text-[10px] px-1.5 py-0.5 rounded border bg-pink-400/10 text-pink-400 border-pink-400/20 shrink-0">
        TTS
      </span>
    );
  if (category === "asr")
    return (
      <span className="text-[10px] px-1.5 py-0.5 rounded border bg-sky-400/10 text-sky-400 border-sky-400/20 shrink-0">
        ASR
      </span>
    );
  return null;
}

/** 价格行；null 表示这个模型没有可展示的计费项，调用方负责回落到「—」。 */
function PriceLines({ lines }: { lines: ModelPriceLine[] | null }) {
  if (lines === null) return <span className="text-zinc-600">—</span>;
  if (lines.length === 0) return <span className="text-zinc-600">未定价</span>;
  return (
    <div className="space-y-0.5">
      {lines.map((line) => (
        <p key={line.label} className="text-xs text-zinc-300 font-mono">
          <span className="font-sans text-zinc-500 mr-1">{line.label}</span>
          {line.text}
        </p>
      ))}
    </div>
  );
}

export default function MyModelsPage() {
  useDocumentTitle("我的模型");

  const isAdmin = useAuthStore((state) => state.isAdmin);
  const [models, setModels] = useState<AIModel[]>([]);
  const [modelTypes, setModelTypes] = useState<ModelType[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  // null = 价格拿不到（计费未启用 / 请求失败），空数组 = 价格表里一条都没配
  const [prices, setPrices] = useState<BillingPrice[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [addOpen, setAddOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editModel, setEditModel] = useState<AIModel | null>(null);
  const [showKey, setShowKey] = useState(false);
  const [activeType, setActiveType] = useState<ModelType | "all">("all");
  const [scope, setScope] = useState<ResourceScope>("all");
  const [viewMode, setViewMode] = useState<ViewMode>(() =>
    localStorage.getItem("modelViewMode") === "table" ? "table" : "grid",
  );
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<{
    provider_id: string;
    name: string;
    type: ModelType;
    base_url: string;
    model_id: string;
  }>({ provider_id: "", name: "", type: "text", base_url: "", model_id: "" });

  const toggleView = (mode: ViewMode) => {
    setViewMode(mode);
    localStorage.setItem("modelViewMode", mode);
  };

  const load = () => {
    setLoading(true);
    Promise.all([
      modelApi.list(),
      providerApi.list(),
      modelApi.types(),
      loadPlatformPrices(),
    ])
      .then(([mr, pr, tr, priceList]) => {
        setModels(mr.data);
        setProviders(pr.data);
        setModelTypes(tr.data);
        setPrices(priceList);
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

  // 页签上的计数跟着范围筛选走：选了「官方」时「文本」数的是官方文本模型。
  const scoped = models.filter((m) =>
    scope === "all" ? true : scope === "system" ? m.is_system : !m.is_system,
  );
  const filtered =
    activeType === "all" ? scoped : scoped.filter((m) => m.type === activeType);
  const priceLinesOf = (model: AIModel): ModelPriceLine[] | null =>
    prices ? modelPriceLines(model, prices) : null;
  const countOf = (type: ModelType) =>
    scoped.filter((m) => m.type === type).length;

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
            <div className="flex flex-wrap items-center justify-between gap-3 mb-6">
              <TabsList className="bg-zinc-900 border border-zinc-800 h-9 p-0.5 gap-0">
                <TabsTrigger
                  value="all"
                  className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
                >
                  全部
                  <span className="ml-1.5 text-[10px] text-zinc-600">
                    ({scoped.length})
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
                      ({countOf(value)})
                    </span>
                  </TabsTrigger>
                ))}
              </TabsList>
              <div className="flex items-center gap-2">
                <ScopeFilter value={scope} onChange={setScope} />
                <ViewToggle value={viewMode} onChange={toggleView} />
              </div>
            </div>

            {["all", ...modelTypes].map((value) => {
              const typeLabel = value === "all" ? "" : (TYPE_LABEL[value] ?? "");
              const emptyTitle =
                scope === "system"
                  ? `还没有官方${typeLabel}模型`
                  : scope === "mine"
                    ? `你还没有添加${typeLabel}模型`
                    : `还没有${typeLabel}模型`;
              return (
                <TabsContent key={value} value={value}>
                  {filtered.length === 0 ? (
                    <div className="flex flex-col items-center py-20">
                      <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
                        <Layers className="w-6 h-6 text-zinc-600" />
                      </div>
                      <p className="text-zinc-400 text-sm">{emptyTitle}</p>
                      <p className="text-zinc-600 text-xs mt-1 mb-4">
                        添加兼容 OpenAI 接口的{typeLabel}模型
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
                        添加{typeLabel}模型
                      </Button>
                    </div>
                  ) : viewMode === "grid" ? (
                    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                      {filtered.map((model) => {
                        const priceLines = priceLinesOf(model);
                        return (
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
                                  {model.is_system && <OfficialBadge />}
                                </div>
                                <div className="flex items-center gap-2 mt-1">
                                  <TypeBadge type={model.type} />
                                  {model.type === "speech" && (
                                    <VoiceCategoryBadge model={model} />
                                  )}
                                  <span className="text-[11px] text-zinc-500">
                                    {model.provider?.name}
                                  </span>
                                </div>
                              </div>
                              {(!model.is_system || isAdmin) && (
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
                              {priceLines !== null && (
                                <div className="bg-zinc-800/60 rounded-lg px-3 py-2">
                                  <p className="text-[10px] text-zinc-600 mb-0.5">
                                    价格
                                  </p>
                                  <PriceLines lines={priceLines} />
                                </div>
                              )}
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
                        );
                      })}
                    </div>
                  ) : (
                    <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-b border-zinc-800 text-left text-[11px] text-zinc-500 uppercase tracking-wide">
                            <th className="px-4 py-3 font-medium">模型</th>
                            <th className="px-4 py-3 font-medium">厂商</th>
                            <th className="px-4 py-3 font-medium">Model ID</th>
                            <th className="px-4 py-3 font-medium">价格</th>
                            <th className="px-4 py-3 font-medium">Base URL</th>
                            <th className="px-4 py-3 font-medium">创建时间</th>
                            <th className="px-4 py-3 font-medium w-20" />
                          </tr>
                        </thead>
                        <tbody>
                          {filtered.map((model) => (
                            <tr
                              key={model.id}
                              className="border-b border-zinc-800/60 hover:bg-zinc-800/40 transition-colors"
                            >
                              <td className="px-4 py-3">
                                <div className="flex items-center gap-2">
                                  <p className="text-sm text-white">
                                    {model.name}
                                  </p>
                                  {model.is_system && <OfficialBadge />}
                                </div>
                                <div className="flex items-center gap-1.5 mt-1">
                                  <TypeBadge type={model.type} />
                                  {model.type === "speech" && (
                                    <VoiceCategoryBadge model={model} />
                                  )}
                                </div>
                              </td>
                              <td className="px-4 py-3 text-xs text-zinc-400">
                                {model.provider?.name ?? "—"}
                              </td>
                              <td className="px-4 py-3 text-xs text-zinc-300 font-mono">
                                {model.model_id}
                              </td>
                              <td className="px-4 py-3">
                                <PriceLines lines={priceLinesOf(model)} />
                              </td>
                              <td className="px-4 py-3 text-xs text-zinc-500 font-mono">
                                <p className="max-w-56 truncate">
                                  {model.base_url ||
                                    model.provider?.base_url ||
                                    "—"}
                                </p>
                              </td>
                              <td className="px-4 py-3 text-xs text-zinc-600 font-mono">
                                {model.created_at.slice(0, 10)}
                              </td>
                              <td className="px-4 py-3">
                                {(!model.is_system || isAdmin) && (
                                  <div className="flex gap-1">
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
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </TabsContent>
              );
            })}
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
                      p.slug.startsWith("tts:") || p.slug.startsWith("asr:");
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
