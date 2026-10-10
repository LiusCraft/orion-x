// 计费的展示层工具：金额换算、口径文案、时间段与错误映射。
//
// 这里是纯函数，不 import React，也不 import axios（错误映射只做结构化取值，见
// billingErrorMessage）。所有金额都是 int64 微单位（1e-6 元，见 docs/billing-design.md
// §3.3），页面里不要再出现裸的 `/ 1e6`。
//
// 两种金额口径，别混用：
//   - formatMicro / formatMicroPlain 是"金额列"口径：按量级定长小数 + 千分位，便于对齐；
//   - microToYuan 是"精确值"口径：把微单位无损写成元并去掉多余的 0，用于单价与说明文案。

import type {
	BillingAccountStatus,
	BillingChargeMode,
	BillingMeterSource,
	BillingPrice,
	BillingResourceType,
	BillingRounding,
	BillingSubjectType,
	BillingUnit,
	BillingUsageStatus,
} from "@/lib/api";
import i18n from "@/i18n";

/** 微单位基数：1 元 = 1_000_000 微元（对应 billing.MicroPerUnit）。 */
export const MICRO_PER_YUAN = 1_000_000;

// ---------------------------------------------------------------- 金额

/** 按 step（微元）把微单位金额四舍五入到最近一档。 */
function roundMicroToStep(micro: number, step: number): number {
	if (step <= 1) return micro;
	return micro >= 0
		? Math.round(micro / step) * step
		: -Math.round(-micro / step) * step;
}

