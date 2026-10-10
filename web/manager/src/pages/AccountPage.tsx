import { useState, useCallback, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@/lib/store";
import { useDocumentTitle } from "@/lib/title";
import { authApi, type OAuthBinding, type OAuthProvider } from "@/lib/api";
import {
  Copy,
  Check,
  User,
  ChevronRight,
  Fingerprint,
  Lock,
  Mail,
  Eye,
  EyeOff,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function CopyBtn({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);

  const handleClick = useCallback(() => {
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  }, [value]);

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
  );
}

function InfoField({
  icon,
  label,
  value,
  copyValue,
  action,
}: {
  icon: React.ReactNode;
  label: string;
  value: string | null;
  copyValue?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex items-center gap-3 py-3 border-b border-zinc-800 last:border-b-0">
      <div className="w-8 h-8 rounded-lg bg-zinc-800/80 border border-zinc-700/60 flex items-center justify-center shrink-0">
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs text-zinc-500">{label}</p>
        <div className="flex items-center gap-1.5">
          <span className="text-sm text-white truncate">{value ?? "-"}</span>
          {copyValue && <CopyBtn value={copyValue} />}
        </div>
      </div>
      {action}
    </div>
  );
}

function ChangePasswordDialog({
  open,
  onClose,
  hasPassword,
}: {
  open: boolean;
  onClose: () => void;
  hasPassword: boolean;
}) {
  const { t } = useTranslation(["account", "common"]);
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [showOld, setShowOld] = useState(false);
  const [showNew, setShowNew] = useState(false);
  const [loading, setLoading] = useState(false);
  const [msg, setMsg] = useState<{ text: string; ok: boolean } | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setMsg(null);
    setLoading(true);
    try {
      await authApi.changePassword(hasPassword ? oldPassword : "", newPassword);
      setMsg({ text: t("passwordChange.success"), ok: true });
      setOldPassword("");
      setNewPassword("");
    } catch {
      setMsg({
        text: hasPassword
          ? t("passwordChange.wrongOld")
          : t("passwordChange.failed"),
        ok: false,
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-white">
            {hasPassword ? t("passwordChange.title") : t("passwordChange.titleSet")}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {hasPassword && (
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-500">{t("passwordChange.old")}</label>
              <div className="relative">
                <Input
                  type={showOld ? "text" : "password"}
                  value={oldPassword}
                  onChange={(e) => setOldPassword(e.target.value)}
                  className="pr-10"
                  required
                />
                <button
                  type="button"
                  onClick={() => setShowOld(!showOld)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 cursor-pointer"
                >
                  {showOld ? (
                    <EyeOff className="w-4 h-4" />
                  ) : (
                    <Eye className="w-4 h-4" />
                  )}
                </button>
              </div>
            </div>
          )}
          <div className="space-y-1.5">
            <label className="text-xs text-zinc-500">{t("passwordChange.new")}</label>
            <div className="relative">
              <Input
                type={showNew ? "text" : "password"}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="pr-10"
                minLength={6}
                required
              />
              <button
                type="button"
                onClick={() => setShowNew(!showNew)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-zinc-500 cursor-pointer"
              >
                {showNew ? (
                  <EyeOff className="w-4 h-4" />
                ) : (
                  <Eye className="w-4 h-4" />
                )}
              </button>
            </div>
          </div>
          {msg && (
            <p
              className={`text-xs ${msg.ok ? "text-emerald-400" : "text-red-400"}`}
            >
              {msg.text}
            </p>
          )}
          <Button
            type="submit"
            disabled={loading}
            className="w-full bg-violet-600 hover:bg-violet-500 text-white cursor-pointer"
          >
            {loading
              ? t("common:action.submitting")
              : hasPassword
                ? t("passwordChange.confirmChange")
                : t("passwordChange.confirmSet")}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function UnbindOAuthDialog({
  open,
  onClose,
  onUnbound,
  provider,
  name,
}: {
  open: boolean;
  onClose: () => void;
  onUnbound: () => void;
  provider: string;
  name: string;
}) {
  const { t } = useTranslation(["account", "common"]);
  const [loading, setLoading] = useState(false);
  const [msg, setMsg] = useState("");

  const handleConfirm = async () => {
    setLoading(true);
    setMsg("");
    try {
      await authApi.unbindOAuth(provider);
      onUnbound();
      onClose();
    } catch {
      setMsg(t("unbind.failed"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-white">
            {t("unbind.title", { name })}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <p className="text-sm text-zinc-400">
            {t("unbind.message", { name })}
          </p>
          {msg && <p className="text-xs text-red-400">{msg}</p>}
          <div className="flex gap-3">
            <Button
              variant="outline"
              onClick={onClose}
              className="flex-1 cursor-pointer"
            >
              {t("common:action.cancel")}
            </Button>
            <Button
              onClick={handleConfirm}
              disabled={loading}
              className="flex-1 bg-red-500 hover:bg-red-400 text-white cursor-pointer"
            >
              {loading ? t("unbind.unbinding") : t("unbind.confirm")}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function BindEmailDialog({
  open,
  onClose,
  currentEmail,
}: {
  open: boolean;
  onClose: () => void;
  currentEmail: string | null;
}) {
  const { t } = useTranslation(["account", "common"]);
  const [email, setEmail] = useState(currentEmail ?? "");
  const [loading, setLoading] = useState(false);
  const [msg, setMsg] = useState<{ text: string; ok: boolean } | null>(null);

  useEffect(() => {
    if (open) setEmail(currentEmail ?? "");
  }, [open, currentEmail]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setMsg(null);
    setLoading(true);
    try {
      const { data } = await authApi.bindEmail(email);
      setMsg({ text: data.message, ok: true });
    } catch {
      setMsg({ text: t("bindEmail.failed"), ok: false });
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-white">{t("bindEmail.title")}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs text-zinc-500">{t("bindEmail.email")}</label>
            <Input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              required
            />
          </div>
          {msg && (
            <p
              className={`text-xs ${msg.ok ? "text-emerald-400" : "text-red-400"}`}
            >
              {msg.text}
            </p>
          )}
          <Button
            type="submit"
            disabled={loading}
            className="w-full bg-violet-600 hover:bg-violet-500 text-white cursor-pointer"
          >
            {loading ? t("common:action.submitting") : t("bindEmail.confirm")}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

		export default function AccountPage() {
			const { t } = useTranslation(["account", "common"]);
  			useDocumentTitle(t("title"));

  			const { username, userId } = useAuthStore();
  			const [showPwd, setShowPwd] = useState(false);
  			const [showEmail, setShowEmail] = useState(false);
  			const [unbindTarget, setUnbindTarget] = useState<{
  				provider: string;
  				name: string;
  			} | null>(null);
	const [email, setEmail] = useState<string | null>(null);
	const [bindings, setBindings] = useState<OAuthBinding[]>([]);
	const [providers, setProviders] = useState<OAuthProvider[]>([]);
	const [hasPassword, setHasPassword] = useState(true);

	useEffect(() => {
		authApi
			.profile()
			.then(({ data }) => {
				setEmail(data.email);
				setBindings(data.bindings);
				setHasPassword(data.has_password);
			})
			.catch(() => {});
		authApi
			.oauthProviders()
			.then(({ data }) => setProviders(data.providers))
			.catch(() => {});
	}, []);

	return (
		<div className="min-h-full max-w-2xl mx-auto px-6 py-10">
			<div className="flex items-center gap-2 text-sm mb-8">
				<div className="w-7 h-7 rounded-full border border-zinc-700 flex items-center justify-center">
					<User className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />
				</div>
				<span className="text-white font-medium">{t("title")}</span>
				<ChevronRight className="w-4 h-4 text-zinc-600" strokeWidth={1.5} />
			</div>

			{!hasPassword && (
				<div className="mb-6 flex items-center gap-3 px-4 py-3.5 rounded-xl border border-amber-400/20 bg-amber-400/5">
					<Lock className="w-4 h-4 text-amber-400 shrink-0" strokeWidth={1.5} />
					<div className="flex-1 min-w-0">
						<p className="text-sm font-medium text-amber-400">
							{t("noPasswordWarning")}
						</p>
					</div>
					<Button
						onClick={() => setShowPwd(true)}
						className="shrink-0 bg-amber-500 hover:bg-amber-400 text-zinc-950 font-semibold cursor-pointer"
					>
						{t("goSetPassword")}
					</Button>
				</div>
			)}

      <div className="bg-zinc-900/80 border border-zinc-800 rounded-2xl overflow-hidden">
        <div className="px-6 py-6 border-b border-zinc-800">
          <div className="flex items-center gap-4">
            <div className="w-16 h-16 rounded-full bg-violet-600/20 border border-violet-500/30 flex items-center justify-center shrink-0">
              <span className="text-xl font-semibold text-violet-400">
                {username?.[0]?.toUpperCase() ?? "U"}
              </span>
            </div>
            <div>
              <div className="flex items-center gap-1.5">
                <span className="text-base font-medium text-white">
                  {username}
                </span>
                <CopyBtn value={username ?? ""} />
              </div>
            </div>
          </div>
        </div>

        <div className="px-6">
          <InfoField
            icon={
              <Fingerprint
                className="w-4 h-4 text-zinc-400"
                strokeWidth={1.5}
              />
            }
            label={t("accountId")}
            value={userId}
            copyValue={userId ?? undefined}
          />
          <InfoField
            icon={<Mail className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />}
            label={t("boundEmail")}
            value={email}
          />
          				{providers.map((p) => {
          					const linked = bindings.some((b) => b.provider === p.provider);
          					return (
          						<InfoField
          							key={p.provider}
          							icon={
          								p.provider === "github" ? (
          									<svg className="w-4 h-4 text-zinc-400" viewBox="0 0 24 24" fill="currentColor">
          										<path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
          									</svg>
          								) : (
          									<Fingerprint
          										className="w-4 h-4 text-zinc-400"
          										strokeWidth={1.5}
          									/>
          								)
          							}
          							label={p.name}
          							value={linked ? t("bound") : t("unbound")}
          							action={
          								linked ? (
          									<Button
          										size="sm"
          										variant="outline"
          										onClick={() =>
          											hasPassword
          												? setUnbindTarget(p)
          												: setShowPwd(true)
          										}
          										title={
          											hasPassword
          												? t("unbind.title", { name: p.name })
          												: t("unbind.setPasswordFirst")
          										}
          										className="shrink-0 border-red-500/30 text-red-400 hover:bg-red-500/10 cursor-pointer"
          									>
          										{t("unbind.action")}
          									</Button>
          								) : (
          									<Button
          										size="sm"
          										onClick={() => {
          											window.location.href = authApi.oauthLoginUrl(p.provider);
          										}}
          										className="shrink-0 bg-violet-600 hover:bg-violet-500 text-white cursor-pointer"
          									>
          										{t("bind.action")}
          									</Button>
          								)
          							}
          							/>
          						);
          					})}
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
            <p className="text-sm font-medium text-white">{t("passwordChange.title")}</p>
            <p className="text-xs text-zinc-500 mt-0.5">
              {t("passwordChange.desc")}
            </p>
          </div>
          <ChevronRight
            className="w-4 h-4 text-zinc-600 shrink-0"
            strokeWidth={1.5}
          />
        </button>
        <button
          onClick={() => setShowEmail(true)}
          className="flex items-center gap-3 px-6 py-4 w-full text-left hover:bg-zinc-800/50 transition-colors cursor-pointer"
        >
          <div className="w-8 h-8 rounded-lg bg-zinc-800/80 border border-zinc-700/60 flex items-center justify-center shrink-0">
            <Mail className="w-4 h-4 text-zinc-400" strokeWidth={1.5} />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-white">{t("bindEmail.title")}</p>
            <p className="text-xs text-zinc-500 mt-0.5">
              {t("bindEmail.desc")}
            </p>
          </div>
          <ChevronRight
            className="w-4 h-4 text-zinc-600 shrink-0"
            strokeWidth={1.5}
          />
        </button>
      </div>

      <ChangePasswordDialog
        open={showPwd}
        onClose={() => setShowPwd(false)}
        hasPassword={hasPassword}
      />
      			<BindEmailDialog
      				open={showEmail}
      				onClose={() => setShowEmail(false)}
      				currentEmail={email}
      			/>
			<UnbindOAuthDialog
				open={!!unbindTarget}
				onClose={() => setUnbindTarget(null)}
				provider={unbindTarget?.provider ?? ""}
				name={unbindTarget?.name ?? ""}
				onUnbound={() =>
					setBindings((prev) =>
						prev.filter((b) => b.provider !== unbindTarget?.provider),
					)
				}
			/>
    </div>
  );
}
