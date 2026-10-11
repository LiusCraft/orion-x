// 「价格」dialog：新建 / 编辑未生效版本 / 调价（对已生效版本新增一版）。
//
// 价格是版本化的运营数据：改价 = 新增一条 effective_from 更晚的版本，不覆盖历史
// （§3.2）。三种模式共用一个表单，差别只在提交动作：
//   - create：新建一版，计费项与资源粒度可改；
//   - edit：改一版还没生效的价格。它没有被任何用量事件匹配过，改单价 / 单位 / 起步价 /
//     舍入 / 生效时间都是安全的（服务端 PUT 只认这几个字段）；
//   - adjust：已生效的版本不原地改价——按事件发生时刻匹配价格的机制下，原地改会改写
//     整个生效窗口（含已记录未结算的用量）。调价 = 以它为模板新增一版，再把旧版本的
//     effective_to 收到新版本生效时刻，一次点击完成。
//
// 录入时最容易错的是微单位——单价必须是整数微元（int64），所以表单里不让人直接填
// micro，而是填「每 N 个 unit 收 X 元」两个框，前端算成 unit_price_micro / unit_size：
// 精度不够就抬 unit_size，而不是把单价舍成 0。
//
// 资源维度优先用下拉（按粒度列出模型 / 厂商 / 音色，提交的是内部 ID），下拉里没有
// （别的用户的资源、列表加载失败）就切「手动填 ID」兜底——手抄 UUID 写错的后果是不
// 报错、永远匹配不上，所以能不手抄就不手抄。
//
// 阶梯的不变量在服务端校验（up_to 严格递增、只有最后一档可为空），这里先拦一道。

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
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
	type BillingPrice,
	type BillingPriceCreate,
	type BillingResourceType,
	type BillingRounding,
	type BillingTier,
} from "@/lib/api";
import {
	billingErrorMessage,
	chargeModeLabel,
	formatDate,
	formatUnitPrice,
	itemLabel,
	meterSourceLabel,
	microToYuan,
	resourceName,
	resourceTypeLabel,
	roundingLabel,
	scopeLabel,
	toDateTimeLocalValue,
	toRFC3339,
	unitLabel,
	yuanToMicro,
} from "@/lib/billing";
import {
	resourceNameIndex,
	resourceOptions,
	type BillingResourceLists,
} from "@/lib/billingResources";

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

/** 把一版已有价格的阶梯摊回表单：上限 0 是「不封顶」，单价从微元无损换算成元。 */
function draftTiers(price?: BillingPrice): TierDraft[] {
	if (!price || !price.tiers) return [];
	return price.tiers.map((tier) => ({
		upTo: tier.up_to > 0 ? String(tier.up_to) : "",
		amount: microToYuan(tier.unit_price_micro),
	}));
}

export type PriceDialogMode = "create" | "edit" | "adjust";

interface PriceDialogBaseProps {
	items: BillingItem[];
	resources: BillingResourceLists;
	onClose: () => void;
	/** 成功后关掉对话框；调价的「新版本建好、旧版本没停成」用 kind=error 报出来。 */
	onDone: (message: string, kind?: "ok" | "error") => void;
}

export type PriceDialogProps =
	| (PriceDialogBaseProps & { mode: "create"; price?: undefined })
	| (PriceDialogBaseProps & { mode: "edit" | "adjust"; price: BillingPrice });

