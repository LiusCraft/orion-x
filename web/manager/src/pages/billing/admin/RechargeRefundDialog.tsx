// 充值订单的退款对话框（仅 admin）。
//
// 这不是「改一个状态」：服务端会先让支付网关把钱退给付款人，再把余额扣回来、写一条
// refund 流水。所以这里的确认文案必须让操作者明白钱是真的要出去——尤其是余额已经
// 被消费过的账户，退完会变成欠费（这是允许的：钱确实退出去了，账必须跟着走）。
//
// 幂等：同一笔订单重复提交不会重复退款（服务端先查 refund:order:<out_trade_no> 这条
// 流水再决定要不要打网关）。所以「点了没反应就再点一次」是安全的。
//
// 注意：对话框里不要出现配置项名、表名、包名这些我们自己的说法——前端是交付给客户
// 的产品界面，只写「会发生什么」和「要付什么责任」。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle, Undo2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { billingAdminApi, type BillingPaymentOrder } from "@/lib/api";
import {
	formatMicro,
	formatTime,
	isBillingDisabled,
	paymentChannelLabel,
	paymentStatusLabel,
	userFacingError,
} from "@/lib/billing";

export function RechargeRefundDialog({
	order,
	onClose,
	onDone,
}: {
	order: BillingPaymentOrder;
	onClose: () => void;
	onDone: (message: string) => void;
}) {
	const { t } = useTranslation(["billingAdmin", "common"]);
	const [note, setNote] = useState("");
	const [saving, setSaving] = useState(false);
	const [formError, setFormError] = useState("");

	const alreadyRefunded = order.status === "order:refunded";

	const submit = async () => {
		setFormError("");
		if (note.trim() === "") {
			setFormError(t("refund.noteRequired"));
			return;
		}

		setSaving(true);
		try {
			await billingAdminApi.refundRecharge(order.out_trade_no, note.trim());
			onDone(
				t("refund.success", {
					amount: formatMicro(order.amount_micro, order.currency),
					order: order.out_trade_no,
				}),
			);
		} catch (err) {
			if (isBillingDisabled(err)) {
				setFormError(t("refund.disabledError"));
				return;
			}
			setFormError(userFacingError(err, t("refund.fail")));
		} finally {
			setSaving(false);
		}
	};

	return (
		<Dialog open onOpenChange={(open: boolean) => !open && onClose()}>
			<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
				<DialogHeader>
					<DialogTitle className="text-white flex items-center gap-2">
						<Undo2 className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
						{alreadyRefunded
							? t("refund.titleDetails")
							: t("recharge.refundTitle")}
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4 py-2">
					<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5">
						<p className="text-[10px] text-zinc-500 mb-1">
							{t("refund.orderLabel")}
						</p>
						<p className="text-xs text-zinc-300 font-mono break-all">
							{order.out_trade_no}
						</p>
						<div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1.5 text-[11px] text-zinc-500">
							<span className="font-mono">
								{t("refund.amountLabel", {
									amount: formatMicro(
										order.amount_micro,
										order.currency,
									),
								})}
							</span>
							<span>{paymentChannelLabel(order.channel)}</span>
							<span>{paymentStatusLabel(order.status)}</span>
							{order.gateway_trade_no && (
								<span className="font-mono truncate" title={order.gateway_trade_no}>
									{t("refund.tradeNo", { no: order.gateway_trade_no })}
								</span>
							)}
						</div>
						{order.credited_at && (
							<p className="text-[11px] text-zinc-500 mt-1">
								{t("refund.creditedAt", {
									time: formatTime(order.credited_at),
								})}
							</p>
						)}
						{order.refunded_at && (
							<p className="text-[11px] text-violet-300/80 mt-1">
								{t("refund.refundedAt", {
									time: formatTime(order.refunded_at),
								})}
							</p>
						)}
					</div>

					{!alreadyRefunded && (
						<div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2.5">
							<p className="text-[11px] text-amber-200/90 leading-relaxed flex items-start gap-2">
								<AlertTriangle
									className="w-3.5 h-3.5 mt-0.5 shrink-0 text-amber-400"
									strokeWidth={1.5}
								/>
								<span>{t("refund.warning")}</span>
							</p>
						</div>
					)}

					{!alreadyRefunded && (
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								{t("shared.note")}
								<span className="ml-1 text-red-400">*</span>
							</Label>
							<Input
								value={note}
								onChange={(e) => setNote(e.target.value)}
								placeholder={t("refund.notePlaceholder")}
								className="text-sm"
							/>
							<p className="text-[11px] text-zinc-600">
								{t("refund.noteHint")}
							</p>
						</div>
					)}

					{formError && (
						<p className="text-xs text-red-400 leading-relaxed">{formError}</p>
					)}
				</div>

				<DialogFooter>
					<Button
						variant="outline"
						onClick={onClose}
						className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
					>
						{alreadyRefunded
							? t("common:action.close")
							: t("common:action.cancel")}
					</Button>
					{!alreadyRefunded && (
						<Button
							onClick={submit}
							disabled={saving || note.trim() === ""}
							className="bg-violet-600 hover:bg-violet-500 text-white"
						>
							{saving ? t("refund.submitting") : t("refund.submit")}
						</Button>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
