# Dark/Light Mode + Top Navigation Bar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add dark/light theme switching with system preference detection and a top navigation bar.

**Architecture:** ThemeProvider React context reads localStorage → `prefers-color-scheme` → toggles `.dark` class on `<html>`. Layout gets a top bar with notification icon (left) and theme toggle (right). All 25+ page components switch from hardcoded dark Tailwind classes to shadcn CSS variables.

**Tech Stack:** React 19, Tailwind CSS v4, shadcn/ui (CSS variables), lucide-react, @base-ui/react

---

### Task 1: ThemeProvider component

**Files:**
- Create: `src/components/ThemeProvider.tsx`
- Modify: `src/main.tsx`

- [ ] **Create ThemeProvider.tsx**

```tsx
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

type Theme = 'light' | 'dark'

interface ThemeContextValue {
  theme: Theme
  setTheme: (t: Theme) => void
}

const ThemeContext = createContext<ThemeContextValue>({
  theme: 'dark',
  setTheme: () => {},
})

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => {
    if (typeof window === 'undefined') return 'dark'
    const saved = localStorage.getItem('theme') as Theme | null
    if (saved === 'light' || saved === 'dark') return saved
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })

  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
    localStorage.setItem('theme', theme)
  }, [theme])

  return (
    <ThemeContext.Provider value={{ theme, setTheme: setThemeState }}>
      {children}
    </ThemeContext.Provider>
  )
}

export const useTheme = () => useContext(ThemeContext)
```

