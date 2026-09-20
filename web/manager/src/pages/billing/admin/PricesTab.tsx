// 计费管理 · 价格版本：列表（含未生效）+ 新建 + 停用 + 删除。
//
// 价格表上没有 enabled 开关，生不生效只由 effective_from / effective_to 决定（§3.2），
// 所以管理动作只有两个：
//   - 已经生效的版本要停用 → 把 effective_to 收到当前时刻（不删行，历史账单还指着它）；
//   - 还没生效的版本要取消 → 直接删（它从没匹配过用量，也没被任何流水引用）。

import { useEffect, useState } from "react";
import { Ban, Plus, RefreshCw, Tag, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { SimpleSelect } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
	billingAdminApi,
	type BillingAccount,
	type BillingItem,
	type BillingPrice,
} from "@/lib/api";
import {
	billingErrorMessage,
	formatDate,
	formatMicro,
	formatTierPrice,
	formatTierRange,
	formatUnitPrice,
	isBillingDisabled,
	isPriceEffective,
	isPricePending,
	itemLabel,
	priceStateLabel,
	resourceTypeLabel,
	roundingLabel,
	scopeLabel,
	subjectTypeLabel,
} from "@/lib/billing";
import { PriceCreateDialog } from "./PriceCreateDialog";
import {
	Banner,
	EmptyState,
	FilterBar,
	Hint,
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

export default function PricesTab() {
	const [prices, setPrices] = useState<BillingPrice[]>([]);
	const [items, setItems] = useState<BillingItem[]>([]);
	const [accounts, setAccounts] = useState<BillingAccount[]>([]);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);
	const [busyId, setBusyId] = useState<string | null>(null);
	const [creating, setCreating] = useState(false);

	const [itemCode, setItemCode] = useState("");
	const [resourceType, setResourceType] = useState("");
	const [scope, setScope] = useState<"" | "platform" | "account">("");
	const [accountId, setAccountId] = useState("");
	const [activeOnly, setActiveOnly] = useState(false);
	const [page, setPage] = useState(1);
	const [reloadKey, setReloadKey] = useState(0);

	// 计费项（新建价格用）与账户（协议价过滤用）各拉一次。
	useEffect(() => {
		let cancelled = false;
		Promise.all([
			billingAdminApi.items(),
			billingAdminApi.accounts({ page: 1, page_size: 100 }),
		])
			.then(([itemsRes, accountsRes]) => {
				if (cancelled) return;
				setItems(itemsRes.data.items);
				setAccounts(accountsRes.data.items);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (!isBillingDisabled(err)) {
					setBanner({
						kind: "error",
						text: billingErrorMessage(err, "加载计费项 / 账户失败"),
					});
				}
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingAdminApi
			.prices({
				page,
				page_size: PAGE_SIZE,
				...(itemCode ? { item_code: itemCode } : {}),
				...(resourceType
					? { resource_type: resourceType as BillingPrice["resource_type"] }
					: {}),
				...(scope === "account" && accountId ? { account_id: accountId } : {}),
				...(activeOnly ? { active_only: "1" } : {}),
			})
			.then(({ data }) => {
				if (cancelled) return;
				setPrices(data.items);
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
					text: billingErrorMessage(err, "加载价格版本失败"),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [itemCode, resourceType, scope, accountId, activeOnly, page, reloadKey]);

	const deactivate = async (price: BillingPrice) => {
		const ok = window.confirm(
			`停用「${itemLabel(price.item_code)}」的这版价格（${scopeLabel(price)}）？\n\n` +
				"停用会把 effective_to 收到当前时刻，之后不再匹配新的用量事件；\n" +
				"已经产生的历史账单不受影响——结算按事件发生时刻的价格走，历史账单永不因调价而变动。",
		);
		if (!ok) return;
		setBusyId(price.id);
		setBanner(null);
		try {
			await billingAdminApi.updatePrice(price.id, {
				effective_to: new Date().toISOString(),
			});
			setBanner({
				kind: "ok",
				text: `已停用「${itemLabel(price.item_code)}」的这版价格（历史账单不受影响）`,
			});
			setReloadKey((key) => key + 1);
		} catch (err) {
			setBanner({ kind: "error", text: billingErrorMessage(err, "停用失败") });
		} finally {
			setBusyId(null);
		}
	};

	const remove = async (price: BillingPrice) => {
		const ok = window.confirm(
			`删除这版还没生效的价格（${scopeLabel(price)}，${formatDate(price.effective_from)} 起生效）？\n\n` +
				"它从来没有匹配过任何用量事件，也没有被任何流水引用，删除后不可恢复。",
		);
		if (!ok) return;
		setBusyId(price.id);
		setBanner(null);
		try {
			await billingAdminApi.deletePrice(price.id);
			setBanner({
				kind: "ok",
				text: `已删除未生效的价格版本（${itemLabel(price.item_code)}）`,
			});
			setReloadKey((key) => key + 1);
		} catch (err) {
			setBanner({ kind: "error", text: billingErrorMessage(err, "删除失败") });
		} finally {
			setBusyId(null);
		}
	};

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={Tag}
					title="计费未启用"
					hint="服务端没有开启计费模块（billing.enabled），价格版本不可用。"
				/>
			</Panel>
		);
	}

	// 「平台标准价」在接口里没有对应的过滤参数（只暴露 account_id），
	// 价格表量级是几十条版本，所以这一项按当前页过滤，标签上直说。
	const visible = prices.filter((price) => {
		if (scope === "platform") return price.account_id === "";
		if (scope === "account" && !accountId) return price.account_id !== "";
		return true;
	});

	return (
		<div className="space-y-4">
			<Banner message={banner} onClose={() => setBanner(null)} />

			<FilterBar>
				<SimpleSelect
					value={itemCode}
					onValueChange={(value) => {
						setItemCode(value);
						setPage(1);
					}}
					className="w-72"
					size="sm"
					placeholder="全部计费项"
					options={items.map((item) => ({
						value: item.code,
						label: `${item.name}（${item.code}）`,
					}))}
				/>
				<SimpleSelect
					value={resourceType}
					onValueChange={(value) => {
						setResourceType(value);
						setPage(1);
					}}
					className="w-40"
					size="sm"
					placeholder="全部粒度"
					options={(["item", "provider", "model", "voice"] as const).map(
						(value) => ({ value, label: resourceTypeLabel(value) }),
					)}
				/>
				<SimpleSelect
					value={scope}
					onValueChange={(value) => {
						setScope(value as "" | "platform" | "account");
						setPage(1);
					}}
					className="w-52"
					size="sm"
					placeholder="全部 scope"
					options={[
						{ value: "platform", label: "平台标准价（本页过滤）" },
						{ value: "account", label: "账户协议价" },
					]}
				/>
				{scope === "account" && (
					<SimpleSelect
						value={accountId}
						onValueChange={(value) => {
							setAccountId(value);
							setPage(1);
						}}
						className="w-64"
						size="sm"
						placeholder="全部账户（本页过滤）"
						options={accounts.map((account) => ({
							value: account.id,
							label: `${subjectTypeLabel(account.subject_type)} · ${account.id}`,
						}))}
					/>
				)}
				<div className="flex items-center gap-2">
					<Switch
						checked={activeOnly}
						onCheckedChange={(checked: boolean) => {
							setActiveOnly(checked);
							setPage(1);
						}}
						size="sm"
						aria-label="只看生效中"
					/>
					<span className="text-xs text-zinc-400">只看生效中</span>
				</div>
				<div className="flex items-center gap-2 ml-auto">
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
					<Button
						onClick={() => setCreating(true)}
						disabled={items.length === 0}
						className="h-7 px-2.5 text-xs bg-violet-600 hover:bg-violet-500 text-white gap-1"
					>
						<Plus className="w-3.5 h-3.5" strokeWidth={1.5} />
						新建价格
					</Button>
				</div>
			</FilterBar>

			<Hint tone="zinc" icon={Tag}>
				改价 = 新增一版 effective_from 更晚的价格，不覆盖历史。同一 scope
				（计费项 + 账户 + 资源粒度）同一时刻只能有一版价格，命中优先级是
				账户协议价 → 音色 → 模型 → 厂商 → 计费项兜底。
			</Hint>

			<Panel
				title="价格版本"
				description="含未生效与已停用的版本；状态的唯一来源是 effective_from / effective_to"
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : visible.length === 0 ? (
					<EmptyState
						icon={Tag}
						title="没有匹配的价格版本"
						hint="价格表为空时每个会话都会被 price_missing 拒掉，上线前记得把真实价格录一遍。"
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>计费项</Th>
								<Th>适用范围</Th>
								<Th className="text-right">单价</Th>
								<Th>舍入</Th>
								<Th className="text-right">起步价</Th>
								<Th>生效区间</Th>
								<Th>状态</Th>
								<Th className="text-right">操作</Th>
							</>
						}
					>
						{visible.map((price, index) => {
							const last = index === visible.length - 1;
							const busy = busyId === price.id;
							const effective = isPriceEffective(price);
							const pending = isPricePending(price);
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
										<div className="flex flex-col gap-0.5">
											<Pill
												tone={
													price.account_id ? "violet" : "zinc"
												}
											>
												{scopeLabel(price)}
											</Pill>
											{price.tiers && price.tiers.length > 0 && (
												<span className="text-[11px] text-zinc-500">
													阶梯 {price.tiers.length} 档：
													{price.tiers
														.map(
															(tier) =>
																formatTierPrice(
																	tier,
																	price,
																),
														)
														.join(" / ")}
												</span>
											)}
											{price.tiers && price.tiers.length > 0 && (
												<span className="text-[11px] text-zinc-600">
													{price.tiers
														.map(
															(tier, tierIndex) =>
																`${tierIndex + 1}. ${formatTierRange(tier.up_to)}`,
														)
														.join("；")}
												</span>
											)}
										</div>
									</Td>
									<Td className="text-right font-mono text-amber-400 whitespace-nowrap">
										{formatUnitPrice(price)}
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
									<Td>
										<Pill
											tone={
												effective
													? "emerald"
													: pending
														? "sky"
														: "zinc"
											}
										>
											{priceStateLabel(price)}
										</Pill>
									</Td>
									<Td>
										<div className="flex items-center justify-end gap-1.5">
											{effective && (
												<button
													onClick={() => deactivate(price)}
													disabled={busy}
													className="inline-flex items-center gap-1 h-7 px-2 text-[11px] rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
												>
													<Ban
														className="w-3 h-3"
														strokeWidth={1.5}
													/>
													停用
												</button>
											)}
											{pending && (
												<button
													onClick={() => remove(price)}
													disabled={busy}
													className="inline-flex items-center gap-1 h-7 px-2 text-[11px] rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:text-red-300 hover:border-red-400/40 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
												>
													<Trash2
														className="w-3 h-3"
														strokeWidth={1.5}
													/>
													删除
												</button>
											)}
											{!effective && !pending && (
												<span className="text-[11px] text-zinc-600">
													已停用，仅历史账单引用
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
				hasNext={prices.length >= PAGE_SIZE}
				loading={loading}
				onPageChange={setPage}
			/>

			{creating && (
				<PriceCreateDialog
					items={items}
					onClose={() => setCreating(false)}
					onDone={(message) => {
						setCreating(false);
						setBanner({ kind: "ok", text: message });
						setReloadKey((key) => key + 1);
					}}
				/>
			)}
		</div>
	);
}
