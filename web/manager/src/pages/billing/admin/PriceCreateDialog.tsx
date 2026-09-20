// 「新建价格」dialog。
//
// 价格是版本化的运营数据：改价 = 新增一条 effective_from 更晚的版本，不覆盖历史
// （§3.2）。录入时最容易错的是微单位——单价必须是整数微元（int64），所以表单里不
// 让人直接填 micro，而是填「每 N 个 unit 收 X 元」两个框，前端算成
// unit_price_micro / unit_size：精度不够就抬 unit_size，而不是把单价舍成 0。
//
// 阶梯的不变量在服务端校验（up_to 严格递增、只有最后一档可为空），这里先拦一道。

import { useEffect, useMemo, useState } from "react";
import { Plus, Tag, Trash2 } from "lucide-react";
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
import {
	billingAdminApi,
	type BillingItem,
	type BillingPriceCreate,
	type BillingResourceType,
	type BillingRounding,
	type BillingTier,
} from "@/lib/api";
import {
	billingErrorMessage,
	chargeModeLabel,
	formatUnitPrice,
	itemLabel,
	meterSourceLabel,
	resourceTypeLabel,
	roundingLabel,
	toDateTimeLocalValue,
	toRFC3339,
	unitLabel,
	yuanToMicro,
} from "@/lib/billing";

/** 各单位的默认计价基数：token 按百万、字符按千，其余按 1（每 1 个单位）。 */
const DEFAULT_UNIT_SIZE: Record<string, string> = {
	token: "1000000",
	char: "1000",
	second: "1",
	call: "1",
	byte: "1024",
	period: "1",
};

interface TierDraft {
	upTo: string;
	amount: string;
}

const emptyTier = (): TierDraft => ({ upTo: "", amount: "" });

