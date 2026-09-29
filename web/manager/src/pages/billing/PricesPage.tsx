// 用户端「价格公示」。
//
// 只公示平台标准价（当前生效的那一版）——账户协议价是别人和平台的约定，不外露。
// 接口是 GET /api/billing/prices（§8），503 = 计费未启用，走空态而不是报错。
//
// 「适用范围」显示资源名而不是内部 ID：名字从 /api/models、/api/providers、
// /api/voices 现成列表解析（lib/billingResources.ts），解析不到（看不到的资源 /
// 列表加载失败）就回落到 ID。价格都是 item 级时不去拉这几个列表。

import { useEffect, useMemo, useState } from "react";
import { AlertCircle, Tag } from "lucide-react";
import { billingApi, type BillingPrice } from "@/lib/api";
import { useDocumentTitle } from "@/lib/title";
import {
	BILLING_DISABLED_TITLE,
	billingErrorMessage,
	formatDate,
	formatMicro,
	formatTierPrice,
	formatTierRange,
	formatUnitPrice,
	isBillingDisabled,
	itemLabel,
	itemMeterSource,
	meterSourceLabel,
	roundingLabel,
	scopeLabel,
} from "@/lib/billing";
import {
	EMPTY_RESOURCE_LISTS,
	loadBillingResourceLists,
	resourceNameIndex,
	type BillingResourceLists,
} from "@/lib/billingResources";
import {
	EmptyState,
	LoadingBlock,
	Panel,
	Pill,
	TableShell,
	Td,
	Th,
	Tr,
} from "@/pages/billing/shared";

/** 计量点的展示顺序与 seed 里的顺序一致，未收录的排在最后。 */
const METER_ORDER = ["llm", "tts", "asr", "voice:clone", "session", "mcp", "kb"];