- [ ] **Modify `src/main.tsx`** — wrap App with ThemeProvider

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import { ThemeProvider } from './components/ThemeProvider'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <App />
    </ThemeProvider>
  </StrictMode>,
)
```

---

### Task 2: Layout top bar with theme toggle + notification

**Files:**
- Modify: `src/components/Layout.tsx`

- [ ] **Restructure Layout** — wrap sidebar+content in `flex flex-col h-screen`, add top bar above

Replace the entire `return (...)` in Layout.tsx:

```tsx
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/lib/store'
import { useTheme } from './ThemeProvider'
import {
  Bot, Store, Cpu, Puzzle, Zap, Brain, BookOpen, Database,
  Mic2, Wand2, Music, Activity, Layers, BarChart3, CreditCard,
  Key, Link2, LogOut, Building2, Sun, Moon, Bell,
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
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex flex-col h-screen bg-background text-foreground overflow-hidden">
      {/* Top bar */}
      <header className="h-14 shrink-0 flex items-center justify-between px-6 border-b border-border bg-background">
        <div className="flex items-center gap-2">
          <button className="relative p-2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer rounded-lg hover:bg-accent">
            <Bell className="w-5 h-5" strokeWidth={1.5} />
          </button>
        </div>
        <button
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          className="p-2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer rounded-lg hover:bg-accent"
          aria-label="切换主题"
        >
          {theme === 'dark' ? <Sun className="w-5 h-5" strokeWidth={1.5} /> : <Moon className="w-5 h-5" strokeWidth={1.5} />}
        </button>
      </header>

      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar */}
        <aside className="w-60 shrink-0 flex flex-col border-r border-border bg-card">
          {/* Logo */}
          <div className="h-14 flex items-center gap-2.5 px-4 border-b border-border">
            <div className="w-7 h-7 rounded-lg bg-violet-600 flex items-center justify-center shadow-lg shadow-violet-600/25">
              <OrionIcon />
            </div>
            <span className="font-semibold text-sm tracking-tight">Orion-X</span>
            <span className="ml-auto text-[10px] text-muted-foreground/60 bg-muted border border-border px-1.5 py-0.5 rounded font-mono">
              Manager
            </span>
          </div>

          {/* Nav */}
          <nav className="flex-1 overflow-y-auto py-3 px-2 scrollbar-none">
            {NAV_GROUPS.map((group) => (
              <div key={group.label} className="mb-5">
                <p className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-widest px-2 mb-1.5">
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
                          : 'text-muted-foreground hover:text-foreground hover:bg-accent'
                      )
                    }
                  >
                    {({ isActive }) => (
                      <>
                        <Icon
                          className={cn('w-4 h-4 shrink-0', isActive ? 'text-violet-200' : 'text-muted-foreground/60')}
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
          <div className="border-t border-border px-2 py-3 space-y-0.5">
            <NavLink
              to="/account"
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2.5 px-2.5 py-[7px] rounded-lg text-sm transition-all duration-150',
                  isActive
                    ? 'bg-violet-600 text-white font-medium'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent'
                )
              }
            >
              {({ isActive }) => (
                <>
                  <Link2 className={cn('w-4 h-4 shrink-0', isActive ? 'text-violet-200' : 'text-muted-foreground/60')} strokeWidth={1.5} />
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
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent'
                )
              }
            >
              {({ isActive }) => (
                <>
                  <Key className={cn('w-4 h-4 shrink-0', isActive ? 'text-violet-200' : 'text-muted-foreground/60')} strokeWidth={1.5} />
                  API Keys
                </>
              )}
            </NavLink>
            <button
              onClick={handleLogout}
              className="w-full flex items-center gap-2.5 px-2.5 py-[7px] rounded-lg text-sm text-muted-foreground/60 hover:text-destructive hover:bg-destructive/10 transition-all duration-150 cursor-pointer"
            >
              <LogOut className="w-4 h-4 shrink-0" strokeWidth={1.5} />
              退出登录
            </button>

            {/* User chip */}
            <div className="flex items-center gap-2.5 px-2.5 pt-3 mt-1 border-t border-border/60">
              <div className="w-6 h-6 rounded-full bg-violet-600/20 border border-violet-500/30 flex items-center justify-center shrink-0">
                <span className="text-[10px] font-semibold text-violet-400">{username?.[0]?.toUpperCase() ?? 'U'}</span>
              </div>
              <span className="text-xs text-muted-foreground/60 truncate">{username}</span>
            </div>
          </div>
        </aside>

        {/* Main content */}
        <main className="flex-1 overflow-y-auto bg-background">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
```

---

### Task 3: LoginPage — CSS variable replacement

**Files:**
- Modify: `src/pages/LoginPage.tsx`

Mapping: LoginPage doesn't use Layout (no sidebar), so its outer wrapper sets theme via class.

- [ ] **Replace hardcoded classes in LoginPage**

Key replacements:
| Line(s) | Before | After |
|---|---|---|
| 41 | `bg-zinc-950` | `bg-background` |
| 63 | `text-white` | `text-foreground` |
| 67 | `text-white` | `text-foreground` |
| 70 | `text-zinc-400` | `text-muted-foreground` |
| 77 | `bg-zinc-800 border border-zinc-700/60` | `bg-muted border border-border/60` |
| 82 | `text-zinc-500` | `text-muted-foreground/60` |
| 88 | `text-zinc-700` | `text-muted-foreground/30` |
| 102 | `text-white` | `text-foreground` |
| 106 | `text-white` | `text-foreground` |
| 107 | `text-zinc-500` | `text-muted-foreground/60` |
| 110 | `bg-zinc-900/70 border border-zinc-800` | `bg-card/70 border border-border` |
| 113 | `text-zinc-400` | `text-muted-foreground` |
| 122 | `bg-zinc-800/80 border-zinc-700 text-white placeholder:text-zinc-600` | `bg-muted/80 border-border text-foreground placeholder:text-muted-foreground` |
| 139 | `bg-zinc-800/80 border-zinc-700 text-white placeholder:text-zinc-600` | `bg-muted/80 border-border text-foreground placeholder:text-muted-foreground` |
| 147 | `text-zinc-500 hover:text-zinc-200` | `text-muted-foreground/60 hover:text-muted-foreground` |

---

### Task 4: VoicebotListPage — CSS variable replacement

**Files:**
- Modify: `src/pages/VoicebotListPage.tsx`

- [ ] **Replace hardcoded classes**

| Line(s) | Before | After |
|---|---|---|
| 45 | `bg-zinc-950 text-white` | `bg-background text-foreground` |
| 47 | `border-zinc-800` | `border-border` |
| 57 | `text-zinc-500` | `text-muted-foreground` |
| 60 | `text-zinc-500 hover:text-zinc-300` | `text-muted-foreground hover:text-foreground` |
| 72 | `text-zinc-500` | `text-muted-foreground` |
| 84 | `text-zinc-500` | `text-muted-foreground` |
| 90 | `bg-zinc-800` | `bg-muted` |
| 91 | `text-zinc-600` | `text-muted-foreground/60` |
| 96 | `text-zinc-600` | `text-muted-foreground/60` |
| 104 | `bg-zinc-900 border border-zinc-800 hover:border-zinc-600 hover:bg-zinc-800/60` | `bg-card border border-border hover:border-foreground/20 hover:bg-accent/60` |
| 112 | `text-zinc-600 group-hover:text-zinc-400` | `text-muted-foreground/60 group-hover:text-muted-foreground` |
| 117 | `text-zinc-500` | `text-muted-foreground` |
| 118 | `text-zinc-600` | `text-muted-foreground/60` |
| 128 | `bg-zinc-900 border-zinc-800 text-white` (DialogContent) | `bg-card border-border text-card-foreground` |
| 133 | `text-zinc-400` (label) | `text-muted-foreground` |
| 138 | `bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500` | `bg-muted border-border text-foreground placeholder:text-muted-foreground` |
| 144 | `border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white` | `border-border text-muted-foreground hover:bg-accent hover:text-foreground` |

---

### Task 5: VoicebotDetailPage — CSS variable replacement

**Files:**
- Modify: `src/pages/VoicebotDetailPage.tsx`

- [ ] **Replace hardcoded classes**

| Line(s) | Before | After |
|---|---|---|
| 94 | `bg-zinc-950` | `bg-background` |
| 95 | `border-zinc-700 border-t-violet-500` | `border-border border-t-violet-500` |
| 100 | `bg-zinc-950 text-zinc-500` | `bg-background text-muted-foreground` |
| 103 | `bg-zinc-950 text-white` | `bg-background text-foreground` |
| 105 | `border-b border-zinc-800` | `border-b border-border` |
| 108 | `text-zinc-500 hover:text-zinc-300` | `text-muted-foreground hover:text-foreground` |
| 114 | `bg-zinc-800` | `bg-muted` |
| 116 | `text-zinc-600` | `text-muted-foreground/60` |
| 121 | `bg-zinc-900 border border-zinc-800` (TabsList) | `bg-card border border-border` |
| 122 | `data-[state=active]:bg-zinc-700 data-[state=active]:text-white text-zinc-400` | `data-[state=active]:bg-accent data-[state=active]:text-accent-foreground text-muted-foreground` |
| 125-126 | same pattern as 122 | same |
| 133 | `text-zinc-400` | `text-muted-foreground` |
| 137 | `bg-zinc-900 border-zinc-800 text-white` (DialogContent) | `bg-card border-border text-card-foreground` |
| 141 | `text-zinc-400` | `text-muted-foreground` |
| 145 | `bg-zinc-900 border-zinc-800 text-green-400` | `bg-card border-border text-green-400` (keep accent colors) |
| 176-178 | `border-zinc-800 border-dashed` / `text-zinc-500` / `text-zinc-600` | `border-border border-dashed` / `text-muted-foreground` / `text-muted-foreground/60` |
| 185 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 189 | `text-white` / `text-zinc-500` | `text-card-foreground` / `text-muted-foreground` |
| 196 | `text-zinc-600 hover:text-red-400` | `text-muted-foreground/60 hover:text-destructive` |
| 209 | `bg-zinc-900 border-zinc-800 text-white` (DialogContent) | `bg-card border-border text-card-foreground` |
| 215 | `text-zinc-400` | `text-muted-foreground` |
| 220 | `bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-500` | `bg-muted border-border text-foreground placeholder:text-muted-foreground` |
| 99 | `inp = 'bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600 ...'` (shared constant) | `inp = 'bg-muted border-border text-foreground placeholder:text-muted-foreground focus-visible:ring-violet-500 h-9'` |
| 240 | `border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white` | `border-border text-muted-foreground hover:bg-accent hover:text-foreground` |

---

### Task 6: AccountPage + ApiKeysPage — CSS variable replacement

**Files:**
- Modify: `src/pages/AccountPage.tsx`
- Modify: `src/pages/ApiKeysPage.tsx`

- [ ] **Replace classes in AccountPage.tsx**

| Line(s) | Before | After |
|---|---|---|
| 21 | `bg-zinc-700` | `bg-accent` |
| 83 | `border-b border-zinc-800/80` | `border-b border-border` |
| 96 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 118 | `border-zinc-700 text-zinc-400` | `border-border text-muted-foreground` |
| 133 | `bg-zinc-900 border border-zinc-800 hover:border-zinc-700` | `bg-card border border-border hover:border-muted-foreground/20` |
| 144 | `bg-zinc-800 hover:bg-zinc-700 text-zinc-300 border border-zinc-700 hover:border-zinc-600` | `bg-muted hover:bg-accent text-muted-foreground border border-border hover:border-foreground/20` |

- [ ] **Replace classes in ApiKeysPage.tsx**

| Line(s) | Before | After |
|---|---|---|
| 61 | `bg-zinc-700/50 text-zinc-400 border-zinc-700` | `bg-accent/50 text-muted-foreground border-border` |
| 69 | `border-b border-zinc-800/80` | `border-b border-border` |
| 72 | `text-white` | `text-foreground` |
| 91 | `text-white` | `text-foreground` |
| 108 | `bg-zinc-800` | `bg-muted` |
| 111 | `text-zinc-400` | `text-muted-foreground` |
| 118 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 121 | `border-b border-zinc-800` | `border-b border-border` |
| 129 | `border-b border-zinc-800/50 hover:bg-zinc-800/30` | `border-b border-border/50 hover:bg-accent/30` |
| 132 | `bg-zinc-800 border border-zinc-700/50` (badge) | `bg-muted border border-border/50` |
| 135 | `text-white` | `text-foreground` |
| 140 | `text-zinc-400` | `text-muted-foreground` |
| 145 | `text-zinc-600 hover:text-zinc-400` | `text-muted-foreground/60 hover:text-muted-foreground` |
| 151 | `text-zinc-600 hover:text-zinc-400` | same |
| 158 | `bg-zinc-800 text-zinc-400 border-zinc-700` (select trigger) | `bg-muted text-muted-foreground border-border` |
| 162 | `text-zinc-400` | `text-muted-foreground` |
| 180 | `bg-zinc-900/50 border border-zinc-800` | `bg-card/50 border border-border` |
| 190 | `bg-zinc-900 border-zinc-800 text-white` (DialogContent) | `bg-card border-border text-card-foreground` |
| 199-200 | `text-zinc-400` / `bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600` | `text-muted-foreground` / `bg-muted border-border text-foreground placeholder:text-muted-foreground` |
| 204 | `bg-zinc-800 border border-zinc-700 text-white` | `bg-muted border border-border text-foreground` |
| 210 | `border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white` | `border-border text-muted-foreground hover:bg-accent hover:text-foreground` |
| 218 | `bg-zinc-900 border-zinc-800 text-white` (DialogContent) | `bg-card border-border text-card-foreground` |
| 222 | `text-zinc-400` | `text-muted-foreground` |
| 224 | `border-zinc-700 text-zinc-300 hover:bg-zinc-800` | `border-border text-muted-foreground hover:bg-accent` |

---

### Task 7: Agent pages — CSS variable replacement

**Files:**
- Modify: `src/pages/agents/AgentDetailPage.tsx`
- Modify: `src/pages/agents/AgentListPage.tsx`
- Modify: `src/pages/agents/AgentPlazaPage.tsx`
- Modify: `src/pages/agents/QuickChat.tsx`

- [ ] **Replace classes in AgentDetailPage.tsx** — See full mapping in Task 5. Also replace the `inp` constant (line 99) and all `bg-zinc-800 border-zinc-700` input patterns. Dialog `bg-zinc-900 border-zinc-800 text-white` → `bg-card border-border text-card-foreground`. TabsList `bg-zinc-900 border border-zinc-800` → `bg-card border border-border`. TabsTrigger `data-[state=active]:bg-zinc-700 data-[state=active]:text-white text-zinc-400` → `data-[state=active]:bg-accent data-[state=active]:text-accent-foreground text-muted-foreground`.

- [ ] **Replace classes in AgentListPage.tsx**

| Line(s) | Before | After |
|---|---|---|
| 129 | `bg-zinc-700 text-white` | `bg-accent text-accent-foreground` |
| 133 | `bg-zinc-700 text-white` | same |
| 138 | `bg-zinc-800 border border-zinc-700 text-white placeholder:text-zinc-500` | `bg-muted border border-border text-foreground placeholder:text-muted-foreground` |
| 158 | `bg-zinc-800` | `bg-muted` |
| 169 | `bg-zinc-800` | `bg-muted` |
| 183 | `bg-zinc-900 border border-zinc-800 hover:border-zinc-600 hover:bg-zinc-800/50` | `bg-card border border-border hover:border-foreground/20 hover:bg-accent/50` |
| 203 | `bg-zinc-800/60` | `bg-muted/60` |
| 210 | `bg-zinc-800/60` | same |
| 217 | `bg-zinc-800/60` | same |
| 224 | `bg-zinc-800/60` | same |
| 243 | `bg-zinc-800 text-zinc-400 border border-zinc-700` | `bg-muted text-muted-foreground border border-border` |
| 268 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 286 | `border-b border-zinc-800/60 hover:bg-zinc-800/40` | `border-b border-border/60 hover:bg-accent/40` |
| 302 | `bg-zinc-800 text-zinc-400 border border-zinc-700` | `bg-muted text-muted-foreground border border-border` |
| 316 | `bg-zinc-900 border-zinc-800 text-white` (DialogContent) | `bg-card border-border text-card-foreground` |
| 330 | `bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600` | `bg-muted border-border text-foreground placeholder:text-muted-foreground` |
| 338 | `border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white` | `border-border text-muted-foreground hover:bg-accent hover:text-foreground` |

- [ ] **Replace classes in AgentPlazaPage.tsx**

| Line(s) | Before | After |
|---|---|---|
| 124 | `bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600` | `bg-card border-border text-foreground placeholder:text-muted-foreground` |
| 137 | `bg-zinc-800 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200` | `bg-muted text-muted-foreground hover:bg-accent hover:text-foreground` |
| 150 | `bg-zinc-800` | `bg-muted` |
| 161 | `bg-zinc-900 border border-zinc-800 hover:border-zinc-700 hover:bg-zinc-800/50` | `bg-card border border-border hover:border-muted-foreground/20 hover:bg-accent/50` |
| 174 | `bg-zinc-800 text-zinc-400 border border-zinc-700/50` | `bg-muted text-muted-foreground border border-border/50` |
| 187 | `text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800` | `text-muted-foreground hover:text-foreground hover:bg-accent` |

- [ ] **Replace classes in QuickChat.tsx**

| Line(s) | Before | After |
|---|---|---|
| 202 | `border-zinc-700 text-zinc-300 hover:bg-zinc-800` | `border-border text-muted-foreground hover:bg-accent` |
| 227-228 | `bg-zinc-800/50 text-zinc-400` / `bg-zinc-800 border border-zinc-700 text-zinc-200` | `bg-muted/50 text-muted-foreground` / `bg-muted border border-border text-foreground` |
| 244 | `bg-zinc-800 border-zinc-700 text-white placeholder:text-zinc-600` | `bg-muted border-border text-foreground placeholder:text-muted-foreground` |
| 259 | `bg-zinc-800 border-zinc-700 text-zinc-300 hover:bg-zinc-700` | `bg-muted border-border text-muted-foreground hover:bg-accent` |
| 260 | `bg-zinc-800/50 border-zinc-800 text-zinc-600` | `bg-muted/50 border-border text-muted-foreground/60` |
| 266 | `border-zinc-700 text-zinc-300 hover:bg-zinc-800` | `border-border text-muted-foreground hover:bg-accent` |

---

### Task 8: Billing pages — CSS variable replacement

**Files:**
- Modify: `src/pages/billing/UsagePage.tsx`
- Modify: `src/pages/billing/ResourcesPage.tsx`

- [ ] **Replace classes in UsagePage.tsx**

| Line(s) | Before | After |
|---|---|---|
| 45 | `border-b border-zinc-800/80` | `border-b border-border` |
| 48 | `text-white` | `text-foreground` |
| 49 | `text-zinc-500` | `text-muted-foreground` |
| 51 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 53 | `bg-zinc-800 text-white` / `text-zinc-500 hover:text-zinc-300` | `bg-muted text-foreground` / `text-muted-foreground hover:text-foreground` |
| 63 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 65 | `text-zinc-500` | `text-muted-foreground` |
| 70-71 | `text-white` / `text-zinc-600` | `text-foreground` / `text-muted-foreground/60` |
| 78 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 80 | `data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500` | `data-[state=active]:bg-accent data-[state=active]:text-accent-foreground text-muted-foreground` |
| 87-88 | `bg-zinc-900 border border-zinc-800` / `text-zinc-500` | `bg-card border border-border` / `text-muted-foreground` |
| 91 | `bg-zinc-900 border border-zinc-800` | `bg-card border border-border` |
| 96 | `text-zinc-500` | `text-muted-foreground` |
| 102 | `border-b border-zinc-800/50 hover:bg-zinc-800/30` | `border-b border-border/50 hover:bg-accent/30` |
| 103 | `text-white` | `text-foreground` |
| 104 | `text-zinc-300` | `text-muted-foreground` |
| 105-106 | `text-zinc-400` | `text-muted-foreground` |
| 111 | `border-t border-zinc-700 bg-zinc-800/30` | `border-t border-border bg-accent/30` |
| 112-113 | `text-zinc-400` / `text-zinc-300` | `text-muted-foreground` / `text-muted-foreground` |

- [ ] **Replace classes in ResourcesPage.tsx** — apply same mapping as UsagePage (identical patterns: `border-b border-zinc-800/80`, `bg-zinc-900 border border-zinc-800`, `bg-zinc-800`, `text-white`, `text-zinc-500`, `border-zinc-700`, `hover:bg-zinc-800/30`, etc.)

---

### Task 9: Data pages — CSS variable replacement

**Files:**
- Modify: `src/pages/data/KnowledgePage.tsx`
- Modify: `src/pages/data/MemoryPage.tsx`
- Modify: `src/pages/data/SourcesPage.tsx`

- [ ] **Replace classes in KnowledgePage.tsx** — applies same mapping:
  - `border-b border-zinc-800/80` → `border-b border-border`
  - `text-white` → `text-foreground`
  - `text-zinc-500` → `text-muted-foreground`
  - `bg-zinc-900 border border-zinc-800` → `bg-card border border-border`
  - `bg-zinc-800` → `bg-muted`
  - `text-zinc-400` → `text-muted-foreground`
  - `text-zinc-600` → `text-muted-foreground/60`
  - `bg-zinc-900 border-zinc-800 text-white placeholder:text-zinc-600` (input/search) → `bg-card border-border text-foreground placeholder:text-muted-foreground`
  - `bg-zinc-800 border border-zinc-700/50` (badge) → `bg-muted border border-border/50`
  - `border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white` (cancel/outline button) → `border-border text-muted-foreground hover:bg-accent hover:text-foreground`
  - `bg-zinc-900 border-zinc-800 text-white` (DialogContent) → `bg-card border-border text-card-foreground`

- [ ] **Replace classes in MemoryPage.tsx** — same mapping as above

- [ ] **Replace classes in SourcesPage.tsx** — same mapping as above

---

### Task 10: Voice pages — CSS variable replacement

**Files:**
- Modify: `src/pages/voice/VoiceClonePage.tsx`
- Modify: `src/pages/voice/VoiceListPage.tsx`
- Modify: `src/pages/voice/VoicePlazaPage.tsx`

- [ ] **Replace classes in VoiceClonePage.tsx** — same standard mapping

- [ ] **Replace classes in VoiceListPage.tsx** — same standard mapping. Note: `bg-amber-900/30 text-amber-500 border border-amber-800/40` (accent tags) should remain as-is (semantic accent colors).

- [ ] **Replace classes in VoicePlazaPage.tsx** — same standard mapping. Note color badges like `bg-zinc-700/50 text-zinc-500 border-zinc-700` → `bg-accent/50 text-muted-foreground border-border`. Also `bg-amber-900/30 text-amber-500 border border-amber-800/40` keep as-is.

---

### Task 11: Components pages — CSS variable replacement

**Files:**
- Modify: `src/pages/components/McpPage.tsx`
- Modify: `src/pages/components/PluginsPage.tsx`
- Modify: `src/pages/components/SkillsPage.tsx`

- [ ] **Replace classes in McpPage.tsx** — same standard mapping. TabsList/TabsTrigger: `bg-zinc-900 border border-zinc-800` → `bg-card border border-border`, `data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500` → `data-[state=active]:bg-accent data-[state=active]:text-accent-foreground text-muted-foreground`.

- [ ] **Replace classes in PluginsPage.tsx** — same mapping as McpPage.

- [ ] **Replace classes in SkillsPage.tsx** — same mapping as McpPage.

---

### Task 12: Models pages — CSS variable replacement

**Files:**
- Modify: `src/pages/models/ModelMonitorPage.tsx`
- Modify: `src/pages/models/MyModelsPage.tsx`
- Modify: `src/pages/models/ProvidersPage.tsx`

- [ ] **Replace classes in ModelMonitorPage.tsx** — same standard mapping

- [ ] **Replace classes in MyModelsPage.tsx** — same standard mapping

- [ ] **Replace classes in ProvidersPage.tsx** — same standard mapping

---

### Task 13: Verify and lint

**Files:**
- All modified files

- [ ] **Run build** to verify no TypeScript errors

```bash
cd web/manager && npm run build
```
Expected: Build succeeds with no errors.

- [ ] **Run lint** to verify no lint errors

```bash
cd web/manager && npx oxlint@latest
```
Expected: No errors or only pre-existing warnings.

---

## Self-Review

1. **Spec coverage**: Every item in the design spec maps to tasks:
   - ThemeProvider → Task 1
   - Layout top bar → Task 2
   - Notification icon placeholder → Task 2 (Bell icon, no handler)
   - Theme toggle → Task 2 (Sun/Moon button)
   - CSS variable replacement → Tasks 3-12 (all pages)
   - main.tsx wrapper → Task 1
   - No changes to shadcn/ui components ✓

2. **Placeholder scan**: All code blocks contain complete, specific class mappings. No "TBD" or "TODO".

3. **Type consistency**: ThemeProvider uses `Theme = 'light' | 'dark'`, `useTheme()` returns `{ theme, setTheme }`. Consistent across all tasks.

4. **Coverage gap**: No item in the spec is left unimplemented.
