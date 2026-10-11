// 计费管理 · 价格版本：列表（含未生效）+ 新建 + 编辑 + 调价 + 停用 + 删除。
//
// 价格表上没有 enabled 开关，生不生效只由 effective_from / effective_to 决定（§3.2），
// 所以管理动作都绕着「版本」转：
//   - 还没生效的版本要改 → 直接编辑（它从没匹配过用量，改单价 / 生效时间都安全）；
//   - 还没生效的版本要取消 → 删除（也没被任何流水引用）；
//   - 已经生效的版本要停用 → 把 effective_to 收到当前时刻（不删行，历史账单还指着它）；
//   - 已经生效的版本要调价 → 「调价」= 以它为新版模板 + 旧版同刻停用，一步完成。
//     不原地改价：按事件发生时刻匹配价格的机制下，原地改会改写整个生效窗口。

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Ban, Pencil, Plus, RefreshCw, Tag, TrendingUp, Trash2 } from "lucide-react";
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
	scopeTitle,
	subjectTypeLabel,
} from "@/lib/billing";
import {
	EMPTY_RESOURCE_LISTS,
	loadBillingResourceLists,
	resourceNameIndex,
	type BillingResourceLists,
} from "@/lib/billingResources";
import { PriceDialog } from "./PriceDialog";
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

/** 打开哪个对话框：新建，或者对某版价格编辑 / 调价。 */
type PriceDialogState =
	| { mode: "create" }
	| { mode: "edit" | "adjust"; price: BillingPrice };

