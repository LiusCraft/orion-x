// 模型监控：按模型看这段时间的用量与消耗。
//
// 数据源是计费的用量事件（billing_usage_events）——全链路唯一记录「哪个模型用了多少」的
// 地方：每个 turn 的 token、合成的字符数、识别到的秒数在 turn 边界上报，事件里带着当时的
// aimodel_id / provider_id。所以这一页没有调用次数、失败率、首包时长：那些指标现在根本没
// 被记录，编一个数字出来不如不显示。
//
// 口径（与「用量与余额」一致，见 docs/billing-design.md §14）：
//   - 数量统计全部已上报的事件，含还没结算的；
//   - 金额只统计 charged（已结算）事件——所以有量没金额时不是页面算错了，而是事件还
//     没算成钱（pending 只是慢一拍，unpaid = 没配价格 / 透支被拒），页面上分开数。
//
// 页面零件复用计费页的（这一页本身就是一张计费报表，见 pages/billing/shared）。

import { Fragment, useEffect, useMemo, useState } from "react";
import {
	Activity,
	ChevronDown,
	ChevronRight,
	Coins,
	Layers,
	RefreshCw,
	TrendingUp,
	type LucideIcon,
} from "lucide-react";
import {
	billingApi,
	type BillingModelUsage,
	type BillingModelUsageRow,
} from "@/lib/api";
import {
	BILLING_DISABLED_TITLE,
	billingErrorMessage,
	formatMicro,
	formatQuantity,
	formatQuantityExact,
	formatTime,
	isBillingDisabled,
	itemLabel,
	itemMeta,
	ratio,
} from "@/lib/billing";
import {
	Banner,
	EmptyState,
	Hint,
	LoadingBlock,
	Panel,
	Pill,
	TableShell,
	Td,
	Th,
	Tr,
	type BannerMessage,
} from "@/pages/billing/shared";

const RANGE_OPTIONS = [
	{ value: "today", label: "今日", days: 1 },
	{ value: "7d", label: "近 7 天", days: 7 },
	{ value: "30d", label: "近 30 天", days: 30 },
] as const;

type RangeKey = (typeof RANGE_OPTIONS)[number]["value"];

type SortKey = "amount" | "events" | "name";

const SORT_OPTIONS: { value: SortKey; label: string }[] = [
	{ value: "amount", label: "按金额" },
	{ value: "events", label: "按事件数" },
	{ value: "name", label: "按名称" },
];

const MODEL_TYPE_LABELS: Record<string, string> = {
	text: "文本",
	vision: "视觉",
	speech: "语音",
	multimodal: "全模态",
	embedding: "向量",
};

const MODEL_TYPE_TONES: Record<
	string,
	"violet" | "sky" | "emerald" | "amber" | "zinc"
> = {
	text: "violet",
	vision: "sky",
	speech: "emerald",
	multimodal: "amber",
	embedding: "zinc",
};

/** 用量明细里最多平铺几个计费项，多出来的收进「+N 项」，展开能看全。 */
const VISIBLE_ITEMS = 4;

/** 时间段的起点：今日 = 本地 0 点，近 N 天 = N-1 天前的 0 点（含今天）。 */
function rangeStart(range: RangeKey): Date {
	const start = new Date();
	start.setHours(0, 0, 0, 0);
	const days = RANGE_OPTIONS.find((option) => option.value === range)?.days ?? 1;
	start.setDate(start.getDate() - (days - 1));
	return start;
}

function modelTypeLabel(type: string): string {
	if (!type) return "未标注";
	return MODEL_TYPE_LABELS[type] ?? type;
}

function itemUnit(code: string): string {
	return itemMeta(code)?.unit ?? "";
}

/** 计费项的展示数量：单位随计费项走（token / 秒 / 字符），跨项不可加。 */
function itemAmountText(row: BillingModelUsageRow): string {
	return formatQuantity(row.quantity, itemUnit(row.item_code));
}

/** 一个模型的合计。服务端给的是「模型 × 计费项」的行，按模型收口是展示层的事。 */
interface ModelUsageGroup {
	modelId: string;
	name: string;
	type: string;
	providerName: string;
	items: BillingModelUsageRow[];
	eventCount: number;
	chargedEvents: number;
	pendingEvents: number;
	unpaidEvents: number;
	skippedEvents: number;
	amountMicro: number;
}

/** 事件的状态分布，用于卡片与行内的副标题。 */
interface EventStatusCounts {
	eventCount: number;
	chargedEvents: number;
	pendingEvents: number;
	unpaidEvents: number;
	skippedEvents: number;
}

