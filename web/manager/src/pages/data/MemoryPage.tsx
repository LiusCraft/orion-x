import { useEffect, useState, useRef, useCallback } from "react";
import {
  ArrowLeft,
  Brain,
  Trash2,
  Search,
  ChevronRight,
  ChevronDown,
  HardDrive,
} from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
  memoryApi,
  type AgentItem,
  type DeviceItem,
  type EntryItem,
} from "@/lib/api";

const PAGE_SIZE = 20;
const DEVICE_PAGE_SIZE = 20;
const SEARCH_DEBOUNCE_MS = 300;

const TARGET_MAP: Record<string, { label: string; cls: string }> = {
  memory: {
    label: "记忆",
    cls: "bg-violet-500/10 text-violet-400 border-violet-500/20",
  },
  user: {
    label: "用户",
    cls: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20",
  },
};

type SearchScope = "" | "agent" | "device" | "content";

const SCOPE_OPTIONS: { value: SearchScope; label: string; hint: string }[] = [
  { value: "", label: "关闭", hint: "" },
  { value: "agent", label: "智能体", hint: "搜索智能体名称..." },
  { value: "device", label: "设备", hint: "搜索设备名称或 ID..." },
  { value: "content", label: "记忆内容", hint: "搜索记忆内容..." },
];

function shortId(id: string) {
  return id.length > 8 ? id.slice(0, 8) + "…" : id;
}

function useDebouncedValue<T>(value: T, delay: number) {
  const [debouncedValue, setDebouncedValue] = useState(value);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedValue(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);

  return debouncedValue;
}

