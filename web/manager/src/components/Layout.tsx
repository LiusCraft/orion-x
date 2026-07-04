import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/lib/store'
import {
  Bot, Store, Cpu, Puzzle, Zap, Brain, BookOpen, Database,
  Mic2, Wand2, Music, Activity, Layers, BarChart3, CreditCard,
  Key, Link2, LogOut, Building2,
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

export default function Layout() {
  const { username, logout } = useAuthStore()
  const navigate = useNavigate()

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex h-screen bg-zinc-950 text-white overflow-hidden">
      {/* Sidebar */}
      <aside className="w-60 shrink-0 flex flex-col border-r border-zinc-800/80 bg-zinc-950">
        {/* Logo */}
        <div className="h-14 flex items-center gap-2.5 px-4 border-b border-zinc-800/80">
          <div className="w-7 h-7 rounded-lg bg-violet-600 flex items-center justify-center shadow-lg shadow-violet-600/25">
            <OrionIcon />
          </div>
          <span className="font-semibold text-sm tracking-tight">Orion-X</span>
          <span className="ml-auto text-[10px] text-zinc-600 bg-zinc-900 border border-zinc-800 px-1.5 py-0.5 rounded font-mono">
            Manager
          </span>
        </div>

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

        {/* Bottom */}
        <div className="border-t border-zinc-800/80 px-2 py-3 space-y-0.5">
          <NavLink
            to="/account"
            className={({ isActive }) =>
              cn(
                'flex items-center gap-2.5 px-2.5 py-[7px] rounded-lg text-sm transition-all duration-150',
                isActive
                  ? 'bg-violet-600 text-white font-medium'
                  : 'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/70'
              )
            }
          >
            {({ isActive }) => (
              <>
                <Link2 className={cn('w-4 h-4 shrink-0', isActive ? 'text-violet-200' : 'text-zinc-500')} strokeWidth={1.5} />
                账号关联
              </>
            )}
          </NavLink>
          <NavLink
            to="/apikeys"
            className={({ isActive }) =>
              cn(
                'flex items-center gap-2.5 px-2.5 py-[7px] rounded-lg text-sm transition-all duration-150',
                isActive
                  ? 'bg-violet-600 text-white font-medium'
                  : 'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/70'
              )
            }
          >
            {({ isActive }) => (
              <>
                <Key className={cn('w-4 h-4 shrink-0', isActive ? 'text-violet-200' : 'text-zinc-500')} strokeWidth={1.5} />
                API Keys
              </>
            )}
          </NavLink>
          <button
            onClick={handleLogout}
            className="w-full flex items-center gap-2.5 px-2.5 py-[7px] rounded-lg text-sm text-zinc-500 hover:text-red-400 hover:bg-red-400/8 transition-all duration-150 cursor-pointer"
          >
            <LogOut className="w-4 h-4 shrink-0" strokeWidth={1.5} />
            退出登录
          </button>

          {/* User chip */}
          <div className="flex items-center gap-2.5 px-2.5 pt-3 mt-1 border-t border-zinc-800/60">
            <div className="w-6 h-6 rounded-full bg-violet-600/20 border border-violet-500/30 flex items-center justify-center shrink-0">
              <span className="text-[10px] font-semibold text-violet-400">{username?.[0]?.toUpperCase() ?? 'U'}</span>
            </div>
            <span className="text-xs text-zinc-500 truncate">{username}</span>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-y-auto bg-zinc-950">
        <Outlet />
      </main>
    </div>
  )
}
