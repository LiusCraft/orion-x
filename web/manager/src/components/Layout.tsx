import { useState, useRef, useCallback } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/lib/store'
import { useTheme } from './ThemeProvider'
import {
  Bot, Store, Cpu, Puzzle, Zap, Brain, BookOpen, Database,
  Mic2, Wand2, Music, Activity, Layers, BarChart3, CreditCard,
  Key, Building2, Sun, Moon, Bell,
  Copy, Check, User, Wallet, ChevronRight, Shield,
} from 'lucide-react'

const NAV_GROUPS = [
  {
    label: '智能体',
    items: [
      { to: '/agents/plaza', icon: Store, label: '广场', end: false },
      { to: '/agents', icon: Bot, label: '我的智能体', end: true },
    ],
  },
  {
    label: '组件',
    items: [
      { to: '/components/mcp', icon: Cpu, label: 'MCP', end: false },
      { to: '/components/plugins', icon: Puzzle, label: '插件', end: false },
      { to: '/components/skills', icon: Zap, label: 'SKILL', end: false },
    ],
  },
  {
    label: '数据',
    items: [
      { to: '/data/memory', icon: Brain, label: '记忆库', end: false },
      { to: '/data/knowledge', icon: BookOpen, label: '知识库', end: false },
      { to: '/data/sources', icon: Database, label: '数据源', end: false },
    ],
  },
  {
    label: '语音',
    items: [
      { to: '/voice/plaza', icon: Music, label: '音色广场', end: false },
      { to: '/voice/clone', icon: Wand2, label: '语音复刻', end: false },
      { to: '/voice', icon: Mic2, label: '已有音色', end: true },
    ],
  },
  {
    label: '模型',
    items: [
      { to: '/models/providers', icon: Building2, label: '厂商管理', end: false },
      { to: '/models/monitor', icon: Activity, label: '模型监控', end: false },
      { to: '/models', icon: Layers, label: '我的模型', end: true },
    ],
  },
  {
    label: '计费',
    items: [
      { to: '/billing/usage', icon: BarChart3, label: '用量统计', end: false },
      { to: '/billing/resources', icon: CreditCard, label: '其他资源', end: false },
    ],
  },
]

function OrionIcon() {
  return (
    <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
    </svg>
  )
}

function CopyBtn({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)

  const handleClick = useCallback(() => {
    navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1200)
  }, [value])

  return (
    <button
      onClick={handleClick}
      className="p-0.5 text-zinc-600 hover:text-zinc-300 transition-colors cursor-pointer"
    >
      {copied ? (
        <Check className="w-3 h-3 text-emerald-400" strokeWidth={2} />
      ) : (
        <Copy className="w-3 h-3" strokeWidth={1.5} />
      )}
    </button>
  )
}

