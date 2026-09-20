// 计费管理页的公共零件：面板、表格、空态、分页与就地提示。
//
// 仓库里没有 toast 组件，写操作的结果统一走就地 banner（Banner）：成功绿色、失败红色，
// 失败文案取服务端的 {error}。样式沿用暗色 zinc + violet 的既有语言。

import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { AlertCircle, CheckCircle2, ChevronLeft, ChevronRight, Loader2, X } from "lucide-react";
import { cn } from "@/lib/utils";

export interface BannerMessage {
	kind: "ok" | "error";
	text: string;
}

const BANNER_TONES: Record<BannerMessage["kind"], string> = {
	ok: "border-emerald-500/30 bg-emerald-500/10",
	error: "border-red-500/30 bg-red-500/10",
};

const BANNER_TEXT_TONES: Record<BannerMessage["kind"], string> = {
	ok: "text-emerald-300",
	error: "text-red-300",
};

/** 就地提示条；message 为 null 时不渲染。 */
export function Banner({
	message,
	onClose,
}: {
	message: BannerMessage | null;
	onClose?: () => void;
}) {
	if (!message) return null;
	const Icon = message.kind === "ok" ? CheckCircle2 : AlertCircle;
	return (
		<div
			className={cn(
				"flex items-start gap-2 rounded-xl border px-4 py-3",
				BANNER_TONES[message.kind],
			)}
		>
			<Icon
				className={cn(
					"w-4 h-4 mt-0.5 shrink-0",
					message.kind === "ok" ? "text-emerald-400" : "text-red-400",
				)}
				strokeWidth={1.5}
			/>
			<p className={cn("text-xs flex-1", BANNER_TEXT_TONES[message.kind])}>
				{message.text}
			</p>
			{onClose && (
				<button
					onClick={onClose}
					className={cn(
						"shrink-0 transition-colors cursor-pointer",
						message.kind === "ok"
							? "text-emerald-400/60 hover:text-emerald-300"
							: "text-red-400/60 hover:text-red-300",
					)}
					aria-label="关闭提示"
				>
					<X className="w-3.5 h-3.5" strokeWidth={1.5} />
				</button>
			)}
		</div>
	);
}

/** 静态说明条（没有关闭按钮的那种），用于口径提示。 */
export function Hint({
	icon: Icon = AlertCircle,
	tone = "amber",
	children,
}: {
	icon?: LucideIcon;
	tone?: "amber" | "zinc";
	children: ReactNode;
}) {
	return (
		<div
			className={cn(
				"flex items-start gap-2 rounded-xl border px-4 py-3",
				tone === "amber"
					? "border-amber-500/30 bg-amber-500/10"
					: "border-zinc-800 bg-zinc-900",
			)}
		>
			<Icon
				className={cn(
					"w-4 h-4 mt-0.5 shrink-0",
					tone === "amber" ? "text-amber-400" : "text-zinc-500",
				)}
				strokeWidth={1.5}
			/>
			<div
				className={cn(
					"text-xs leading-relaxed flex-1",
					tone === "amber" ? "text-amber-200/90" : "text-zinc-400",
				)}
			>
				{children}
			</div>
		</div>
	);
}

/** 带标题的卡片。表格类内容传 bodyClassName="p-0"。 */
export function Panel({
	title,
	description,
	actions,
	bodyClassName,
	children,
}: {
	title?: ReactNode;
	description?: ReactNode;
	actions?: ReactNode;
	bodyClassName?: string;
	children: ReactNode;
}) {
	return (
		<div className="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
			{(title || actions) && (
				<div className="flex items-center justify-between gap-3 px-5 py-4 border-b border-zinc-800">
					<div className="min-w-0">
						{title && (
							<p className="text-sm font-medium text-white">{title}</p>
						)}
						{description && (
							<p className="text-xs text-zinc-500 mt-0.5">{description}</p>
						)}
					</div>
					{actions && (
						<div className="flex items-center gap-2 shrink-0">{actions}</div>
					)}
				</div>
			)}
			<div className={cn("p-5", bodyClassName)}>{children}</div>
		</div>
	);
}

/**
 * 表格骨架：表头样式是仓库既有的 text-[11px] uppercase tracking-wider。
 * 行用 Tr（zinc-800/50 分隔 + hover 高亮）。
 */
export function TableShell({
	head,
	children,
	className,
}: {
	head: ReactNode;
	children: ReactNode;
	className?: string;
}) {
	return (
		<div className={cn("overflow-x-auto", className)}>
			<table className="w-full">
				<thead>
					<tr className="border-b border-zinc-800">{head}</tr>
				</thead>
				<tbody>{children}</tbody>
			</table>
		</div>
	);
}

export function Th({
	children,
	className,
}: {
	children?: ReactNode;
	className?: string;
}) {
	return (
		<th
			className={cn(
				"text-left px-4 py-3 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider whitespace-nowrap",
				className,
			)}
		>
			{children}
		</th>
	);
}