export default function MemoryPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const deviceId = searchParams.get("device_id")?.trim() || "";
  const [agents, setAgents] = useState<AgentItem[]>([]);
  const [agentTotal, setAgentTotal] = useState(0);
  const [agentPage, setAgentPage] = useState(1);
  const [loadingAgents, setLoadingAgents] = useState(true);

  const [expandedAgent, setExpandedAgent] = useState<string | null>(null);
  const [devices, setDevices] = useState<DeviceItem[]>([]);
  const [deviceTotal, setDeviceTotal] = useState(0);
  const [devicePage, setDevicePage] = useState(1);
  const [loadingDevices, setLoadingDevices] = useState(false);

  const [expandedDevice, setExpandedDevice] = useState<string | null>(null);
  const [entries, setEntries] = useState<EntryItem[]>([]);
  const [entryTotal, setEntryTotal] = useState(0);
  const [entryPage, setEntryPage] = useState(1);
  const [loadingEntries, setLoadingEntries] = useState(false);

  const [searchText, setSearchText] = useState("");
  const debouncedSearchText = useDebouncedValue(searchText, SEARCH_DEBOUNCE_MS);
  const [searchScope, setSearchScope] = useState<SearchScope>("");
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState<Set<string>>(new Set());

  // 用 ref 记住最新的搜索参数，避免闭包过期
  const searchRef = useRef({
    searchScope: "" as SearchScope,
    expandedAgent: null as string | null,
    expandedDevice: null as string | null,
  });
  searchRef.current = { searchScope, expandedAgent, expandedDevice };

  const currentHint =
    SCOPE_OPTIONS.find((o) => o.value === searchScope)?.hint || "";

  // ── 加载智能体 ──
  const loadAgents = useCallback(async (page = 1, q?: string) => {
    setLoadingAgents(true);
    try {
      const { data } = await memoryApi.listAgents({
        page,
        page_size: PAGE_SIZE,
        q: q || undefined,
      });
      setAgents(data.agents || []);
      setAgentTotal(data.total || 0);
      setAgentPage(page);
    } catch {
      setError("加载智能体列表失败");
    } finally {
      setLoadingAgents(false);
    }
  }, []);

  useEffect(() => {
    if (!deviceId) loadAgents();
  }, [deviceId, loadAgents]);

  // ── 加载设备 ──
  const loadDeviceList = useCallback(
    async (agentId: string, page: number, q?: string) => {
      setLoadingDevices(true);
      try {
        const { data } = await memoryApi.listDevices(agentId, {
          page,
          page_size: DEVICE_PAGE_SIZE,
          q: q || undefined,
        });
        setDevices(data.devices || []);
        setDeviceTotal(data.total || 0);
        setDevicePage(page);
      } catch {
        setError("加载设备列表失败");
      } finally {
        setLoadingDevices(false);
      }
    },
    [],
  );

  // ── 加载条目 ──
  const loadEntryList = useCallback(
    async (deviceId: string, page: number, q?: string) => {
      setLoadingEntries(true);
      try {
        const { data } = await memoryApi.listEntries(deviceId, {
          page,
          page_size: PAGE_SIZE,
          q: q || undefined,
        });
        setEntries(data.entries || []);
        setEntryTotal(data.total || 0);
        setEntryPage(page);
      } catch {
        setError("加载记忆条目失败");
      } finally {
        setLoadingEntries(false);
      }
    },
    [],
  );

  useEffect(() => {
    if (!deviceId) {
      setExpandedDevice(null);
      setSearchScope("");
      setSearchText("");
      return;
    }
    setExpandedDevice(deviceId);
    setSearchScope("content");
    loadEntryList(deviceId, 1);
  }, [deviceId, loadEntryList]);

  // ── 统一搜索：停止输入后再触发 ──
  useEffect(() => {
    const { searchScope, expandedAgent, expandedDevice } = searchRef.current;
    if (!searchScope) return;
    if (searchScope === "agent") {
      loadAgents(1, debouncedSearchText);
    } else if (searchScope === "device" && expandedAgent) {
      loadDeviceList(expandedAgent, 1, debouncedSearchText);
    } else if (searchScope === "content" && expandedDevice) {
      loadEntryList(expandedDevice, 1, debouncedSearchText);
    }
  }, [debouncedSearchText]); // eslint-disable-line react-hooks/exhaustive-deps

  // ── 展开/收起智能体 ──
  const toggleAgent = async (id: string) => {
    if (expandedAgent === id) {
      setExpandedAgent(null);
      setDevices([]);
      setExpandedDevice(null);
      return;
    }
    setExpandedAgent(id);
    setExpandedDevice(null);
    setDevicePage(1);
    const q = searchScope === "device" ? searchText || undefined : undefined;
    loadDeviceList(id, 1, q);
  };

  // ── 展开/收起设备 ──
  const toggleDevice = async (id: string) => {
    if (expandedDevice === id) {
      setExpandedDevice(null);
      return;
    }
    setExpandedDevice(id);
    setEntryPage(1);
    const q = searchScope === "content" ? searchText || undefined : undefined;
    loadEntryList(id, 1, q);
  };

  // ── 翻页 ──
  const toDevicePage = (page: number) => {
    if (!expandedAgent) return;
    const q = searchScope === "device" ? searchText || undefined : undefined;
    loadDeviceList(expandedAgent, page, q);
  };
  const toEntryPage = (page: number) => {
    if (!expandedDevice) return;
    const q = searchScope === "content" ? searchText || undefined : undefined;
    loadEntryList(expandedDevice, page, q);
  };

  // ── 删除 ──
  const handleDelete = async (id: string) => {
    if (!confirm("确认删除这条记忆？")) return;
    setDeleting((p) => new Set(p).add(id));
    try {
      await memoryApi.remove(id);
      setEntries((prev) => prev.filter((e) => e.id !== id));
      setEntryTotal((prev) => prev - 1);
    } catch {
      setError("删除失败");
    } finally {
      setDeleting((p) => {
        const n = new Set(p);
        n.delete(id);
        return n;
      });
    }
  };

  const agentPages = Math.max(1, Math.ceil(agentTotal / PAGE_SIZE));
  const devicePages = Math.max(1, Math.ceil(deviceTotal / DEVICE_PAGE_SIZE));
  const entryPages = Math.max(1, Math.ceil(entryTotal / PAGE_SIZE));

  const handleScopeChange = (v: SearchScope) => {
    setSearchScope(v);
    setSearchText("");
  };

  if (deviceId) {
    return (
      <div className="min-h-full">
        <div className="border-b border-zinc-800/80 px-8 py-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2.5">
                <button
                  type="button"
                  onClick={() => setSearchParams({})}
                  aria-label="返回完整记忆库"
                  title="返回完整记忆库"
                  className="p-1.5 -ml-1.5 rounded-md text-zinc-500 hover:text-zinc-200 hover:bg-zinc-800 transition-colors cursor-pointer"
                >
                  <ArrowLeft className="w-4 h-4" />
                </button>
                <h1 className="text-lg font-semibold text-white">设备记忆</h1>
              </div>
              <p className="text-xs text-zinc-500 font-mono mt-1 ml-8 break-all">
                {deviceId}
              </p>
            </div>
            <div className="flex items-center gap-2 text-sm text-zinc-500 shrink-0">
              <Brain className="w-4 h-4 text-zinc-600" />
              {entryTotal} 条记忆
            </div>
          </div>
          <div className="relative max-w-sm mt-4">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500 pointer-events-none" />
            <Input
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              placeholder="搜索该设备的记忆内容..."
              className="pl-9 h-9 text-sm "
            />
          </div>
        </div>

        <div className="px-8 py-6">
          {error && (
            <div className="flex items-center gap-2 text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-4 py-2.5 mb-4">
              <span>{error}</span>
              <Button
                onClick={() => {
                  setError("");
                  loadEntryList(deviceId, 1, searchText || undefined);
                }}
                className="ml-auto bg-red-500/20 hover:bg-red-500/30 text-red-400 h-7 px-3 text-xs"
              >
                重试
              </Button>
            </div>
          )}

          <div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
            {loadingEntries ? (
              <div className="flex justify-center py-20">
                <div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
              </div>
            ) : entries.length === 0 ? (
              <div className="flex flex-col items-center py-20">
                <Brain className="w-6 h-6 text-zinc-700 mb-3" />
                <p className="text-sm text-zinc-500">
                  {searchText ? "没有匹配的记忆条目" : "该设备暂无记忆条目"}
                </p>
              </div>
            ) : (
              <div>
                {entries.map((mem) => {
                  const target = TARGET_MAP[mem.target] || TARGET_MAP.memory;
                  return (
                    <div
                      key={mem.id}
                      className="flex items-start gap-3 px-5 py-3 hover:bg-zinc-800/20 transition-colors group border-b border-zinc-800/50 last:border-0"
                    >
                      <span
                        className={`shrink-0 mt-0.5 text-[10px] px-1.5 py-0.5 rounded border ${target.cls}`}
                      >
                        {target.label}
                      </span>
                      <span className="flex-1 text-sm text-zinc-300 leading-relaxed break-words min-w-0">
                        {mem.content}
                      </span>
                      <span className="shrink-0 text-[11px] text-zinc-600 font-mono mt-0.5 whitespace-nowrap">
                        {mem.created_at}
                      </span>
                      <button
                        type="button"
                        onClick={() => handleDelete(mem.id)}
                        disabled={deleting.has(mem.id)}
                        aria-label="删除记忆"
                        title="删除记忆"
                        className="shrink-0 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 text-zinc-600 hover:text-red-400 transition-all p-1 rounded hover:bg-red-400/10 disabled:opacity-40 cursor-pointer -mr-1"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  );
                })}
                {entryPages > 1 && (
                  <div className="flex items-center justify-end px-5 py-2.5 border-t border-zinc-800/50">
                    <Pagination
                      page={entryPage}
                      totalPages={entryPages}
                      onChange={(page) =>
                        loadEntryList(deviceId, page, searchText || undefined)
                      }
                    />
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">记忆库</h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              终端用户与智能体对话中积累的长期记忆
            </p>
          </div>
          <div className="flex items-center gap-2 text-sm text-zinc-500 shrink-0">
            <Brain className="w-4 h-4 text-zinc-600" />
            {agentTotal} 个智能体
          </div>
        </div>

        {/* 搜索 */}
        <div className="flex gap-2 mt-4 items-center">
          <div className="flex bg-zinc-900 border border-zinc-800 rounded-lg p-0.5">
            {SCOPE_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                onClick={() => handleScopeChange(opt.value)}
                className={`px-3 py-1.5 text-xs rounded-md transition-colors cursor-pointer whitespace-nowrap ${
                  searchScope === opt.value
                    ? "bg-violet-600 text-white"
                    : "text-zinc-400 hover:text-zinc-200"
                }`}
              >
                {opt.label}
              </button>
            ))}
          </div>
          {searchScope && (
            <div className="relative max-w-sm flex-1 min-w-0">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500 pointer-events-none" />
              <Input
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
                placeholder={currentHint}
                className="pl-9 h-9 text-sm "
              />
            </div>
          )}
          {searchScope === "device" && !expandedAgent && (
            <span className="text-xs text-zinc-500 shrink-0">
              请先展开一个智能体
            </span>
          )}
          {searchScope === "content" && !expandedDevice && (
            <span className="text-xs text-zinc-500 shrink-0">
              请先展开一个设备
            </span>
          )}
        </div>
      </div>

      <div className="px-8 py-6 space-y-4">
        {error && (
          <div className="flex items-center gap-2 text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-4 py-2.5">
            <span>{error}</span>
            <Button
              onClick={() => {
                setError("");
                loadAgents();
              }}
              className="ml-auto bg-red-500/20 hover:bg-red-500/30 text-red-400 h-7 px-3 text-xs"
            >
              重试
            </Button>
          </div>
        )}

        {loadingAgents && (
          <div className="flex justify-center py-20">
            <div className="w-5 h-5 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
          </div>
        )}

        {!loadingAgents && agents.length === 0 && (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Brain className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">
              {searchScope === "agent" && searchText
                ? "没有匹配的智能体"
                : "暂无智能体"}
            </p>
            <p className="text-zinc-600 text-xs mt-1">
              创建智能体后，终端用户对话产生的记忆将自动存储
            </p>
          </div>
        )}

        {!loadingAgents && agents.length > 0 && (
          <div className="space-y-2">
            {agents.map((agent) => {
              const agentExpanded = expandedAgent === agent.id;
              return (
                <div
                  key={agent.id}
                  className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden"
                >
                  <button
                    onClick={() => toggleAgent(agent.id)}
                    className="w-full flex items-center gap-2.5 px-5 py-3 hover:bg-zinc-800/30 transition-colors text-left cursor-pointer"
                  >
                    {agentExpanded ? (
                      <ChevronDown className="w-4 h-4 text-zinc-500 shrink-0" />
                    ) : (
                      <ChevronRight className="w-4 h-4 text-zinc-500 shrink-0" />
                    )}
                    <Brain className="w-4 h-4 text-violet-400 shrink-0" />
                    <span className="text-sm font-medium text-white">
                      {agent.name}
                    </span>
                    <span className="text-xs text-zinc-500">
                      {agent.total} 条
                    </span>
                    <span className="ml-auto text-xs text-zinc-600">
                      {agent.device_count} 设备
                    </span>
                  </button>

                  {agentExpanded && (
                    <div className="border-t border-zinc-800">
                      {loadingDevices && (
                        <div className="flex justify-center py-8">
                          <div className="w-4 h-4 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
                        </div>
                      )}

                      {!loadingDevices && devices.length === 0 && (
                        <div className="px-5 py-6 text-center text-xs text-zinc-600">
                          暂无设备
                        </div>
                      )}

                      {!loadingDevices && devices.length > 0 && (
                        <div>
                          {devices.map((device) => {
                            const devExpanded = expandedDevice === device.id;
                            return (
                              <div key={device.id}>
                                <button
                                  onClick={() => toggleDevice(device.id)}
                                  className="w-full flex items-center gap-2.5 px-5 py-2.5 hover:bg-zinc-800/15 transition-colors text-left cursor-pointer border-b border-zinc-800/40"
                                >
                                  {devExpanded ? (
                                    <ChevronDown className="w-3.5 h-3.5 text-zinc-600 shrink-0 ml-1" />
                                  ) : (
                                    <ChevronRight className="w-3.5 h-3.5 text-zinc-600 shrink-0 ml-1" />
                                  )}
                                  <HardDrive className="w-3.5 h-3.5 text-zinc-500 shrink-0" />
                                  <span className="text-sm text-zinc-300">
                                    {device.name || shortId(device.id)}
                                  </span>
                                  {device.name && (
                                    <span className="text-xs text-zinc-600 font-mono">
                                      {shortId(device.id)}
                                    </span>
                                  )}
                                  <span className="text-xs text-zinc-500">
                                    {device.total} 条
                                  </span>
                                </button>

                                {devExpanded && (
                                  <div>
                                    {loadingEntries ? (
                                      <div className="flex justify-center py-8">
                                        <div className="w-4 h-4 border-2 border-zinc-700 border-t-violet-500 rounded-full animate-spin" />
                                      </div>
                                    ) : entries.length === 0 ? (
                                      <div className="px-5 py-6 text-center text-xs text-zinc-600">
                                        暂无记忆条目
                                      </div>
                                    ) : (
                                      <div>
                                        {entries.map((mem) => {
                                          const t =
                                            TARGET_MAP[mem.target] ||
                                            TARGET_MAP.memory;
                                          return (
                                            <div
                                              key={mem.id}
                                              className="flex items-start gap-3 px-5 py-2.5 hover:bg-zinc-800/10 transition-colors group border-b border-zinc-800/20 last:border-0"
                                            >
                                              <span
                                                className={`shrink-0 mt-0.5 text-[10px] px-1.5 py-0.5 rounded border ${t.cls}`}
                                              >
                                                {t.label}
                                              </span>
                                              <span className="flex-1 text-sm text-zinc-300 leading-relaxed break-words min-w-0">
                                                {mem.content}
                                              </span>
                                              <span className="shrink-0 text-[11px] text-zinc-600 font-mono mt-0.5 whitespace-nowrap">
                                                {mem.created_at}
                                              </span>
                                              <button
                                                onClick={() =>
                                                  handleDelete(mem.id)
                                                }
                                                className="shrink-0 opacity-0 group-hover:opacity-100 text-zinc-600 hover:text-red-400 transition-all p-1 rounded hover:bg-red-400/10 cursor-pointer -mr-1"
                                              >
                                                <Trash2 className="w-3.5 h-3.5" />
                                              </button>
                                            </div>
                                          );
                                        })}
                                        {entryPages > 1 && (
                                          <div className="flex items-center justify-end gap-2 px-5 py-2 border-t border-zinc-800/30">
                                            <Pagination
                                              page={entryPage}
                                              totalPages={entryPages}
                                              onChange={toEntryPage}
                                            />
                                          </div>
                                        )}
                                      </div>
                                    )}
                                  </div>
                                )}
                              </div>
                            );
                          })}
                          {devicePages > 1 && (
                            <div className="flex items-center justify-end gap-2 px-5 py-2 border-t border-zinc-800/30">
                              <Pagination
                                page={devicePage}
                                totalPages={devicePages}
                                onChange={toDevicePage}
                              />
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
            {agentPages > 1 && (
              <div className="flex items-center justify-center gap-2 pt-2">
                <Pagination
                  page={agentPage}
                  totalPages={agentPages}
                  onChange={(p) => loadAgents(p)}
                />
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function Pagination({
  page,
  totalPages,
  onChange,
}: {
  page: number;
  totalPages: number;
  onChange: (p: number) => void;
}) {
  return (
    <div className="flex items-center gap-1.5">
      <button
        onClick={() => onChange(Math.max(1, page - 1))}
        disabled={page <= 1}
        className="px-2 py-1 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
      >
        上一页
      </button>
      <span className="text-xs text-zinc-500 px-1">
        {page} / {totalPages}
      </span>
      <button
        onClick={() => onChange(Math.min(totalPages, page + 1))}
        disabled={page >= totalPages}
        className="px-2 py-1 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
      >
        下一页
      </button>
    </div>
  );
}
