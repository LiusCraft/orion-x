// 计费管理 · 账户：过滤 + 分页 + 行内「调整」「赠款」。
//
// 账户 ID 是内部主键（acct_ 前缀），不是 user id：计费主体落在账户这一行上（§12）。

import { useEffect, useState } from "react";
import { HandCoins, RefreshCw, Search, SlidersHorizontal, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SimpleSelect } from "@/components/ui/select";
import { billingAdminApi, type BillingAccount } from "@/lib/api";
import {
	accountStatusLabel,
	BILLING_DISABLED_TITLE,
	billingErrorMessage,
	formatMicro,
	formatTime,
	isBillingDisabled,
	subjectTypeLabel,
} from "@/lib/billing";
import { AccountAdjustDialog } from "./AccountAdjustDialog";
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

const STATUS_TONES: Record<string, "emerald" | "red" | "zinc"> = {
	active: "emerald",
	suspended: "red",
	closed: "zinc",
};

export default function AccountsTab() {
	const [accounts, setAccounts] = useState<BillingAccount[]>([]);
	const [total, setTotal] = useState(0);
	const [page, setPage] = useState(1);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);

	const [subjectType, setSubjectType] = useState("");
	const [status, setStatus] = useState("");
	const [keyword, setKeyword] = useState("");
	const [query, setQuery] = useState("");
	const [reloadKey, setReloadKey] = useState(0);

	const [target, setTarget] = useState<{ account: BillingAccount; grant: boolean } | null>(
		null,
	);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingAdminApi
			.accounts({
				page,
				page_size: PAGE_SIZE,
				...(subjectType ? { subject_type: subjectType as "user" | "org" } : {}),
				...(status ? { status: status as "active" | "suspended" | "closed" } : {}),
				...(query ? { keyword: query } : {}),
			})
			.then(({ data }) => {
				if (cancelled) return;
				setAccounts(data.items);
				setTotal(data.total);
				setDisabled(false);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setDisabled(true);
					return;
				}
				setBanner({ kind: "error", text: billingErrorMessage(err, "加载账户失败") });
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [page, subjectType, status, query, reloadKey]);

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={SlidersHorizontal}
					title={BILLING_DISABLED_TITLE}
					hint="服务端没有开启计费模块（billing.enabled），账户、流水与定价都不可用。"
				/>
			</Panel>
		);
	}

	return (
		<div className="space-y-4">
			<Banner message={banner} onClose={() => setBanner(null)} />

			<FilterBar>
				<SimpleSelect
					value={subjectType}
					onValueChange={(value) => {
						setSubjectType(value);
						setPage(1);
					}}
					className="w-36"
					size="sm"
					placeholder="全部主体"
					options={[
						{ value: "user", label: "用户" },
						{ value: "org", label: "组织" },
					]}
				/>
				<SimpleSelect
					value={status}
					onValueChange={(value) => {
						setStatus(value);
						setPage(1);
					}}
					className="w-36"
					size="sm"
					placeholder="全部状态"
					options={[
						{ value: "active", label: "正常" },
						{ value: "suspended", label: "已暂停" },
						{ value: "closed", label: "已关闭" },
					]}
				/>
				<div className="flex items-center gap-2 flex-1 min-w-56">
					<Input
						value={keyword}
						onChange={(e) => setKeyword(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") {
								setQuery(keyword.trim());
								setPage(1);
							}
						}}
						placeholder="账户 ID / 主体 ID"
						className="h-7 text-xs font-mono"
					/>
					<Button
						variant="outline"
						onClick={() => {
							setQuery(keyword.trim());
							setPage(1);
						}}
						className="h-7 px-2.5 text-xs border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1"
					>
						<Search className="w-3.5 h-3.5" strokeWidth={1.5} />
						搜索
					</Button>
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
				title="账户列表"
				description="余额可以为负（后付欠款）；冻结 frozen 是会话预授权占用的金额"
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : accounts.length === 0 ? (
					<EmptyState
						icon={Users}
						title="没有匹配的账户"
						hint="账户在用户第一次产生用量时自动创建，也可以换个过滤条件再试。"
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>账户</Th>
								<Th>主体</Th>
								<Th className="text-right">可用余额</Th>
								<Th className="text-right">预冻结</Th>
								<Th className="text-right">信用额度</Th>
								<Th>状态</Th>
								<Th>创建时间</Th>
								<Th className="text-right">操作</Th>
							</>
						}
					>
						{accounts.map((account, index) => {
							const last = index === accounts.length - 1;
							const overdrawn =
								account.balance_micro < -account.credit_limit_micro;
							return (
								<Tr key={account.id} last={last}>
									<Td className="font-mono text-xs text-zinc-300 break-all max-w-52">
										{account.id}
									</Td>
									<Td>
										<div className="flex items-center gap-1.5">
											<Pill tone="violet">
												{subjectTypeLabel(account.subject_type)}
											</Pill>
											<span className="font-mono text-xs text-zinc-400 break-all">
												{account.subject_id}
											</span>
										</div>
									</Td>
									<Td className="text-right font-mono">
										<span
											className={
												account.balance_micro < 0
													? "text-red-400"
													: "text-amber-400"
											}
										>
											{formatMicro(
												account.balance_micro,
												account.currency,
											)}
										</span>
										{account.balance_micro < 0 && (
											<p className="text-[10px] text-red-400/80 mt-0.5">
												{overdrawn ? "已超信用额度" : "欠费"}
											</p>
										)}
									</Td>
									<Td className="text-right font-mono text-zinc-400">
										{formatMicro(account.frozen_micro, account.currency)}
									</Td>
									<Td className="text-right font-mono text-zinc-400">
										{formatMicro(
											account.credit_limit_micro,
											account.currency,
										)}
									</Td>
									<Td>
										<Pill tone={STATUS_TONES[account.status] ?? "zinc"}>
											{accountStatusLabel(account.status)}
										</Pill>
									</Td>
									<Td className="text-xs text-zinc-500 font-mono whitespace-nowrap">
										{formatTime(account.created_at)}
									</Td>
									<Td>
										<div className="flex items-center justify-end gap-1.5">
											<button
												onClick={() =>
													setTarget({ account, grant: false })
												}
												className="inline-flex items-center gap-1 h-7 px-2 text-[11px] rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 transition-colors cursor-pointer"
											>
												<SlidersHorizontal
													className="w-3 h-3"
													strokeWidth={1.5}
												/>
												调整
											</button>
											<button
												onClick={() =>
													setTarget({ account, grant: true })
												}
												className="inline-flex items-center gap-1 h-7 px-2 text-[11px] rounded bg-violet-600/15 border border-violet-500/30 text-violet-300 hover:bg-violet-600/25 transition-colors cursor-pointer"
											>
												<HandCoins
													className="w-3 h-3"
													strokeWidth={1.5}
												/>
												赠款
											</button>
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

			{target && (
				<AccountAdjustDialog
					key={`${target.account.id}:${target.grant ? "grant" : "adjust"}`}
					account={target.account}
					grant={target.grant}
					onClose={() => setTarget(null)}
					onDone={(message) => {
						setTarget(null);
						setBanner({ kind: "ok", text: message });
						setReloadKey((key) => key + 1);
					}}
				/>
			)}
		</div>
	);
}
