// 计费管理 · 计费项：目录表 + 启用开关。
//
// 计费项目录来自代码里的 seed（internal/billing/item.go），管理端只能改 enabled：
// 新增/删除计费项是改代码重启，不是改数据。启停开关的真实用途是卡住互斥口径
// （tts:characters 与 tts:audio:seconds），避免同一份用量被计两次（§3.1 / §12）。

import { useEffect, useState } from "react";
import { AlertCircle, ListFilter, RefreshCw } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { billingAdminApi, type BillingItem } from "@/lib/api";
import {
	BILLING_DISABLED_TITLE,
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
					text: billingErrorMessage(err, "加载计费项目录失败"),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	const toggle = async (item: BillingItem, enabled: boolean) => {
		setSavingCode(item.code);
		setBanner(null);
		try {
			await billingAdminApi.setItem(item.code, enabled);
			setBanner({
				kind: "ok",
				text: `已${enabled ? "启用" : "停用"}「${itemLabel(item.code)}」（${item.code}）`,
			});
			setReloadKey((key) => key + 1);
		} catch (err) {
			setBanner({
				kind: "error",
				text: billingErrorMessage(err, "更新计费项失败"),
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
					title={BILLING_DISABLED_TITLE}
					hint="服务端没有开启计费模块（billing.enabled），计费项目录不可用。"
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
					互斥口径同时启用：{conflict.note}
				</Hint>
			))}

			<Panel
				title="计费项目录"
				description="内置项由服务端 seed 同步，管理端只能启停；停用一个计费项后，它的事件仍会入库但不会计费"
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
						刷新
					</button>
				}
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : items.length === 0 ? (
					<EmptyState
						icon={ListFilter}
						title="计费项目录为空"
						hint="服务端启动时会按 internal/billing 里的 seed 补齐目录，空表通常意味着 seed 没跑。"
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>计费项</Th>
								<Th>计量点</Th>
								<Th>计费模式</Th>
								<Th>单位</Th>
								<Th>口径提示</Th>
								<Th>来源</Th>
								<Th className="text-right">启用</Th>
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
											<Pill tone="violet">内置</Pill>
										) : (
											<Pill>自定义</Pill>
										)}
									</Td>
									<Td>
										<div className="flex items-center justify-end gap-2">
											{saving && (
												<span className="text-[11px] text-zinc-500">
													保存中
												</span>
											)}
											<Switch
												checked={item.enabled}
												disabled={saving}
												onCheckedChange={(checked: boolean) =>
													toggle(item, checked)
												}
												aria-label={`启用 ${item.code}`}
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
