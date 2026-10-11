// 用户端「余额充值」。
//
// 钱走的是支付渠道（易支付），不是计费引擎：这个页面只下单、展示付款入口、轮询状态。
// 能给余额加钱的只有服务端的回调处理（回调 → 校验 → Service.Credit），所以这里
// **不做任何乐观更新**——状态一律以 GET /api/billing/recharge/:out_trade_no 为准，
// 前端看到「已到账」的那一秒，钱已经在余额里了。
//
// 两种 503 要分开说：整个计费关掉（billing.enabled: false）和只没接支付渠道
// （payment 段没配）。前者连余额都查不到，后者只是不能充值——文案混在一起，运维
// 会去翻错地方。

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
	AlertCircle,
	CheckCircle2,
	Clock,
	Copy,
	ExternalLink,
	QrCode,
	RefreshCw,
	Wallet,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SimpleSelect } from "@/components/ui/select";
import {
	billingApi,
	type BillingPaymentOrder,
	type BillingRechargeConfig,
	type BillingRechargeResult,
	type BillingSummary,
} from "@/lib/api";
import { useDocumentTitle } from "@/lib/title";
import {
	formatMicro,
	formatTime,
	isBillingDisabled,
	isPaymentCredited,
	isPaymentPending,
	paymentChannelLabel,
	paymentExpiryHint,
	paymentStatusLabel,
	paymentStatusTone,
	RECHARGE_PRESETS,
	userFacingError,
	yuanToMicro,
} from "@/lib/billing";
import {
	Banner,
	EmptyState,
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
} from "./shared";

const PAGE_SIZE = 10;
/** 轮询间隔：支付是人在扫码/输密码，3 秒足够，也不至于把 manager 打满。 */
const POLL_INTERVAL_MS = 3000;
/** 网关的 money 只有两位小数，所以金额必须是整分。 */
const MICRO_PER_CENT = 10_000;

