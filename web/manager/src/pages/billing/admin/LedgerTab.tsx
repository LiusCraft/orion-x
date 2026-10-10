// 计费管理 · 流水：账户 / 类型 / 时间段过滤 + 分页。
//
// 流水是 append-only 的事实，这里只有读：退款与人工调整写的是新的 adjust 流水，
// 不改历史行（§3.5）。金额符号按 direction 显示（debit 支出 / credit 入账）；
// reserve / release 只动冻结，balance_after 不变。

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeftRight, RefreshCw } from "lucide-react";
import { Input } from "@/components/ui/input";
import { SimpleSelect } from "@/components/ui/select";
import { billingAdminApi, type BillingAccount, type BillingLedger } from "@/lib/api";
import {
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
	const { t } = useTranslation(["billingAdmin", "common"]);
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
				setBanner({
					kind: "error",
					text: billingErrorMessage(err, t("ledger.loadFailed")),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [page, accountId, kind, from, to, reloadKey, t]);

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={ArrowLeftRight}
					title={t("billing:disabledTitle")}
					hint={t("ledger.disabledHint")}
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
					placeholder={t("shared.allAccounts")}
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
					placeholder={t("ledger.kindPlaceholder")}
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
							{t("ledger.clear")}
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
					{t("common:action.refresh")}
				</button>
			</FilterBar>

			<Panel
				title={t("ledger.panelTitle")}
				description={t("ledger.panelDescription")}
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : rows.length === 0 ? (
					<EmptyState
						icon={ArrowLeftRight}
						title={t("ledger.emptyTitle")}
						hint={t("ledger.emptyHint")}
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>{t("shared.time")}</Th>
								<Th>{t("shared.account")}</Th>
								<Th>{t("ledger.columnDirection")}</Th>
								<Th className="text-right">{t("shared.amount")}</Th>
								<Th className="text-right">{t("ledger.columnBalanceAfter")}</Th>
								<Th>{t("ledger.columnKind")}</Th>
								<Th>{t("shared.item")}</Th>
								<Th>{t("ledger.columnRef")}</Th>
								<Th>{t("shared.note")}</Th>
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
													{t("ledger.creator", { name: row.creator })}
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
