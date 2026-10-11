// 用户端「用量与余额」。
//
// 数据全部来自 /api/billing/summary 与 /api/billing/usage：账户与余额由控制面按 JWT
// 里的 user id 反查（§14.2），前端不传账户 ID。503 表示平台没开计费，渲染"计费未启用"
// 而不是报错；查不到账户也不是错误，是"还没产生过用量"。

import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
	Activity,
	AlertCircle,
	BarChart3,
	CircleDollarSign,
	Coins,
	Lock,
	Plus,
	RefreshCw,
} from "lucide-react";
import { SimpleSelect } from "@/components/ui/select";
import { billingApi, type BillingSummary, type BillingUsageEvent } from "@/lib/api";
import { useDocumentTitle } from "@/lib/title";
import {
	accountStatusLabel,
	BILLING_ITEM_CODES,
	billingErrorMessage,
	eventStatusLabel,
	formatMicro,
	formatQuantity,
	formatQuantityExact,
	formatTime,
	isBillingDisabled,
	itemLabel,
	itemMeta,
	PERIOD_PRESETS,
	periodRange,
	type PeriodPreset,
	ratio,
} from "@/lib/billing";
import {
	EmptyState,
	LoadingBlock,
	PaginationBar,
	Panel,
	Pill,
	TableShell,
	Td,
	Th,
	Tr,
} from "@/pages/billing/shared";

const PAGE_SIZE = 20;

const EVENT_STATUS_TONES: Record<
	string,
	"emerald" | "amber" | "red" | "zinc" | "violet"
> = {
	charged: "emerald",
	pending: "amber",
	skipped: "zinc",
	unpaid: "red",
	rejected: "zinc",
};

const ACCOUNT_STATUS_TONES: Record<string, "emerald" | "red" | "zinc"> = {
	active: "emerald",
	suspended: "red",
	closed: "zinc",
};