export default function RechargePage() {
	const { t } = useTranslation(["billingPages", "common"]);
	useDocumentTitle(t("recharge.title"));

	const [summary, setSummary] = useState<BillingSummary | null>(null);
	const [summaryLoading, setSummaryLoading] = useState(true);

	const [config, setConfig] = useState<BillingRechargeConfig | null>(null);
	const [configLoading, setConfigLoading] = useState(true);
	const [channel, setChannel] = useState("");
	const [amount, setAmount] = useState("");
	const [submitting, setSubmitting] = useState(false);

	// pending 是刚下单的付款入口；activeStatus 是它的最新状态（由轮询刷新）。
	const [pending, setPending] = useState<BillingRechargeResult | null>(null);
	const [activeStatus, setActiveStatus] = useState("");
	const [qrBroken, setQrBroken] = useState(false);

	const [orders, setOrders] = useState<BillingPaymentOrder[]>([]);
	const [total, setTotal] = useState(0);
	const [page, setPage] = useState(1);
	const [listLoading, setListLoading] = useState(true);

	const [billingOff, setBillingOff] = useState(false);
	const [paymentOff, setPaymentOff] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);
	const [reloadKey, setReloadKey] = useState(0);

	// 余额：503 只可能是「计费整个关了」，它是分辨两种 503 的基准。
	useEffect(() => {
		let cancelled = false;
		setSummaryLoading(true);
		billingApi
			.summary()
			.then(({ data }) => {
				if (cancelled) return;
				setSummary(data);
				setBillingOff(false);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setBillingOff(true);
					return;
				}
				setBanner({
					kind: "error",
					text: userFacingError(err, t("recharge.loadBalanceFailed")),
				});
			})
			.finally(() => {
				if (!cancelled) setSummaryLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	// 充值额度与可选的支付方式都来自服务端：前端写死渠道清单的结果是「界面上能选、
	// 提交后被拒」。这条 503 就是「这个部署没接在线充值」。
	useEffect(() => {
		let cancelled = false;
		setConfigLoading(true);
		billingApi
			.rechargeConfig()
			.then(({ data }) => {
				if (cancelled) return;
				setConfig(data);
				setPaymentOff(false);
				setChannel((current) =>
					current && data.channels.includes(current)
						? current
						: (data.channels[0] ?? ""),
				);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setPaymentOff(true);
					return;
				}
				setBanner({
					kind: "error",
					text: userFacingError(err, t("recharge.loadConfigFailed")),
				});
			})
			.finally(() => {
				if (!cancelled) setConfigLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	// 充值记录：永远只看自己（subject 由服务端从 token 里取，前端不传）。
	useEffect(() => {
		let cancelled = false;
		setListLoading(true);
		billingApi
			.rechargeOrders({ page, page_size: PAGE_SIZE })
			.then(({ data }) => {
				if (cancelled) return;
				setOrders(data.items);
				setTotal(data.total);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isBillingDisabled(err)) {
					setPaymentOff(true);
					return;
				}
				setBanner({
					kind: "error",
					text: userFacingError(err, t("recharge.loadOrdersFailed")),
				});
			})
			.finally(() => {
				if (!cancelled) setListLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [page, reloadKey]);

	// 轮询待支付的单子。终态（到账/关闭/退款）就不再轮了：订单不会自己变回去。
	useEffect(() => {
		if (!pending || !isPaymentPending(activeStatus)) return;
		let cancelled = false;

		const timer = window.setInterval(() => {
			billingApi
				.rechargeOrder(pending.out_trade_no)
				.then(({ data }) => {
					if (cancelled) return;
					setActiveStatus(data.status);
					if (isPaymentPending(data.status)) return;
					setBanner(
						isPaymentCredited(data.status)
							? {
									kind: "ok",
									text: t("recharge.creditedBanner", {
										amount: formatMicro(
											data.amount_micro,
											data.currency,
										),
									}),
								}
							: {
									kind: "error",
									text: t("recharge.notCreditedBanner", {
										status: paymentStatusLabel(data.status),
									}),
								},
					);
					setPending(null);
					setReloadKey((key) => key + 1);
				})
				.catch(() => {
					// 轮询失败不打扰用户：网络抖一下就弹个红条，比状态晚几秒烦人得多。
				});
		}, POLL_INTERVAL_MS);

		return () => {
			cancelled = true;
			window.clearInterval(timer);
		};
	}, [pending, activeStatus]);

	const micro = yuanToMicro(amount);
	const currency = config?.currency ?? "CNY";
	const minMicro = config?.min_amount_micro ?? 0;
	const maxMicro = config?.max_amount_micro ?? Number.MAX_SAFE_INTEGER;

	// 上下限也来自服务端：写死在页面上的话，改了配置这里就会说错话。
	const amountError = (() => {
		if (amount.trim() === "") return "";
		if (micro === null) return t("recharge.amountInvalid");
		if (micro <= 0) return t("recharge.amountPositive");
		if (micro % MICRO_PER_CENT !== 0) return t("recharge.amountTwoDecimals");
		if (micro < minMicro)
			return t("recharge.amountMin", {
				amount: formatMicro(minMicro, currency),
			});
		if (micro > maxMicro)
			return t("recharge.amountMax", {
				amount: formatMicro(maxMicro, currency),
			});
		return "";
	})();
	const canSubmit =
		!submitting &&
		config !== null &&
		channel !== "" &&
		micro !== null &&
		micro > 0 &&
		amountError === "";

	const submit = async () => {
		if (!canSubmit || micro === null) return;
		setSubmitting(true);
		setBanner(null);
		try {
			const { data } = await billingApi.recharge({
				channel,
				amount_micro: micro,
			});
			setQrBroken(false);
			setPending(data);
			setActiveStatus(data.status);
			setAmount("");
			setBanner({
				kind: "ok",
				text: t("recharge.orderCreated", { order: data.out_trade_no }),
			});
			setReloadKey((key) => key + 1);
		} catch (err) {
			if (isBillingDisabled(err)) {
				setPaymentOff(true);
				return;
			}
			setBanner({
				kind: "error",
				text: userFacingError(err, t("recharge.createOrderFailed")),
			});
		} finally {
			setSubmitting(false);
		}
	};

	if (billingOff) {
		return (
			<div className="min-h-full">
				<Header balanceHint={null} />
				<div className="px-8 py-6">
					<Panel bodyClassName="p-0">
						<EmptyState
							icon={Wallet}
							title={t("recharge.balanceDisabledTitle")}
							hint={t("recharge.balanceDisabledHint")}
						/>
					</Panel>
				</div>
			</div>
		);
	}

	// billingOff 只管余额那一栏（它此时也没东西可显），currency 优先用充值配置里的。
	// 还没开户的人显示「—」而不是 ¥0.00：没开户和余额为零不是一回事。
	const balanceHint = summaryLoading
		? null
		: summary?.account
			? formatMicro(summary.balance_micro, currency)
			: "—";
	const expiryHint = pending ? paymentExpiryHint(pending.expires_at) : null;

	return (
		<div className="min-h-full">
			<Header balanceHint={balanceHint} />

			<div className="px-8 py-6 space-y-4">
				<Banner message={banner} onClose={() => setBanner(null)} />

				{paymentOff ? (
					<Panel bodyClassName="p-0">
						<EmptyState
							icon={Wallet}
							title={t("recharge.paymentDisabledTitle")}
							hint={t("recharge.paymentDisabledHint")}
						/>
					</Panel>
				) : (
					<div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
						<RechargeForm
							loading={configLoading}
							channel={channel}
							onChannel={setChannel}
							channels={config?.channels ?? []}
							amount={amount}
							onAmount={setAmount}
							amountError={amountError}
							micro={micro}
							currency={currency}
							minMicro={minMicro}
							submitting={submitting}
							canSubmit={canSubmit}
							onSubmit={submit}
						/>

						<PendingPanel
							pending={pending}
							status={activeStatus}
							expiryHint={expiryHint}
							qrBroken={qrBroken}
							onQrBroken={() => setQrBroken(true)}
							onDismiss={() => {
								setPending(null);
								setActiveStatus("");
							}}
							onBanner={setBanner}
						/>
					</div>
				)}

				<Panel
					title={t("recharge.ordersTitle")}
					description={t("recharge.ordersHint")}
					actions={
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
					}
					bodyClassName="p-0"
				>
					{listLoading ? (
						<LoadingBlock />
					) : orders.length === 0 ? (
						<EmptyState
							icon={QrCode}
							title={t("recharge.ordersEmptyTitle")}
							hint={t("recharge.ordersEmptyHint")}
						/>
					) : (
						<TableShell
							head={
								<>
									<Th>{t("recharge.column.orderNo")}</Th>
									<Th>{t("recharge.column.amount")}</Th>
									<Th>{t("recharge.column.paymentMethod")}</Th>
									<Th>{t("recharge.column.status")}</Th>
									<Th>{t("recharge.column.createdAt")}</Th>
									<Th>{t("recharge.column.creditedAt")}</Th>
								</>
							}
						>
							{orders.map((order, index) => (
								<Tr key={order.out_trade_no} last={index === orders.length - 1}>
									<Td className="font-mono text-xs text-zinc-400 break-all max-w-48">
										{order.out_trade_no}
									</Td>
									<Td className="font-mono whitespace-nowrap">
										{formatMicro(order.amount_micro, order.currency)}
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
								</Tr>
							))}
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

function Header({ balanceHint }: { balanceHint: string | null }) {
	const { t } = useTranslation(["billingPages", "common"]);
	return (
		<div className="border-b border-zinc-800/80 px-8 py-5">
			<div className="flex items-center justify-between gap-4">
				<div>
					<h1 className="text-lg font-semibold text-white flex items-center gap-2">
						<Wallet className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
						{t("recharge.title")}
					</h1>
					<p className="text-sm text-zinc-500 mt-0.5">
						{t("recharge.subtitle")}
					</p>
				</div>
				{balanceHint && (
					<div className="text-right shrink-0">
						<p className="text-[11px] text-zinc-500 uppercase tracking-wider">
							{t("recharge.currentBalance")}
						</p>
						<p className="text-xl font-semibold text-amber-400 font-mono">
							{balanceHint}
						</p>
					</div>
				)}
			</div>
		</div>
	);
}

function RechargeForm({
	loading,
	channel,
	onChannel,
	channels,
	amount,
	onAmount,
	amountError,
	micro,
	currency,
	minMicro,
	submitting,
	canSubmit,
	onSubmit,
}: {
	loading: boolean;
	channel: string;
	onChannel: (value: string) => void;
	channels: string[];
	amount: string;
	onAmount: (value: string) => void;
	amountError: string;
	micro: number | null;
	currency: string;
	minMicro: number;
	submitting: boolean;
	canSubmit: boolean;
	onSubmit: () => void;
}) {
	const { t } = useTranslation(["billingPages", "common"]);
	if (loading) {
		return (
			<Panel title={t("recharge.selectAmount")}>
				<LoadingBlock label={t("recharge.loadingConfig")} />
			</Panel>
		);
	}

	return (
		<Panel
			title={t("recharge.selectAmount")}
			description={t("recharge.selectAmountHint")}
		>
			<div className="space-y-4">
				<div className="space-y-1.5">
					<Label className="text-xs text-zinc-400 uppercase tracking-wide">
						{t("recharge.amountLabel")}
					</Label>
					<div className="flex flex-wrap items-center gap-2">
						{RECHARGE_PRESETS.map((preset) => (
							<button
								key={preset}
								onClick={() => onAmount(String(preset))}
								className={`h-8 px-3 text-xs rounded border transition-colors cursor-pointer font-mono ${
									amount === String(preset)
										? "bg-violet-600/20 border-violet-500/40 text-violet-200"
										: "bg-zinc-800 border-zinc-700 text-zinc-300 hover:bg-zinc-700"
								}`}
							>
								¥{preset}
							</button>
						))}
						<Input
							value={amount}
							onChange={(e) => onAmount(e.target.value)}
							onKeyDown={(e) => {
								if (e.key === "Enter" && canSubmit) onSubmit();
							}}
							placeholder={t("recharge.customAmount")}
							inputMode="decimal"
							className="h-8 w-36 text-sm font-mono"
						/>
					</div>
					<p
						className={
							amountError ? "text-[11px] text-red-400" : "text-[11px] text-zinc-600"
						}
					>
						{amountError ||
							(amount.trim() === ""
								? minMicro > 0
									? t("recharge.amountHintMin", {
											amount: formatMicro(
												minMicro,
												currency,
											),
										})
									: t("recharge.amountHintTwoDecimals")
								: micro !== null
									? t("recharge.amountToRecharge", {
											amount: formatMicro(micro, currency),
										})
									: "")}
					</p>
				</div>

				<div className="space-y-1.5">
					<Label className="text-xs text-zinc-400 uppercase tracking-wide">
						{t("recharge.paymentMethod")}
					</Label>
					<SimpleSelect
						value={channel}
						onValueChange={onChannel}
						options={channels.map((value) => ({
							value,
							label: paymentChannelLabel(value),
						}))}
						className="max-w-56"
					/>
				</div>

				<Button
					onClick={onSubmit}
					disabled={!canSubmit}
					className="bg-violet-600 hover:bg-violet-500 text-white"
				>
					{submitting ? t("recharge.creating") : t("recharge.payNow")}
				</Button>
			</div>
		</Panel>
	);
}

function PendingPanel({
	pending,
	status,
	expiryHint,
	qrBroken,
	onQrBroken,
	onDismiss,
	onBanner,
}: {
	pending: BillingRechargeResult | null;
	status: string;
	expiryHint: string | null;
	qrBroken: boolean;
	onQrBroken: () => void;
	onDismiss: () => void;
	onBanner: (message: BannerMessage) => void;
}) {
	const { t } = useTranslation(["billingPages", "common"]);
	if (!pending) {
		return (
			<Panel title={t("recharge.howTo")}>
				<div className="space-y-3">
					<Hint icon={AlertCircle} tone="zinc">
						{t("recharge.howToAutoRefresh")}
					</Hint>
					<Hint icon={Clock} tone="zinc">
						{t("recharge.howToExpiry")}
					</Hint>
					<Hint icon={CheckCircle2} tone="zinc">
						{t("recharge.howToFullAmount")}
					</Hint>
				</div>
			</Panel>
		);
	}

	const copyLink = () => {
		const link = pending.pay_url || pending.qrcode || pending.url_scheme;
		if (!link) return;
		navigator.clipboard
			.writeText(link)
			.then(() => onBanner({ kind: "ok", text: t("recharge.payLinkCopied") }))
			.catch(() =>
				onBanner({ kind: "error", text: t("recharge.copyLinkFailed") }),
			);
	};

	return (
		<Panel
			title={t("recharge.pendingTitle")}
			description={`${formatMicro(pending.amount_micro, pending.currency)} · ${paymentChannelLabel(pending.channel)}`}
		>
			<div className="space-y-4">
				{pending.qrcode && !qrBroken && (
					<div className="flex flex-col items-center gap-2">
						<img
							src={pending.qrcode}
							alt={t("recharge.qrAlt")}
							className="w-40 h-40 rounded-lg bg-white p-2"
							onError={onQrBroken}
						/>
						<p className="text-[11px] text-zinc-500">
							{t("recharge.scanHint", {
								channel: paymentChannelLabel(pending.channel),
							})}
						</p>
					</div>
				)}

				<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5">
					<p className="text-[10px] text-zinc-500 mb-1">
						{t("recharge.orderNo")}
					</p>
					<p className="text-xs text-zinc-300 font-mono break-all">
						{pending.out_trade_no}
					</p>
					<div className="flex items-center gap-2 mt-1.5">
						<Pill tone={paymentStatusTone(status || pending.status)}>
							{paymentStatusLabel(status || pending.status)}
						</Pill>
						{expiryHint && (
							<span className="inline-flex items-center gap-1 text-[11px] text-zinc-500">
								<Clock className="w-3 h-3" strokeWidth={1.5} />
								{expiryHint}
							</span>
						)}
					</div>
				</div>

				<div className="flex flex-wrap items-center gap-2">
					{pending.pay_url && (
						<a
							href={pending.pay_url}
							target="_blank"
							rel="noreferrer"
							className="inline-flex items-center gap-1.5 h-8 px-3 text-xs rounded bg-violet-600 hover:bg-violet-500 text-white transition-colors"
						>
							<ExternalLink className="w-3.5 h-3.5" strokeWidth={1.5} />
							{t("recharge.openCheckout")}
						</a>
					)}
					<button
						onClick={copyLink}
						className="inline-flex items-center gap-1.5 h-8 px-3 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 transition-colors cursor-pointer"
					>
						<Copy className="w-3.5 h-3.5" strokeWidth={1.5} />
						{t("recharge.copyLink")}
					</button>
					<button
						onClick={onDismiss}
						className="inline-flex items-center gap-1.5 h-8 px-3 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-400 hover:bg-zinc-700 transition-colors cursor-pointer"
					>
						{t("common:action.close")}
					</button>
				</div>

				<p className="text-[11px] text-zinc-600 leading-relaxed">
					{t("recharge.pendingFootnote")}
				</p>
			</div>
		</Panel>
	);
}
