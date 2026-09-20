// 计费管理 · 报表：按账户 + 时间段聚合。
//
// 报表读的是已结算（charged）的用量事件，未结算的 pending 事件不在里面，所以它和
// ledger 的合计会差一个结算延迟——这一点在页面上直说，不然对账的人会以为丢钱了。

import { useEffect, useState } from "react";
import { Activity, Coins, LineChart, Lock, RefreshCw, Scale } from "lucide-react";
import { Input } from "@/components/ui/input";
import { SimpleSelect } from "@/components/ui/select";
import { billingAdminApi, type BillingAccount, type BillingStats } from "@/lib/api";
import {
	accountStatusLabel,
	BILLING_DISABLED_TITLE,
	billingErrorMessage,
	formatMicro,
	formatQuantity,
	formatTime,
	isBillingDisabled,
	itemLabel,
	itemMeta,
	ratio,
	startOfLocalDay,
	endOfLocalDay,
	subjectTypeLabel,
} from "@/lib/billing";
import {
	Banner,
	EmptyState,
	FilterBar,
	Hint,
	LoadingBlock,
	Panel,
	Pill,
	TableShell,
	Td,
	Th,
	Tr,
	type BannerMessage,
} from "../shared";

const STATUS_TONES: Record<string, "emerald" | "red" | "zinc"> = {
	active: "emerald",
	suspended: "red",
	closed: "zinc",
};

