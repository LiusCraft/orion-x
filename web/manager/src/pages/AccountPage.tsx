import { useState } from 'react'
import { Link2, CheckCircle2, XCircle, ExternalLink } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface AccountBinding {
  id: string
  name: string
  icon: string
  iconBg: string
  desc: string
  bound: boolean
  boundAs?: string
  boundAt?: string
}

const MOCK_BINDINGS: AccountBinding[] = [
  {
    id: 'github',
    name: 'GitHub',
    icon: '⬛',
    iconBg: 'bg-zinc-700',
    desc: '关联后可使用 GitHub 账号登录，并授权访问仓库',
    bound: true,
    boundAs: 'liushunshun',
    boundAt: '2025-05-10',
  },
  {
    id: 'google',
    name: 'Google',
    icon: '🔵',
    iconBg: 'bg-blue-500/10',
    desc: '关联 Google 账号用于统一登录和日历同步',
    bound: false,
  },
  {
    id: 'wechat',
    name: '微信',
    icon: '💚',
    iconBg: 'bg-emerald-500/10',
    desc: '绑定微信账号，支持微信扫码登录和消息通知',
    bound: false,
  },
  {
    id: 'dingtalk',
    name: '钉钉',
    icon: '🔷',
    iconBg: 'bg-blue-400/10',
    desc: '绑定钉钉账号，接收智能体通知并调用钉钉 MCP',
    bound: true,
    boundAs: '刘顺顺',
    boundAt: '2025-06-20',
  },
  {
    id: 'feishu',
    name: '飞书',
    icon: '🟣',
    iconBg: 'bg-violet-400/10',
    desc: '绑定飞书账号，接收消息推送并使用飞书工作台',
    bound: false,
  },
]

export default function AccountPage() {
  const [bindings, setBindings] = useState<AccountBinding[]>(MOCK_BINDINGS)

  const toggle = (id: string) => {
    setBindings((prev) =>
      prev.map((b) =>
        b.id === id
          ? b.bound
            ? { ...b, bound: false, boundAs: undefined, boundAt: undefined }
            : { ...b, bound: true, boundAs: 'example_user', boundAt: new Date().toISOString().slice(0, 10) }
          : b
      )
    )
  }

  const bound = bindings.filter((b) => b.bound)
  const unbound = bindings.filter((b) => !b.bound)

  return (
    <div className="min-h-full">
      <div className="border-b border-zinc-800/80 px-8 py-5">
        <div>
          <h1 className="text-lg font-semibold text-white">账号关联</h1>
          <p className="text-sm text-zinc-500 mt-0.5">关联第三方账号，支持多种登录方式和服务集成</p>
        </div>
      </div>

      <div className="px-8 py-6 max-w-2xl space-y-8">
        {bound.length > 0 && (
          <div>
            <h2 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4">已关联（{bound.length}）</h2>
            <div className="space-y-3">
              {bound.map((b) => (
                <div key={b.id} className="bg-zinc-900 border border-zinc-800 rounded-xl px-5 py-4 flex items-center gap-4">
                  <div className={`w-10 h-10 rounded-xl ${b.iconBg} flex items-center justify-center text-xl shrink-0`}>
                    {b.icon}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <p className="font-medium text-sm text-white">{b.name}</p>
                      <span className="flex items-center gap-1 text-[11px] text-emerald-400">
                        <CheckCircle2 className="w-3 h-3" />已关联
                      </span>
                    </div>
                    {b.boundAs && (
                      <p className="text-xs text-zinc-500 mt-0.5">
                        账号：<span className="text-zinc-400">{b.boundAs}</span>
                        <span className="text-zinc-600 ml-2">· {b.boundAt}</span>
                      </p>
                    )}
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => toggle(b.id)}
                    className="h-7 px-3 text-xs border-zinc-700 text-zinc-400 hover:text-red-400 hover:border-red-400/30 hover:bg-red-400/8 shrink-0"
                  >
                    解除关联
                  </Button>
                </div>
              ))}
            </div>
          </div>
        )}

        {unbound.length > 0 && (
          <div>
            <h2 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-4">未关联</h2>
            <div className="space-y-3">
              {unbound.map((b) => (
                <div key={b.id} className="bg-zinc-900 border border-zinc-800 rounded-xl px-5 py-4 flex items-center gap-4 hover:border-zinc-700 transition-all">
                  <div className={`w-10 h-10 rounded-xl ${b.iconBg} flex items-center justify-center text-xl shrink-0 opacity-60`}>
                    {b.icon}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="font-medium text-sm text-white">{b.name}</p>
                    <p className="text-xs text-zinc-500 mt-0.5">{b.desc}</p>
                  </div>
                  <Button
                    size="sm"
                    onClick={() => toggle(b.id)}
                    className="h-7 px-3 text-xs bg-zinc-800 hover:bg-zinc-700 text-zinc-300 border border-zinc-700 hover:border-zinc-600 shrink-0"
                    variant="outline"
                  >
                    <Link2 className="w-3 h-3 mr-1.5" />关联
                  </Button>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
