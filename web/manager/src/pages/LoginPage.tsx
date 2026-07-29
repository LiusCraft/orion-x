import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Eye, EyeOff, Mic, Cpu, Zap, Globe } from "lucide-react";
import { authApi } from "@/lib/api";
import { useAuthStore } from "@/lib/store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const FEATURES = [
  { icon: Mic, title: "多路语音会话", desc: "每个设备独立会话，互不干扰" },
  {
    icon: Cpu,
    title: "灵活 LLM 接入",
    desc: "支持 OpenAI 兼容接口，自由切换模型",
  },
  {
    icon: Zap,
    title: "实时流式响应",
    desc: "VAD 检测 + 流式 ASR/TTS，延迟极低",
  },
  {
    icon: Globe,
    title: "MCP 工具扩展",
    desc: "通过 MCP 协议接入外部工具与服务",
  },
];

export default function LoginPage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [username, setUsername] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const { setAuth } = useAuthStore();
  const navigate = useNavigate();

  // Handle GitHub OAuth redirect — read token/error from URL params
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get("token");
    const userId = params.get("user_id");
    const usernameParam = params.get("username");
    const errorParam = params.get("error");
    if (token && userId) {
      setAuth(token, userId, usernameParam || "", false);
      // Clean URL
      window.history.replaceState({}, document.title, window.location.pathname);
      navigate("/agents");
    } else if (errorParam) {
      setError(decodeURIComponent(errorParam));
      // Clean URL
      window.history.replaceState({}, document.title, window.location.pathname);
    }
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      let res;
      if (mode === "login") {
        res = await authApi.login(email, password);
      } else {
        res = await authApi.register(email, password, username || undefined);
      }
      const { data } = res;
      setAuth(data.token, data.user_id, data.username, data.is_admin);
      navigate("/agents");
    } catch (err: unknown) {
      if (err && typeof err === "object" && "response" in err) {
        const axiosErr = err as { response?: { data?: { error?: string } } };
        setError(axiosErr.response?.data?.error || (mode === "login" ? "邮箱或密码错误" : "注册失败"));
      } else {
        setError(mode === "login" ? "邮箱或密码错误" : "注册失败");
      }
    } finally {
      setLoading(false);
    }
  };

  const handleGithubLogin = () => {
    window.location.href = authApi.githubLoginUrl();
  };

  return (
    <div className="min-h-screen bg-zinc-950 flex">
      {/* Grid background */}
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          backgroundImage:
            "linear-gradient(rgba(124,58,237,0.045) 1px, transparent 1px)," +
            "linear-gradient(90deg, rgba(124,58,237,0.045) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
        }}
      />
      <div className="absolute top-1/2 left-1/4 -translate-y-1/2 w-120 h-120 bg-violet-600/10 rounded-full blur-3xl pointer-events-none" />

      {/* ── Left panel (branding) ── */}
      <div className="hidden lg:flex w-1/2 items-center justify-center p-16 relative">
        <div className="w-full max-w-sm">
          <div className="flex items-center gap-3 mb-12">
            <div className="w-11 h-11 rounded-2xl bg-violet-600 flex items-center justify-center shrink-0 shadow-lg shadow-violet-600/30">
              <svg
                className="w-6 h-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z"
                />
              </svg>
            </div>
            <span className="text-xl font-semibold text-white tracking-tight">
              Orion-X
            </span>
          </div>

          <p className="text-xs font-semibold text-violet-400 uppercase tracking-widest mb-3">
            AI Voice Platform
          </p>
          <h2 className="text-[2.1rem] font-semibold text-white leading-tight mb-4">
            为你的设备注入语音智能
          </h2>
          <p className="text-zinc-400 text-sm leading-relaxed mb-10">
            开源 AI 语音机器人框架，支持多设备管理、可插拔 LLM
            和低延迟流式语音对话。
          </p>

          <div className="space-y-5">
            {FEATURES.map(({ icon: Icon, title, desc }) => (
              <div key={title} className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-xl bg-zinc-800 border border-zinc-700/60 flex items-center justify-center shrink-0">
                  <Icon className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
                </div>
                <div>
                  <p className="text-sm font-medium text-white leading-snug">
                    {title}
                  </p>
                  <p className="text-xs text-zinc-500">{desc}</p>
                </div>
              </div>
            ))}
          </div>

          <p className="text-xs text-zinc-700 mt-14">
            © 2025 Orion-X · MIT License
          </p>
        </div>
      </div>

      {/* ── Right panel (form) ── */}
      <div className="flex flex-1 items-center justify-center p-8 relative">
        <div className="w-full max-w-sm">
          {/* Mobile logo */}
          <div className="flex items-center gap-3 mb-8 lg:hidden">
            <div className="w-10 h-10 rounded-2xl bg-violet-600 flex items-center justify-center shadow-lg shadow-violet-600/30">
              <svg
                className="w-5 h-5 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z"
                />
              </svg>
            </div>
            <span className="text-xl font-semibold text-white">Orion-X</span>
          </div>

          <div className="mb-7">
            <h1 className="text-2xl font-semibold text-white">
              {mode === "login" ? "欢迎回来" : "创建账号"}
            </h1>
            <p className="text-sm text-zinc-500 mt-1">
              {mode === "login"
                ? "登录到管理控制台"
                : "注册一个管理控制台账号"}
            </p>
          </div>

          <div className="bg-zinc-900/70 border border-zinc-800 rounded-2xl p-6 backdrop-blur-sm shadow-xl shadow-black/30">
            <form onSubmit={handleSubmit} className="space-y-5">
              <div className="space-y-1.5">
                <label
                  htmlFor="email"
                  className="block text-xs font-medium text-zinc-400 uppercase tracking-wider"
                >
                  邮箱
                </label>
                <Input
                  id="email"
                  type="email"
                  autoComplete="email"
                  autoFocus
                  value={email}
                  onChange={(e) => {
                    setError("");
                    setEmail(e.target.value);
                  }}
                  className="h-11 transition-[border-color,box-shadow] duration-150"
                  placeholder="admin@example.com"
                  required
                />
              </div>

              {mode === "register" && (
                <div className="space-y-1.5">
                  <label
                    htmlFor="username"
                    className="block text-xs font-medium text-zinc-400 uppercase tracking-wider"
                  >
                    昵称（选填）
                  </label>
                  <Input
                    id="username"
                    autoComplete="name"
                    value={username}
                    onChange={(e) => {
                      setError("");
                      setUsername(e.target.value);
                    }}
                    className="h-11 transition-[border-color,box-shadow] duration-150"
                    placeholder="你的昵称"
                  />
                </div>
              )}

              <div className="space-y-1.5">
                <label
                  htmlFor="password"
                  className="block text-xs font-medium text-zinc-400 uppercase tracking-wider"
                >
                  密码
                </label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    autoComplete={mode === "login" ? "current-password" : "new-password"}
                    value={password}
                    onChange={(e) => {
                      setError("");
                      setPassword(e.target.value);
                    }}
                    className="h-11 pr-11 transition-[border-color,box-shadow] duration-150"
                    placeholder="••••••••"
                    required
                    minLength={6}
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((v) => !v)}
                    tabIndex={-1}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-200 transition-colors duration-150 cursor-pointer"
                    aria-label={showPassword ? "隐藏密码" : "显示密码"}
                  >
                    {showPassword ? (
                      <EyeOff className="w-4.5 h-4.5" />
                    ) : (
                      <Eye className="w-4.5 h-4.5" />
                    )}
                  </button>
                </div>
              </div>

              {error && (
                <div
                  role="alert"
                  className="flex items-center gap-2 text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-3 py-2.5"
                >
                  <svg
                    className="w-3.5 h-3.5 shrink-0"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    strokeWidth={2}
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126z"
                    />
                  </svg>
                  {error}
                </div>
              )}

              <Button
                type="submit"
                className="w-full bg-violet-600 hover:bg-violet-500 active:scale-[0.98] text-white h-11 font-medium transition-all duration-150 cursor-pointer shadow-md shadow-violet-600/20 mt-1"
                disabled={loading}
              >
                {loading ? (
                  <span className="flex items-center gap-2">
                    <svg
                      className="w-4 h-4 animate-spin"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        className="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        strokeWidth="4"
                      />
                      <path
                        className="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                      />
                    </svg>
                    {mode === "login" ? "登录中..." : "注册中..."}
                  </span>
                ) : mode === "login" ? (
                  "登录"
                ) : (
                  "注册"
                )}
              </Button>
            </form>

            {/* Divider */}
            <div className="relative my-5">
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-zinc-800" />
              </div>
              <div className="relative flex justify-center text-xs">
                <span className="bg-zinc-900/70 px-2 text-zinc-500">或</span>
              </div>
            </div>

            {/* GitHub OAuth button */}
            <button
              type="button"
              onClick={handleGithubLogin}
              className="w-full flex items-center justify-center gap-2.5 h-11 rounded-xl border border-zinc-700 bg-zinc-800/50 hover:bg-zinc-800 active:scale-[0.98] text-zinc-300 hover:text-white text-sm font-medium transition-all duration-150 cursor-pointer"
            >
              <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
              </svg>
              使用 GitHub 登录
            </button>

            {/* Toggle mode */}
            <p className="text-center text-xs text-zinc-500 mt-5">
              {mode === "login" ? (
                <>
                  还没有账号？{" "}
                  <button
                    type="button"
                    onClick={() => {
                      setMode("register");
                      setError("");
                    }}
                    className="text-violet-400 hover:text-violet-300 cursor-pointer"
                  >
                    注册
                  </button>
                </>
              ) : (
                <>
                  已有账号？{" "}
                  <button
                    type="button"
                    onClick={() => {
                      setMode("login");
                      setError("");
                    }}
                    className="text-violet-400 hover:text-violet-300 cursor-pointer"
                  >
                    登录
                  </button>
                </>
              )}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