export default function StatsTab() {
	const [accounts, setAccounts] = useState<BillingAccount[]>([]);
	const [accountId, setAccountId] = useState("");
	const [from, setFrom] = useState("");
	const [to, setTo] = useState("");
	const [stats, setStats] = useState<BillingStats | null>(null);
	const [loading, setLoading] = useState(false);
	const [accountsLoading, setAccountsLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);
	const [reloadKey, setReloadKey] = useState(0);

	useEffect(() => {
		let cancelled = false;
		billingAdminApi
			.accounts({ page: 1, page_size: 100 })
			.then(({ data }) => {
				if (cancelled) return;
				setAccounts(data.items);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setBanner({
					kind: "error",
					text: billingErrorMessage(err, "加载账户列表失败"),
				});
			})
			.finally(() => {
				if (!cancelled) setAccountsLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	useEffect(() => {
		if (!accountId) {
			setStats(null);
			return;
		}
		let cancelled = false;
		setLoading(true);
		billingAdminApi
			.stats({
				account_id: accountId,
				...(from
					? { from: startOfLocalDay(from)?.toISOString() }
					: {}),
				...(to ? { to: endOfLocalDay(to)?.toISOString() } : {}),
			})
			.then(({ data }) => {
				if (cancelled) return;
				setStats(data);
				setDisabled(false);
				setBanner(null);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setBanner({ kind: "error", text: billingErrorMessage(err, "加载报表失败") });
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [accountId, from, to, reloadKey]);

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={LineChart}
					title={BILLING_DISABLED_TITLE}
					hint="服务端没有开启计费模块（billing.enabled），报表不可用。"
				/>
			</Panel>
		);
	}

	const currency = stats?.account.currency ?? "CNY";
	const byItem = stats?.usage_by_item ?? [];
	const itemTotal = byItem.reduce((sum, row) => sum + row.amount_micro, 0);
	const top = stats?.summary.top_items?.[0];

	return (
		<div className="space-y-4">
			<Banner message={banner} onClose={() => setBanner(null)} />

			<FilterBar>
				<div className="min-w-72">
					<SimpleSelect
						value={accountId}
						onValueChange={setAccountId}
						size="sm"
						placeholder={
							accountsLoading
								? "加载账户中..."
								: accounts.length === 0
									? "没有可用账户"
									: "选择账户"
						}
						disabled={accountsLoading || accounts.length === 0}
						className="w-72"
						options={accounts.map((account) => ({
							value: account.id,
							label: `${subjectTypeLabel(account.subject_type)} · ${account.subject_id} · ${account.id}`,
						}))}
					/>
				</div>
				<div className="flex items-center gap-2">
					<Input
						type="date"
						value={from}
						onChange={(e) => setFrom(e.target.value)}
						className="h-7 w-36 text-xs font-mono"
					/>
					<span className="text-xs text-zinc-600">~</span>
					<Input
						type="date"
						value={to}
						onChange={(e) => setTo(e.target.value)}
						className="h-7 w-36 text-xs font-mono"
					/>
					{(from || to) && (
						<button
							onClick={() => {
								setFrom("");
								setTo("");
							}}
							className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
						>
							清空（本账期）
						</button>
					)}
				</div>
				<button
					onClick={() => setReloadKey((key) => key + 1)}
					disabled={loading || !accountId}
					className="inline-flex items-center gap-1.5 h-7 px-2.5 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
				>
					<RefreshCw
						className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`}
						strokeWidth={1.5}
					/>
					刷新
				</button>
			</FilterBar>

			{!accountId ? (
				<Panel bodyClassName="p-0">
					<EmptyState
						icon={LineChart}
						title="先选一个账户"
						hint="报表按账户聚合，account_id 是这个接口的必填参数。时间段留空即本账期到现在。"
					/>
				</Panel>
			) : loading && !stats ? (
				<Panel bodyClassName="p-0">
					<LoadingBlock />
				</Panel>
			) : stats ? (
				<>
					<div className="bg-zinc-900 border border-zinc-800 rounded-xl px-5 py-4 flex flex-wrap items-center gap-x-4 gap-y-2">
						<span className="font-mono text-xs text-zinc-300">
							{stats.account.id}
						</span>
						<span className="text-xs text-zinc-500">
							{subjectTypeLabel(stats.account.subject_type)} ·{" "}
							{stats.account.subject_id}
						</span>
						<Pill tone={STATUS_TONES[stats.account.status] ?? "zinc"}>
							{accountStatusLabel(stats.account.status)}
						</Pill>
						<span className="text-xs text-zinc-500">
							{stats.account.currency}
						</span>
						<span className="text-xs text-zinc-500 ml-auto font-mono">
							{formatTime(stats.from)} ~ {formatTime(stats.to)}
						</span>
					</div>

					<div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
						<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<div className="flex items-center justify-between mb-3">
								<p className="text-xs text-zinc-500">可用余额</p>
								<div className="w-8 h-8 rounded-lg bg-amber-400/10 flex items-center justify-center">
									<Coins
										className="w-4 h-4 text-amber-400"
										strokeWidth={1.5}
									/>
								</div>
							</div>
							<p
								className={`text-xl font-semibold font-mono ${
									stats.summary.balance_micro < 0
										? "text-red-400"
										: "text-white"
								}`}
							>
								{formatMicro(stats.summary.balance_micro, currency)}
							</p>
							<p className="text-[11px] text-zinc-600 mt-0.5">
								可以为负 = 后付欠款
							</p>
						</div>
						<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<div className="flex items-center justify-between mb-3">
								<p className="text-xs text-zinc-500">预冻结</p>
								<div className="w-8 h-8 rounded-lg bg-sky-400/10 flex items-center justify-center">
									<Lock
										className="w-4 h-4 text-sky-400"
										strokeWidth={1.5}
									/>
								</div>
							</div>
							<p className="text-xl font-semibold text-white font-mono">
								{formatMicro(stats.summary.frozen_micro, currency)}
							</p>
							<p className="text-[11px] text-zinc-600 mt-0.5">
								会话预授权占用
							</p>
						</div>
						<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<div className="flex items-center justify-between mb-3">
								<p className="text-xs text-zinc-500">账期消耗</p>
								<div className="w-8 h-8 rounded-lg bg-violet-400/10 flex items-center justify-center">
									<Activity
										className="w-4 h-4 text-violet-400"
										strokeWidth={1.5}
									/>
								</div>
							</div>
							<p className="text-xl font-semibold text-white font-mono">
								{formatMicro(
									stats.summary.period_charged_micro,
									currency,
								)}
							</p>
							<p className="text-[11px] text-zinc-600 mt-0.5 truncate">
								{top
									? `Top：${itemLabel(top.item_code)}`
									: "账期内暂无消耗"}
							</p>
						</div>
						<div className="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<div className="flex items-center justify-between mb-3">
								<p className="text-xs text-zinc-500">信用额度</p>
								<div className="w-8 h-8 rounded-lg bg-emerald-400/10 flex items-center justify-center">
									<Scale
										className="w-4 h-4 text-emerald-400"
										strokeWidth={1.5}
									/>
								</div>
							</div>
							<p className="text-xl font-semibold text-white font-mono">
								{formatMicro(
									stats.summary.credit_limit_micro,
									currency,
								)}
							</p>
							<p className="text-[11px] text-zinc-600 mt-0.5">
								0 = 纯预付费
							</p>
						</div>
					</div>

					<Panel
						title="按计费项聚合"
						description="读的是已结算的用量事件；数量口径见各计费项的单位"
						bodyClassName="p-0"
					>
						{byItem.length === 0 ? (
							<EmptyState
								icon={LineChart}
								title="这段时间没有已结算的用量"
								hint="还没有产生用量，或者事件还在 pending 等结算。"
							/>
						) : (
							<TableShell
								head={
									<>
										<Th>计费项</Th>
										<Th className="text-right">数量</Th>
										<Th className="text-right">金额</Th>
										<Th className="w-64">占比</Th>
									</>
								}
							>
								{byItem.map((row, index) => {
									const last = index === byItem.length - 1;
									const share = ratio(row.amount_micro, itemTotal);
									return (
										<Tr key={row.item_code} last={last}>
											<Td>
												<div className="flex flex-col">
													<span className="text-zinc-200">
														{itemLabel(row.item_code)}
													</span>
													<span className="text-[11px] text-zinc-500 font-mono">
														{row.item_code}
													</span>
												</div>
											</Td>
											<Td className="text-right font-mono text-zinc-400">
												{formatQuantity(
													row.quantity,
													itemMeta(row.item_code)?.unit ?? "",
												)}
											</Td>
											<Td className="text-right font-mono text-amber-400">
												{formatMicro(row.amount_micro, currency)}
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
									);
								})}
								<Tr className="bg-zinc-800/30">
									<Td className="text-xs font-semibold text-zinc-400">
										合计
									</Td>
									<Td />
									<Td className="text-right font-mono font-semibold text-amber-300">
										{formatMicro(itemTotal, currency)}
									</Td>
									<Td />
								</Tr>
							</TableShell>
						)}
					</Panel>
				</>
			) : (
				<Panel bodyClassName="p-0">
					<EmptyState icon={LineChart} title="暂无报表数据" />
				</Panel>
			)}

			<Hint tone="zinc">
				报表只统计已结算的用量。数据面在 turn 边界上报、控制面 worker 每 1~5 秒跑一轮，
				正在 pending 的事件会让报表与流水差一个结算延迟（对不上时先看 pending 数）。
			</Hint>
		</div>
	);
}