function PCard({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <div className={`px-4 py-3 ${className}`}>{children}</div>
}

function UserPopover({ username, userId, onLogout }: { username: string | null; userId: string | null; onLogout: () => void }) {
  const [open, setOpen] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const hoverRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()

  const enter = () => {
    if (timerRef.current) clearTimeout(timerRef.current)
    setOpen(true)
  }

  const leave = () => {
    timerRef.current = setTimeout(() => setOpen(false), 200)
  }

  return (
    <div ref={hoverRef} className="relative">
      <div
        className="flex items-center gap-2.5 ml-2 pl-3 border-l border-zinc-800/60 cursor-pointer"
        onMouseEnter={enter}
        onMouseLeave={leave}
      >
        <div className="w-6 h-6 rounded-full bg-violet-600/20 border border-violet-500/30 flex items-center justify-center shrink-0">
          <span className="text-[10px] font-semibold text-violet-400">{username?.[0]?.toUpperCase() ?? 'U'}</span>
        </div>
        <span className="text-xs text-zinc-400">{username}</span>
      </div>
      <div
        className="absolute right-0 top-full mt-2 w-64 rounded-2xl border border-zinc-800 bg-zinc-900 shadow-2xl shadow-black/50 z-50 overflow-hidden"
        style={{ display: open ? undefined : 'none' }}
        onMouseEnter={enter}
        onMouseLeave={leave}
      >
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-3 border-b border-zinc-800">
              <button
                onClick={() => { setOpen(false); navigate('/account') }}
                className="flex items-center gap-1.5 text-sm cursor-pointer"
              >
                <div className="w-6 h-6 rounded-full border border-zinc-700 flex items-center justify-center">
                  <User className="w-3.5 h-3.5 text-zinc-400" strokeWidth={1.5} />
                </div>
                <span className="text-white font-medium">账号</span>
                <ChevronRight className="w-3.5 h-3.5 text-zinc-600" strokeWidth={1.5} />
              </button>
              <button
                onClick={onLogout}
                className="text-xs text-red-400 border border-red-400/40 rounded-lg px-2.5 py-1 hover:text-red-300 hover:border-red-400 transition-colors cursor-pointer"
              >
                退出登录
              </button>
            </div>

            {/* Profile */}
            <PCard className="border-b border-zinc-800">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-violet-600/20 border border-violet-500/30 flex items-center justify-center shrink-0">
                  <span className="text-sm font-semibold text-violet-400">
                    {username?.[0]?.toUpperCase() ?? 'U'}
                  </span>
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-1">
                    <span className="text-sm font-medium text-white">{username}</span>
                    <CopyBtn value={username ?? ''} />
                  </div>
                  <div className="flex items-center gap-1">
                    <span className="text-xs text-zinc-500 truncate">{userId}</span>
                    <CopyBtn value={userId ?? ''} />
                  </div>
                </div>
              </div>
            </PCard>

            {/* 权限与安全 */}
            <PCard className="border-b border-zinc-800">
              <div className="flex items-center gap-1.5 mb-3">
                <Shield className="w-3.5 h-3.5 text-violet-400" strokeWidth={1.5} />
                <span className="text-sm font-semibold text-white flex-1">权限与安全</span>
              </div>
              <button
                onClick={() => { setOpen(false); navigate('/apikeys') }}
                className="flex items-center gap-2 w-full text-left text-xs text-violet-400 hover:text-violet-300 transition-colors cursor-pointer"
              >
                <Key className="w-3.5 h-3.5" strokeWidth={1.5} />
                API Keys
              </button>
            </PCard>

            {/* 费用与成本 */}
            <PCard>
              <button
                onClick={() => navigate('/billing/usage')}
                className="flex items-center gap-1.5 w-[calc(100%+32px)] -mx-4 px-4 py-1.5 -my-1.5 text-left transition-colors hover:bg-zinc-800 cursor-pointer"
              >
                <Wallet className="w-3.5 h-3.5 text-violet-400" strokeWidth={1.5} />
                <span className="text-sm font-semibold text-white flex-1">费用与成本</span>
                <ChevronRight className="w-3 h-3 text-zinc-600" strokeWidth={1.5} />
              </button>

              <div className="flex items-center gap-5 mt-3">
                <div>
                  <p className="text-[10px] text-zinc-500 mb-0.5">可用额度</p>
                  <p className="text-lg font-semibold text-white tracking-tight">¥ 4.64</p>
                </div>
                <button className="text-xs text-violet-400 hover:text-violet-300 mt-1 cursor-pointer">充值汇款</button>
              </div>
            </PCard>
          </div>
    </div>
  )
}

export default function Layout() {
  const { username, userId, logout } = useAuthStore()
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex flex-col h-screen bg-zinc-950 text-white overflow-hidden">
      {/* Top bar */}
      <header className="h-14 shrink-0 flex items-center justify-between px-6 border-b border-zinc-800/80 bg-zinc-950">
        <div className="flex items-center gap-2.5">
          <div className="w-7 h-7 rounded-lg bg-violet-600 flex items-center justify-center shadow-lg shadow-violet-600/25">
            <OrionIcon />
          </div>
          <span className="font-semibold text-sm tracking-tight">Orion-X</span>
          <span className="text-[10px] text-zinc-600 bg-zinc-900 border border-zinc-800 px-1.5 py-0.5 rounded font-mono">
            Manager
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button className="relative p-2 text-zinc-500 hover:text-zinc-200 transition-colors cursor-pointer rounded-lg hover:bg-zinc-800/70">
            <Bell className="w-5 h-5" strokeWidth={1.5} />
          </button>
          <button
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            className="p-2 text-zinc-500 hover:text-zinc-200 transition-colors cursor-pointer rounded-lg hover:bg-zinc-800/70"
            aria-label="切换主题"
          >
            {theme === 'dark' ? <Sun className="w-5 h-5" strokeWidth={1.5} /> : <Moon className="w-5 h-5" strokeWidth={1.5} />}
          </button>
          <UserPopover username={username} userId={userId} onLogout={handleLogout} />
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar */}
        <aside className="w-60 shrink-0 flex flex-col border-r border-zinc-800/80 bg-zinc-950">
          {/* Nav */}
          <nav className="flex-1 overflow-y-auto py-3 px-2 scrollbar-none">
            {NAV_GROUPS.map((group) => (
              <div key={group.label} className="mb-5">
                <p className="text-[10px] font-semibold text-zinc-600 uppercase tracking-widest px-2 mb-1.5">
                  {group.label}
                </p>
                {group.items.map(({ to, icon: Icon, label, end }) => (
                  <NavLink
                    key={to}
                    to={to}
                    end={end}
                    className={({ isActive }) =>
                      cn(
                        'flex items-center gap-2.5 px-2.5 py-[7px] rounded-lg text-sm transition-all duration-150 mb-0.5',
                        isActive
                          ? 'bg-violet-600 text-white font-medium shadow-md shadow-violet-600/20'
                          : 'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/70'
                      )
                    }
                  >
                    {({ isActive }) => (
                      <>
                        <Icon
                          className={cn('w-4 h-4 shrink-0', isActive ? 'text-violet-200' : 'text-zinc-500')}
                          strokeWidth={1.5}
                        />
                        {label}
                      </>
                    )}
                  </NavLink>
                ))}
              </div>
            ))}
          </nav>


        </aside>

        {/* Main content */}
        <main className="flex-1 overflow-y-auto bg-zinc-950">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
