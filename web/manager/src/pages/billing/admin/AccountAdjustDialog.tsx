// 账户行内操作的两个 dialog：人工调整余额（可正可负）与限定计费项的赠款。
//
// 两者的接口是同一个（POST /billing/accounts/:id/adjust），差别只在 grant 标志：
//   - 调整：金额可正可负，note 必填，直接改 balance_micro；
//   - 赠款：金额必须为正 + item_code 必填，落 billing_grants 而不是余额，ref_id 是
//     幂等来源（同 account + item + ref_id 只发一次）。

import { useEffect, useState } from "react";
import { HandCoins, SlidersHorizontal } from "lucide-react";
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
import { SimpleSelect } from "@/components/ui/select";
import { billingAdminApi, type BillingAccount, type BillingItem } from "@/lib/api";
import {
	billingErrorMessage,
	formatMicro,
	isBillingDisabled,
	meterSourceLabel,
	subjectTypeLabel,
	yuanToMicro,
} from "@/lib/billing";

const GRANT_HINT =
	"赠款是限定计费项的额度，不写进可用余额：结算时按 grant → balance → credit_limit 的顺序消耗。ref_id 是幂等来源，同一 ref_id 只发一次——重复提交不会重复发放。";

export function AccountAdjustDialog({
	account,
	grant,
	onClose,
	onDone,
}: {
	account: BillingAccount;
	grant: boolean;
	onClose: () => void;
	onDone: (message: string) => void;
}) {
	const [amount, setAmount] = useState("");
	const [note, setNote] = useState("");
	const [itemCode, setItemCode] = useState("");
	const [refId, setRefId] = useState("");
	const [items, setItems] = useState<BillingItem[]>([]);
	const [itemsError, setItemsError] = useState("");
	const [saving, setSaving] = useState(false);
	const [formError, setFormError] = useState("");

	useEffect(() => {
		let cancelled = false;
		billingAdminApi
			.items()
			.then(({ data }) => {
				if (!cancelled) setItems(data.items);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				setItemsError(
					isBillingDisabled(err)
						? "计费未启用，无法加载计费项"
						: billingErrorMessage(err, "加载计费项目录失败"),
				);
			});
		return () => {
			cancelled = true;
		};
	}, []);

	const micro = yuanToMicro(amount);
	const amountHint =
		amount.trim() === ""
			? "元，最多 6 位小数"
			: micro === null
				? "金额格式不对"
				: `${micro.toLocaleString("en-US")} 微元`;

	const canSubmit =
		!saving &&
		micro !== null &&
		micro !== 0 &&
		(!grant || micro > 0) &&
		note.trim() !== "" &&
		(!grant || (itemCode !== "" && refId.trim() !== ""));

	const submit = async () => {
		setFormError("");
		if (micro === null) {
			setFormError("金额格式不对：只接受数字，最多 6 位小数。");
			return;
		}
		if (grant && micro <= 0) {
			setFormError("赠款金额必须为正数。");
			return;
		}
		if (!grant && micro === 0) {
			setFormError("调整金额不能为 0。");
			return;
		}
		if (note.trim() === "") {
			setFormError("备注必填：这笔调整要能被人看懂。");
			return;
		}
		if (grant && itemCode === "") {
			setFormError("赠款必须指定计费项。");
			return;
		}
		if (grant && refId.trim() === "") {
			setFormError("赠款必须带 ref_id（幂等来源），否则重试会重复发放。");
			return;
		}

		setSaving(true);
		try {
			await billingAdminApi.adjust(account.id, {
				amount_micro: micro,
				note: note.trim(),
				...(itemCode ? { item_code: itemCode } : {}),
				...(grant ? { grant: true, ref_id: refId.trim() } : {}),
			});
			const message = grant
				? `已向账户 ${account.id} 发放赠款 ${formatMicro(micro, account.currency)}（限定 ${itemCode}），ref_id=${refId.trim()}`
				: `已${micro > 0 ? "增加" : "扣减"}账户 ${account.id} 余额 ${formatMicro(Math.abs(micro), account.currency)}`;
			onDone(message);
		} catch (err) {
			setFormError(billingErrorMessage(err, "调整失败，请重试"));
		} finally {
			setSaving(false);
		}
	};

	return (
		<Dialog open onOpenChange={(open: boolean) => !open && onClose()}>
			<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-md">
				<DialogHeader>
					<DialogTitle className="text-white flex items-center gap-2">
						{grant ? (
							<HandCoins className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
						) : (
							<SlidersHorizontal
								className="w-4 h-4 text-violet-400"
								strokeWidth={1.5}
							/>
						)}
						{grant ? "发放赠款" : "调整余额"}
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4 py-2">
					<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5">
						<p className="text-[10px] text-zinc-500 mb-1">账户</p>
						<p className="text-xs text-zinc-300 font-mono break-all">
							{account.id}
						</p>
						<div className="flex items-center gap-3 mt-1.5 text-[11px] text-zinc-500">
							<span>
								{subjectTypeLabel(account.subject_type)} · {account.subject_id}
							</span>
							<span className="font-mono">
								可用 {formatMicro(account.balance_micro, account.currency)}
							</span>
							<span className="font-mono">
								冻结 {formatMicro(account.frozen_micro, account.currency)}
							</span>
						</div>
					</div>

					{grant && (
						<div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2.5">
							<p className="text-[11px] text-amber-200/90 leading-relaxed">
								{GRANT_HINT}
							</p>
						</div>
					)}

					<div className="space-y-1.5">
						<Label className="text-xs text-zinc-400 uppercase tracking-wide">
							金额（元）
							<span className="ml-1 text-zinc-600 normal-case">
								{grant ? "必须为正数" : "可正可负"}
							</span>
						</Label>
						<Input
							value={amount}
							onChange={(e) => setAmount(e.target.value)}
							placeholder={grant ? "5.00" : "-5.00"}
							inputMode="decimal"
							className="text-sm font-mono"
						/>
						<p
							className={
								micro === null && amount.trim() !== ""
									? "text-[11px] text-red-400"
									: "text-[11px] text-zinc-600"
							}
						>
							{amountHint}
						</p>
					</div>

					<div className="space-y-1.5">
						<Label className="text-xs text-zinc-400 uppercase tracking-wide">
							计费项
							<span className="ml-1 text-zinc-600 normal-case">
								{grant ? "必填，赠款限定这个计费项" : "可选，会记进流水"}
							</span>
						</Label>
						<SimpleSelect
							value={itemCode}
							onValueChange={setItemCode}
							placeholder="不限定计费项"
							className="font-mono"
							options={items.map((item) => ({
								value: item.code,
								label: `${item.name}（${item.code}）`,
								group: meterSourceLabel(item.meter_source),
							}))}
						/>
						{itemsError && (
							<p className="text-[11px] text-red-400">{itemsError}</p>
						)}
					</div>

					{grant && (
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								ref_id
								<span className="ml-1 text-zinc-600 normal-case">
									幂等来源，同一 ref_id 只发一次
								</span>
							</Label>
							<Input
								value={refId}
								onChange={(e) => setRefId(e.target.value)}
								placeholder="signup:2026-09"
								className="text-sm font-mono"
							/>
						</div>
					)}

					<div className="space-y-1.5">
						<Label className="text-xs text-zinc-400 uppercase tracking-wide">
							备注<span className="ml-1 text-red-400">*</span>
						</Label>
						<Input
							value={note}
							onChange={(e) => setNote(e.target.value)}
							placeholder="例如：注册赠送 / 误扣回退（工单 #123）"
							className="text-sm"
						/>
					</div>

					{formError && (
						<p className="text-xs text-red-400 leading-relaxed">
							{formError}
						</p>
					)}
				</div>

				<DialogFooter>
					<Button
						variant="outline"
						onClick={onClose}
						className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
					>
						取消
					</Button>
					<Button
						onClick={submit}
						disabled={!canSubmit}
						className="bg-violet-600 hover:bg-violet-500 text-white"
					>
						{saving ? "提交中..." : grant ? "发放赠款" : "确认调整"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
