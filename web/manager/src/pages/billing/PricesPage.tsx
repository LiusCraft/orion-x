// 用户端「价格公示」。
//
// 只公示平台标准价（当前生效的那一版）——账户协议价是别人和平台的约定，不外露。
// 接口是 GET /api/billing/prices（§8），503 = 计费未启用，走空态而不是报错。
//
// 底部的口径说明只提炼自 docs/billing-design.md §3.3（金额与舍入）与 §13（quantity
// 数的是什么），没有写进文档的规则不要往这里加。

import { useEffect, useMemo, useState } from "react";
import { AlertCircle, Tag } from "lucide-react";
import { billingApi, type BillingPrice } from "@/lib/api";
import {
	BILLING_DISABLED_TITLE,
	billingErrorMessage,
	currencySymbol,
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
	const [prices, setPrices] = useState<BillingPrice[]>([]);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [error, setError] = useState("");

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingApi
			.prices()
			.then(({ data }) => {
				if (cancelled) return;
				setPrices(data.items);
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
													{scopeLabel(price)}
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

				<BillingRules />
			</div>
		</div>
	);
}

/**
 * 计费口径说明。三条来源：§3.3 金额与舍入、§13 quantity 数的是什么、§3.2 变价规则。
 * 没写进设计文档的规则不要加进来——公示页说错话比不写更糟。
 */
function BillingRules() {
	return (
		<Panel title="计费口径说明" description="来源：docs/billing-design.md §3.2 / §3.3 / §13">
			<div className="space-y-4 text-xs text-zinc-400 leading-relaxed">
				<section>
					<p className="text-zinc-300 font-medium mb-1">金额与舍入（§3.3）</p>
					<ul className="list-disc pl-4 space-y-1">
						<li>
							金额全链路是整数微单位（1e-6 元），不用浮点；<span className="font-mono">qty × 单价 ÷ unit_size</span>{" "}
							在<span className="text-zinc-300">一次结算内只舍入一次</span>，避免逐条舍入累积误差。
						</li>
						<li>
							按时长计费的项默认<span className="text-zinc-300">向上取整</span>（ceil），按用量的项默认
							<span className="text-zinc-300">精确</span>（丢弃不足 1 微元的部分）；起步价在舍入之后取{" "}
							<span className="font-mono">max(用量价, 起步价)</span>。
						</li>
						<li>
							单价必须是整数微元，精度不够就抬高 <span className="font-mono">unit_size</span>
							（表里的"每百万 token ¥x"就是这个意思），而不是把单价舍成 0。
						</li>
					</ul>
				</section>

				<section>
					<p className="text-zinc-300 font-medium mb-1">
						每个计费项数的是什么（§13）
					</p>
					<ul className="list-disc pl-4 space-y-1">
						<li>
							任意两个计费项的 quantity 不重叠：LLM 输入已剔除缓存读写、输出已剔除推理 token，
							缓存命中与缓存写入单独列项。
						</li>
						<li>
							数量跟厂商计费的那个量对齐：token 用厂商报的 token 数，语音识别用厂商返回的时长，
							语音合成按实际发给厂商的文本字符（rune）数。
						</li>
						<li>
							被用户打断时，已经合成/已经产生的用量照常计费——厂商那边已经计过费了；
							还没发给厂商的文本不算。
						</li>
						<li>
							自带 key（BYOK）的调用不收平台费用，只记用量，明细里会标注「自带 key，不计费」。
						</li>
					</ul>
				</section>

				<section>
					<p className="text-zinc-300 font-medium mb-1">互斥口径（§3.1 / §12）</p>
					<ul className="list-disc pl-4 space-y-1">
						<li>
							<span className="font-mono">tts:characters</span> 与{" "}
							<span className="font-mono">tts:audio:seconds</span> 互斥；
							<span className="font-mono">asr:audio:seconds</span> 与按通话时长打包计价互斥。
							同一部署只启用一组，否则同一份用量会被计两次。
						</li>
					</ul>
				</section>

				<section>
					<p className="text-zinc-300 font-medium mb-1">变价与历史账单（§3.2）</p>
					<ul className="list-disc pl-4 space-y-1">
						<li>
							价格是版本化的：改价 = 新增一版生效时间更晚的价格，不覆盖历史版本。
						</li>
						<li>
							结算按<span className="text-zinc-300">事件发生时刻</span>的价格匹配，并把命中的价格快照写进用量事件；
							历史账单永不因之后调价而变动。
						</li>
						<li>
							已经生效的价格版本只能"停用"（把失效时间收到当前时刻，不删行）；只有还没生效的版本可以删除。
						</li>
					</ul>
				</section>

				<section>
					<p className="text-zinc-300 font-medium mb-1">额度与准入（§3.3 / §3.5）</p>
					<ul className="list-disc pl-4 space-y-1">
						<li>
							限定计费项的赠送额度优先消耗，顺序是{" "}
							<span className="font-mono">grant → balance → credit_limit</span>。
						</li>
						<li>
							准入判断是{" "}
							<span className="font-mono">
								amount ≤ 余额 + 信用额度 − 预冻结
							</span>
							；余额可以为负 = 后付欠款。
						</li>
					</ul>
				</section>

				<section>
					<p className="text-zinc-300 font-medium mb-1">价格匹配优先级（§3.2）</p>
					<p>
						同一计费项可能同时存在多版价格，命中顺序是：账户协议价 → 音色 → 模型 → 厂商 →
						计费项兜底；同一 scope 同一时刻只有一版价格。本页只公示
						<span className="text-zinc-300">平台标准价</span>（
						{currencySymbol("CNY")}
						计价，单币种部署不做汇率），账户协议价按合同约定另行提供。
					</p>
				</section>
			</div>
		</Panel>
	);
}
