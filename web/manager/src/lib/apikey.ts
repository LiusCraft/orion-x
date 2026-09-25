// API Key 页面的展示口径：状态、权限文案、错误文案。
//
// 与 billing.ts 同一套分工：只把服务端的事实翻成客户能读懂的话，不发请求、不碰 React。
// 权限的标题 / 说明 / 分组一律取自服务端下发的目录（前端不硬编码，改文案不用发版）；
// 这里只补一条兼容规则——目录里已经没有的权限值原样显示，不隐藏，这样"服务端先上了
// 新权限、前端文案还没跟上"时列表里不会凭空少一项。

import type { ApiKeyScopeInfo, ApiKeyView } from "@/lib/api";
import { billingErrorMessage, billingErrorStatus, formatTime, userFacingError } from "@/lib/billing";

/** 一把 Key 的当前状态。 */
export type ApiKeyStatus = "valid" | "revoked" | "expired";

/** 与 billing/shared 的 Pill 同一套色板。 */
export type ApiKeyTone = "emerald" | "zinc" | "amber";

/** 撤销优先于过期：两样都占时说的是终态。 */
export function apiKeyStatus(
	key: Pick<ApiKeyView, "revoked_at" | "expires_at">,
): ApiKeyStatus {
	if (key.revoked_at) return "revoked";
	if (key.expires_at) {
		const expires = new Date(key.expires_at).getTime();
		if (!Number.isNaN(expires) && expires <= Date.now()) return "expired";
	}
	return "valid";
}

const STATUS_LABELS: Record<ApiKeyStatus, string> = {
	valid: "有效",
	revoked: "已撤销",
	expired: "已过期",
};

const STATUS_TONES: Record<ApiKeyStatus, ApiKeyTone> = {
	valid: "emerald",
	revoked: "zinc",
	expired: "amber",
};

export function apiKeyStatusLabel(status: ApiKeyStatus): string {
	return STATUS_LABELS[status];
}

export function apiKeyStatusTone(status: ApiKeyStatus): ApiKeyTone {
	return STATUS_TONES[status];
}

/** 权限的展示文案：目录里有就用标题，没有就原样显示权限值（兼容规则）。 */
export function scopeTitle(catalog: ApiKeyScopeInfo[], value: string): string {
	return catalog.find((scope) => scope.value === value)?.title ?? value;
}

/** 权限的一句话说明（悬停提示用）；目录里没有就没得显示。 */
export function scopeDescription(
	catalog: ApiKeyScopeInfo[],
	value: string,
): string | undefined {
	return catalog.find((scope) => scope.value === value)?.desc;
}

/** 最后使用：空值在这里是有含义的（从未用过），所以不走 formatTime 的「—」。 */
export function formatLastUsed(value?: string | null): string {
	return value ? formatTime(value) : "从未使用";
}

/** 503 = 这个部署没开凭证功能。这是默认状态，不是错误。 */
export function isApiKeyDisabled(err: unknown): boolean {
	return billingErrorStatus(err) === 503;
}

/** 403 = 灰度期只有管理员能创建（按钮本应已置灰，这里兜底）。 */
export function isAdminOnly(err: unknown): boolean {
	return billingErrorStatus(err) === 403;
}

/** 服务端表达“账号的 Key 数量到顶”的固定文案（400，没有专门的错误码）。 */
const QUOTA_ERROR = "too many keys for this account";

/**
 * 账号的 Key 数量到顶。服务端用 400 + 那句固定文案表达这件事，所以只能认文案；
 * 认不出来就当普通 400，走兜底文案。
 */
export function isKeyQuotaExceeded(err: unknown): boolean {
	return (
		billingErrorStatus(err) === 400 &&
		billingErrorMessage(err, "") === QUOTA_ERROR
	);
}

/** 界面文案全是中文；不含汉字的（英文契约串、axios 的 Network Error）一律不用。 */
const HAN = /[\u4e00-\u9fff]/;

/**
 * 页面统一的取词：能用的只有我们自己的中文文案。
 *
 * 400 / 403 的响应体是写给 API 调用方看的英文契约（`at least one scope is required`、
 * `admin only`），控制台一个字都不能直出，所以这几种状态直接给兜底；残余分支
 * （网络错误、5xx）先走共用的 userFacingError，再把不含汉字的结果换回兜底。
 */
export function apiKeyErrorText(err: unknown, fallback: string): string {
	if (billingErrorStatus(err) === 400 || billingErrorStatus(err) === 403) {
		return fallback;
	}
	const text = userFacingError(err, fallback);
	return HAN.test(text) ? text : fallback;
}

/** 没有创建权限时的说明；按钮的置灰理由与提交失败的兜底共用一句话。 */
export const ADMIN_ONLY_HINT = "当前暂只允许管理员创建 Key。";

/** 创建失败给客户看的话。 */
export function createKeyErrorText(err: unknown): string {
	if (isKeyQuotaExceeded(err)) {
		return "Key 数量已达上限，请先撤销不再使用的 Key，再创建新的。";
	}
	if (isAdminOnly(err)) return ADMIN_ONLY_HINT;
	return apiKeyErrorText(err, "创建失败，请检查名称与勾选的权限后重试");
}

/** 撤销失败给客户看的话。 */
export function revokeKeyErrorText(err: unknown): string {
	if (isApiKeyDisabled(err)) return "当前环境还没有开通 API Key 功能。";
	// 404：列表里的行已经过时了（被别人撤了，或者这行本来就不是自己的）。
	if (billingErrorStatus(err) === 404) {
		return "这把 Key 已经不在列表里了，请刷新看看最新状态。";
	}
	return apiKeyErrorText(err, "撤销失败，请重试");
}