function statusCountsOf(row: BillingModelUsageRow): EventStatusCounts {
	return {
		eventCount: row.event_count,
		chargedEvents:
			row.event_count - row.pending_events - row.unpaid_events - row.skipped_events,
		pendingEvents: row.pending_events,
		unpaidEvents: row.unpaid_events,
		skippedEvents: row.skipped_events,
	};
}

function addCounts(a: EventStatusCounts, b: EventStatusCounts): EventStatusCounts {
	return {
		eventCount: a.eventCount + b.eventCount,
		chargedEvents: a.chargedEvents + b.chargedEvents,
		pendingEvents: a.pendingEvents + b.pendingEvents,
		unpaidEvents: a.unpaidEvents + b.unpaidEvents,
		skippedEvents: a.skippedEvents + b.skippedEvents,
	};
}

/** 「已计费 12 · 待结算 2 · 未计费 1」：只列非零的那些（已计费总是列出来）。 */
function countsSummary(counts: EventStatusCounts): string[] {
	const parts = [`已计费 ${counts.chargedEvents.toLocaleString()}`];
	if (counts.pendingEvents > 0) parts.push(`待结算 ${counts.pendingEvents.toLocaleString()}`);
	if (counts.unpaidEvents > 0) parts.push(`未计费 ${counts.unpaidEvents.toLocaleString()}`);
	if (counts.skippedEvents > 0) parts.push(`跳过 ${counts.skippedEvents.toLocaleString()}`);
	return parts;
}

function groupByModel(rows: BillingModelUsageRow[]): ModelUsageGroup[] {
	const groups = new Map<string, ModelUsageGroup>();
	for (const row of rows) {
		let group = groups.get(row.aimodel_id);
		if (!group) {
			group = {
				modelId: row.aimodel_id,
				// 模型行被删掉时事件里只剩 id；连 id 都没有 = 平台默认模型，量照样算。
				name: row.model_name || row.aimodel_id || "未归属模型",
				type: row.model_type,
				providerName: row.provider_name,
				items: [],
				eventCount: 0,
				chargedEvents: 0,
				pendingEvents: 0,
				unpaidEvents: 0,
				skippedEvents: 0,
				amountMicro: 0,
			};
			groups.set(row.aimodel_id, group);
		}
		const counts = statusCountsOf(row);
		group.items.push(row);
		group.eventCount += counts.eventCount;
		group.chargedEvents += counts.chargedEvents;
		group.pendingEvents += counts.pendingEvents;
		group.unpaidEvents += counts.unpaidEvents;
		group.skippedEvents += counts.skippedEvents;
		group.amountMicro += row.amount_micro;
	}

	for (const group of groups.values()) {
		group.items.sort(
			(a, b) => b.amount_micro - a.amount_micro || b.quantity - a.quantity,
		);
	}
	return [...groups.values()];
}

function sortGroups(groups: ModelUsageGroup[], key: SortKey): ModelUsageGroup[] {
	const sorted = [...groups];
	switch (key) {
		case "events":
			sorted.sort(
				(a, b) => b.eventCount - a.eventCount || b.amountMicro - a.amountMicro,
			);
			return sorted;
		case "name":
			sorted.sort((a, b) => a.name.localeCompare(b.name, "zh-Hans-CN"));
			return sorted;
		default:
			sorted.sort(
				(a, b) => b.amountMicro - a.amountMicro || b.eventCount - a.eventCount,
			);
			return sorted;
	}
}