export function PriceDialog(props: PriceDialogProps) {
	const { t } = useTranslation(["billingAdmin", "common"]);
	const { items, resources, mode, onClose, onDone } = props;
	const price = props.price;
	const editing = mode !== "create";

	const names = useMemo(() => resourceNameIndex(resources), [resources]);

	const [itemCode, setItemCode] = useState(price?.item_code ?? items[0]?.code ?? "");
	const [unitSize, setUnitSize] = useState(
		price ? String(price.unit_size) : (DEFAULT_UNIT_SIZE[items[0]?.unit ?? ""] ?? "1"),
	);
	const [amount, setAmount] = useState(price ? microToYuan(price.unit_price_micro) : "");
	const [rounding, setRounding] = useState<BillingRounding>(price?.rounding ?? "none");
	const [minCharge, setMinCharge] = useState(
		price && price.min_charge_micro > 0 ? microToYuan(price.min_charge_micro) : "",
	);
	const [currency, setCurrency] = useState(price?.currency ?? "CNY");
	const [resourceType, setResourceType] = useState<BillingResourceType>(
		price?.resource_type ?? "item",
	);
	const [resourceId, setResourceId] = useState(price?.resource_id ?? "");
	const [manualResource, setManualResource] = useState(false);
	const [effectiveFrom, setEffectiveFrom] = useState(
		toDateTimeLocalValue(
			mode === "edit" && price ? new Date(price.effective_from) : new Date(),
		),
	);
	const [tiers, setTiers] = useState<TierDraft[]>(() => draftTiers(price));
	const [saving, setSaving] = useState(false);
	const [formError, setFormError] = useState("");

	const item = items.find((candidate) => candidate.code === itemCode);

	// 新建时切换计费项才重置依赖计费项的默认值：单位基数与默认舍入口径（duration → ceil）。
	// 编辑 / 调价沿用这一版自己的值，不能被默认值覆盖。
	useEffect(() => {
		if (mode !== "create" || !item) return;
		setUnitSize(DEFAULT_UNIT_SIZE[item.unit] ?? "1");
		setRounding(item.charge_mode === "duration" ? "ceil" : "none");
	}, [mode, item]);

	// 列表拉取失败或目录为空时都能走到这里：没有计费项就录不了价
	const itemsError = items.length === 0 ? t("price.catalogEmpty") : "";

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
				return t("price.tierUpToInteger", { n: index + 1 });
			}
			if (!isLast && row.upToText === "") {
				return t("price.tierUpToRequired", { n: index + 1 });
			}
			if (row.upTo !== 0 && row.upTo <= previous) {
				return t("price.tierUpToIncreasing", {
					n: index + 1,
					previous,
				});
			}
			if (row.amountMicro === null || row.amountMicro < 0) {
				return t("price.tierAmountInvalid", { n: index + 1 });
			}
			previous = row.upTo === 0 ? previous : row.upTo;
		}
		return "";
	}, [tierRows, t]);

	const previewPrice =
		amountMicro === null || !unitSizeValid
			? null
			: {
					item_code: itemCode,
					currency: currency.trim() || "CNY",
					unit_price_micro: amountMicro,
					unit_size: unitSizeValue,
				};

	const options = useMemo(() => {
		const list = resourceOptions(resourceType, resources);
		// 当前值不在列表里（别的用户的资源 / 列表没加载出来）也要能看见、不被静默清掉
		if (resourceId && !list.some((option) => option.value === resourceId)) {
			return [
				{
					value: resourceId,
					label: t("price.notInList", { id: resourceId }),
				},
				...list,
			];
		}
		return list;
	}, [resourceType, resources, resourceId, t]);

	const manualResourceEntry = manualResource || options.length === 0;
	const lockedResourceName = price
		? resourceName(names, price.resource_type, price.resource_id) || price.resource_id
		: "";

	const canSubmit =
		!saving &&
		itemCode !== "" &&
		unitSizeValid &&
		amountMicro !== null &&
		amountMicro >= 0 &&
		minChargeMicro !== null &&
		tierError === "" &&
		(resourceType === "item" || resourceId.trim() !== "");

	/** 提交时说清这一版是怎么来的，调价的那条路要额外把旧版本停掉。 */
	const modeLabel =
		mode === "create"
			? t("price.submitCreate")
			: mode === "edit"
				? t("price.submitEdit")
				: t("price.submitAdjust");
	const failureLabel =
		mode === "create"
			? t("price.failCreate")
			: mode === "edit"
				? t("price.failEdit")
				: t("price.failAdjust");

	const tierPayload = () =>
		tierRows.map<BillingTier>((row) => ({
			up_to: row.upToText === "" ? 0 : row.upTo,
			unit_price_micro: row.amountMicro ?? 0,
		}));

	const submit = async () => {
		setFormError("");
		if (!unitSizeValid) {
			setFormError(t("price.errUnitSize"));
			return;
		}
		if (amountMicro === null) {
			setFormError(t("price.errUnitPrice"));
			return;
		}
		if (minChargeMicro === null) {
			setFormError(t("price.errMinCharge"));
			return;
		}
		if (resourceType !== "item" && resourceId.trim() === "") {
			setFormError(
				t("price.errResourceRequired", {
					type: resourceTypeLabel(resourceType),
				}),
			);
			return;
		}
		if (tierError !== "") {
			setFormError(tierError);
			return;
		}
		const parsedEffective = new Date(effectiveFrom);
		const effectiveValid = !Number.isNaN(parsedEffective.getTime());
		// 新建 / 调价留空按「现在」处理（和服务端一致）；调价时这一刻也是旧版本的停用时刻。
		// 编辑留空则不动原来的生效时间。
		const effectiveDate = effectiveValid ? parsedEffective : new Date();
		if (mode === "adjust" && price && effectiveDate <= new Date(price.effective_from)) {
			setFormError(
				t("price.errEffectiveOrder", {
					date: formatDate(price.effective_from),
				}),
			);
			return;
		}

		setSaving(true);
		try {
			if (mode === "edit" && price) {
				await billingAdminApi.updatePrice(price.id, {
					currency: currency.trim() || "CNY",
					unit_price_micro: amountMicro,
					unit_size: unitSizeValue,
					min_charge_micro: minChargeMicro,
					rounding,
					// 空数组 = 清掉阶梯，所以这里总是带上 tiers
					tiers: tierPayload(),
					effective_from: effectiveValid ? toRFC3339(parsedEffective) : undefined,
				});
				onDone(
					t("price.successEdit", {
						item: itemLabel(price.item_code),
						scope: scopeLabel(price, names),
					}),
				);
				return;
			}

			const payload: BillingPriceCreate = {
				item_code: itemCode,
				account_id: price?.account_id ? price.account_id : undefined,
				currency: currency.trim() || "CNY",
				unit_price_micro: amountMicro,
				unit_size: unitSizeValue,
				min_charge_micro: minChargeMicro,
				rounding,
				resource_type: resourceType,
				resource_id: resourceType === "item" ? "" : resourceId.trim(),
				effective_from: toRFC3339(effectiveDate),
				...(tiers.length > 0 ? { tiers: tierPayload() } : {}),
			};
			const created = (await billingAdminApi.createPrice(payload)).data;
			if (mode === "adjust" && price) {
				// 旧版本还有一段没被覆盖的窗口才收紧它；收紧失败不算全败，但要说清楚
				const overlaps =
					!price.effective_to || new Date(price.effective_to) > effectiveDate;
				if (overlaps) {
					try {
						await billingAdminApi.updatePrice(price.id, {
							effective_to: effectiveDate.toISOString(),
						});
					} catch (err) {
						onDone(
							t("price.adjustPartialFail", {
								date: formatDate(created.effective_from),
								error: billingErrorMessage(
									err,
									t("shared.deactivateFailed"),
								),
							}),
							"error",
						);
						return;
					}
				}
				onDone(
					t("price.successAdjust", {
						item: itemLabel(created.item_code),
						scope: scopeLabel(created, names),
						date: formatDate(created.effective_from),
					}),
				);
				return;
			}

			onDone(
				t("price.successCreate", {
					item: itemLabel(itemCode),
					price: previewPrice ? formatUnitPrice(previewPrice) : "",
				}),
			);
		} catch (err) {
			setFormError(billingErrorMessage(err, failureLabel));
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
						{mode === "create"
							? t("price.titleCreate")
							: mode === "edit"
								? t("price.titleEdit")
								: t("price.titleAdjust")}
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4 py-2">
					{mode === "adjust" && (
						<p className="rounded-lg border border-amber-400/20 bg-amber-400/10 px-3 py-2 text-[11px] text-amber-300/90 leading-relaxed">
							{t("price.adjustNotice")}
						</p>
					)}

					{editing && price && (
						<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5 space-y-1">
							<div className="flex flex-wrap items-center gap-x-4 gap-y-1">
								<span className="text-[11px] text-zinc-500">
									{t("price.scopeLabel")}
									<span className="ml-1 text-zinc-300">
										{scopeLabel(price, names)}
									</span>
								</span>
								{price.account_id && (
									<span className="text-[11px] text-zinc-500">
										{t("shared.account")}
										<span className="ml-1 text-zinc-300 font-mono">
											{price.account_id}
										</span>
									</span>
								)}
								<span className="text-[11px] text-zinc-500">
									{t("price.currentEffective")}
									<span className="ml-1 text-zinc-300 font-mono">
										{formatDate(price.effective_from)} ~{" "}
										{price.effective_to
											? formatDate(price.effective_to)
											: t("shared.forever")}
									</span>
								</span>
							</div>
							<p className="text-[11px] text-zinc-600">
								{mode === "edit"
									? t("price.noteEdit")
									: t("price.noteAdjust")}
							</p>
						</div>
					)}

					<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								{t("shared.item")}
							</Label>
							<SimpleSelect
								value={itemCode}
								onValueChange={setItemCode}
								disabled={editing}
								placeholder={t("price.itemPlaceholder")}
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
								{t("price.effectiveFrom")}
							</Label>
							<Input
								type="datetime-local"
								value={effectiveFrom}
								onChange={(e) => setEffectiveFrom(e.target.value)}
								className="text-sm font-mono"
							/>
							<p className="text-[11px] text-zinc-600">
								{mode === "adjust"
									? t("price.effectiveHintAdjust")
									: mode === "edit"
										? t("price.effectiveHintEdit")
										: t("price.effectiveHintCreate")}
							</p>
						</div>
					</div>

					{itemsError && (
						<p className="text-xs text-red-400 leading-relaxed">{itemsError}</p>
					)}

					{item && (
						<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5 flex flex-wrap items-center gap-x-4 gap-y-1">
							<span className="text-[11px] text-zinc-500">
								{t("price.unitLabel")}
								<span className="ml-1 text-zinc-300 font-mono">
									{unitLabel(item.unit)}
								</span>
							</span>
							<span className="text-[11px] text-zinc-500">
								{t("price.modeLabel")}
								<span className="ml-1 text-zinc-300">
									{chargeModeLabel(item.charge_mode)}
								</span>
							</span>
							<span className="text-[11px] text-zinc-500">
								{t("price.defaultRoundingLabel")}
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
							{t("price.unitPrice")}
							<span className="ml-1 text-zinc-600 normal-case">
								{t("price.unitPricePer", { unit: unitLabel(item?.unit) })}
							</span>
						</Label>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
							<div className="space-y-1.5">
								<div className="flex items-center gap-2">
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										{t("price.per")}
									</span>
									<Input
										value={unitSize}
										onChange={(e) => setUnitSize(e.target.value)}
										inputMode="numeric"
										className="text-sm font-mono"
									/>
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										{t("price.countUnit")}
									</span>
								</div>
								<p
									className={
										unitSizeValid
											? "text-[11px] text-zinc-600"
											: "text-[11px] text-red-400"
									}
								>
									{t("price.unitSizeHint")}
								</p>
							</div>
							<div className="space-y-1.5">
								<div className="flex items-center gap-2">
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										{t("price.charge")}
									</span>
									<Input
										value={amount}
										onChange={(e) => setAmount(e.target.value)}
										placeholder="2"
										inputMode="decimal"
										className="text-sm font-mono"
									/>
									<span className="text-xs text-zinc-500 whitespace-nowrap">
										{t("price.currencyUnit")}
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
										? t("price.maxDecimals")
										: amountMicro === null
											? t("price.invalidFormat")
											: t("price.microEquals", {
													amount: amountMicro.toLocaleString("en-US"),
												})}
								</p>
							</div>
						</div>
						{previewPrice && (
							<div className="rounded-lg border border-violet-500/20 bg-violet-600/10 px-3 py-2 flex items-center gap-2">
								<span className="text-[11px] text-zinc-400">
									{t("price.converted")}
								</span>
								<span className="text-xs text-violet-300 font-mono">
									{formatUnitPrice(previewPrice)}
								</span>
								<span className="text-[11px] text-zinc-500 font-mono ml-auto">
									unit_price_micro={amountMicro} / unit_size={unitSize}
								</span>
							</div>
						)}
						<p className="text-[11px] text-zinc-600 leading-relaxed">
							{t("price.microNote")}
						</p>
					</div>

					<div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								{t("price.minCharge")}
							</Label>
							<Input
								value={minCharge}
								onChange={(e) => setMinCharge(e.target.value)}
								placeholder={t("price.minChargePlaceholder")}
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
									? t("price.invalidFormat")
									: t("price.minChargeHint")}
							</p>
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								{t("price.roundingLabel")}
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
								{t("price.roundingHint")}
							</p>
						</div>
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								{t("price.currency")}
							</Label>
							<Input
								value={currency}
								onChange={(e) => setCurrency(e.target.value)}
								className="text-sm font-mono"
							/>
							<p className="text-[11px] text-zinc-600">
								{t("price.currencyHint")}
							</p>
						</div>
					</div>

					<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
						<div className="space-y-1.5">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								{t("price.resourceTypeLabel")}
							</Label>
							<SimpleSelect
								value={resourceType}
								disabled={editing}
								onValueChange={(value) => {
									setResourceType(value as BillingResourceType);
									setResourceId("");
									setManualResource(false);
								}}
								options={(
									["item", "provider", "model", "voice"] as const
								).map((value) => ({
									value,
									label:
										value === "item"
											? t("price.resourceTypeItem")
											: resourceTypeLabel(value),
								}))}
							/>
							<p className="text-[11px] text-zinc-600">
								{t("price.priorityHint")}
							</p>
						</div>
						<div className="space-y-1.5">
							<div className="flex items-center justify-between gap-2">
								<Label className="text-xs text-zinc-400 uppercase tracking-wide">
									{t("price.resourceLabel")}
									<IdHint resourceType={resourceType} />
								</Label>
								{!editing &&
									resourceType !== "item" &&
									options.length > 0 && (
										<button
											type="button"
											onClick={() =>
												setManualResource((value) => !value)
											}
											className="text-[11px] text-violet-400 hover:text-violet-300 transition-colors cursor-pointer"
										>
											{manualResource
												? t("price.listToggle")
												: t("price.manualToggle")}
										</button>
									)}
							</div>
							{editing ? (
								<Input
									value={lockedResourceName}
									disabled
									title={price?.resource_id}
									placeholder={t("price.lockedPlaceholder")}
									className="text-sm font-mono disabled:opacity-40"
								/>
							) : manualResourceEntry ? (
								<Input
									value={resourceId}
									onChange={(e) => setResourceId(e.target.value)}
									disabled={resourceType === "item"}
									placeholder={
										resourceType === "item"
											? t("price.lockedPlaceholder")
											: t("price.manualPlaceholder")
									}
									className="text-sm font-mono disabled:opacity-40"
								/>
							) : (
								<SimpleSelect
									value={resourceId}
									onValueChange={setResourceId}
									placeholder={t("price.selectPlaceholder", {
										type: resourceTypeLabel(resourceType),
									})}
									options={options}
								/>
							)}
							<p className="text-[11px] text-zinc-600">
								{editing
									? t("price.resourceHintEdit")
									: resourceType === "item"
										? t("price.resourceHintItem")
										: manualResourceEntry
											? t("price.resourceHintManual")
											: t("price.resourceHintSelect")}
							</p>
							{!editing &&
								resourceType !== "item" &&
								resources.missing.includes(resourceType) && (
									<p className="text-[11px] text-amber-400/80">
										{t("price.resourceMissing", {
											type: resourceTypeLabel(resourceType),
										})}
									</p>
								)}
						</div>
					</div>

					<div className="space-y-2">
						<div className="flex items-center justify-between">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								{t("price.tiers")}
								<span className="ml-1 text-zinc-600 normal-case">
									{t("price.tiersHint")}
								</span>
							</Label>
							<button
								onClick={() => setTiers((rows) => [...rows, emptyTier()])}
								className="inline-flex items-center gap-1 text-[11px] text-violet-400 hover:text-violet-300 transition-colors cursor-pointer"
							>
								<Plus className="w-3 h-3" strokeWidth={1.5} />
								{t("price.addTier")}
							</button>
						</div>
						{tiers.length === 0 ? (
							<p className="text-[11px] text-zinc-600">
								{t("price.noTiers")}
							</p>
						) : (
							<div className="space-y-2">
								{tiers.map((tier, index) => (
									<div
										key={index}
										className="flex items-center gap-2"
									>
										<span className="text-[11px] text-zinc-500 w-10">
											{t("price.tierN", { n: index + 1 })}
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
											placeholder={t("price.tierUpToPlaceholder")}
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
											placeholder={t("price.tierAmountPlaceholder", {
												size: unitSize || "N",
											})}
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
											aria-label={t("price.removeTier")}
										>
											<Trash2
												className="w-3.5 h-3.5"
												strokeWidth={1.5}
											/>
										</button>
									</div>
								))}
								<p className="text-[11px] text-zinc-600 leading-relaxed">
									{t("price.tiersNote")}
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
						{t("common:action.cancel")}
					</Button>
					<Button
						onClick={submit}
						disabled={!canSubmit}
						className="bg-violet-600 hover:bg-violet-500 text-white"
					>
						{saving ? t("common:action.submitting") : modeLabel}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

function IdHint({ resourceType }: { resourceType: BillingResourceType }) {
	const { t } = useTranslation("billingAdmin");
	if (resourceType === "item") {
		return (
			<span className="ml-1 text-zinc-600 normal-case">{t("price.idHintItem")}</span>
		);
	}
	return (
		<span className="ml-1 text-zinc-600 normal-case">
			{t("price.idHintOther", { type: resourceTypeLabel(resourceType) })}
		</span>
	);
}
