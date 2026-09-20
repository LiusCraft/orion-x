// 计费管理 · 流水：账户 / 类型 / 时间段过滤 + 分页。
//
// 流水是 append-only 的事实，这里只有读：退款与人工调整写的是新的 adjust 流水，
// 不改历史行（§3.5）。金额符号按 direction 显示（debit 支出 / credit 入账）；
// reserve / release 只动冻结，balance_after 不变。

import { useEffect, useState } from "react";
import { ArrowLeftRight, RefreshCw } from "lucide-react";
import { Input } from "@/components/ui/input";
import { SimpleSelect } from "@/components/ui/select";
import { billingAdminApi, type BillingAccount, type BillingLedger } from "@/lib/api";
import {
	BILLING_DISABLED_TITLE,
	billingErrorMessage,
	directionLabel,
	formatMicro,
	formatTime,
	isBillingDisabled,
	itemLabel,
	ledgerKindLabel,
	refTypeLabel,
	startOfLocalDay,
	endOfLocalDay,
	subjectTypeLabel,
} from "@/lib/billing";
import {
	Banner,
	EmptyState,
	FilterBar,
	LoadingBlock,
	PaginationBar,
	Panel,
	Pill,
	TableShell,
	Td,
	Th,
	Tr,
	type BannerMessage,
} from "../shared";

const PAGE_SIZE = 20;

const LEDGER_KINDS = [
	"charge",
	"grant",
	"recharge",
	"refund",
	"adjust",
	"reserve",
	"release",
	"expire",
];