export default function ModelMonitorPage() {
	const [range, setRange] = useState<RangeKey>("7d");
	const [sort, setSort] = useState<SortKey>("amount");
	const [expanded, setExpanded] = useState<string | null>(null);
	const [usage, setUsage] = useState<BillingModelUsage | null>(null);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);
	const [reloadKey, setReloadKey] = useState(0);

	const from = useMemo(() => rangeStart(range).toISOString(), [range]);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingApi
			.modelUsage({ from })
			.then(({ data }) => {
				if (cancelled) return;
				setUsage(data);
				setDisabled(false);
				setBanner(null);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setBanner({
					kind: "error",
					text: billingErrorMessage(err, "加载模型用量失败"),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [from, reloadKey]);

	const currency = usage?.currency ?? "CNY";
	const groups = useMemo(
		() => sortGroups(groupByModel(usage?.usage_by_model ?? []), sort),
		[usage, sort],
	);
	const totalAmount = groups.reduce((sum, group) => sum + group.amountMicro, 0);
	const totalCounts = groups.reduce(
		(sum, group) => addCounts(sum, group),
		{
			eventCount: 0,
			chargedEvents: 0,
			pendingEvents: 0,
			unpaidEvents: 0,
			skippedEvents: 0,
		} satisfies EventStatusCounts,
	);
	const top = groups[0];
	const rangeHint = usage ? `${formatTime(usage.from)} ~ ${formatTime(usage.to)}` : "";

	const header = (
		<MonitorHeader
			range={range}
			rangeHint={rangeHint}
			onRange={setRange}
			onReload={() => setReloadKey((key) => key + 1)}
			reloading={loading}
		/>
	);

	if (disabled) {
		return (
			<div className="min-h-full">
				{header}
				<div className="px-8 py-6">
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<EmptyState
							icon={Activity}
							title={BILLING_DISABLED_TITLE}
							hint="服务端没有开启计费模块（billing.enabled），用量事件不会被上报，这一页没有数据可看。"
						/>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="min-h-full">
			{header}

			<div className="px-8 py-6 space-y-6">
				<Banner message={banner} onClose={() => setBanner(null)} />

				{loading && !usage ? (
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<LoadingBlock label="加载模型用量..." />
					</div>
				) : groups.length === 0 ? (
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<EmptyState
							icon={Activity}
							title="这段时间没有模型用量"
							hint="用量事件在 turn 边界上报，会话跑过一轮才会有数据；换个时间段或等一轮会话再看。"
						/>
					</div>
				) : (
					<>
						{totalCounts.unpaidEvents > 0 && (
							<Hint tone="amber">
								有 {totalCounts.unpaidEvents.toLocaleString()} 条事件没算成钱（状态
								unpaid）：通常是这个计费项还没配价格，或者账户被停服。它们的用量照算在上面，
								金额记 ¥0；控制面补上价格后这批事件重放即可结算。
							</Hint>
						)}

						{/* 总览 */}
						<div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
							<StatCard
								label="覆盖模型"
								value={String(groups.length)}
								hint="这段时间有用量的模型数"
								icon={Layers}
								tone="violet"
							/>
							<StatCard
								label="用量事件"
								value={totalCounts.eventCount.toLocaleString()}
								hint={countsSummary(totalCounts).join(" · ")}
								icon={Activity}
								tone="sky"
								hintTone={
									totalCounts.unpaidEvents > 0 || totalCounts.pendingEvents > 0
										? "amber"
										: "zinc"
								}
							/>
							<StatCard
								label="已结算金额"
								value={formatMicro(totalAmount, currency)}
								hint="只算 charged 事件；待结算与未计费的不在内"
								icon={Coins}
								tone="amber"
							/>
							<StatCard
								label="最耗模型"
								value={top.name}
								hint={
									totalAmount > 0
										? `占已结算金额 ${(ratio(top.amountMicro, totalAmount) * 100).toFixed(1)}%`
										: "暂无已结算金额"
								}
								icon={TrendingUp}
								tone="emerald"
							/>
						</div>

						{/* 各模型明细 */}
						<Panel
							title="各模型用量"
							description="一行一个模型：数量含未结算事件，金额只算已结算的；展开看计费项明细"
							actions={
								<div className="flex gap-1 bg-zinc-900 border border-zinc-800 rounded-lg p-0.5">
									{SORT_OPTIONS.map((option) => (
										<button
											key={option.value}
											onClick={() => setSort(option.value)}
											className={`px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer ${
												sort === option.value
													? "bg-zinc-800 text-white"
													: "text-zinc-500 hover:text-zinc-300"
											}`}
										>
											{option.label}
										</button>
									))}
								</div>
							}
							bodyClassName="p-0"
						>
							<TableShell
								head={
									<>
										<Th className="w-72">模型</Th>
										<Th className="text-right">事件数</Th>
										<Th>用量明细</Th>
										<Th className="text-right">已结算金额</Th>
										<Th className="w-56">占比</Th>
									</>
								}
							>
								{groups.map((group, index) => {
									const share = ratio(group.amountMicro, totalAmount);
									const open = expanded === group.modelId;
									return (
										<Fragment key={group.modelId || "unassigned"}>
											<Tr
												last={!open && index === groups.length - 1}
												className={open ? "bg-zinc-800/30" : undefined}
											>
												<Td>
													<button
														onClick={() =>
															setExpanded(open ? null : group.modelId)
														}
														className="flex items-start gap-2 text-left w-full cursor-pointer"
														title={open ? "收起计费项明细" : "展开计费项明细"}
													>
														{open ? (
															<ChevronDown
																className="w-3.5 h-3.5 text-zinc-500 mt-0.5 shrink-0"
																strokeWidth={1.5}
															/>
														) : (
															<ChevronRight
																className="w-3.5 h-3.5 text-zinc-500 mt-0.5 shrink-0"
																strokeWidth={1.5}
															/>
														)}
														<div className="min-w-0">
															<div className="flex items-center gap-2">
																<span
																	className="text-zinc-200 truncate"
																	title={group.name}
																>
																	{group.name}
																</span>
																<Pill
																	tone={
																		MODEL_TYPE_TONES[group.type] ?? "zinc"
																	}
																>
																	{modelTypeLabel(group.type)}
																</Pill>
															</div>
															<p
																className="text-[11px] text-zinc-500 font-mono truncate mt-0.5"
																title={`${group.providerName} · ${group.modelId}`}
															>
																{group.providerName || "未知厂商"} ·{" "}
																{group.modelId || "无模型 ID"}
															</p>
														</div>
													</button>
												</Td>
												<Td className="text-right font-mono text-zinc-300">
													{group.eventCount.toLocaleString()}
													{group.pendingEvents > 0 && (
														<p className="text-[11px] text-amber-400/90 font-sans">
															{group.pendingEvents.toLocaleString()} 待结算
														</p>
													)}
													{group.unpaidEvents > 0 && (
														<p className="text-[11px] text-red-400/90 font-sans">
															{group.unpaidEvents.toLocaleString()} 未计费
														</p>
													)}
												</Td>
												<Td>
													<div className="flex flex-wrap items-center gap-1">
														{group.items.slice(0, VISIBLE_ITEMS).map((item) => (
															<span
																key={item.item_code}
																className="inline-flex items-center gap-1.5 text-[11px] px-1.5 py-0.5 rounded border border-zinc-700/70 bg-zinc-800/50"
																title={`${itemLabel(item.item_code)} · ${formatQuantityExact(item.quantity, itemUnit(item.item_code))}`}
															>
																<span className="text-zinc-400">
																	{itemLabel(item.item_code)}
																</span>
																<span className="text-zinc-200 font-mono">
																	{itemAmountText(item)}
																</span>
															</span>
														))}
														{group.items.length > VISIBLE_ITEMS && (
															<span className="text-[11px] text-zinc-500">
																+{group.items.length - VISIBLE_ITEMS} 项
															</span>
														)}
													</div>
												</Td>
												<Td className="text-right font-mono text-amber-400">
													{formatMicro(group.amountMicro, currency)}
												</Td>
												<Td>
													<div className="flex items-center gap-2">
														<div className="flex-1 h-1.5 rounded-full bg-zinc-800 overflow-hidden">
															<div
																className="h-full rounded-full bg-violet-500"
																style={{
																	width: `${(share * 100).toFixed(1)}%`,
																}}
															/>
														</div>
														<span className="text-[11px] text-zinc-500 font-mono w-12 text-right">
															{(share * 100).toFixed(1)}%
														</span>
													</div>
												</Td>
											</Tr>
											{open && (
												<tr className="border-b border-zinc-800/50 bg-zinc-950/40">
													<td colSpan={5} className="px-4 pb-4 pt-1">
														<table className="w-full">
															<thead>
																<tr className="border-b border-zinc-800">
																	<Th>计费项</Th>
																	<Th className="text-right">数量</Th>
																	<Th className="text-right">事件数</Th>
																	<Th className="text-right">
																		已结算金额
																	</Th>
																</tr>
															</thead>
															<tbody>
																{group.items.map((item) => (
																	<tr
																		key={item.item_code}
																		className="border-b border-zinc-800/40 last:border-0"
																	>
																		<Td>
																			<span className="text-zinc-300">
																				{itemLabel(item.item_code)}
																			</span>
																			<span className="text-[11px] text-zinc-500 font-mono ml-2">
																				{item.item_code}
																			</span>
																		</Td>
																		<Td
																			className="text-right font-mono text-zinc-300"
																			title={formatQuantityExact(
																				item.quantity,
																				itemUnit(item.item_code),
																			)}
																		>
																			{itemAmountText(item)}
																		</Td>
																		<Td className="text-right font-mono text-zinc-400">
																			{item.event_count.toLocaleString()}
																			{item.pending_events > 0 && (
																				<span className="text-amber-400/90 font-sans">
																					{" "}
																					·{" "}
																					{item.pending_events.toLocaleString()}{" "}
																					待结算
																				</span>
																			)}
																			{item.unpaid_events > 0 && (
																				<span className="text-red-400/90 font-sans">
																					{" "}
																					·{" "}
																					{item.unpaid_events.toLocaleString()}{" "}
																					未计费
																				</span>
																			)}
																		</Td>
																		<Td className="text-right font-mono text-amber-400">
																			{formatMicro(
																				item.amount_micro,
																				currency,
																			)}
																		</Td>
																	</tr>
																))}
															</tbody>
														</table>
													</td>
												</tr>
											)}
										</Fragment>
									);
								})}
								<Tr className="bg-zinc-800/30">
									<Td className="text-xs font-semibold text-zinc-400">
										合计 · {groups.length} 个模型
									</Td>
									<Td className="text-right font-mono font-semibold text-zinc-300">
										{totalCounts.eventCount.toLocaleString()}
									</Td>
									<Td />
									<Td className="text-right font-mono font-semibold text-amber-300">
										{formatMicro(totalAmount, currency)}
									</Td>
									<Td />
								</Tr>
							</TableShell>
						</Panel>
					</>
				)}

				<Hint tone="zinc">
					这一页只看自己账户的用量。数据来自计费的用量事件：每个 turn 的 token、合成字符数、
					识别时长在 turn 边界上报，事件里带着当时的模型与厂商。事件数是事件条数而不是调用次数
					（一次 LLM 调用会拆成输入 / 输出等几条），数量含未结算事件，金额只算已结算
					（charged）的——worker 每 1~5 秒结算一轮，所以金额会比数量“慢一拍”；对不上时看
					“待结算”与“未计费”那两个数。
				</Hint>
			</div>
		</div>
	);
}

function MonitorHeader({
	range,
	rangeHint,
	onRange,
	onReload,
	reloading,
}: {
	range: RangeKey;
	rangeHint: string;
	onRange: (range: RangeKey) => void;
	onReload: () => void;
	reloading: boolean;
}) {
	return (
		<div className="border-b border-zinc-800/80 px-8 py-5">
			<div className="flex items-center justify-between gap-4">
				<div>
					<h1 className="text-lg font-semibold text-white">模型监控</h1>
					<p className="text-sm text-zinc-500 mt-0.5">
						各模型的用量与消耗{rangeHint ? ` · ${rangeHint}` : ""}
					</p>
				</div>
				<div className="flex items-center gap-2">
					<div className="flex gap-1 bg-zinc-900 border border-zinc-800 rounded-lg p-0.5">
						{RANGE_OPTIONS.map((option) => (
							<button
								key={option.value}
								onClick={() => onRange(option.value)}
								className={`px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer ${
									range === option.value
										? "bg-zinc-800 text-white"
										: "text-zinc-500 hover:text-zinc-300"
								}`}
							>
								{option.label}
							</button>
						))}
					</div>
					<button
						onClick={onReload}
						disabled={reloading}
						className="inline-flex items-center gap-1.5 h-8 px-2.5 text-xs rounded-lg bg-zinc-900 border border-zinc-800 text-zinc-300 hover:bg-zinc-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
					>
						<RefreshCw
							className={`w-3.5 h-3.5 ${reloading ? "animate-spin" : ""}`}
							strokeWidth={1.5}
						/>
						刷新
					</button>
				</div>
			</div>
		</div>
	);
}

const STAT_TONES = {
	violet: { box: "bg-violet-400/10", icon: "text-violet-400" },
	sky: { box: "bg-sky-400/10", icon: "text-sky-400" },
	amber: { box: "bg-amber-400/10", icon: "text-amber-400" },
	emerald: { box: "bg-emerald-400/10", icon: "text-emerald-400" },
} as const;

function StatCard({
	label,
	value,
	hint,
	icon: Icon,
	tone,
	hintTone = "zinc",
}: {
	label: string;
	value: string;
	hint: string;
	icon: LucideIcon;
	tone: keyof typeof STAT_TONES;
	hintTone?: "zinc" | "amber";
}) {
	return (
		<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
			<div className="flex items-center justify-between mb-3">
				<p className="text-xs text-zinc-500">{label}</p>
				<div
					className={`w-8 h-8 rounded-lg ${STAT_TONES[tone].box} flex items-center justify-center`}
				>
					<Icon
						className={`w-4 h-4 ${STAT_TONES[tone].icon}`}
						strokeWidth={1.5}
					/>
				</div>
			</div>
			<p className="text-xl font-semibold text-white truncate" title={value}>
				{value}
			</p>
			<p
				className={`text-[11px] mt-0.5 truncate ${
					hintTone === "amber" ? "text-amber-400/90" : "text-zinc-600"
				}`}
				title={hint}
			>
				{hint}
			</p>
		</div>
	);
}