export default function PricesTab() {
	const { t } = useTranslation(["billingAdmin", "common"]);
	const [prices, setPrices] = useState<BillingPrice[]>([]);
	const [items, setItems] = useState<BillingItem[]>([]);
	const [accounts, setAccounts] = useState<BillingAccount[]>([]);
	const [resources, setResources] =
		useState<BillingResourceLists>(EMPTY_RESOURCE_LISTS);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);
	const [busyId, setBusyId] = useState<string | null>(null);
	const [dialog, setDialog] = useState<PriceDialogState | null>(null);

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
						text: billingErrorMessage(err, t("prices.loadItemsAccountsFailed")),
					});
				}
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey, t]);

	// 资源名称只影响展示，不阻断页面：三个列表各自容错，失败的那类在文案里说明。
	useEffect(() => {
		let cancelled = false;
		loadBillingResourceLists().then((lists) => {
			if (!cancelled) setResources(lists);
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
					text: billingErrorMessage(err, t("prices.loadFailed")),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [itemCode, resourceType, scope, accountId, activeOnly, page, reloadKey, t]);

	const names = useMemo(() => resourceNameIndex(resources), [resources]);

	/** 对话框成功收工：关掉、报信、重拉列表（价格行变了，统计也要重算）。 */
	const finishDialog = (message: string, kind: "ok" | "error" = "ok") => {
		setDialog(null);
		setBanner({ kind, text: message });
		setReloadKey((key) => key + 1);
	};

	const deactivate = async (price: BillingPrice) => {
		const ok = window.confirm(
			t("prices.deactivateConfirm", {
				item: itemLabel(price.item_code),
				scope: scopeLabel(price, names),
			}),
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
				text: t("prices.deactivateSuccess", {
					item: itemLabel(price.item_code),
				}),
			});
			setReloadKey((key) => key + 1);
		} catch (err) {
			setBanner({
				kind: "error",
				text: billingErrorMessage(err, t("shared.deactivateFailed")),
			});
		} finally {
			setBusyId(null);
		}
	};

	const remove = async (price: BillingPrice) => {
		const ok = window.confirm(
			t("prices.deleteConfirm", {
				scope: scopeLabel(price, names),
				date: formatDate(price.effective_from),
			}),
		);
		if (!ok) return;
		setBusyId(price.id);
		setBanner(null);
		try {
			await billingAdminApi.deletePrice(price.id);
			setBanner({
				kind: "ok",
				text: t("prices.deleteSuccess", {
					item: itemLabel(price.item_code),
				}),
			});
			setReloadKey((key) => key + 1);
		} catch (err) {
			setBanner({
				kind: "error",
				text: billingErrorMessage(err, t("common:error.deleteFailed")),
			});
		} finally {
			setBusyId(null);
		}
	};

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={Tag}
					title={t("billing:disabledTitle")}
					hint={t("prices.disabledHint")}
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
					placeholder={t("shared.allItems")}
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
					placeholder={t("prices.resourceTypePlaceholder")}
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
					placeholder={t("prices.scopePlaceholder")}
					options={[
						{ value: "platform", label: t("prices.scopePlatform") },
						{ value: "account", label: t("billing:scope.account") },
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
						placeholder={t("prices.accountPlaceholder")}
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
						aria-label={t("prices.activeOnly")}
					/>
					<span className="text-xs text-zinc-400">{t("prices.activeOnly")}</span>
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
						{t("common:action.refresh")}
					</button>
					<Button
						onClick={() => setDialog({ mode: "create" })}
						disabled={items.length === 0}
						className="h-7 px-2.5 text-xs bg-violet-600 hover:bg-violet-500 text-white gap-1"
					>
						<Plus className="w-3.5 h-3.5" strokeWidth={1.5} />
						{t("prices.newPrice")}
					</Button>
				</div>
			</FilterBar>

			<Hint tone="zinc" icon={Tag}>
				{t("prices.markupHint")}
			</Hint>

			{resources.missing.length > 0 && (
				<Hint tone="zinc" icon={Tag}>
					{t("prices.missingResources", {
						types: resources.missing.map(resourceTypeLabel).join(" / "),
					})}
				</Hint>
			)}

			<Panel
				title={t("prices.panelTitle")}
				description={t("prices.panelDescription")}
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : visible.length === 0 ? (
					<EmptyState
						icon={Tag}
						title={t("prices.emptyTitle")}
						hint={t("prices.emptyHint")}
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>{t("shared.item")}</Th>
								<Th>{t("prices.columnScope")}</Th>
								<Th className="text-right">{t("prices.columnUnitPrice")}</Th>
								<Th>{t("prices.columnRounding")}</Th>
								<Th className="text-right">{t("prices.columnMinCharge")}</Th>
								<Th>{t("prices.columnEffectiveRange")}</Th>
								<Th>{t("common:field.status")}</Th>
								<Th className="text-right">{t("common:field.actions")}</Th>
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
												tone={price.account_id ? "violet" : "zinc"}
												title={scopeTitle(price, names)}
											>
												{scopeLabel(price, names)}
											</Pill>
											{price.tiers && price.tiers.length > 0 && (
												<span className="text-[11px] text-zinc-500">
													{t("prices.tiersSummary", {
														n: price.tiers.length,
													})}
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
											: t("shared.forever")}
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
													onClick={() =>
														setDialog({ mode: "adjust", price })
													}
													disabled={busy}
													className="inline-flex items-center gap-1 h-7 px-2 text-[11px] rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
												>
													<TrendingUp
														className="w-3 h-3"
														strokeWidth={1.5}
													/>
													{t("prices.adjust")}
												</button>
											)}
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
													{t("prices.deactivate")}
												</button>
											)}
											{pending && (
												<button
													onClick={() =>
														setDialog({ mode: "edit", price })
													}
													disabled={busy}
													className="inline-flex items-center gap-1 h-7 px-2 text-[11px] rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
												>
													<Pencil
														className="w-3 h-3"
														strokeWidth={1.5}
													/>
													{t("common:action.edit")}
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
													{t("common:action.delete")}
												</button>
											)}
											{!effective && !pending && (
												<span className="text-[11px] text-zinc-600">
													{t("prices.retiredHint")}
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

			{dialog?.mode === "create" && (
				<PriceDialog
					mode="create"
					items={items}
					resources={resources}
					onClose={() => setDialog(null)}
					onDone={finishDialog}
				/>
			)}
			{dialog && dialog.mode !== "create" && (
				<PriceDialog
					mode={dialog.mode}
					price={dialog.price}
					items={items}
					resources={resources}
					onClose={() => setDialog(null)}
					onDone={finishDialog}
				/>
			)}
		</div>
	);
}
