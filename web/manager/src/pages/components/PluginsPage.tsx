import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Puzzle,
  Plus,
  Globe,
  Trash2,
  Edit2,
  CheckCircle2,
  Lock,
  Zap,
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

const MARKET_PLUGINS = [
  {
    id: "p1",
    nameKey: "plugins.market.weather.name",
    icon: "🌤️",
    descKey: "plugins.market.weather.desc",
    tags: ["plugins.tags.tools", "plugins.tags.free"],
    installed: true,
  },
  {
    id: "p2",
    nameKey: "plugins.market.exchange.name",
    icon: "💱",
    descKey: "plugins.market.exchange.desc",
    tags: ["plugins.tags.finance", "plugins.tags.free"],
    installed: false,
  },
  {
    id: "p3",
    nameKey: "plugins.market.wikipedia.name",
    icon: "📖",
    descKey: "plugins.market.wikipedia.desc",
    tags: ["plugins.tags.knowledge", "plugins.tags.free"],
    installed: true,
  },
  {
    id: "p4",
    nameKey: "plugins.market.arxiv.name",
    icon: "📄",
    descKey: "plugins.market.arxiv.desc",
    tags: ["plugins.tags.academic", "plugins.tags.free"],
    installed: false,
  },
];

interface MyPlugin {
  id: string;
  name?: string;
  nameKey?: string;
  url: string;
  method: string;
  authType: string;
  desc?: string;
  descKey?: string;
  createdAt: string;
}

const MOCK_MY_PLUGINS: MyPlugin[] = [
  {
    id: "mp1",
    nameKey: "plugins.my.companyApi.name",
    url: "https://api.internal.com/v1/query",
    method: "POST",
    authType: "bearer",
    descKey: "plugins.my.companyApi.desc",
    createdAt: "2025-06-10",
  },
  {
    id: "mp2",
    nameKey: "plugins.my.stockQuery.name",
    url: "https://erp.company.com/api/stock",
    method: "GET",
    authType: "apiKey",
    descKey: "plugins.my.stockQuery.desc",
    createdAt: "2025-05-28",
  },
];

const METHODS = ["GET", "POST", "PUT", "DELETE"];
const AUTH_TYPES = ["none", "bearer", "apiKey", "basic"] as const;