export default function LedgerTab() {
	const [rows, setRows] = useState<BillingLedger[]>([]);
	const [total, setTotal] = useState(0);
	const [page, setPage] = useState(1);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);

	const [accounts, setAccounts] = useState<BillingAccount[]>([]);
	const [accountId, setAccountId] = useState("");
	const [kind, setKind] = useState("");
	const [from, setFrom] = useState("");
	const [to, setTo] = useState("");
	const [reloadKey, setReloadKey] = useState(0);

	// 账户下拉只需要一份候选，翻页不受它影响。
	useEffect(() => {
		let cancelled = false;
		billingAdminApi
			.accounts({ page: 1, page_size: 100 })
			.then(({ data }) => {
				if (!cancelled) setAccounts(data.items);
			})
			.catch(() => {
				// 账户列表只是过滤器，加载失败不阻塞流水查询
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingAdminApi
			.ledger({
				page,
				page_size: PAGE_SIZE,
				...(accountId ? { account_id: accountId } : {}),
				...(kind ? { kind } : {}),
				...(from
					? { from: startOfLocalDay(from)?.toISOString() }
					: {}),
				...(to ? { to: endOfLocalDay(to)?.toISOString() } : {}),
			})
			.then(({ data }) => {
				if (cancelled) return;
				setRows(data.items);
				setTotal(data.total);
				setDisabled(false);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setBanner({ kind: "error", text: billingErrorMessage(err, "加载流水失败") });
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [page, accountId, kind, from, to, reloadKey]);

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={ArrowLeftRight}
					title={BILLING_DISABLED_TITLE}
					hint="服务端没有开启计费模块（billing.enabled），流水不可用。"
				/>
			</Panel>
		);
	}

	const currencyOf = (id: string) =>
		accounts.find((account) => account.id === id)?.currency ?? "CNY";

	return (
		<div className="space-y-4">
			<Banner message={banner} onClose={() => setBanner(null)} />

			<FilterBar>
				<SimpleSelect
					value={accountId}
					onValueChange={(value) => {
						setAccountId(value);
						setPage(1);
					}}
					className="w-64"
					size="sm"
					placeholder="全部账户"
					options={accounts.map((account) => ({
						value: account.id,
						label: `${subjectTypeLabel(account.subject_type)} · ${account.id}`,
					}))}
				/>
				<SimpleSelect
					value={kind}
					onValueChange={(value) => {
						setKind(value);
						setPage(1);
					}}
					className="w-32"
					size="sm"
					placeholder="全部类型"
					options={LEDGER_KINDS.map((value) => ({
						value,
						label: ledgerKindLabel(value),
					}))}
				/>
				<div className="flex items-center gap-2">
					<Input
						type="date"
						value={from}
						onChange={(e) => {
							setFrom(e.target.value);
							setPage(1);
						}}
						className="h-7 w-36 text-xs font-mono"
					/>
					<span className="text-xs text-zinc-600">~</span>
					<Input
						type="date"
						value={to}
						onChange={(e) => {
							setTo(e.target.value);
							setPage(1);
						}}
						className="h-7 w-36 text-xs font-mono"
					/>
					{(from || to) && (
						<button
							onClick={() => {
								setFrom("");
								setTo("");
								setPage(1);
							}}
							className="text-xs text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
						>
							清空
						</button>
					)}
				</div>
				<button
					onClick={() => setReloadKey((key) => key + 1)}
					disabled={loading}
					className="inline-flex items-center gap-1.5 h-7 px-2.5 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
				>
					<RefreshCw
						className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`}
						strokeWidth={1.5}
					/>
					刷新
				</button>
			</FilterBar>

			<Panel
				title="账本流水"
				description="append-only：只增不改。reserve / release 只动冻结，balance_after 不变"
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : rows.length === 0 ? (
					<EmptyState
						icon={ArrowLeftRight}
						title="没有匹配的流水"
						hint="换个账户或时间段再试；用量结算后会写 charge 流水，人工操作写 adjust 流水。"
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>时间</Th>
								<Th>账户</Th>
								<Th>方向</Th>
								<Th className="text-right">金额</Th>
								<Th className="text-right">余额后</Th>
								<Th>类型</Th>
								<Th>计费项</Th>
								<Th>关联</Th>
								<Th>备注</Th>
							</>
						}
					>
						{rows.map((row, index) => {
							const last = index === rows.length - 1;
							const currency = currencyOf(row.account_id);
							const amount = Math.abs(row.amount_micro);
							return (
								<Tr key={row.id} last={last}>
									<Td className="text-xs text-zinc-500 font-mono whitespace-nowrap">
										{formatTime(row.occurred_at)}
									</Td>
									<Td
										className="font-mono text-xs text-zinc-400 max-w-40 truncate"
										title={row.account_id}
									>
										{row.account_id}
									</Td>
									<Td>
										<Pill
											tone={row.direction === "debit" ? "red" : "emerald"}
										>
											{directionLabel(row.direction)}
										</Pill>
									</Td>
									<Td
										className={`text-right font-mono ${
											row.direction === "debit"
												? "text-red-400"
												: "text-emerald-400"
										}`}
									>
										{row.direction === "debit" ? "-" : "+"}
										{formatMicro(amount, currency)}
									</Td>
									<Td className="text-right font-mono text-zinc-400">
										{formatMicro(row.balance_after_micro, currency)}
									</Td>
									<Td>
										<span className="text-xs text-zinc-300">
											{ledgerKindLabel(row.kind)}
										</span>
									</Td>
									<Td className="text-xs">
										{row.item_code ? (
											<div className="flex flex-col">
												<span className="text-zinc-300">
													{itemLabel(row.item_code)}
												</span>
												<span className="text-[11px] text-zinc-600 font-mono">
													{row.item_code}
												</span>
											</div>
										) : (
											<span className="text-zinc-600">—</span>
										)}
									</Td>
									<Td className="text-xs max-w-52">
										<div
											className="flex flex-col"
											title={`${row.ref_type}:${row.ref_id}`}
										>
											<span className="text-zinc-400">
												{refTypeLabel(row.ref_type)}
											</span>
											{row.ref_id && (
												<span className="text-[11px] text-zinc-600 font-mono truncate">
													{row.ref_type}:{row.ref_id}
												</span>
											)}
										</div>
									</Td>
									<Td className="text-xs max-w-56">
										<div className="flex flex-col gap-0.5">
											<span
												className="text-zinc-400 truncate"
												title={row.note ?? ""}
											>
												{row.note || "—"}
											</span>
											{row.creator && (
												<span className="text-[11px] text-zinc-600 truncate">
													操作人 {row.creator}
												</span>
											)}
										</div>
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
				loading={loading}
				onPageChange={setPage}
			/>
		</div>
	);
}
