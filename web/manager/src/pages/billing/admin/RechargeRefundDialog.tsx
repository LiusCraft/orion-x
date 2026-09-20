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
	const [note, setNote] = useState("");
	const [saving, setSaving] = useState(false);
	const [formError, setFormError] = useState("");

	const alreadyRefunded = order.status === "order:refunded";

	const submit = async () => {
		setFormError("");
		if (note.trim() === "") {
			setFormError("备注必填：这笔退款要能被人看懂（工单号、原因）。");
			return;
		}

		setSaving(true);
		try {
			await billingAdminApi.refundRecharge(order.out_trade_no, note.trim());
			onDone(
				`已退款 ${formatMicro(order.amount_micro, order.currency)}（订单 ${order.out_trade_no}）`,
			);
		} catch (err) {
			if (isBillingDisabled(err)) {
				setFormError("当前环境未开通在线充值，无法退款。");
				return;
			}
			setFormError(userFacingError(err, "退款失败，请重试"));
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
						{alreadyRefunded ? "退款详情" : "整单退款"}
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4 py-2">
					<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5">
						<p className="text-[10px] text-zinc-500 mb-1">订单</p>
						<p className="text-xs text-zinc-300 font-mono break-all">
							{order.out_trade_no}
						</p>
						<div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1.5 text-[11px] text-zinc-500">
							<span className="font-mono">
								金额 {formatMicro(order.amount_micro, order.currency)}
							</span>
							<span>{paymentChannelLabel(order.channel)}</span>
							<span>{paymentStatusLabel(order.status)}</span>
							{order.gateway_trade_no && (
								<span className="font-mono truncate" title={order.gateway_trade_no}>
									交易号 {order.gateway_trade_no}
								</span>
							)}
						</div>
						{order.credited_at && (
							<p className="text-[11px] text-zinc-500 mt-1">
								到账时间 {formatTime(order.credited_at)}
							</p>
						)}
						{order.refunded_at && (
							<p className="text-[11px] text-violet-300/80 mt-1">
								退款时间 {formatTime(order.refunded_at)}
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
								<span>
									提交后会把款项退回给付款人，并扣回该账户的余额。余额已消费的账户
									会因此变为欠费。退款只能整单退，不支持部分退款。
								</span>
							</p>
						</div>
					)}

					{!alreadyRefunded && (
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								备注<span className="ml-1 text-red-400">*</span>
							</Label>
							<Input
								value={note}
								onChange={(e) => setNote(e.target.value)}
								placeholder="例如：用户申请退款（工单 #123）"
								className="text-sm"
							/>
							<p className="text-[11px] text-zinc-600">
								会记入退款记录，用于日后查询。
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
						{alreadyRefunded ? "关闭" : "取消"}
					</Button>
					{!alreadyRefunded && (
						<Button
							onClick={submit}
							disabled={saving || note.trim() === ""}
							className="bg-violet-600 hover:bg-violet-500 text-white"
						>
							{saving ? "退款中..." : "确认退款"}
						</Button>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