function groupThousands(digits: string): string {
	return digits.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/**
 * 把微单位金额无损写成元，去掉小数末尾的 0：`2000000 → "2"`、`800 → "0.0008"`、
 * `4640000 → "4.64"`。不做千分位，负数带 `-`。超出安全整数范围的输入会被拒绝。
 */
export function microToYuan(micro: number): string {
	if (!Number.isFinite(micro)) return "0";
	const sign = micro < 0 ? "-" : "";
	const abs = Math.abs(Math.trunc(micro));
	const whole = Math.floor(abs / MICRO_PER_YUAN);
	const frac = abs - whole * MICRO_PER_YUAN;
	if (frac === 0) return sign + String(whole);
	return `${sign}${whole}.${String(frac).padStart(6, "0").replace(/0+$/, "")}`;
}

/**
 * 金额列的展示值（无货币符号，带千分位）。小数位按量级自适应：≥¥1 两位、≥¥0.01 四位、
 * 更小六位（微单位的精度上限），舍入后跨过 ¥1 就收成两位；最小的一档去掉多余的 0，
 * 免得 ¥0.0008 印刷成 ¥0.000800。
 * `4640000 → "4.64"`、`2500 → "0.0025"`、`1284500000 → "1,284.50"`。
 */
export function formatMicroPlain(micro: number): string {
	if (!Number.isFinite(micro)) return "0.00";
	const abs = Math.abs(micro);
	let decimals = abs >= 1e6 ? 2 : abs >= 1e4 ? 4 : 6;
	let rounded = roundMicroToStep(micro, 10 ** (6 - decimals));
	if (decimals !== 2 && Math.abs(rounded) >= 1e6) {
		decimals = 2;
		rounded = roundMicroToStep(micro, 1e4);
	}
	const sign = rounded < 0 ? "-" : "";
	const text = microToYuan(Math.abs(rounded));
	const [whole, frac = ""] = text.split(".");
	let padded = (frac + "0".repeat(decimals)).slice(0, decimals);
	if (decimals === 6) {
		const trimmed = padded.replace(/0+$/, "");
		padded = trimmed.length >= 2 ? trimmed : trimmed.padEnd(2, "0");
	}
	return `${sign}${groupThousands(whole)}.${padded}`;
}

/** 带货币符号的金额，负数把符号放在货币符之前：`-¥ 1.20`。 */
export function formatMicro(micro: number, currency = "CNY"): string {
	const symbol = currencySymbol(currency);
	const text = formatMicroPlain(micro);
	return text.startsWith("-") ? `-${symbol}${text.slice(1)}` : `${symbol}${text}`;
}

/** 币种符号；不认识的币种直接用三位代码加空格。 */
export function currencySymbol(currency: string): string {
	switch (currency.toUpperCase()) {
		case "":
		case "CNY":
		case "RMB":
			return "¥";
		case "USD":
			return "$";
		case "EUR":
			return "€";
		default:
			return `${currency.toUpperCase()} `;
	}
}

/**
 * 把用户输入的元（`"1.20"`、`"0.0008"`）解析成整数微元。非法输入返回 null：
 * 空串、非数字、超过 6 位小数（微单位以下没有精度）、超出安全范围。
 */
export function yuanToMicro(input: string): number | null {
	if (typeof input !== "string") return null;
	let text = input.trim().replace(/[,\s]/g, "").replace(/^[¥￥]/, "");
	if (text === "") return null;
	let sign = 1;
	if (text.startsWith("+")) {
		text = text.slice(1);
	} else if (text.startsWith("-")) {
		sign = -1;
		text = text.slice(1);
	}
	if (!/^\d*(\.\d*)?$/.test(text) || text === "" || text === ".") return null;
	const [whole = "", frac = ""] = text.split(".");
	if (frac.length > 6) return null;
	if (whole.length > 12) return null;
	const micro =
		Number(whole === "" ? "0" : whole) * MICRO_PER_YUAN +
		Number(frac.padEnd(6, "0") || "0");
	if (!Number.isSafeInteger(micro)) return null;
	return sign * micro;
}

/** 数量级的单位前缀：unit_size = 1_000_000 读作"百万"。 */
const UNIT_SCALE_KEYS: Record<number, string> = {
	[10 ** 2]: "hundred",
	[10 ** 3]: "thousand",
	[10 ** 4]: "tenThousand",
	[10 ** 5]: "hundredThousand",
	[10 ** 6]: "million",
	[10 ** 7]: "tenMillion",
	[10 ** 8]: "hundredMillion",
	[10 ** 9]: "billion",
};

function unitScaleLabel(size: number): string {
	const key = UNIT_SCALE_KEYS[size];
	return key ? i18n.t(`billing:scale.${key}`) : groupThousands(String(size));
}

/**
 * 单价的展示：`unit_price_micro` 与 `unit_size` 合读。
 * `unit_size = 1` 时是"每 1 个 unit"：`¥0.0008 / 字符`；否则写成"每 N 个 unit"：
 * `2_000_000 / 1_000_000` → `¥2 / 百万 token`。
 */
export function formatUnitPrice(
	price: Pick<
		BillingPrice,
		"item_code" | "currency" | "unit_price_micro" | "unit_size"
	>,
	unit?: BillingUnit | string,
): string {
	const label = unitLabel(unit ?? itemMeta(price.item_code)?.unit ?? "");
	const per = price.unit_size <= 1 ? "" : `${unitScaleLabel(price.unit_size)} `;
	return `${currencySymbol(price.currency)}${microToYuan(price.unit_price_micro)} / ${per}${label}`;
}

/** 阶梯价把单价写在每一档里，这里格式化一档的价格。 */
export function formatTierPrice(
	tier: { unit_price_micro: number },
	price: Pick<BillingPrice, "item_code" | "currency" | "unit_size">,
): string {
	const label = unitLabel(itemMeta(price.item_code)?.unit ?? "");
	const per = price.unit_size <= 1 ? "" : `${unitScaleLabel(price.unit_size)} `;
	return `${currencySymbol(price.currency)}${microToYuan(tier.unit_price_micro)} / ${per}${label}`;
}

/** 阶梯的分档说明：`1. 前 5,000,000 个；2. 最后一档，不封顶`。 */
export function formatTierRange(upTo: number): string {
	if (upTo > 0) {
		return i18n.t("billing:tierRange.cumulative", {
			amount: groupThousands(String(upTo)),
		});
	}
	return i18n.t("billing:tierRange.last");
}

/** 可以按万 / 亿压缩的量级单位；次数、账期这类小数量保持原样更好读。 */
const COMPACT_UNITS = new Set<string>(["token", "char", "byte"]);

/** 拉丁单位（token / byte）跟数字之间留一个空格；中文单位（字符 / 次 / 账期）不加。 */
const LATIN_UNITS = new Set<string>(["token", "byte"]);

/**
 * 数量的展示值。中文的 token / char 到万、亿级换量级；英文用 K / M / B：
 * `12_300_000 token → "1,230 万 token" / "12.3M tokens"`、`90 second → "1 分 30 秒"`、
 * `12840 call → "12,840 次"`。完整的原始数字用 formatQuantityExact 放在 title 里。
 */
export function formatQuantity(quantity: number, unit: BillingUnit | string): string {
	if (!Number.isFinite(quantity)) return "—";
	const label = unitLabel(unit);
	if (unit === "second") return formatSeconds(quantity);
	const abs = Math.abs(quantity);
	if (COMPACT_UNITS.has(unit)) {
		if (i18n.language.startsWith("zh")) {
			if (abs >= 1e8) {
				return joinQuantity(trimDecimal(quantity / 1e8, 2), i18n.t("billing:quantity.yi"), label);
			}
			if (abs >= 1e4) {
				return joinQuantity(trimDecimal(quantity / 1e4, 1), i18n.t("billing:quantity.wan"), label);
			}
		} else {
			if (abs >= 1e9) return joinQuantity(trimDecimal(quantity / 1e9, 2), "B", label);
			if (abs >= 1e6) return joinQuantity(trimDecimal(quantity / 1e6, 1), "M", label);
			if (abs >= 1e3) return joinQuantity(trimDecimal(quantity / 1e3, 1), "K", label);
		}
	}
	return joinQuantity(groupThousands(String(quantity)), "", label);
}

/** 数量的原始值（千分位，不做量级换算），用于 title。 */
export function formatQuantityExact(
	quantity: number,
	unit: BillingUnit | string,
): string {
	return joinQuantity(groupThousands(String(quantity)), "", unitLabel(unit));
}

/** `12,840 calls` / `1,230 万 token`：中文量级后缀紧跟数字，英文量级后缀也不留空格。 */
function joinQuantity(value: string, suffix: string, label: string): string {
	if (label === "") return `${value}${suffix}`;
	if (suffix === "") return `${value} ${label}`;
	if (i18n.language.startsWith("zh")) {
		return `${value} ${suffix}${LATIN_UNITS.has(label) ? " " : ""}${label}`;
	}
	return `${value}${suffix} ${label}`;
}

function trimDecimal(value: number, decimals: number): string {
	return value
		.toFixed(decimals)
		.replace(/\.?0+$/, "")
		.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

function formatSeconds(seconds: number): string {
	const total = Math.max(0, Math.round(seconds));
	const hours = Math.floor(total / 3600);
	const minutes = Math.floor((total % 3600) / 60);
	const rest = total % 60;
	if (hours > 0) {
		return minutes > 0
			? i18n.t("billing:seconds.hoursMinutes", { hours, minutes })
			: i18n.t("billing:seconds.hours", { hours });
	}
	if (minutes > 0) {
		return rest > 0
			? i18n.t("billing:seconds.minutesSeconds", { minutes, seconds: rest })
			: i18n.t("billing:seconds.minutes", { minutes });
	}
	return i18n.t("billing:seconds.seconds", { seconds: rest });
}

/** 占比（0~1）：用于 Top 计费项的占比条，0 分布时返回 0。 */
export function ratio(numerator: number, denominator: number): number {
	if (!Number.isFinite(numerator) || !Number.isFinite(denominator) || denominator <= 0) {
		return 0;
	}
	return Math.min(1, Math.max(0, numerator / denominator));
}

// ---------------------------------------------------------------- 口径文案
//
// 标签统一走 billing 命名空间；未知取值原样显示（code 是权威标识）。

const UNIT_LABELS: Record<string, string> = {
	call: "unit.call",
	second: "unit.second",
	token: "unit.token",
	char: "unit.char",
	byte: "unit.byte",
	period: "unit.period",
};

export function unitLabel(unit: BillingUnit | string | undefined): string {
	if (!unit) return "";
	const key = UNIT_LABELS[unit];
	return key ? i18n.t(`billing:${key}`) : unit;
}

const CHARGE_MODE_LABELS: Record<string, string> = {
	count: "chargeMode.count",
	duration: "chargeMode.duration",
	usage: "chargeMode.usage",
	recurring: "chargeMode.recurring",
};

export function chargeModeLabel(mode: BillingChargeMode | string): string {
	const key = CHARGE_MODE_LABELS[mode];
	return key ? i18n.t(`billing:${key}`) : mode;
}

const METER_SOURCE_LABELS: Record<string, string> = {
	llm: "meterSource.llm",
	tts: "meterSource.tts",
	asr: "meterSource.asr",
	"voice:clone": "meterSource.voiceClone",
	session: "meterSource.session",
	mcp: "meterSource.mcp",
	kb: "meterSource.kb",
};

export function meterSourceLabel(source: BillingMeterSource | string): string {
	const key = METER_SOURCE_LABELS[source];
	return key ? i18n.t(`billing:${key}`) : source;
}

const RESOURCE_TYPE_LABELS: Record<string, string> = {
	item: "resourceType.item",
	provider: "resourceType.provider",
	model: "resourceType.model",
	voice: "resourceType.voice",
};

export function resourceTypeLabel(type: BillingResourceType | string): string {
	const key = RESOURCE_TYPE_LABELS[type];
	return key ? i18n.t(`billing:${key}`) : type;
}

const ROUNDING_LABELS: Record<string, string> = {
	none: "rounding.none",
	ceil: "rounding.ceil",
	"half:up": "rounding.halfUp",
};

export function roundingLabel(rounding: BillingRounding | string): string {
	return ROUNDING_LABELS[rounding] ?? rounding;
}

/**
 * 资源 ID → 展示名的映射（provider / model / voice 各一张），由
 * lib/billingResources.ts 从列表接口建出来。列表接口只给「系统内置 + 当前用户」
 * 可见的资源，所以查不到名字时回落到 ID 是正常路径。
 */
export interface BillingResourceNames {
	provider: Record<string, string>;
	model: Record<string, string>;
	voice: Record<string, string>;
}

/** 资源 ID 的展示名；没有映射（列表没加载 / 不在可见范围）时返回空串。 */
export function resourceName(
	names: BillingResourceNames | undefined,
	type: BillingResourceType | string,
	id: string,
): string {
	if (!names || !id) return "";
	switch (type) {
		case "provider":
			return names.provider[id] ?? "";
		case "model":
			return names.model[id] ?? "";
		case "voice":
			return names.voice[id] ?? "";
		default:
			return "";
	}
}

/**
 * 价格的适用范围。主体维度（账户协议价 / 平台标准价）与资源维度合起来读：
 * `平台标准价 · 模型 deepseek-flash`、`账户协议价 · 音色 voice_1`（§3.2）。
 * 给了 names 就显示资源名，查不到名字仍然显示 ID。
 */
export function scopeLabel(
	price: Pick<BillingPrice, "account_id" | "resource_type" | "resource_id">,
	names?: BillingResourceNames,
): string {
	const parts = [
		price.account_id
			? i18n.t("billing:scope.account")
			: i18n.t("billing:scope.platform"),
	];
	if (price.resource_type !== "item" && price.resource_id) {
		const name = resourceName(names, price.resource_type, price.resource_id);
		parts.push(`${resourceTypeLabel(price.resource_type)} ${name || price.resource_id}`);
	}
	return parts.join(" · ");
}

/** 适用范围的完整口径（悬浮提示用）：带上内部 ID，方便回资源页对照。 */
export function scopeTitle(
	price: Pick<BillingPrice, "account_id" | "resource_type" | "resource_id">,
	names?: BillingResourceNames,
): string {
	const parts = [
		price.account_id
			? `${i18n.t("billing:scope.account")} ${price.account_id}`
			: i18n.t("billing:scope.platform"),
	];
	if (price.resource_type !== "item" && price.resource_id) {
		const name = resourceName(names, price.resource_type, price.resource_id);
		parts.push(
			`${resourceTypeLabel(price.resource_type)} ${
				name
					? i18n.t("billing:resourceOption.model", {
							name,
							id: price.resource_id,
						})
					: price.resource_id
			}`,
		);
	}
	return parts.join(" · ");
}

const LEDGER_KIND_LABELS: Record<string, string> = {
	charge: "ledgerKind.charge",
	grant: "ledgerKind.grant",
	recharge: "ledgerKind.recharge",
	refund: "ledgerKind.refund",
	adjust: "ledgerKind.adjust",
	reserve: "ledgerKind.reserve",
	release: "ledgerKind.release",
	expire: "ledgerKind.expire",
};

export function ledgerKindLabel(kind: string): string {
	const key = LEDGER_KIND_LABELS[kind];
	return key ? i18n.t(`billing:${key}`) : kind;
}

/** 流水的方向：debit 减余额、credit 加余额；reserve/release 只动冻结不动余额。 */
export function directionLabel(direction: string): string {
	switch (direction) {
		case "debit":
			return i18n.t("billing:direction.debit");
		case "credit":
			return i18n.t("billing:direction.credit");
		default:
			return direction;
	}
}

const USAGE_STATUS_LABELS: Record<string, string> = {
	pending: "usageStatus.pending",
	charged: "usageStatus.charged",
	skipped: "usageStatus.skipped",
	unpaid: "usageStatus.unpaid",
	rejected: "usageStatus.rejected",
};

export function eventStatusLabel(status: BillingUsageStatus | string): string {
	const key = USAGE_STATUS_LABELS[status];
	return key ? i18n.t(`billing:${key}`) : status;
}

const ACCOUNT_STATUS_LABELS: Record<string, string> = {
	active: "accountStatus.active",
	suspended: "accountStatus.suspended",
	closed: "accountStatus.closed",
};

export function accountStatusLabel(status: BillingAccountStatus | string): string {
	const key = ACCOUNT_STATUS_LABELS[status];
	return key ? i18n.t(`billing:${key}`) : status;
}

export function subjectTypeLabel(type: BillingSubjectType | string): string {
	switch (type) {
		case "user":
			return i18n.t("billing:subjectType.user");
		case "org":
			return i18n.t("billing:subjectType.org");
		default:
			return type;
	}
}

const REF_TYPE_LABELS: Record<string, string> = {
	"usage:event": "refType.usageEvent",
	reservation: "refType.reservation",
	order: "refType.order",
	manual: "refType.manual",
	"voice:clone": "refType.voiceClone",
};

export function refTypeLabel(refType: string): string {
	if (refType === "") return "—";
	const key = REF_TYPE_LABELS[refType];
	return key ? i18n.t(`billing:${key}`) : refType;
}

// ---------------------------------------------------------------- 充值 / 支付订单

const PAYMENT_STATUS_LABELS: Record<string, string> = {
	"order:pending": "paymentStatus.pending",
	"order:paid": "paymentStatus.paid",
	"order:credited": "paymentStatus.credited",
	"order:closed": "paymentStatus.closed",
	"order:refunded": "paymentStatus.refunded",
};

export function paymentStatusLabel(status: string): string {
	const key = PAYMENT_STATUS_LABELS[status];
	return key ? i18n.t(`billing:${key}`) : status;
}

export type PaymentTone = "zinc" | "emerald" | "amber" | "red" | "violet";

export function paymentStatusTone(status: string): PaymentTone {
	switch (status) {
		case "order:pending":
			return "amber";
		case "order:credited":
			return "emerald";
		case "order:refunded":
			return "violet";
		case "order:closed":
			return "zinc";
		default:
			return "red";
	}
}

const PAYMENT_CHANNEL_LABELS: Record<string, string> = {
	"epay:alipay": "paymentChannel.alipay",
	"epay:wxpay": "paymentChannel.wxpay",
	"epay:qqpay": "paymentChannel.qqpay",
};

export function paymentChannelLabel(channel: string): string {
	const key = PAYMENT_CHANNEL_LABELS[channel];
	return key ? i18n.t(`billing:${key}`) : channel;
}

/**
 * 待支付的订单只应该由二维码/收银台推着走；其余三个状态都是终点（closed 也算：
 * 晚到的回调会把它拉回 paid，但那是服务端的事，页面不用等）。
 */
export function isPaymentPending(status: string): boolean {
	return status === "order:pending";
}

/** 这笔订单的钱到底落到余额里没有——只有 credited 是“到了”。 */
export function isPaymentCredited(status: string): boolean {
	return status === "order:credited";
}

/**
 * 订单倒计时文案。返回 null 表示已经过期（或时间戳解析不了）——过期的 pending
 * 订单不算失败：网关那边可能还能付，Sweep 会先查单再决定关不关。
 */
export function paymentExpiryHint(
	expiresAt: string,
	now = new Date(),
): string | null {
	const target = new Date(expiresAt);
	if (Number.isNaN(target.getTime())) return null;
	const rest = target.getTime() - now.getTime();
	if (rest <= 0) return null;

	const minutes = Math.floor(rest / 60000);
	if (minutes >= 60) {
		const hours = Math.floor(minutes / 60);
		return i18n.t("billing:expiry.hoursMinutes", { hours, minutes: minutes % 60 });
	}
	if (minutes >= 1) return i18n.t("billing:expiry.minutes", { minutes });
	return i18n.t("billing:expiry.seconds", {
		seconds: Math.max(1, Math.floor(rest / 1000)),
	});
}

/** 充值金额的快捷档位（元）——只是减少输入，不是限额；限额在服务端。 */
export const RECHARGE_PRESETS = [10, 50, 100, 500] as const;

/** 价格版本的状态由 effective_from / effective_to 决定，没有独立的开关（§3.2）。 */
export function isPriceEffective(price: Pick<BillingPrice, "effective_from" | "effective_to">, now = new Date()): boolean {
	const from = new Date(price.effective_from);
	if (!Number.isNaN(from.getTime()) && from.getTime() > now.getTime()) return false;
	if (!price.effective_to) return true;
	const to = new Date(price.effective_to);
	return Number.isNaN(to.getTime()) || to.getTime() > now.getTime();
}

/** 还没到 effective_from 的版本：可以删，不能停用。 */
export function isPricePending(
	price: Pick<BillingPrice, "effective_from">,
	now = new Date(),
): boolean {
	const from = new Date(price.effective_from);
	return !Number.isNaN(from.getTime()) && from.getTime() > now.getTime();
}

/** 已经过 effective_to 的版本：历史账单还指着它，只能查不能改。 */
export function isPriceExpired(
	price: Pick<BillingPrice, "effective_to">,
	now = new Date(),
): boolean {
	if (!price.effective_to) return false;
	const to = new Date(price.effective_to);
	return !Number.isNaN(to.getTime()) && to.getTime() <= now.getTime();
}

export function priceStateLabel(
	price: Pick<BillingPrice, "effective_from" | "effective_to">,
	now = new Date(),
): string {
	if (isPricePending(price, now)) return i18n.t("billing:priceState.pending");
	if (isPriceExpired(price, now)) return i18n.t("billing:priceState.expired");
	return i18n.t("billing:priceState.active");
}

// ---------------------------------------------------------------- 计费项目录
//
// 这份表是 internal/billing/item.go DefaultItems 的展示镜像：用户端拿不到计费项目录，
// 但用量事件里只有 item_code，得有个名字和单位可读。管理端以 /billing/items 为准，
// 这里只做兜底。加了新计费项就两边一起补。

export interface BillingItemMeta {
	/** billing 命名空间下的展示名 key */
	labelKey: string;
	unit: BillingUnit;
	chargeMode: BillingChargeMode;
	meterSource: BillingMeterSource;
}

export const BILLING_ITEM_CATALOG: Record<string, BillingItemMeta> = {
	"llm:tokens:input": { labelKey: "item.llmInput", unit: "token", chargeMode: "usage", meterSource: "llm" },
	"llm:tokens:output": { labelKey: "item.llmOutput", unit: "token", chargeMode: "usage", meterSource: "llm" },
	"llm:tokens:reasoning": { labelKey: "item.llmReasoning", unit: "token", chargeMode: "usage", meterSource: "llm" },
	"llm:tokens:cache:read": { labelKey: "item.llmCacheRead", unit: "token", chargeMode: "usage", meterSource: "llm" },
	"llm:tokens:cache:write": { labelKey: "item.llmCacheWrite", unit: "token", chargeMode: "usage", meterSource: "llm" },
	"tts:characters": { labelKey: "item.ttsCharacters", unit: "char", chargeMode: "usage", meterSource: "tts" },
	"tts:audio:seconds": { labelKey: "item.ttsAudioSeconds", unit: "second", chargeMode: "duration", meterSource: "tts" },
	"asr:audio:seconds": { labelKey: "item.asrAudioSeconds", unit: "second", chargeMode: "duration", meterSource: "asr" },
	"voice:clone": { labelKey: "item.voiceClone", unit: "call", chargeMode: "count", meterSource: "voice:clone" },
	"voice:retention": { labelKey: "item.voiceRetention", unit: "period", chargeMode: "recurring", meterSource: "voice:clone" },
	"mcp:tool:call": { labelKey: "item.mcpToolCall", unit: "call", chargeMode: "count", meterSource: "mcp" },
	"kb:embedding:tokens": { labelKey: "item.kbEmbeddingTokens", unit: "token", chargeMode: "usage", meterSource: "kb" },
};

/** 内置计费项的 code 列表，顺序与 seed 一致。 */
export const BILLING_ITEM_CODES = Object.keys(BILLING_ITEM_CATALOG);

export function itemMeta(code: string): BillingItemMeta | undefined {
	return BILLING_ITEM_CATALOG[code];
}

/** 计费项的展示名；不认识的 code 原样显示（code 是权威标识）。 */
export function itemLabel(code: string): string {
	const key = itemMeta(code)?.labelKey;
	return key ? i18n.t(`billing:${key}`) : code;
}

/** 计费项的计量点文案；未知 code 用 code 的第一段兜底。 */
export function itemMeterSource(code: string): string {
	const meta = itemMeta(code);
	if (meta) return meta.meterSource;
	return code.split(":")[0] ?? code;
}

/**
 * 互斥口径的提示（§3.1 / §12 / §13）。TTS 与 ASR 各只有一组口径能启用，
 * 同时开会把同一份用量计两次。
 */
export interface ExclusiveItemHint {
	codes: string[];
	note: string;
}

const EXCLUSIVE_ITEM_HINT_DEFS: { codes: string[]; noteKey: string }[] = [
	{
		codes: ["tts:characters", "tts:audio:seconds"],
		noteKey: "billing:exclusive.tts",
	},
	{
		codes: ["asr:audio:seconds"],
		noteKey: "billing:exclusive.asr",
	},
];

function exclusiveHint(noteKey: string, codes: string[]): ExclusiveItemHint {
	return { codes, note: i18n.t(noteKey) };
}

/** 某个计费项参与的互斥口径提示。 */
export function exclusiveHintsFor(code: string): ExclusiveItemHint[] {
	return EXCLUSIVE_ITEM_HINT_DEFS.filter((hint) => hint.codes.includes(code)).map(
		(hint) => exclusiveHint(hint.noteKey, hint.codes),
	);
}

/** 已经同时启用的互斥项（≥2 个 code 同时 enabled），用来在管理端报警。 */
export function conflictingItemHints(enabledCodes: string[]): ExclusiveItemHint[] {
	const enabled = new Set(enabledCodes);
	return EXCLUSIVE_ITEM_HINT_DEFS.filter(
		(hint) => hint.codes.filter((code) => enabled.has(code)).length >= 2,
	).map((hint) => exclusiveHint(hint.noteKey, hint.codes));
}

// ---------------------------------------------------------------- 时间

function pad(value: number): string {
	return String(value).padStart(2, "0");
}

/** 本地时间展示：`2026-09-20 14:03`；空值或非法值给 `—`。 */
export function formatTime(value?: string | null): string {
	if (!value) return "—";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return "—";
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** 只要日期：`2026-09-20`。 */
export function formatDate(value?: string | null): string {
	if (!value) return "—";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return "—";
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/**
 * 转成 API 要的 RFC3339（UTC，带 `Z`，语义无歧义）。
 * `"2026-09-01"` 按本地日期 0 点解释；空值返回 undefined，让 axios 直接省掉这个参数。
 */
export function toRFC3339(value?: string | Date | null): string | undefined {
	if (!value) return undefined;
	if (value instanceof Date) {
		return Number.isNaN(value.getTime()) ? undefined : value.toISOString();
	}
	const dateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
	if (dateOnly) {
		const [, y, m, d] = dateOnly;
		return new Date(Number(y), Number(m) - 1, Number(d)).toISOString();
	}
	const parsed = new Date(value);
	return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
}

/** `<input type="date">` 的值（本地日期）。 */
export function toDateInputValue(value: Date | string): string {
	const date = value instanceof Date ? value : new Date(value);
	if (Number.isNaN(date.getTime())) return "";
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** `<input type="datetime-local">` 的值（本地时间，分钟精度）。 */
export function toDateTimeLocalValue(value: Date | string): string {
	const date = value instanceof Date ? value : new Date(value);
	if (Number.isNaN(date.getTime())) return "";
	return `${toDateInputValue(date)}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/**
 * 把筛选用的日期区间收成一天的边界：`from` 取本地 0 点、`to` 取本地 23:59:59.999。
 * 传进来的就是 Date 时原样返回，方便 datetime-local 里已经带时刻的场景。
 */
export function startOfLocalDay(value: Date | string): Date | null {
	if (value instanceof Date) return value;
	const parsed = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
	if (!parsed) {
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? null : date;
	}
	const [, y, m, d] = parsed;
	return new Date(Number(y), Number(m) - 1, Number(d), 0, 0, 0, 0);
}

export function endOfLocalDay(value: Date | string): Date | null {
	if (value instanceof Date) return value;
	const parsed = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
	if (!parsed) {
		const date = new Date(value);
		return Number.isNaN(date.getTime()) ? null : date;
	}
	const [, y, m, d] = parsed;
	return new Date(Number(y), Number(m) - 1, Number(d), 23, 59, 59, 999);
}

/** 账期预设：自然月，按浏览器本地时区切边界。 */
export type PeriodPreset = "current" | "last" | "last3";

export const PERIOD_PRESETS: { value: PeriodPreset; labelKey: string }[] = [
	{ value: "current", labelKey: "billing:periods.current" },
	{ value: "last", labelKey: "billing:periods.last" },
	{ value: "last3", labelKey: "billing:periods.last3" },
];

function periodPresetLabel(preset: PeriodPreset): string {
	const key = PERIOD_PRESETS.find((item) => item.value === preset)?.labelKey;
	return key ? i18n.t(key) : preset;
}

export interface PeriodRange {
	preset: PeriodPreset;
	label: string;
	from: Date;
	to: Date;
	fromRFC: string;
	toRFC: string;
	/** 人读的区间，如 `2026-08-01 ~ 2026-09-20`。 */
	hint: string;
}

/**
 * 把预设换算成 from/to。`current` 是本账期到现在（和接口缺省口径一致），
 * `last` 是上一个完整自然月，`last3` 是本月起的三个自然月。
 */
export function periodRange(preset: PeriodPreset, now = new Date()): PeriodRange {
	const monthStart = (date: Date, offset: number) =>
		new Date(date.getFullYear(), date.getMonth() + offset, 1, 0, 0, 0, 0);
	const monthEnd = (date: Date, offset: number) =>
		new Date(date.getFullYear(), date.getMonth() + offset + 1, 0, 23, 59, 59, 999);

	let from: Date;
	let to: Date;
	switch (preset) {
		case "last":
			from = monthStart(now, -1);
			to = monthEnd(now, -1);
			break;
		case "last3":
			from = monthStart(now, -2);
			to = now;
			break;
		default:
			from = monthStart(now, 0);
			to = now;
	}
	const label = periodPresetLabel(preset);
	return {
		preset,
		label,
		from,
		to,
		fromRFC: from.toISOString(),
		toRFC: to.toISOString(),
		hint: `${toDateInputValue(from)} ~ ${toDateInputValue(to)}`,
	};
}

// ---------------------------------------------------------------- 错误

interface ErrorResponseShape {
	response?: { status?: number; data?: { error?: string } };
}

function asErrorResponse(err: unknown): ErrorResponseShape | null {
	if (typeof err !== "object" || err === null) return null;
	return err as ErrorResponseShape;
}

/** HTTP 状态码；拿不到就是 undefined（比如网络错误、超时）。 */
export function billingErrorStatus(err: unknown): number | undefined {
	return asErrorResponse(err)?.response?.status;
}

/**
 * 503 = 计费未启用（billing.enabled: false）。这不是报错，页面应该渲染"计费未启用"
 * 空态，而不是弹一个红色错误（cmd/manager/handler/billing.go）。
 */
export function isBillingDisabled(err: unknown): boolean {
	return billingErrorStatus(err) === 503;
}

/**
 * 给客户看的错误文案。
 *
 * 服务端的 `{error}` 是给我们自己排障用的（"billing: amount must be between 1 and
 * 10000"、"epay: create payment: ..."），原样贴到界面上就是让客户读我们的代码。
 * 规则：带着内部前缀的一律换成人话，其余（本来就是给人看的提示）原样用。
 */
export function userFacingError(err: unknown, fallback: string): string {
	const text = billingErrorMessage(err, "");
	if (text === "") return fallback;
	for (const prefix of INTERNAL_ERROR_PREFIXES) {
		if (text.startsWith(prefix)) return fallback;
	}
	return text;
}

/** 服务端错误的命名空间前缀（与 Go 侧的包名前缀一致）。 */
const INTERNAL_ERROR_PREFIXES = ["billing:", "epay:", "store:", "payment:", "apikey:"];

/** 取服务端的 `{error}` 文案，拿不到就用兜底文案。 */
export function billingErrorMessage(err: unknown, fallback: string): string {
	const detail = asErrorResponse(err)?.response?.data?.error;
	if (typeof detail === "string" && detail.trim() !== "") return detail;
	if (err instanceof Error && err.message) return err.message;
	return fallback;
}
