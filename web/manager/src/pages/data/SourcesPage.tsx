import { useState } from "react";
import {
  Database,
  Plus,
  CheckCircle2,
  AlertCircle,
  Plug,
  Trash2,
  Edit2,
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

const DB_TYPES = [
  "PostgreSQL",
  "MySQL",
  "SQLite",
  "MongoDB",
  "Redis",
  "HTTP API",
];

const TYPE_ICONS: Record<string, { color: string; bg: string }> = {
  PostgreSQL: { color: "text-blue-400", bg: "bg-blue-400/10" },
  MySQL: { color: "text-orange-400", bg: "bg-orange-400/10" },
  MongoDB: { color: "text-emerald-400", bg: "bg-emerald-400/10" },
  Redis: { color: "text-red-400", bg: "bg-red-400/10" },
  SQLite: { color: "text-zinc-300", bg: "bg-zinc-700/50" },
  "HTTP API": { color: "text-violet-400", bg: "bg-violet-400/10" },
};

interface DataSource {
  id: string;
  name: string;
  type: string;
  host: string;
  status: "connected" | "disconnected" | "error";
  createdAt: string;
}

const MOCK_SOURCES: DataSource[] = [
  {
    id: "ds1",
    name: "生产数据库",
    type: "PostgreSQL",
    host: "db.prod.internal:5432/app_db",
    status: "connected",
    createdAt: "2025-06-01",
  },
  {
    id: "ds2",
    name: "用户分析库",
    type: "MySQL",
    host: "analytics.internal:3306/user_data",
    status: "connected",
    createdAt: "2025-06-15",
  },
  {
    id: "ds3",
    name: "缓存服务",
    type: "Redis",
    host: "redis.internal:6379",
    status: "disconnected",
    createdAt: "2025-06-20",
  },
  {
    id: "ds4",
    name: "ERP API",
    type: "HTTP API",
    host: "https://erp.company.com/api/v2",
    status: "error",
    createdAt: "2025-07-01",
  },
];

const STATUS_CFG = {
  connected: { label: "已连接", cls: "text-emerald-400", icon: CheckCircle2 },
  disconnected: { label: "未连接", cls: "text-zinc-500", icon: Plug },
  error: { label: "连接错误", cls: "text-red-400", icon: AlertCircle },
};

export default function SourcesPage() {
  const [sources, setSources] = useState<DataSource[]>(MOCK_SOURCES);
  const [addOpen, setAddOpen] = useState(false);
  const [form, setForm] = useState({
    name: "",
    type: "PostgreSQL",
    host: "",
    username: "",
    password: "",
  });

  const handleAdd = () => {
    if (!form.name.trim() || !form.host.trim()) return;
    setSources((prev) => [
      {
        id: `ds_${Date.now()}`,
        name: form.name,
        type: form.type,
        host: form.host,
        status: "disconnected",
        createdAt: new Date().toISOString().slice(0, 10),
      },
      ...prev,
    ]);
    setForm({
      name: "",
      type: "PostgreSQL",
      host: "",
      username: "",
      password: "",
    });
    setAddOpen(false);
  };

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-white">数据源</h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              连接外部数据库和 API，供智能体查询使用
            </p>
          </div>
          <Button
            onClick={() => setAddOpen(true)}
            className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
          >
            <Plus className="w-4 h-4" />
            添加数据源
          </Button>
        </div>
      </div>

      <div className="px-8 py-6">
        {sources.length === 0 ? (
          <div className="flex flex-col items-center py-20">
            <div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
              <Database className="w-6 h-6 text-zinc-600" />
            </div>
            <p className="text-zinc-400 text-sm">还没有数据源</p>
            <p className="text-zinc-600 text-xs mt-1 mb-4">
              连接数据库或 API 让智能体访问实时数据
            </p>
            <Button
              onClick={() => setAddOpen(true)}
              className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5"
            >
              <Plus className="w-3.5 h-3.5" />
              添加数据源
            </Button>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {sources.map((src) => {
              const { label, cls, icon: StatusIcon } = STATUS_CFG[src.status];
              const typeStyle = TYPE_ICONS[src.type] ?? TYPE_ICONS["HTTP API"];
              return (
                <div
                  key={src.id}
                  className="bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-700 transition-all group"
                >
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex items-center gap-3">
                      <div
                        className={`w-9 h-9 rounded-xl ${typeStyle.bg} flex items-center justify-center shrink-0`}
                      >
                        <Database
                          className={`w-4 h-4 ${typeStyle.color}`}
                          strokeWidth={1.5}
                        />
                      </div>
                      <div>
                        <p className="font-medium text-sm text-white">
                          {src.name}
                        </p>
                        <span
                          className={`text-[11px] ${typeStyle.color} bg-zinc-800 px-1.5 py-0.5 rounded font-mono`}
                        >
                          {src.type}
                        </span>
                      </div>
                    </div>
                    <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button className="text-zinc-500 hover:text-zinc-300 p-1 rounded hover:bg-zinc-800 cursor-pointer transition-colors">
                        <Edit2 className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() =>
                          setSources((prev) =>
                            prev.filter((s) => s.id !== src.id),
                          )
                        }
                        className="text-zinc-500 hover:text-red-400 p-1 rounded hover:bg-red-400/10 cursor-pointer transition-colors"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  <p className="text-xs text-zinc-600 font-mono truncate mb-3">
                    {src.host}
                  </p>

                  <div className="flex items-center justify-between">
                    <span className={`flex items-center gap-1 text-xs ${cls}`}>
                      <StatusIcon className="w-3 h-3" />
                      {label}
                    </span>
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-7 px-2.5 text-xs border-zinc-700 text-zinc-400 hover:bg-zinc-800 hover:text-white"
                    >
                      测试连接
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-white flex items-center gap-2">
              <Database className="w-4 h-4 text-violet-400" />
              添加数据源
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
                  placeholder="我的数据库"
                  className="text-sm "
                />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                  类型
                </Label>
                <SimpleSelect
                  value={form.type}
                  onValueChange={(type) => setForm((f) => ({ ...f, type }))}
                  options={DB_TYPES.map((type) => ({
                    value: type,
                    label: type,
                  }))}
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                连接地址
              </Label>
              <Input
                value={form.host}
                onChange={(e) =>
                  setForm((f) => ({ ...f, host: e.target.value }))
                }
                placeholder="host:port/database"
                className="text-sm font-mono "
              />
            </div>
            {form.type !== "HTTP API" && (
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                    用户名
                  </Label>
                  <Input
                    value={form.username}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, username: e.target.value }))
                    }
                    placeholder="username"
                    className="text-sm "
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs text-zinc-400 uppercase tracking-wide">
                    密码
                  </Label>
                  <Input
                    type="password"
                    value={form.password}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, password: e.target.value }))
                    }
                    placeholder="••••••"
                    className="text-sm "
                  />
                </div>
              </div>
            )}
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
              disabled={!form.name.trim() || !form.host.trim()}
              className="bg-violet-600 hover:bg-violet-500 text-white"
            >
              添加
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