export default function PricesPage() {
	useDocumentTitle("价格公示");

	const [prices, setPrices] = useState<BillingPrice[]>([]);
	const [resources, setResources] =
		useState<BillingResourceLists>(EMPTY_RESOURCE_LISTS);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [error, setError] = useState("");

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingApi
			// 同一个路径在服务端按 is_admin 分流：管理员拿到的是「全部版本 + 账户协议价」那个
			// 列表，所以这里显式要 active_only 并带上分页上限；普通用户这一支本来就只回
			// 当前生效的平台标准价，参数被忽略。协议价再在下面逐行滤掉。
			.prices({ active_only: "1", page: 1, page_size: 100 })
			.then(({ data }) => {
				if (cancelled) return;
				setPrices(data.items.filter((price) => price.account_id === ""));
				setDisabled(false);
				setError("");
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setError(billingErrorMessage(err, "加载价格失败"));
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, []);

	// 资源名只在公示里有非 item 级价格时才去解析：全是兜底价就不多发四个请求。
	useEffect(() => {
		if (!prices.some((price) => price.resource_type !== "item")) return;
		let cancelled = false;
		loadBillingResourceLists().then((lists) => {
			if (!cancelled) setResources(lists);
		});
		return () => {
			cancelled = true;
		};
	}, [prices]);

	const names = useMemo(() => resourceNameIndex(resources), [resources]);

	const groups = useMemo(() => {
		const byMeter = new Map<string, BillingPrice[]>();
		for (const price of prices) {
			const source = itemMeterSource(price.item_code);
			const bucket = byMeter.get(source);
			if (bucket) {
				bucket.push(price);
			} else {
				byMeter.set(source, [price]);
			}
		}
		return [...byMeter.entries()].sort(
			([a], [b]) => METER_ORDER.indexOf(a) - METER_ORDER.indexOf(b),
		);
	}, [prices]);

	return (
		<div className="min-h-full">
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-lg font-semibold text-white">价格公示</h1>
						<p className="text-sm text-zinc-500 mt-0.5">
							当前生效的平台标准价，按计量点分组
						</p>
					</div>
					{!loading && !disabled && prices.length > 0 && (
						<div className="text-right">
							<p className="text-xs text-zinc-500">生效中的价格版本</p>
							<p className="text-xl font-semibold text-white font-mono">
								{prices.length}
							</p>
						</div>
					)}
				</div>
			</div>

			<div className="px-8 py-6 space-y-6">
				{error && (
					<div className="flex items-start gap-2 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3">
						<AlertCircle className="w-4 h-4 text-red-400 mt-0.5 shrink-0" />
						<p className="text-xs text-red-300">{error}</p>
					</div>
				)}

				{loading ? (
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<LoadingBlock label="加载价格中..." />
					</div>
				) : disabled ? (
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<EmptyState
							icon={Tag}
							title={BILLING_DISABLED_TITLE}
							hint="服务端没有开启计费模块（billing.enabled），价格公示暂不可用。"
						/>
					</div>
				) : groups.length === 0 ? (
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<EmptyState
							icon={Tag}
							title="还没有公示的价格"
							hint="平台标准价为空时，新会话会因为匹配不到价格被拒绝；运营把价格录进来之后这里就会显示。"
						/>
					</div>
				) : (
					groups.map(([source, rows]) => (
						<Panel
							key={source}
							title={meterSourceLabel(source)}
							description={`${rows.length} 条生效中的平台标准价`}
							bodyClassName="p-0"
						>
							<TableShell
								head={
									<>
										<Th>计费项</Th>
										<Th>适用范围</Th>
										<Th className="text-right">单价</Th>
										<Th>舍入</Th>
										<Th className="text-right">起步价</Th>
										<Th>生效时间</Th>
									</>
								}
							>
								{rows.map((price, index) => {
									const last = index === rows.length - 1;
									return (
										<Tr key={price.id} last={last}>
											<Td>
												<div className="flex flex-col">
													<span className="text-zinc-200">
														{itemLabel(price.item_code)}
													</span>
													<span className="text-[11px] text-zinc-500 font-mono">
														{price.item_code}
													</span>
												</div>
											</Td>
											<Td>
												<Pill tone="zinc">
													{scopeLabel(price, names)}
												</Pill>
											</Td>
											<Td className="text-right whitespace-nowrap">
												<div className="flex flex-col items-end">
													<span className="font-mono text-amber-400">
														{formatUnitPrice(price)}
													</span>
													{price.tiers &&
														price.tiers.length > 0 && (
															<span className="text-[11px] text-zinc-500">
																阶梯{" "}
																{price.tiers.length}{" "}
																档：按账期累计量分档累进
															</span>
														)}
												</div>
											</Td>
											<Td className="text-xs text-zinc-400 whitespace-nowrap">
												{roundingLabel(price.rounding)}
											</Td>
											<Td className="text-right font-mono text-zinc-400 whitespace-nowrap">
												{price.min_charge_micro > 0
													? formatMicro(
															price.min_charge_micro,
															price.currency,
														)
													: "—"}
											</Td>
											<Td className="text-xs text-zinc-500 font-mono whitespace-nowrap">
												{formatDate(price.effective_from)}
												{" ~ "}
												{price.effective_to
													? formatDate(price.effective_to)
													: "长期"}
											</Td>
										</Tr>
									);
								})}
							</TableShell>

							{rows.some(
								(price) => price.tiers && price.tiers.length > 0,
							) && (
								<div className="px-5 py-4 border-t border-zinc-800 space-y-3">
									{rows
										.filter(
											(price) =>
												price.tiers &&
												price.tiers.length > 0,
										)
										.map((price) => (
											<div key={price.id}>
												<p className="text-[11px] text-zinc-500 mb-1.5">
													{itemLabel(price.item_code)}{" "}
													的阶梯（分档累进，不是达标后全量按新价）
												</p>
												<div className="flex flex-wrap gap-x-4 gap-y-1">
													{price.tiers?.map(
														(tier, tierIndex) => (
															<span
																key={tierIndex}
																className="text-[11px] text-zinc-400 font-mono"
															>
																{tierIndex + 1}.{" "}
																{formatTierRange(
																	tier.up_to,
																)}{" "}
																·{" "}
																{formatTierPrice(
																	tier,
																	price,
																)}
															</span>
														),
													)}
												</div>
											</div>
										))}
								</div>
							)}
						</Panel>
					))
				)}
			</div>
		</div>
	);
}
