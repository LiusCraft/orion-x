// 计费管理 · 计费项：目录表 + 启用开关。
//
// 计费项目录来自代码里的 seed（internal/billing/item.go），管理端只能改 enabled：
// 新增/删除计费项是改代码重启，不是改数据。启停开关的真实用途是卡住互斥口径
// （tts:characters 与 tts:audio:seconds），避免同一份用量被计两次（§3.1 / §12）。

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertCircle, ListFilter, RefreshCw } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { billingAdminApi, type BillingItem } from "@/lib/api";
import {
	chargeModeLabel,
	conflictingItemHints,
	exclusiveHintsFor,
	itemLabel,
	isBillingDisabled,
	billingErrorMessage,
	meterSourceLabel,
	unitLabel,
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
} from "../shared";

export default function ItemsTab() {
	const { t } = useTranslation(["billingAdmin", "common"]);
	const [items, setItems] = useState<BillingItem[]>([]);
	const [loading, setLoading] = useState(true);
	const [savingCode, setSavingCode] = useState<string | null>(null);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);
	const [reloadKey, setReloadKey] = useState(0);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingAdminApi
			.items()
			.then(({ data }) => {
				if (cancelled) return;
				// 目录按计量点分组展示更贴近阅读顺序
				const order = [
					"llm",
					"tts",
					"asr",
					"voice:clone",
					"session",
					"mcp",
					"kb",
				];
				const sorted = [...data.items].sort((a, b) => {
					const byMeter =
						order.indexOf(a.meter_source) - order.indexOf(b.meter_source);
					return byMeter !== 0 ? byMeter : a.code.localeCompare(b.code);
				});
				setItems(sorted);
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
					text: billingErrorMessage(err, t("items.loadFailed")),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey, t]);

	const toggle = async (item: BillingItem, enabled: boolean) => {
		setSavingCode(item.code);
		setBanner(null);
		try {
			await billingAdminApi.setItem(item.code, enabled);
			setBanner({
				kind: "ok",
				text: t(enabled ? "items.toggleEnabled" : "items.toggleDisabled", {
					item: itemLabel(item.code),
					code: item.code,
				}),
			});
			setReloadKey((key) => key + 1);
		} catch (err) {
			setBanner({
				kind: "error",
				text: billingErrorMessage(err, t("items.updateFailed")),
			});
		} finally {
			setSavingCode(null);
		}
	};

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={ListFilter}
					title={t("billing:disabledTitle")}
					hint={t("items.disabledHint")}
				/>
			</Panel>
		);
	}

	const conflicts = conflictingItemHints(
		items.filter((item) => item.enabled).map((item) => item.code),
	);

	return (
		<div className="space-y-4">
			<Banner message={banner} onClose={() => setBanner(null)} />

			{conflicts.map((conflict) => (
				<Hint key={conflict.codes.join(":")} icon={AlertCircle}>
					{t("items.conflict", { note: conflict.note })}
				</Hint>
			))}

			<Panel
				title={t("items.panelTitle")}
				description={t("items.panelDescription")}
				actions={
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
				}
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : items.length === 0 ? (
					<EmptyState
						icon={ListFilter}
						title={t("items.emptyTitle")}
						hint={t("items.emptyHint")}
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>{t("shared.item")}</Th>
								<Th>{t("items.columnMeterSource")}</Th>
								<Th>{t("items.columnChargeMode")}</Th>
								<Th>{t("items.columnUnit")}</Th>
								<Th>{t("items.columnHints")}</Th>
								<Th>{t("items.columnSource")}</Th>
								<Th className="text-right">{t("items.columnEnabled")}</Th>
							</>
						}
					>
						{items.map((item, index) => {
							const last = index === items.length - 1;
							const hints = exclusiveHintsFor(item.code);
							const saving = savingCode === item.code;
							return (
								<Tr key={item.code} last={last}>
									<Td>
										<div className="flex flex-col gap-0.5">
											<span className="text-zinc-200">
												{item.name || itemLabel(item.code)}
											</span>
											<span className="text-[11px] text-zinc-500 font-mono">
												{item.code}
											</span>
										</div>
									</Td>
									<Td>
										<Pill tone="sky">
											{meterSourceLabel(item.meter_source)}
										</Pill>
									</Td>
									<Td className="text-zinc-400 text-xs">
										{chargeModeLabel(item.charge_mode)}
									</Td>
									<Td className="text-zinc-400 text-xs">
										{unitLabel(item.unit)}
									</Td>
									<Td className="max-w-80">
										{hints.length === 0 ? (
											<span className="text-xs text-zinc-600">—</span>
										) : (
											<div className="space-y-1">
												{hints.map((hint) => (
													<p
														key={hint.note}
														className="text-[11px] text-amber-400/90 leading-relaxed"
													>
														{hint.note}
													</p>
												))}
											</div>
										)}
									</Td>
									<Td>
										{item.is_system ? (
											<Pill tone="violet">{t("items.builtin")}</Pill>
										) : (
											<Pill>{t("items.custom")}</Pill>
										)}
									</Td>
									<Td>
										<div className="flex items-center justify-end gap-2">
											{saving && (
												<span className="text-[11px] text-zinc-500">
													{t("items.saving")}
												</span>
											)}
											<Switch
												checked={item.enabled}
												disabled={saving}
												onCheckedChange={(checked: boolean) =>
													toggle(item, checked)
												}
												aria-label={t("items.enableAria", {
													code: item.code,
												})}
											/>
										</div>
									</Td>
								</Tr>
							);
						})}
					</TableShell>
				)}
			</Panel>
		</div>
	);
}
