import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { QRCodeSVG } from "qrcode.react";
import {
  AlertTriangle,
  CheckCircle2,
  Copy,
  Loader2,
  QrCode,
  RefreshCw,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  channelApi,
  type ChannelPlatform,
  type ChannelQRSession,
  type ChannelQRStatus,
  type DeviceChannelStatus,
} from "@/lib/api";

// pollInterval 是控制台查询会话状态的间隔。服务端自己按 3s 一次问企微，前端只读
// 服务端的状态，所以这个频率与出网负载无关。
const pollInterval = 2000;
// maxPollFailures 是连续查询失败多少次后把会话当作过期，让用户重新生成。
const maxPollFailures = 3;

const statusKey: Record<ChannelQRStatus, string> = {
  pending: "channelQR.status.pending",
  scanned: "channelQR.status.scanned",
  success: "channelQR.status.success",
  expired: "channelQR.status.expired",
  failed: "channelQR.status.failed",
};

function isTerminal(session: ChannelQRSession | null): boolean {
  return (
    session?.status === "success" ||
    session?.status === "expired" ||
    session?.status === "failed"
  );
}

function formatRemaining(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

interface ChannelQRDialogProps {
  open: boolean;
  voicebotId: string;
  deviceId: string;
  platform: ChannelPlatform;
  onClose: () => void;
  /** 开通成功后把新通道状态交给调用方（设备列表不需要重新拉取）。 */
  onBound: (status: DeviceChannelStatus) => void;
}

/**
 * ChannelQRDialog 是「扫码开通」弹窗：打开即生成二维码并轮询状态，关闭即取消会话。
 * 手机端流程：扫码 → 一键创建机器人 → 会话变 success，凭证由服务端直接落库。
 */
export function ChannelQRDialog({
  open,
  voicebotId,
  deviceId,
  platform,
  onClose,
  onBound,
}: ChannelQRDialogProps) {
  const { t } = useTranslation(["agents", "common"]);
  const [session, setSession] = useState<ChannelQRSession | null>(null);
  const [startError, setStartError] = useState("");
  const [starting, setStarting] = useState(false);
  const [copied, setCopied] = useState(false);
  const [remaining, setRemaining] = useState(0);
  const failures = useRef(0);

  const start = useCallback(async () => {
    setStarting(true);
    setStartError("");
    setSession(null);
    failures.current = 0;
    try {
      const { data } = await channelApi.startQR(
        voicebotId,
        deviceId,
        platform.name,
      );
      setSession(data);
    } catch (e: unknown) {
      setStartError(
        (e as { response?: { data?: { error?: string } } })?.response?.data
          ?.error ?? t("channelQR.startFailed"),
      );
    } finally {
      setStarting(false);
    }
  }, [voicebotId, deviceId, platform.name, t]);

  useEffect(() => {
    if (!open) {
      setSession(null);
      setStartError("");
      setCopied(false);
      return;
    }
    void start();
  }, [open, start]);

  const terminal = isTerminal(session);

  // 轮询会话状态；成功后把通道状态回填给设备列表。
  useEffect(() => {
    if (!open || !session || terminal) return;
    let cancelled = false;
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const { data } = await channelApi.getQR(
            voicebotId,
            deviceId,
            platform.name,
            session.session_id,
          );
          if (cancelled) return;
          failures.current = 0;
          setSession(data);
          if (data.status === "success" && data.channel) onBound(data.channel);
        } catch {
          if (cancelled) return;
          // 会话可能已被清理（manager 重启 / 超过保留窗口）：连续失败后让用户重新生成。
          failures.current += 1;
          if (failures.current >= maxPollFailures) {
            setSession({ ...session, status: "expired", qr_content: undefined });
          }
        }
      })();
    }, pollInterval);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [open, session, terminal, voicebotId, deviceId, platform.name, onBound]);

  // 二维码有效期倒计时。
  useEffect(() => {
    if (!open || !session || terminal) return;
    const tick = () =>
      setRemaining(
        Math.max(
          0,
          Math.floor(
            (new Date(session.expires_at).getTime() - Date.now()) / 1000,
          ),
        ),
      );
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [open, session, terminal]);

  const close = () => {
    if (session && session.status !== "success") {
      // 关掉弹窗就别让服务端再轮询到 TTL；失败不影响关闭。
      void channelApi
        .cancelQR(voicebotId, deviceId, platform.name, session.session_id)
        .catch(() => {});
    }
    onClose();
  };

  const copyLink = () => {
    if (!session?.qr_content) return;
    navigator.clipboard
      .writeText(session.qr_content)
      .then(() => setCopied(true))
      .catch(() => setCopied(false));
  };

  const showQR = session && !terminal && session.qr_content;

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) close();
      }}
    >
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-white flex items-center gap-2">
            <QrCode className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
            {t("channelQR.title", { platform: platform.display_name })}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2 text-sm">
          {starting && (
            <div className="flex items-center justify-center gap-2 py-10 text-zinc-400 text-xs">
              <Loader2 className="w-4 h-4 animate-spin" />
              {t("channelQR.generating")}
            </div>
          )}

          {startError && (
            <div className="space-y-3 py-2">
              <p className="flex items-start gap-2 text-xs text-red-400">
                <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                {startError}
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => void start()}
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
              >
                <RefreshCw className="w-3.5 h-3.5" />
                {t("channelQR.regenerate")}
              </Button>
            </div>
          )}

          {showQR && (
            <>
              <p className="text-xs text-zinc-400">
                {t("channelQR.scanHint")}
              </p>
              <div className="flex flex-col items-center gap-3">
                <div className="rounded-lg bg-white p-3">
                  <QRCodeSVG
                    value={session.qr_content ?? ""}
                    size={168}
                    marginSize={0}
                  />
                </div>
                <p className="text-xs text-zinc-500">
                  {t("channelQR.expires", {
                    status: t(statusKey[session.status]),
                    time: formatRemaining(remaining),
                  })}
                </p>
              </div>
              <p className="flex items-start gap-2 text-xs text-amber-400/90">
                <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                {t("channelQR.replaceWarning")}
              </p>
            </>
          )}

          {session?.status === "success" && (
            <div className="flex flex-col items-center gap-2 py-6">
              <CheckCircle2 className="w-8 h-8 text-emerald-400" />
              <p className="text-sm text-zinc-200">
                {t(statusKey.success)}
              </p>
              <p className="text-xs text-zinc-500">
                {t("channelQR.successHint")}
              </p>
            </div>
          )}

          {(session?.status === "expired" || session?.status === "failed") && (
            <div className="space-y-3 py-2">
              <p className="flex items-start gap-2 text-xs text-red-400">
                <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                {session.error || t(statusKey[session.status])}
              </p>
              <p className="text-xs text-zinc-500">
                {t("channelQR.manualHint")}
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => void start()}
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
              >
                <RefreshCw className="w-3.5 h-3.5" />
                {t("channelQR.regenerateQR")}
              </Button>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2">
          {showQR && (
            <button
              onClick={copyLink}
              className="inline-flex items-center gap-1.5 text-xs text-zinc-400 hover:text-white transition-colors px-2 py-1"
            >
              <Copy className="w-3.5 h-3.5" />
              {copied ? t("common:action.copied") : t("channelQR.copyLink")}
            </button>
          )}
          <Button
            variant="outline"
            onClick={close}
            className="h-8 border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
          >
            {session?.status === "success"
              ? t("channelQR.done")
              : t("common:action.cancel")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