export default function PluginsPage() {
  const { t } = useTranslation(["components", "common"]);
  const [installedIds, setInstalledIds] = useState<Set<string>>(
    new Set(["p1", "p3"]),
  );
  const [myPlugins, setMyPlugins] = useState<MyPlugin[]>(MOCK_MY_PLUGINS);
  const [addOpen, setAddOpen] = useState(false);
  const [form, setForm] = useState({
    name: "",
    url: "",
    method: "POST",
    authType: "none",
    authValue: "",
    desc: "",
  });

  const handleAdd = () => {
    if (!form.name.trim() || !form.url.trim()) return;
    const plugin: MyPlugin = {
      id: `mp_${Date.now()}`,
      name: form.name.trim(),
      url: form.url.trim(),
      method: form.method,
      authType: form.authType,
      desc: form.desc.trim(),
      createdAt: new Date().toISOString().slice(0, 10),
    };
    setMyPlugins((prev) => [plugin, ...prev]);
    setForm({
      name: "",
      url: "",
      method: "POST",
      authType: "none",
      authValue: "",
      desc: "",
    });
    setAddOpen(false);
  };

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">
              {t("plugins.title")}
            </h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              {t("plugins.subtitle")}
            </p>
          </div>
          <Button
            onClick={() => setAddOpen(true)}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            {t("plugins.addHttp")}
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
              {t("market")}
            </TabsTrigger>
            <TabsTrigger
              value="mine"
              className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
            >
              {t("plugins.tab.mine", { total: myPlugins.length })}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="market">
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {MARKET_PLUGINS.map((plugin) => {
                const isInstalled = installedIds.has(plugin.id);
                return (
                  <div
                    key={plugin.id}
                    className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all"
                  >
                    <div className="text-2xl mb-3">{plugin.icon}</div>
                    <p className="font-medium text-sm text-white mb-1">
                      {t(plugin.nameKey)}
                    </p>
                    <p className="text-xs text-zinc-500 leading-relaxed mb-3 line-clamp-2">
                      {t(plugin.descKey)}
                    </p>
                    <div className="flex flex-wrap gap-1 mb-4">
                      {plugin.tags.map((tag) => (
                        <span
                          key={tag}
                          className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-500 border border-zinc-700/50"
                        >
                          {t(tag)}
                        </span>
                      ))}
                    </div>
                    <Button
                      size="sm"
                      onClick={() =>
                        setInstalledIds((prev) => {
                          const n = new Set(prev);
                          if (n.has(plugin.id)) n.delete(plugin.id);
                          else n.add(plugin.id);
                          return n;
                        })
                      }
                      className={`h-7 w-full text-xs ${
                        isInstalled
                          ? "border-zinc-700 text-zinc-400 hover:text-red-400 hover:border-red-400/30 hover:bg-red-400/8"
                          : "bg-violet-600 hover:bg-violet-500 text-white"
                      }`}
                      variant={isInstalled ? "outline" : "default"}
                    >
                      {isInstalled ? (
                        <span className="flex items-center gap-1.5">
                          <CheckCircle2 className="w-3 h-3" />
                          {t("installed")}
                        </span>
                      ) : (
                        t("install")
                      )}
                    </Button>
                  </div>
                );
              })}
            </div>
          </TabsContent>

          <TabsContent value="mine">
            {myPlugins.length === 0 ? (
              <div className="flex flex-col items-center py-20">
                <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
                  <Puzzle className="w-6 h-6 text-zinc-600" />
                </div>
                <p className="text-zinc-400 text-sm">{t("plugins.mineEmpty")}</p>
                <p className="text-zinc-600 text-xs mt-1 mb-4">
                  {t("plugins.mineEmptyHint")}
                </p>
                <Button
                  onClick={() => setAddOpen(true)}
                  className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5"
                >
                  <Plus className="w-3.5 h-3.5" />
                  {t("plugins.addHttp")}
                </Button>
              </div>
            ) : (
              <div className="space-y-3">
                {myPlugins.map((p) => (
                  <div
                    key={p.id}
                    className="bg-zinc-900 border border-zinc-800 rounded-xl px-5 py-4 flex items-center gap-4 hover:border-zinc-700 transition-all group"
                  >
                    <div className="w-9 h-9 rounded-xl bg-zinc-800 border border-zinc-700 flex items-center justify-center shrink-0">
                      <Globe
                        className="w-4 h-4 text-zinc-400"
                        strokeWidth={1.5}
                      />
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-0.5">
                        <p className="font-medium text-sm text-white">
                          {p.nameKey ? t(p.nameKey) : p.name}
                        </p>
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 border border-zinc-700/50 text-zinc-400 font-mono">
                          {p.method}
                        </span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 border border-zinc-700/50 text-zinc-500 flex items-center gap-1">
                          <Lock className="w-2.5 h-2.5" />
                          {t(`plugins.auth.${p.authType}`)}
                        </span>
                      </div>
                      <p className="text-xs text-zinc-500 font-mono truncate">
                        {p.url}
                      </p>
                    </div>
                    <div className="flex gap-1.5 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button className="text-zinc-500 hover:text-zinc-300 p-1.5 rounded hover:bg-zinc-800 cursor-pointer transition-colors">
                        <Edit2 className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() =>
                          setMyPlugins((prev) =>
                            prev.filter((x) => x.id !== p.id),
                          )
                        }
                        className="text-zinc-500 hover:text-red-400 p-1.5 rounded hover:bg-red-400/10 cursor-pointer transition-colors"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                    <span className="text-[11px] text-zinc-600 shrink-0">
                      {p.createdAt}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>
        </Tabs>
      </div>

      {/* Add HTTP Plugin Dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Zap className="w-4 h-4 text-violet-400" />
              {t("plugins.dialog.title")}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-2 space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  {t("common:field.name")}
                </Label>
                <Input
                  value={form.name}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, name: e.target.value }))
                  }
                  placeholder={t("plugins.dialog.namePlaceholder")}
                  className="text-sm "
                />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  {t("plugins.method")}
                </Label>
                <SimpleSelect
                  value={form.method}
                  onValueChange={(method) => setForm((f) => ({ ...f, method }))}
                  options={METHODS.map((method) => ({
                    value: method,
                    label: method,
                  }))}
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                {t("plugins.apiUrl")}
              </Label>
              <Input
                value={form.url}
                onChange={(e) =>
                  setForm((f) => ({ ...f, url: e.target.value }))
                }
                placeholder="https://api.example.com/v1/endpoint"
                className="text-sm font-mono "
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  {t("plugins.authType")}
                </Label>
                <SimpleSelect
                  value={form.authType}
                  onValueChange={(authType) =>
                    setForm((f) => ({ ...f, authType }))
                  }
                  options={AUTH_TYPES.map((authType) => ({
                    value: authType,
                    label: t(`plugins.auth.${authType}`),
                  }))}
                />
              </div>
              {form.authType !== "none" && (
                <div className="space-y-1.5">
                  <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                    {form.authType === "bearer"
                      ? t("plugins.auth.tokenLabel")
                      : form.authType === "apiKey"
                        ? t("plugins.auth.apiKey")
                        : t("plugins.auth.passwordLabel")}
                  </Label>
                  <Input
                    type="password"
                    value={form.authValue}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, authValue: e.target.value }))
                    }
                    placeholder="••••••••"
                    className="text-sm "
                  />
                </div>
              )}
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                {t("plugins.dialog.descLabel")}
              </Label>
              <Input
                value={form.desc}
                onChange={(e) =>
                  setForm((f) => ({ ...f, desc: e.target.value }))
                }
                placeholder={t("plugins.dialog.descPlaceholder")}
                className="text-sm "
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setAddOpen(false)}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
            >
              {t("common:action.cancel")}
            </Button>
            <Button
              onClick={handleAdd}
              disabled={!form.name.trim() || !form.url.trim()}
              className="bg-violet-600 hover:bg-violet-500 text-white"
            >
              {t("plugins.dialog.submit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
