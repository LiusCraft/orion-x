// 账户行内操作的两个 dialog：人工调整余额（可正可负）与限定计费项的赠款。
//
// 两者的接口是同一个（POST /billing/accounts/:id/adjust），差别只在 grant 标志：
//   - 调整：金额可正可负，note 必填，直接改 balance_micro；
//   - 赠款：金额必须为正 + item_code 必填，落 billing_grants 而不是余额，ref_id 是
//     幂等来源（同 account + item + ref_id 只发一次）。

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
	const { t } = useTranslation(["billingAdmin", "common"]);
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
						? t("adjust.loadDisabled")
						: billingErrorMessage(err, t("adjust.loadFailed")),
				);
			});
		return () => {
			cancelled = true;
		};
	}, [t]);

	const micro = yuanToMicro(amount);
	const amountHint =
		amount.trim() === ""
			? t("adjust.amountHint")
			: micro === null
				? t("adjust.amountInvalid")
				: t("adjust.microHint", { amount: micro.toLocaleString("en-US") });

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
			setFormError(t("adjust.errAmount"));
			return;
		}
		if (grant && micro <= 0) {
			setFormError(t("adjust.errGrantPositive"));
			return;
		}
		if (!grant && micro === 0) {
			setFormError(t("adjust.errAdjustZero"));
			return;
		}
		if (note.trim() === "") {
			setFormError(t("adjust.errNote"));
			return;
		}
		if (grant && itemCode === "") {
			setFormError(t("adjust.errItem"));
			return;
		}
		if (grant && refId.trim() === "") {
			setFormError(t("adjust.errRefId"));
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
				? t("adjust.successGrant", {
						account: account.id,
						amount: formatMicro(micro, account.currency),
						item: itemCode,
						refId: refId.trim(),
					})
				: micro > 0
					? t("adjust.successIncrease", {
							account: account.id,
							amount: formatMicro(micro, account.currency),
						})
					: t("adjust.successDecrease", {
							account: account.id,
							amount: formatMicro(Math.abs(micro), account.currency),
						});
			onDone(message);
		} catch (err) {
			setFormError(billingErrorMessage(err, t("adjust.fail")));
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
						{grant ? t("adjust.grantTitle") : t("adjust.adjustTitle")}
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4 py-2">
					<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5">
						<p className="text-[10px] text-zinc-500 mb-1">
							{t("shared.account")}
						</p>
						<p className="text-xs text-zinc-300 font-mono break-all">
							{account.id}
						</p>
						<div className="flex items-center gap-3 mt-1.5 text-[11px] text-zinc-500">
							<span>
								{subjectTypeLabel(account.subject_type)} · {account.subject_id}
							</span>
							<span className="font-mono">
								{t("adjust.available", {
									amount: formatMicro(
										account.balance_micro,
										account.currency,
									),
								})}
							</span>
							<span className="font-mono">
								{t("adjust.frozen", {
									amount: formatMicro(
										account.frozen_micro,
										account.currency,
									),
								})}
							</span>
						</div>
					</div>

					{grant && (
						<div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2.5">
							<p className="text-[11px] text-amber-200/90 leading-relaxed">
								{t("adjust.grantHint")}
							</p>
						</div>
					)}

					<div className="space-y-1.5">
						<Label className="text-xs text-zinc-400 uppercase tracking-wide">
							{t("adjust.amountLabel")}
							<span className="ml-1 text-zinc-600 normal-case">
								{grant
									? t("adjust.amountPositiveHint")
									: t("adjust.amountSignedHint")}
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
							{t("shared.item")}
							<span className="ml-1 text-zinc-600 normal-case">
								{grant ? t("adjust.itemGrantHint") : t("adjust.itemOptionalHint")}
							</span>
						</Label>
						<SimpleSelect
							value={itemCode}
							onValueChange={setItemCode}
							placeholder={t("adjust.itemPlaceholder")}
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
									{t("adjust.refIdHint")}
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
							{t("shared.note")}
							<span className="ml-1 text-red-400">*</span>
						</Label>
						<Input
							value={note}
							onChange={(e) => setNote(e.target.value)}
							placeholder={t("adjust.notePlaceholder")}
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
						{t("common:action.cancel")}
					</Button>
					<Button
						onClick={submit}
						disabled={!canSubmit}
						className="bg-violet-600 hover:bg-violet-500 text-white"
					>
						{saving
							? t("common:action.submitting")
							: grant
								? t("adjust.grantTitle")
								: t("adjust.submitAdjust")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
