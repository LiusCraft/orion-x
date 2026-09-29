// 模型列表上的价格：把 /api/billing/prices 的公示价摊到每个模型上。
//
// 匹配口径与结算一致（docs/billing-design.md §3.2 的优先级）：模型级 → 厂商级 → 计费项
// 兜底，同一 scope 同一时刻只有一版生效。这里只看平台标准价——账户协议价是别人和平台
// 的约定，不该在资源列表里露出来。
//
// 计费未启用（503）或请求失败返回 null：调用方据此区分「没有价格」和「拿不到价格」。

import { billingApi, type AIModel, type BillingPrice } from "@/lib/api";
import { formatUnitPrice } from "@/lib/billing";

/**
 * 拉当前生效的平台标准价；拿不到（计费未启用 / 请求失败）返回 null。
 *
 * 价格表按“几十条版本”设计，取一页就够（价格公示页同样只取一页）；协议价由服务端
 * 在管理端列表里分流，这里再滤一道，用户端本来就只会拿到平台标准价。
 */
export async function loadPlatformPrices(): Promise<BillingPrice[] | null> {
	try {
		const { data } = await billingApi.prices({
			active_only: "1",
			platform_only: "1",
			page: 1,
			page_size: 100,
		});
		return data.items.filter((price) => price.account_id === "");
	} catch {
		return null;
	}
}

/** 模型在计费里的类别：决定看哪几个计费项的价。 */
export type ModelCategory = "llm" | "asr" | "tts" | "embedding" | "unknown";

/** 语音类模型是 ASR 还是 TTS 看厂商 slug 的前缀（provider.go 的 slug 约定）。 */
export function modelCategory(model: AIModel): ModelCategory {
	if (model.type === "embedding") return "embedding";
	if (model.type !== "speech") return "llm";
	const slug = model.provider?.slug ?? "";
	if (slug.startsWith("asr:")) return "asr";
	if (slug.startsWith("tts:")) return "tts";
	return "unknown";
}

/** 每个类别要看价格的计费项，数组顺序即展示顺序。 */
const CATEGORY_ITEMS: Record<
	ModelCategory,
	Array<{ code: string; label: string }>
> = {
	llm: [
		{ code: "llm:tokens:input", label: "输入" },
		{ code: "llm:tokens:output", label: "输出" },
	],
	asr: [{ code: "asr:audio:seconds", label: "识别" }],
	// tts:characters 与 tts:audio:seconds 是互斥口径，只展示配了价的那个。
	tts: [
		{ code: "tts:characters", label: "合成" },
		{ code: "tts:audio:seconds", label: "音频" },
	],
	embedding: [{ code: "kb:embedding:tokens", label: "入库" }],
	unknown: [],
};

export interface ModelPriceLine {
	/** 计费项的口径前缀，如「输入」 */
	label: string;
	/** 带币种与单位的单价文案，如「¥1.5 / 百万 token」 */
	text: string;
}

/**
 * 模型的价格行。null = 这个类别没有可展示的计费项（未知厂商）；空数组 = 有计费项但一条价
 * 都没配。两者对调用方的含义不同（前者不显示价格块，后者显示「未定价」）。
 */
export function modelPriceLines(
	model: AIModel,
	prices: BillingPrice[],
): ModelPriceLine[] | null {
	const items = CATEGORY_ITEMS[modelCategory(model)];
	if (items.length === 0) return null;
	return items.flatMap((item) => {
		const price = resolvePrice(prices, model, item.code);
		return price ? [{ label: item.label, text: formatUnitPrice(price) }] : [];
	});
}

/** 一个计费项命中的价格：模型级 → 厂商级 → 计费项兜底。 */
function resolvePrice(
	prices: BillingPrice[],
	model: AIModel,
	itemCode: string,
): BillingPrice | undefined {
	const candidates = prices.filter((price) => price.item_code === itemCode);
	return (
		candidates.find(
			(price) =>
				price.resource_type === "model" && price.resource_id === model.id,
		) ??
		candidates.find(
			(price) =>
				price.resource_type === "provider" &&
				price.resource_id === model.provider_id,
		) ??
		candidates.find((price) => price.resource_type === "item")
	);
}
