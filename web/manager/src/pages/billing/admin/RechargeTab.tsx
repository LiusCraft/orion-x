// 计费管理 · 充值订单：过滤 + 分页 + 行内退款。
//
// 这里看到的是「钱进来之前」的单据（billing_payment_orders），和流水那一栏是两回事：
// 订单记的是网关那边的事实（收款、状态），流水记的是我们这边的账（余额怎么变的）。
// 一笔订单的 credited 与它的 recharge 流水是同时出现的，对不上就是 bug。
//
// 退款是真把钱退给付款人，所以只有 admin 能看到这个 tab（路由层已经拦了）。

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { CreditCard, RefreshCw, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SimpleSelect } from "@/components/ui/select";
import { billingAdminApi, type BillingPaymentOrder } from "@/lib/api";
import {
	formatMicro,
	formatTime,
	isBillingDisabled,
	paymentChannelLabel,
	paymentStatusLabel,
	paymentStatusTone,
	userFacingError,
} from "@/lib/billing";
import { RechargeRefundDialog } from "./RechargeRefundDialog";
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

const STATUS_OPTIONS = [
	{ value: "order:pending", labelKey: "billing:paymentStatus.pending" },
	{ value: "order:paid", labelKey: "recharge.statusPaid" },
	{ value: "order:credited", labelKey: "billing:paymentStatus.credited" },
	{ value: "order:closed", labelKey: "billing:paymentStatus.closed" },
	{ value: "order:refunded", labelKey: "billing:paymentStatus.refunded" },
];

const CHANNEL_OPTIONS = [
	{ value: "epay:alipay", labelKey: "billing:paymentChannel.alipay" },
	{ value: "epay:wxpay", labelKey: "billing:paymentChannel.wxpay" },
	{ value: "epay:qqpay", labelKey: "billing:paymentChannel.qqpay" },
];