export default function UsagePage() {
	const { t, i18n } = useTranslation(["billingPages", "common"]);
	useDocumentTitle(t("usage.title"));

	const [preset, setPreset] = useState<PeriodPreset>("current");
	const [summary, setSummary] = useState<BillingSummary | null>(null);
	const [summaryLoading, setSummaryLoading] = useState(true);
	const [events, setEvents] = useState<BillingUsageEvent[]>([]);
	const [total, setTotal] = useState(0);
	const [page, setPage] = useState(1);
	const [itemCode, setItemCode] = useState("");
	const [listLoading, setListLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [error, setError] = useState("");
	const [rechargeAvailable, setRechargeAvailable] = useState(false);
	const [reloadKey, setReloadKey] = useState(0);

	const range = useMemo(() => periodRange(preset), [preset, i18n.language]);

	// 充值入口只在服务端接了支付渠道时才给：没接的话点进去只有一句「暂不支持在线充值」，
	// 与其给个死胡同，不如不给这个按钮。判据是 recharge/config 返回 503（和计费未启用
	// 同一个信号），其余错误也当不可用——宁可少一个入口，也不给一个会失败的入口。
	useEffect(() => {
		let cancelled = false;
		billingApi
			.rechargeConfig()
			.then(() => {
				if (!cancelled) setRechargeAvailable(true);
			})
			.catch(() => {
				if (!cancelled) setRechargeAvailable(false);
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	useEffect(() => {
		let cancelled = false;
		setSummaryLoading(true);
		billingApi
			.summary({ from: range.fromRFC, to: range.toRFC })
			.then(({ data }) => {
				if (cancelled) return;
				setSummary(data);
				setDisabled(false);
				setError("");
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setError(billingErrorMessage(err, t("usage.loadSummaryFailed")));
			})
			.finally(() => {
				if (!cancelled) setSummaryLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [range.fromRFC, range.toRFC, reloadKey]);

	useEffect(() => {
		let cancelled = false;
		setListLoading(true);
		billingApi
			.usage({
				from: range.fromRFC,
				to: range.toRFC,
				page,
				page_size: PAGE_SIZE,
				...(itemCode ? { item_code: itemCode } : {}),
			})
			.then(({ data }) => {
				if (cancelled) return;
				setEvents(data.items);
				setTotal(data.total);
				setDisabled(false);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setError(billingErrorMessage(err, t("usage.loadUsageFailed")));
			})
			.finally(() => {
				if (!cancelled) setListLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [range.fromRFC, range.toRFC, page, itemCode, reloadKey]);

	const currency = summary?.currency ?? "CNY";
	const account = summary?.account ?? null;
	const balance = summary?.balance_micro ?? 0;
	const topItems = summary?.top_items ?? [];
	const topTotal = topItems.reduce((sum, item) => sum + item.amount_micro, 0);

	if (disabled) {
		return (
			<div className="min-h-full">
				<UsageHeader
					preset={preset}
					rangeHint={range.hint}
					onPreset={setPreset}
					onReload={() => setReloadKey((key) => key + 1)}
					reloading={false}
				/>
				<div className="px-8 py-6">
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<EmptyState
							icon={Coins}
							title={t("billing:disabledTitle")}
							hint={t("usage.disabledHint")}
						/>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="min-h-full">
			<UsageHeader
				preset={preset}
				rangeHint={range.hint}
				onPreset={(next) => {
					setPreset(next);
					setPage(1);
				}}
				onReload={() => setReloadKey((key) => key + 1)}
				reloading={summaryLoading || listLoading}
			/>

			<div className="px-8 py-6 space-y-6">
				{error && (
					<div className="flex items-start gap-2 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3">
						<AlertCircle className="w-4 h-4 text-red-400 mt-0.5 shrink-0" />
						<p className="text-xs text-red-300">{error}</p>
					</div>
				)}

				{summaryLoading && !summary ? (
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<LoadingBlock label={t("usage.loadingSummary")} />
					</div>
				) : !account ? (
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl">
						<EmptyState
							icon={CircleDollarSign}
							title={t("usage.noAccountTitle")}
							hint={t("usage.noAccountHint")}
							action={rechargeAvailable ? <RechargeButton /> : undefined}
						/>
					</div>
				) : (
					<>
						{/* 总览 */}
						<div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
							<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
								<div className="flex items-center justify-between mb-3">
									<p className="text-xs text-zinc-500">
										{t("usage.availableBalance")}
									</p>
									<div className="w-8 h-8 rounded-lg bg-amber-400/10 flex items-center justify-center">
										<Coins
											className="w-4 h-4 text-amber-400"
											strokeWidth={1.5}
										/>
									</div>
								</div>
								<div className="flex flex-wrap items-center gap-x-3 gap-y-2">
									<p
										className={`text-xl font-semibold font-mono ${
											balance < 0 ? "text-red-400" : "text-white"
										}`}
									>
										{formatMicro(balance, currency)}
									</p>
									{rechargeAvailable && <RechargeButton className="ml-auto" />}
								</div>
								<p className="text-[11px] text-zinc-600 mt-0.5">
									{balance < 0 ? (
										<span className="text-red-400/90">
											{t("usage.negativeBalance")}
											{account.credit_limit_micro > 0
												? t(
														"usage.creditLimitParenthetical",
														{
															amount: formatMicro(
																account.credit_limit_micro,
																currency,
															),
														},
													)
												: t("usage.prepaidParenthetical")}
										</span>
									) : account.credit_limit_micro > 0 ? (
										t("usage.creditLimit", {
											amount: formatMicro(
												account.credit_limit_micro,
												currency,
											),
										})
									) : (
										t("usage.prepaidAccount")
									)}
								</p>
							</div>

							<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
								<div className="flex items-center justify-between mb-3">
									<p className="text-xs text-zinc-500">
										{t("usage.frozen")}
									</p>
									<div className="w-8 h-8 rounded-lg bg-sky-400/10 flex items-center justify-center">
										<Lock
											className="w-4 h-4 text-sky-400"
											strokeWidth={1.5}
										/>
									</div>
								</div>
								<p className="text-xl font-semibold text-white font-mono">
									{formatMicro(summary?.frozen_micro ?? 0, currency)}
								</p>
								<p className="text-[11px] text-zinc-600 mt-0.5">
									{t("usage.frozenHint")}
								</p>
							</div>

							<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
								<div className="flex items-center justify-between mb-3">
									<p className="text-xs text-zinc-500">
										{t("usage.periodConsumption", {
											period: range.label,
										})}
									</p>
									<div className="w-8 h-8 rounded-lg bg-violet-400/10 flex items-center justify-center">
										<Activity
											className="w-4 h-4 text-violet-400"
											strokeWidth={1.5}
										/>
									</div>
								</div>
								<p className="text-xl font-semibold text-amber-400 font-mono">
									{formatMicro(
										summary?.period_charged_micro ?? 0,
										currency,
									)}
								</p>
								<p className="text-[11px] text-zinc-600 mt-0.5">
									{range.hint}
								</p>
							</div>

							<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
								<div className="flex items-center justify-between mb-3">
									<p className="text-xs text-zinc-500">
										{t("usage.accountStatus")}
									</p>
									<div className="w-8 h-8 rounded-lg bg-emerald-400/10 flex items-center justify-center">
										<BarChart3
											className="w-4 h-4 text-emerald-400"
											strokeWidth={1.5}
										/>
									</div>
								</div>
								<p
									className={`text-xl font-semibold ${
										(ACCOUNT_STATUS_TONES[account.status] ?? "zinc") ===
										"emerald"
											? "text-emerald-400"
											: (ACCOUNT_STATUS_TONES[account.status] ??
													"zinc") === "red"
												? "text-red-400"
												: "text-white"
									}`}
								>
									{accountStatusLabel(account.status)}
								</p>
								<p
									className="text-[11px] text-zinc-600 mt-0.5 font-mono truncate"
									title={account.id}
								>
									{account.id} · {account.currency}
								</p>
							</div>
						</div>

						{/* Top 计费项 */}
						<Panel
							title={t("usage.topItemsTitle", { period: range.label })}
							description={t("usage.topItemsHint")}
						>
							{topItems.length === 0 ? (
								<p className="text-xs text-zinc-600 py-2">
									{t("usage.noChargedUsage")}
								</p>
							) : (
								<div className="space-y-3">
									{topItems.map((item) => {
										const share = ratio(item.amount_micro, topTotal);
										return (
											<div key={item.item_code}>
												<div className="flex items-center justify-between gap-3 mb-1.5">
													<div className="flex items-center gap-2 min-w-0">
														<span className="text-sm text-zinc-200 truncate">
															{itemLabel(item.item_code)}
														</span>
														<span className="text-[11px] text-zinc-600 font-mono truncate">
															{item.item_code}
														</span>
													</div>
													<div className="flex items-center gap-3 shrink-0">
														<span
															className="text-xs text-zinc-500 font-mono"
															title={formatQuantityExact(
																item.quantity,
																itemMeta(item.item_code)
																	?.unit ?? "",
															)}
														>
															{formatQuantity(
																item.quantity,
																itemMeta(item.item_code)
																	?.unit ?? "",
															)}
														</span>
														<span className="text-sm text-amber-400 font-mono w-24 text-right">
															{formatMicro(
																item.amount_micro,
																currency,
															)}
														</span>
													</div>
												</div>
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
											</div>
										);
									})}
									<div className="flex items-center justify-between pt-2 border-t border-zinc-800">
										<span className="text-xs text-zinc-500">
											{t("usage.total")}
										</span>
										<span className="text-sm text-amber-300 font-mono">
											{formatMicro(topTotal, currency)}
										</span>
									</div>
								</div>
							)}
						</Panel>
					</>
				)}

				{/* 用量明细 */}
				<Panel
					title={t("usage.detailsTitle")}
					description={t("usage.detailsHint")}
					actions={
						<>
							<SimpleSelect
								value={itemCode}
								onValueChange={(value) => {
									setItemCode(value);
									setPage(1);
								}}
								className="w-56"
								size="sm"
								placeholder={t("usage.allItems")}
								options={BILLING_ITEM_CODES.map((code) => ({
									value: code,
									label: itemLabel(code),
								}))}
							/>
							<button
								onClick={() => setReloadKey((key) => key + 1)}
								disabled={listLoading}
								className="inline-flex items-center gap-1.5 h-7 px-2.5 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
							>
								<RefreshCw
									className={`w-3.5 h-3.5 ${listLoading ? "animate-spin" : ""}`}
									strokeWidth={1.5}
								/>
								{t("common:action.refresh")}
							</button>
						</>
					}
					bodyClassName="p-0"
				>
					{listLoading ? (
						<LoadingBlock />
					) : events.length === 0 ? (
						<EmptyState
							icon={Coins}
							title={
								itemCode
									? t("usage.emptyWithItem", {
											item: itemLabel(itemCode),
										})
									: t("usage.empty")
							}
							hint={t("usage.emptyHint")}
						/>
					) : (
						<TableShell
							head={
								<>
									<Th>{t("usage.column.time")}</Th>
									<Th>{t("usage.column.item")}</Th>
									<Th className="text-right">
										{t("usage.column.quantity")}
									</Th>
									<Th>{t("usage.column.status")}</Th>
									<Th className="text-right">
										{t("usage.column.amount")}
									</Th>
									<Th>{t("usage.column.session")}</Th>
								</>
							}
						>
							{events.map((event, index) => {
								const unit = itemMeta(event.item_code)?.unit ?? "";
								const last = index === events.length - 1;
								return (
									<Tr key={event.id} last={last}>
										<Td className="text-xs text-zinc-500 font-mono whitespace-nowrap">
											{formatTime(event.occurred_at)}
										</Td>
										<Td>
											<div className="flex flex-col">
												<span className="text-zinc-200">
													{itemLabel(event.item_code)}
												</span>
												<span className="text-[11px] text-zinc-500 font-mono">
													{event.item_code}
												</span>
											</div>
										</Td>
										<Td
											className="text-right font-mono text-zinc-300 whitespace-nowrap"
											title={formatQuantityExact(
												event.quantity,
												unit,
											)}
										>
											{formatQuantity(event.quantity, unit)}
										</Td>
										<Td>
											<div className="flex items-center gap-1.5">
												<Pill
													tone={
														EVENT_STATUS_TONES[
															event.status
														] ?? "zinc"
													}
												>
													{eventStatusLabel(event.status)}
												</Pill>
												{event.byok && (
													<Pill tone="sky">
														{t("usage.byok")}
													</Pill>
												)}
											</div>
											{event.last_error && (
												<p className="text-[11px] text-red-400/80 mt-1 max-w-56">
													{event.last_error}
												</p>
											)}
										</Td>
										<Td className="text-right font-mono text-amber-400 whitespace-nowrap">
											{event.amount_micro > 0
												? formatMicro(
														event.amount_micro,
														currency,
													)
												: "—"}
										</Td>
										<Td className="text-xs">
											{event.session_id ? (
												<div className="flex flex-col">
													<span
														className="text-zinc-400 font-mono truncate max-w-40"
														title={event.session_id}
													>
														{event.session_id}
													</span>
													<span className="text-[11px] text-zinc-600">
														{t("usage.turnIndex", {
															index: event.turn_index,
														})}
													</span>
												</div>
											) : (
												<span className="text-zinc-600">
													—
												</span>
											)}
										</Td>
									</Tr>
								);
							})}
						</TableShell>
					)}
				</Panel>

				<PaginationBar
					page={page}
					pageSize={PAGE_SIZE}
					total={total}
					loading={listLoading}
					onPageChange={setPage}
				/>
			</div>
		</div>
	);
}

/**
 * 充值入口。调用方只在服务端接了支付渠道（rechargeAvailable）时才渲染它：它就是个
 * 跳 `/billing/recharge` 的按钮，付钱的事全在那边（这里不做任何下单）。
 */
function RechargeButton({ className }: { className?: string }) {
	const { t } = useTranslation(["billingPages", "common"]);
	const navigate = useNavigate();
	return (
		<button
			onClick={() => navigate("/billing/recharge")}
			className={cn(
				"inline-flex items-center gap-1 h-7 px-2.5 text-xs rounded-lg bg-violet-600 hover:bg-violet-500 text-white shadow-md shadow-violet-600/20 transition-colors cursor-pointer shrink-0",
				className,
			)}
		>
			<Plus className="w-3.5 h-3.5" strokeWidth={2} />
			{t("usage.rechargeAction")}
		</button>
	);
}

function UsageHeader({
	preset,
	rangeHint,
	onPreset,
	onReload,
	reloading,
}: {
	preset: PeriodPreset;
	rangeHint: string;
	onPreset: (preset: PeriodPreset) => void;
	onReload: () => void;
	reloading: boolean;
}) {
	const { t } = useTranslation(["billingPages", "common"]);
	return (
		<div className="border-b border-zinc-800/80 px-8 py-5">
			<div className="flex items-center justify-between gap-4">
				<div>
					<h1 className="text-lg font-semibold text-white">
						{t("usage.title")}
					</h1>
					<p className="text-sm text-zinc-500 mt-0.5">
						{t("usage.subtitle", { range: rangeHint })}
					</p>
				</div>
				<div className="flex items-center gap-2">
					<div className="flex gap-1 bg-zinc-900 border border-zinc-800 rounded-lg p-0.5">
						{PERIOD_PRESETS.map((option) => (
							<button
								key={option.value}
								onClick={() => onPreset(option.value)}
								className={`px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer ${
									preset === option.value
										? "bg-zinc-800 text-white"
										: "text-zinc-500 hover:text-zinc-300"
								}`}
							>
								{t(option.labelKey)}
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
						{t("common:action.refresh")}
					</button>
				</div>
			</div>
		</div>
	);
}
