// 价格页要用的资源目录：把 /api/models、/api/providers、/api/voices 的列表汇成两份东西。
//
//   - resourceNameIndex：ID → 展示名，给 scopeLabel / scopeTitle 用（列表上让人直接
//     看出这版价格作用于哪个模型 / 厂商 / 音色，而不是一串 UUID）；
//   - resourceOptions：某一粒度的选择项，给新建价格的下拉用（不用手抄 UUID）。
//
// 两者都只是展示与录入的便利，不是事实来源：价格可以停在当前用户看不到的资源上
// （列表接口只返回系统内置 + 自己的），所以查不到名字一律回落到 ID，下拉之外也留着
// 「手动填 ID」的入口。

import {
	modelApi,
	providerApi,
	voiceApi,
	type AIModel,
	type BillingResourceType,
	type ModelVoice,
	type Provider,
} from "@/lib/api";
import type { BillingResourceNames } from "@/lib/billing";

export interface BillingResourceLists {
	providers: Provider[];
	models: AIModel[];
	voices: ModelVoice[];
	/** 拉取失败的粒度：列表可能不全，UI 该说明而不是假装库里没有可选的资源。 */
	missing: BillingResourceType[];
}

/** 列表还没加载出来（或全部失败）时的空目录。 */
export const EMPTY_RESOURCE_LISTS: BillingResourceLists = {
	providers: [],
	models: [],
	voices: [],
	missing: [],
};

/** 拉一次三个列表；单个失败不影响其它两个（名称只影响展示，不阻断页面）。 */
export async function loadBillingResourceLists(): Promise<BillingResourceLists> {
	const [providers, models, voices] = await Promise.allSettled([
		providerApi.list(),
		modelApi.list(),
		loadVoices(),
	]);
	const missing: BillingResourceType[] = [];
	if (providers.status === "rejected") missing.push("provider");
	if (models.status === "rejected") missing.push("model");
	if (voices.status === "rejected") missing.push("voice");
	return {
		providers: providers.status === "fulfilled" ? providers.value.data : [],
		models: models.status === "fulfilled" ? models.value.data : [],
		voices: voices.status === "fulfilled" ? voices.value : [],
		missing,
	};
}

/** 系统音色 + 当前用户自建（含复刻）的音色，按 ID 去重；两边都失败才算失败。 */
async function loadVoices(): Promise<ModelVoice[]> {
	const [system, mine] = await Promise.allSettled([
		voiceApi.listSystem(),
		voiceApi.listMine(),
	]);
	if (system.status === "rejected" && mine.status === "rejected") {
		throw system.reason;
	}
	const byId = new Map<string, ModelVoice>();
	if (system.status === "fulfilled") {
		for (const voice of system.value.data) byId.set(voice.id, voice);
	}
	if (mine.status === "fulfilled") {
		for (const voice of mine.value.data) byId.set(voice.id, voice);
	}
	return [...byId.values()];
}

/** ID → 展示名，给 scopeLabel / scopeTitle 用。 */
export function resourceNameIndex(
	lists: BillingResourceLists,
): BillingResourceNames {
	const index: BillingResourceNames = { provider: {}, model: {}, voice: {} };
	for (const provider of lists.providers) index.provider[provider.id] = provider.name;
	for (const model of lists.models) index.model[model.id] = model.name;
	for (const voice of lists.voices) index.voice[voice.id] = voice.name;
	return index;
}

export interface BillingResourceOption {
	value: string;
	label: string;
	group?: string;
}

/**
 * 某一粒度的选择项：模型按所属厂商分组、音色按所属模型分组，
 * 让「deepseek-flash」不至于在两个厂商下重名时无从分辨。
 */
export function resourceOptions(
	type: BillingResourceType,
	lists: BillingResourceLists,
): BillingResourceOption[] {
	const names = resourceNameIndex(lists);
	if (type === "provider") {
		return [...lists.providers]
			.sort((a, b) => a.name.localeCompare(b.name))
			.map((provider) => ({ value: provider.id, label: provider.name }));
	}
	if (type === "model") {
		return [...lists.models]
			.sort((a, b) => a.name.localeCompare(b.name))
			.map((model) => ({
				value: model.id,
				label: `${model.name}（${model.model_id}）`,
				group: names.provider[model.provider_id] || undefined,
			}));
	}
	if (type === "voice") {
		return [...lists.voices]
			.sort((a, b) => a.name.localeCompare(b.name))
			.map((voice) => ({
				value: voice.id,
				label: voice.name,
				group: names.model[voice.model_id] || undefined,
			}));
	}
	return [];
}