export default function RechargeTab() {
	const { t } = useTranslation(["billingAdmin", "common"]);
	const [orders, setOrders] = useState<BillingPaymentOrder[]>([]);
	const [total, setTotal] = useState(0);
	const [page, setPage] = useState(1);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);

	const [status, setStatus] = useState("");
	const [channel, setChannel] = useState("");
	const [subjectID, setSubjectID] = useState("");
	const [keyword, setKeyword] = useState("");
	const [query, setQuery] = useState("");
	const [subjectQuery, setSubjectQuery] = useState("");
	const [reloadKey, setReloadKey] = useState(0);

	const [target, setTarget] = useState<BillingPaymentOrder | null>(null);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		billingAdminApi
			.rechargeOrders({
				page,
				page_size: PAGE_SIZE,
				...(status ? { status } : {}),
				...(channel ? { channel } : {}),
				...(subjectQuery ? { subject_id: subjectQuery } : {}),
				...(query ? { keyword: query } : {}),
			})
			.then(({ data }) => {
				if (cancelled) return;
				setOrders(data.items);
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
					text: userFacingError(err, t("recharge.loadFailed")),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [page, status, channel, subjectQuery, query, reloadKey, t]);

	if (disabled) {
		return (
			<Panel bodyClassName="p-0">
				<EmptyState
					icon={CreditCard}
					title={t("recharge.disabledTitle")}
					hint={t("recharge.disabledHint")}
				/>
			</Panel>
		);
	}

	return (
		<div className="space-y-4">
			<Banner message={banner} onClose={() => setBanner(null)} />

			<FilterBar>
				<SimpleSelect
					value={status}
					onValueChange={(value) => {
						setStatus(value);
						setPage(1);
					}}
					className="w-32"
					size="sm"
					placeholder={t("shared.allStatuses")}
					options={STATUS_OPTIONS.map(({ value, labelKey }) => ({
						value,
						label: t(labelKey),
					}))}
				/>
				<SimpleSelect
					value={channel}
					onValueChange={(value) => {
						setChannel(value);
						setPage(1);
					}}
					className="w-32"
					size="sm"
					placeholder={t("recharge.channelPlaceholder")}
					options={CHANNEL_OPTIONS.map(({ value, labelKey }) => ({
						value,
						label: t(labelKey),
					}))}
				/>
				<Input
					value={subjectID}
					onChange={(e) => setSubjectID(e.target.value)}
					onKeyDown={(e) => {
						if (e.key === "Enter") {
							setSubjectQuery(subjectID.trim());
							setPage(1);
						}
					}}
					placeholder={t("recharge.subjectPlaceholder")}
					className="h-7 w-40 text-xs font-mono"
				/>
				<div className="flex items-center gap-2 flex-1 min-w-56">
					<Input
						value={keyword}
						onChange={(e) => setKeyword(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") {
								setQuery(keyword.trim());
								setSubjectQuery(subjectID.trim());
								setPage(1);
							}
						}}
						placeholder={t("recharge.keywordPlaceholder")}
						className="h-7 text-xs font-mono"
					/>
					<Button
						variant="outline"
						onClick={() => {
							setQuery(keyword.trim());
							setSubjectQuery(subjectID.trim());
							setPage(1);
						}}
						className="h-7 px-2.5 text-xs border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white gap-1"
					>
						<Search className="w-3.5 h-3.5" strokeWidth={1.5} />
						{t("common:action.search")}
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
					{t("common:action.refresh")}
				</button>
			</FilterBar>

			<Panel
				title={t("recharge.panelTitle")}
				description={t("recharge.panelDescription")}
				bodyClassName="p-0"
			>
				{loading ? (
					<LoadingBlock />
				) : orders.length === 0 ? (
					<EmptyState
						icon={CreditCard}
						title={t("recharge.emptyTitle")}
						hint={t("recharge.emptyHint")}
					/>
				) : (
					<TableShell
						head={
							<>
								<Th>{t("recharge.columnOrderNo")}</Th>
								<Th>{t("recharge.columnSubject")}</Th>
								<Th className="text-right">{t("shared.amount")}</Th>
								<Th>{t("recharge.columnChannel")}</Th>
								<Th>{t("common:field.status")}</Th>
								<Th>{t("common:field.createdAt")}</Th>
								<Th>{t("recharge.columnCreditedAt")}</Th>
								<Th className="text-right">{t("common:field.actions")}</Th>
							</>
						}
					>
						{orders.map((order, index) => {
							const refundable = order.status === "order:credited";
							const refunded = order.status === "order:refunded";
							return (
								<Tr key={order.out_trade_no} last={index === orders.length - 1}>
									<Td className="font-mono text-xs text-zinc-400 break-all max-w-44">
										{order.out_trade_no}
										{order.gateway_trade_no && (
											<p
												className="text-[10px] text-zinc-600 truncate"
												title={order.gateway_trade_no}
											>
												{order.gateway_trade_no}
											</p>
										)}
									</Td>
									<Td className="font-mono text-xs text-zinc-400 break-all max-w-40">
										{order.subject_id || "—"}
									</Td>
									<Td className="text-right font-mono whitespace-nowrap">
										<span className={refunded ? "text-zinc-500 line-through" : ""}>
											{formatMicro(order.amount_micro, order.currency)}
										</span>
									</Td>
									<Td className="text-xs text-zinc-400 whitespace-nowrap">
										{paymentChannelLabel(order.channel)}
									</Td>
									<Td>
										<Pill tone={paymentStatusTone(order.status)}>
											{paymentStatusLabel(order.status)}
										</Pill>
									</Td>
									<Td className="text-xs text-zinc-500 font-mono whitespace-nowrap">
										{formatTime(order.created_at)}
									</Td>
									<Td className="text-xs text-zinc-500 font-mono whitespace-nowrap">
										{formatTime(order.credited_at) || "—"}
									</Td>
									<Td>
										<div className="flex items-center justify-end">
											<button
												onClick={() => setTarget(order)}
												disabled={!refundable && !refunded}
												title={
													refundable
														? t("recharge.refundTitle")
														: refunded
															? t("recharge.refundView")
															: t("recharge.refundUnavailable")
												}
												className="inline-flex items-center gap-1 h-7 px-2 text-[11px] rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
											>
												{refunded
													? t("recharge.details")
													: t("recharge.refund")}
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
				<RechargeRefundDialog
					key={target.out_trade_no}
					order={target}
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
