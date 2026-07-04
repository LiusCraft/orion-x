import { useState, useCallback, useEffect } from 'react'
import { useAuthStore } from '@/lib/store'
import { authApi } from '@/lib/api'
import { Copy, Check, User, ChevronRight, Fingerprint, Lock, Mail, Eye, EyeOff } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'

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
      className="p-0.5 text-zinc-600 hover:text-zinc-300 transition-colors cursor-pointer shrink-0"
    >
      {copied ? (
        <Check className="w-3.5 h-3.5 text-emerald-400" strokeWidth={2} />
      ) : (
        <Copy className="w-3.5 h-3.5" strokeWidth={1.5} />
      )}
    </button>
  )
}

function InfoField({ icon, label, value, copyValue }: { icon: React.ReactNode; label: string; value: string | null; copyValue?: string }) {
  return (
    <div className="flex items-center gap-3 py-3 border-b border-zinc-800 last:border-b-0">
      <div className="w-8 h-8 rounded-lg bg-zinc-800/80 border border-zinc-700/60 flex items-center justify-center shrink-0">
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs text-zinc-500">{label}</p>
        <div className="flex items-center gap-1.5">
          <span className="text-sm text-white truncate">{value ?? '-'}</span>
          {copyValue && <CopyBtn value={copyValue} />}
        </div>
      </div>
    </div>
  )
}

function ChangePasswordDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [showOld, setShowOld] = useState(false)
  const [showNew, setShowNew] = useState(false)
  const [loading, setLoading] = useState(false)
  const [msg, setMsg] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setMsg('')
    setLoading(true)
    try {
      await authApi.changePassword(oldPassword, newPassword)
      setMsg('密码修改成功')
      setOldPassword('')
      setNewPassword('')
    } catch {
      setMsg('旧密码不正确')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-white">密码修改</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs text-zinc-500">当前密码</label>
            <div className="relative">
              <Input
                type={showOld ? 'text' : 'password'}
                value={oldPassword}
                onChange={(e) => setOldPassword(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-white pr-10"
                required
              />
              <button type="button" onClick={() => setShowOld(!showOld)} className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 cursor-pointer">
                {showOld ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>
          <div className="space-y-1.5">
            <label className="text-xs text-zinc-500">新密码</label>
            <div className="relative">
              <Input
                type={showNew ? 'text' : 'password'}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-white pr-10"
                minLength={6}
                required
              />
              <button type="button" onClick={() => setShowNew(!showNew)} className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 cursor-pointer">
                {showNew ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>
          {msg && (
            <p className={`text-xs ${msg === '密码修改成功' ? 'text-emerald-400' : 'text-red-400'}`}>{msg}</p>
          )}
          <Button type="submit" disabled={loading} className="w-full bg-violet-600 hover:bg-violet-500 text-white cursor-pointer">
            {loading ? '提交中...' : '确认修改'}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function BindEmailDialog({ open, onClose, currentEmail }: { open: boolean; onClose: () => void; currentEmail: string | null }) {
  const [email, setEmail] = useState(currentEmail ?? '')
  const [loading, setLoading] = useState(false)
  const [msg, setMsg] = useState('')

  useEffect(() => {
    if (open) setEmail(currentEmail ?? '')
  }, [open, currentEmail])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setMsg('')
    setLoading(true)
    try {
      const { data } = await authApi.bindEmail(email)
      setMsg(data.message)
    } catch {
      setMsg('绑定失败，请检查邮箱格式')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-white">邮箱绑定</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs text-zinc-500">邮箱地址</label>
            <Input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="bg-zinc-800 border-zinc-700 text-white"
              placeholder="you@example.com"
              required
            />
          </div>
          {msg && (
            <p className={`text-xs ${msg === '绑定失败，请检查邮箱格式' ? 'text-red-400' : 'text-emerald-400'}`}>{msg}</p>
          )}
          <Button type="submit" disabled={loading} className="w-full bg-violet-600 hover:bg-violet-500 text-white cursor-pointer">
            {loading ? '提交中...' : '确认绑定'}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default function AccountPage() {
  const { username, userId } = useAuthStore()
  const [showPwd, setShowPwd] = useState(false)
  const [showEmail, setShowEmail] = useState(false)
  const [email, setEmail] = useState<string | null>(null)

  useEffect(() => {
    if (!showEmail) {
      authApi.profile().then(({ data }) => setEmail(data.email)).catch(() => {})
    }
  }, [showEmail])

  return (
    <div className="min-h-full max-w-2xl mx-auto px-6 py-10">
      <div className="flex items-center gap-2 text-sm mb-8">
        <div className="w-7 h-7 rounded-full border border-zinc-700 flex items-center justify-center">
          <User className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />
        </div>
        <span className="text-white font-medium">账号</span>
        <ChevronRight className="w-4 h-4 text-zinc-600" strokeWidth={1.5} />
      </div>

      <div className="bg-zinc-900/80 border border-zinc-800 rounded-2xl overflow-hidden">
        <div className="px-6 py-6 border-b border-zinc-800">
          <div className="flex items-center gap-4">
            <div className="w-16 h-16 rounded-full bg-violet-600/20 border border-violet-500/30 flex items-center justify-center shrink-0">
              <span className="text-xl font-semibold text-violet-400">
                {username?.[0]?.toUpperCase() ?? 'U'}
              </span>
            </div>
            <div>
              <div className="flex items-center gap-1.5">
                <span className="text-base font-medium text-white">{username}</span>
                <CopyBtn value={username ?? ''} />
              </div>
            </div>
          </div>
        </div>

        <div className="px-6">
          <InfoField
            icon={<Fingerprint className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />}
            label="账号 ID"
            value={userId}
            copyValue={userId ?? undefined}
          />
          <InfoField
            icon={<Mail className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />}
            label="绑定邮箱"
            value={email}
          />
        </div>
      </div>

      <div className="mt-4 bg-zinc-900/80 border border-zinc-800 rounded-2xl overflow-hidden">
        <button
          onClick={() => setShowPwd(true)}
          className="flex items-center gap-3 px-6 py-4 w-full text-left hover:bg-zinc-800/50 transition-colors cursor-pointer border-b border-zinc-800"
        >
          <div className="w-8 h-8 rounded-lg bg-zinc-800/80 border border-zinc-700/60 flex items-center justify-center shrink-0">
            <Lock className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-white">密码修改</p>
            <p className="text-xs text-zinc-500 mt-0.5">定期更换密码可提高账号安全性</p>
          </div>
          <ChevronRight className="w-4 h-4 text-zinc-600 shrink-0" strokeWidth={1.5} />
        </button>
        <button
          onClick={() => setShowEmail(true)}
          className="flex items-center gap-3 px-6 py-4 w-full text-left hover:bg-zinc-800/50 transition-colors cursor-pointer"
        >
          <div className="w-8 h-8 rounded-lg bg-zinc-800/80 border border-zinc-700/60 flex items-center justify-center shrink-0">
            <Mail className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-white">邮箱绑定</p>
            <p className="text-xs text-zinc-500 mt-0.5">绑定邮箱后可接收系统通知和重置密码</p>
          </div>
          <ChevronRight className="w-4 h-4 text-zinc-600 shrink-0" strokeWidth={1.5} />
        </button>
      </div>

      <ChangePasswordDialog open={showPwd} onClose={() => setShowPwd(false)} />
      <BindEmailDialog open={showEmail} onClose={() => setShowEmail(false)} currentEmail={email} />
    </div>
  )
}