export function PriceCreateDialog({
	items,
	onClose,
	onDone,
}: {
	items: BillingItem[];
	onClose: () => void;
	onDone: (message: string) => void;
}) {
	const [itemCode, setItemCode] = useState(items[0]?.code ?? "");
	const [unitSize, setUnitSize] = useState(
		DEFAULT_UNIT_SIZE[items[0]?.unit ?? ""] ?? "1",
	);
	const [amount, setAmount] = useState("");
	const [rounding, setRounding] = useState<BillingRounding>("none");
	const [minCharge, setMinCharge] = useState("");
	const [currency, setCurrency] = useState("CNY");
	const [resourceType, setResourceType] = useState<BillingResourceType>("item");
	const [resourceId, setResourceId] = useState("");
	const [effectiveFrom, setEffectiveFrom] = useState(
		toDateTimeLocalValue(new Date()),
	);
	const [tiers, setTiers] = useState<TierDraft[]>([]);
	const [saving, setSaving] = useState(false);
	const [formError, setFormError] = useState("");

	const item = items.find((candidate) => candidate.code === itemCode);

	// 切换计费项时把依赖计费项的默认值重置：单位基数与默认舍入口径（duration → ceil）。
	useEffect(() => {
		if (!item) return;
		setUnitSize(DEFAULT_UNIT_SIZE[item.unit] ?? "1");
		setRounding(item.charge_mode === "duration" ? "ceil" : "none");
	}, [item]);

	// 列表拉取失败或目录为空时都能走到这里：没有计费项就录不了价
	const itemsError =
		items.length === 0
			? "计费项目录是空的：先确认服务端 seed 跑过，再来录价格（价格表为空时每个会话都会被 price_missing 拒掉）。"
			: "";

	const unitSizeValue = Number(unitSize);
	const unitSizeValid = Number.isInteger(unitSizeValue) && unitSizeValue > 0;
	const amountMicro = yuanToMicro(amount);
	const minChargeMicro = minCharge.trim() === "" ? 0 : yuanToMicro(minCharge);

	const tierRows = useMemo(
		() =>
			tiers.map((tier) => ({
				upTo: tier.upTo.trim() === "" ? 0 : Number(tier.upTo),
				amountMicro: yuanToMicro(tier.amount),
				upToText: tier.upTo,
				amountText: tier.amount,
			})),
		[tiers],
	);

	/** 阶梯校验：中间档必须填、严格递增、整数；价格必须合法。 */
	const tierError = useMemo(() => {
		let previous = 0;
		for (let index = 0; index < tierRows.length; index += 1) {
			const row = tierRows[index];
			const isLast = index === tierRows.length - 1;
			if (row.upToText !== "" && (!Number.isInteger(row.upTo) || row.upTo <= 0)) {
				return `第 ${index + 1} 档的上限必须是正整数`;
			}
			if (!isLast && row.upToText === "") {
				return `第 ${index + 1} 档还没填上限：只有最后一档可以不封顶`;
			}
			if (row.upTo !== 0 && row.upTo <= previous) {
				return `第 ${index + 1} 档的上限必须大于上一档（当前上一档是 ${previous}）`;
			}
			if (row.amountMicro === null || row.amountMicro < 0) {
				return `第 ${index + 1} 档的单价不合法`;
			}
			previous = row.upTo === 0 ? previous : row.upTo;
		}
		return "";
	}, [tierRows]);

	const previewPrice =
		amountMicro === null || !unitSizeValid
			? null
			: {
					item_code: itemCode,
					currency: currency.trim() || "CNY",
					unit_price_micro: amountMicro,
					unit_size: unitSizeValue,
				};

	const canSubmit =
		!saving &&
		itemCode !== "" &&
		unitSizeValid &&
		amountMicro !== null &&
		amountMicro >= 0 &&
		minChargeMicro !== null &&
		tierError === "" &&
		(resourceType === "item" || resourceId.trim() !== "");

	const submit = async () => {
		setFormError("");
		if (!unitSizeValid) {
			setFormError("计价基数要是正整数：例如每 1000000 个 token 收 X 元。");
			return;
		}
		if (amountMicro === null) {
			setFormError("单价不合法：只接受数字，最多 6 位小数。");
			return;
		}
		if (minChargeMicro === null) {
			setFormError("起步价不合法：只接受数字，最多 6 位小数。");
			return;
		}
		if (resourceType !== "item" && resourceId.trim() === "") {
			setFormError(
				`停在 ${resourceTypeLabel(resourceType)} 这一级时必须填资源 ID；只做全平台兜底价就用 item 级。`,
			);
			return;
		}
		if (tierError !== "") {
			setFormError(tierError);
			return;
		}

		const payload: BillingPriceCreate = {
			item_code: itemCode,
			currency: currency.trim() || "CNY",
			unit_price_micro: amountMicro,
			unit_size: unitSizeValue,
			min_charge_micro: minChargeMicro,
			rounding,
			resource_type: resourceType,
			resource_id: resourceType === "item" ? "" : resourceId.trim(),
			effective_from: toRFC3339(new Date(effectiveFrom)) ?? undefined,
			...(tiers.length > 0
				? {
						tiers: tierRows.map<BillingTier>((row) => ({
							up_to: row.upToText === "" ? 0 : row.upTo,
							unit_price_micro: row.amountMicro ?? 0,
						})),
					}
				: {}),
		};

		setSaving(true);
		try {
			await billingAdminApi.createPrice(payload);
			onDone(
				`已新增价格版本：${itemLabel(itemCode)} · ${previewPrice ? formatUnitPrice(previewPrice) : ""}`,
			);
		} catch (err) {
			setFormError(billingErrorMessage(err, "新增价格失败"));
		} finally {
			setSaving(false);
		}
	};

	return (
		<Dialog open onOpenChange={(open: boolean) => !open && onClose()}>
			<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-2xl">
				<DialogHeader>
					<DialogTitle className="text-white flex items-center gap-2">
						<Tag className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
						新建价格版本
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4 py-2">
					<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								计费项
							</Label>
							<SimpleSelect
								value={itemCode}
								onValueChange={setItemCode}
								placeholder="选择计费项"
								className="font-mono"
								options={items.map((option) => ({
									value: option.code,
									label: `${option.name}（${option.code}）`,
									group: meterSourceLabel(option.meter_source),
								}))}
							/>
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								生效时间
							</Label>
							<Input
								type="datetime-local"
								value={effectiveFrom}
								onChange={(e) => setEffectiveFrom(e.target.value)}
								className="text-sm font-mono"
							/>
							<p className="text-[11px] text-zinc-600">
								留空按"现在"处理；同一 scope 同一时刻只能有一版价格。
							</p>
						</div>
					</div>

					{itemsError && (
						<p className="text-xs text-red-400 leading-relaxed">{itemsError}</p>
					)}

					{item && (
						<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5 flex flex-wrap items-center gap-x-4 gap-y-1">
							<span className="text-[11px] text-zinc-500">
								单位
								<span className="ml-1 text-zinc-300 font-mono">
									{unitLabel(item.unit)}
								</span>
							</span>
							<span className="text-[11px] text-zinc-500">
								模式
								<span className="ml-1 text-zinc-300">
									{chargeModeLabel(item.charge_mode)}
								</span>
							</span>
							<span className="text-[11px] text-zinc-500">
								默认舍入
								<span className="ml-1 text-zinc-300">
									{roundingLabel(
										item.charge_mode === "duration" ? "ceil" : "none",
									)}
								</span>
							</span>
						</div>
					)}

					<div className="space-y-2">
						<Label className="text-xs text-zinc-400 uppercase tracking-wide">
							单价
							<span className="ml-1 text-zinc-600 normal-case">
								每 N 个 {unitLabel(item?.unit)} 收 X 元
							</span>
						</Label>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
							<div className="space-y-1.5">
								<div className="flex items-center gap-2">
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										每
									</span>
									<Input
										value={unitSize}
										onChange={(e) => setUnitSize(e.target.value)}
										inputMode="numeric"
										className="text-sm font-mono"
									/>
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										个
									</span>
								</div>
								<p
									className={
										unitSizeValid
											? "text-[11px] text-zinc-600"
											: "text-[11px] text-red-400"
									}
								>
									unit_size 必须是正整数
								</p>
							</div>
							<div className="space-y-1.5">
								<div className="flex items-center gap-2">
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										收
									</span>
									<Input
										value={amount}
										onChange={(e) => setAmount(e.target.value)}
										placeholder="2"
										inputMode="decimal"
										className="text-sm font-mono"
									/>
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										元
									</span>
								</div>
								<p
									className={
										amountMicro === null && amount.trim() !== ""
											? "text-[11px] text-red-400"
											: "text-[11px] text-zinc-600"
									}
								>
									{amount.trim() === ""
										? "最多 6 位小数"
										: amountMicro === null
											? "格式不对"
											: `unit_price_micro = ${amountMicro.toLocaleString("en-US")}`}
								</p>
							</div>
						</div>
						{previewPrice && (
							<div className="rounded-lg border border-violet-500/20 bg-violet-600/10 px-3 py-2 flex items-center gap-2">
								<span className="text-[11px] text-zinc-400">换算结果</span>
								<span className="text-xs text-violet-300 font-mono">
									{formatUnitPrice(previewPrice)}
								</span>
								<span className="text-[11px] text-zinc-500 font-mono ml-auto">
									unit_price_micro={amountMicro} / unit_size={unitSize}
								</span>
							</div>
						)}
						<p className="text-[11px] text-zinc-600 leading-relaxed">
							单价必须是整数微元（1e-6 元），精度不够就抬 unit_size，别把价格舍成 0：
							例如「每 token ¥0.0000008」写成「每 1000 万个 token ¥8」
							（unit_price_micro = 8,000,000，unit_size = 10,000,000）。结算时
							qty × 单价 ÷ unit_size 在一次结算内只舍入一次。
						</p>
					</div>

					<div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								起步价（元）
							</Label>
							<Input
								value={minCharge}
								onChange={(e) => setMinCharge(e.target.value)}
								placeholder="留空 = 0"
								inputMode="decimal"
								className="text-sm font-mono"
							/>
							<p
								className={
									minChargeMicro === null
										? "text-[11px] text-red-400"
										: "text-[11px] text-zinc-600"
								}
							>
								{minChargeMicro === null
									? "格式不对"
									: "舍入后取 max(qty × 单价, 起步价)"}
							</p>
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								舍入口径
							</Label>
							<SimpleSelect
								value={rounding}
								onValueChange={(value) =>
									setRounding(value as BillingRounding)
								}
								options={(["none", "ceil", "half:up"] as const).map(
									(value) => ({
										value,
										label: roundingLabel(value),
									}),
								)}
							/>
							<p className="text-[11px] text-zinc-600">
								duration 类默认向上取整，usage 类默认精确
							</p>
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								币种
							</Label>
							<Input
								value={currency}
								onChange={(e) => setCurrency(e.target.value)}
								className="text-sm font-mono"
							/>
							<p className="text-[11px] text-zinc-600">
								单币种部署，P1 不做汇率
							</p>
						</div>
					</div>

					<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								资源粒度
							</Label>
							<SimpleSelect
								value={resourceType}
								onValueChange={(value) => {
									const next = value as BillingResourceType;
									setResourceType(next);
									if (next === "item") setResourceId("");
								}}
								options={(
									["item", "provider", "model", "voice"] as const
								).map((value) => ({
									value,
									label:
										value === "item"
											? "计费项（全平台兜底价）"
											: resourceTypeLabel(value),
								}))}
							/>
							<p className="text-[11px] text-zinc-600">
								匹配优先级：账户协议价 → 音色 → 模型 → 厂商 → 计费项
							</p>
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								资源 ID
								<IdHint resourceType={resourceType} />
							</Label>
							<Input
								value={resourceId}
								onChange={(e) => setResourceId(e.target.value)}
								disabled={resourceType === "item"}
								placeholder={
									resourceType === "item"
										? "item 级价格不带 resource_id"
										: "provider / model / voice 的 ID"
								}
								className="text-sm font-mono disabled:opacity-40"
							/>
							<p className="text-[11px] text-zinc-600">
								item 级留空；停在某一级就必填（层级链：voice → model → provider）
							</p>
						</div>
					</div>

					<div className="space-y-2">
						<div className="flex items-center justify-between">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								阶梯价
								<span className="ml-1 text-zinc-600 normal-case">
									可选，按账期累计量分档累进
								</span>
							</Label>
							<button
								onClick={() => setTiers((rows) => [...rows, emptyTier()])}
								className="inline-flex items-center gap-1 text-[11px] text-violet-400 hover:text-violet-300 transition-colors cursor-pointer"
							>
								<Plus className="w-3 h-3" strokeWidth={1.5} />
								添加一档
							</button>
						</div>
						{tiers.length === 0 ? (
							<p className="text-[11px] text-zinc-600">
								不用阶梯就留空，全部按上面的单价算。
							</p>
						) : (
							<div className="space-y-2">
								{tiers.map((tier, index) => (
									<div
										key={index}
										className="flex items-center gap-2"
									>
										<span className="text-[11px] text-zinc-500 w-10">
											第 {index + 1} 档
										</span>
										<Input
											value={tier.upTo}
											onChange={(e) =>
												setTiers((rows) =>
													rows.map((row, i) =>
														i === index
															? {
																	...row,
																	upTo: e.target
																		.value,
																}
															: row,
													),
												)
											}
											placeholder="累计上限（最后一档留空 = 不封顶）"
											inputMode="numeric"
											className="h-8 text-xs font-mono flex-1"
										/>
										<Input
											value={tier.amount}
											onChange={(e) =>
												setTiers((rows) =>
													rows.map((row, i) =>
														i === index
															? {
																	...row,
																	amount: e.target
																		.value,
																}
															: row,
													),
												)
											}
											placeholder={`单价（元 / ${unitSize || "N"} 个）`}
											inputMode="decimal"
											className="h-8 text-xs font-mono flex-1"
										/>
										<button
											onClick={() =>
												setTiers((rows) =>
													rows.filter((_, i) => i !== index),
												)
											}
											className="p-1.5 text-zinc-500 hover:text-red-400 hover:bg-red-400/10 rounded transition-colors cursor-pointer"
											aria-label="删除这一档"
										>
											<Trash2
												className="w-3.5 h-3.5"
												strokeWidth={1.5}
											/>
										</button>
									</div>
								))}
								<p className="text-[11px] text-zinc-600 leading-relaxed">
									阶梯单价按同一个 unit_size 计；中间档必须填上限且严格递增，只有最后一档可以为空。
									阶梯是分档累进（像个税），不是达标后全量按新价。
								</p>
								{tierError && (
									<p className="text-[11px] text-red-400">{tierError}</p>
								)}
							</div>
						)}
					</div>

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
						取消
					</Button>
					<Button
						onClick={submit}
						disabled={!canSubmit}
						className="bg-violet-600 hover:bg-violet-500 text-white"
					>
						{saving ? "提交中..." : "新增价格"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

function IdHint({ resourceType }: { resourceType: BillingResourceType }) {
	if (resourceType === "item") {
		return <span className="ml-1 text-zinc-600 normal-case">（item 级留空）</span>;
	}
	return (
		<span className="ml-1 text-zinc-600 normal-case">
			（{resourceTypeLabel(resourceType)} 级必填）
		</span>
	);
}