export function Td({
	children,
	className,
	title,
}: {
	children?: ReactNode;
	className?: string;
	title?: string;
}) {
	return (
		<td
			title={title}
			className={cn("px-4 py-3.5 text-sm text-zinc-300 align-middle", className)}
		>
			{children}
		</td>
	);
}

export function Tr({
	children,
	last,
	className,
}: {
	children: ReactNode;
	last?: boolean;
	className?: string;
}) {
	return (
		<tr
			className={cn(
				"border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors",
				last && "border-0",
				className,
			)}
		>
			{children}
		</tr>
	);
}

const PILL_TONES = {
	zinc: "bg-zinc-800 text-zinc-400 border-zinc-700",
	emerald: "bg-emerald-400/10 text-emerald-400 border-emerald-400/20",
	amber: "bg-amber-400/10 text-amber-400 border-amber-400/20",
	red: "bg-red-400/10 text-red-400 border-red-400/20",
	violet: "bg-violet-600/15 text-violet-400 border-violet-500/20",
	sky: "bg-sky-400/10 text-sky-400 border-sky-400/20",
} as const;

export function Pill({
	tone = "zinc",
	children,
	className,
}: {
	tone?: keyof typeof PILL_TONES;
	children: ReactNode;
	className?: string;
}) {
	return (
		<span
			className={cn(
				"inline-flex items-center text-[11px] px-1.5 py-0.5 rounded border whitespace-nowrap",
				PILL_TONES[tone],
				className,
			)}
		>
			{children}
		</span>
	);
}

/** 加载态：表格区与整页共用。 */
export function LoadingBlock({ label = "加载中..." }: { label?: string }) {
	return (
		<div className="flex items-center justify-center py-16 text-zinc-600 gap-2">
			<Loader2 className="w-4 h-4 animate-spin" />
			<span className="text-sm">{label}</span>
		</div>
	);
}

/** 空态：无数据、无账户、计费未启用都用它，只换图标与文案。 */
export function EmptyState({
	icon: Icon,
	title,
	hint,
	action,
	className,
}: {
	icon: LucideIcon;
	title: string;
	hint?: string;
	action?: ReactNode;
	className?: string;
}) {
	return (
		<div
			className={cn(
				"flex flex-col items-center justify-center py-16 px-6 text-center",
				className,
			)}
		>
			<div className="w-12 h-12 rounded-2xl bg-zinc-800 flex items-center justify-center mb-4">
				<Icon className="w-6 h-6 text-zinc-600" strokeWidth={1.5} />
			</div>
			<p className="text-zinc-400 text-sm">{title}</p>
			{hint && (
				<p className="text-zinc-600 text-xs mt-1 max-w-md leading-relaxed">
					{hint}
				</p>
			)}
			{action && <div className="mt-4">{action}</div>}
		</div>
	);
}

/**
 * 分页条。`total` 已知时（账户/流水/用量）按 total 算页数；价格列表接口不返回
 * total，传 `hasNext` 由调用方按"本页是否填满"判断。total <= pageSize 时整条不渲染。
 */
export function PaginationBar({
	page,
	pageSize,
	total,
	hasNext,
	loading,
	onPageChange,
}: {
	page: number;
	pageSize: number;
	total?: number;
	hasNext?: boolean;
	loading?: boolean;
	onPageChange: (page: number) => void;
}) {
	if (total !== undefined && total <= pageSize) return null;
	const totalPages =
		total !== undefined ? Math.max(1, Math.ceil(total / pageSize)) : undefined;
	const canPrev = page > 1 && !loading;
	const canNext =
		(totalPages !== undefined ? page < totalPages : Boolean(hasNext)) && !loading;

	return (
		<div className="flex items-center justify-between mt-4">
			<span className="text-xs text-zinc-500">
				{total !== undefined
					? `共 ${total} 条 · 第 ${page}/${totalPages} 页`
					: `第 ${page} 页`}
			</span>
			<div className="flex items-center gap-2">
				<button
					onClick={() => onPageChange(page - 1)}
					disabled={!canPrev}
					className="inline-flex items-center gap-1 h-7 px-2 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
				>
					<ChevronLeft className="w-3.5 h-3.5" strokeWidth={1.5} />
					上一页
				</button>
				<button
					onClick={() => onPageChange(page + 1)}
					disabled={!canNext}
					className="inline-flex items-center gap-1 h-7 px-2 text-xs rounded bg-zinc-800 border border-zinc-700 text-zinc-300 hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
				>
					下一页
					<ChevronRight className="w-3.5 h-3.5" strokeWidth={1.5} />
				</button>
			</div>
		</div>
	);
}

/** 顶部的筛选行容器，保证各 tab 的过滤区长得一样。 */
export function FilterBar({ children }: { children: ReactNode }) {
	return (
		<div className="flex flex-wrap items-center gap-2 bg-zinc-900 border border-zinc-800 rounded-xl px-4 py-3">
			{children}
		</div>
	);
}
