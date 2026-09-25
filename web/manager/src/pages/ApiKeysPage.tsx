// API Key 管理页。
//
// 数据全部来自 /api/api-keys（列表 / 创建 / 撤销）与 /api/api-keys/scopes（权限目录）：
// 权限的标题、说明、分组与快捷组合都由服务端下发，前端不硬编码，改文案不用发版。
//
// 三条容易踩的边界：
//   ① 503 = 这个部署没开凭证功能。这是**默认状态**，整页走空态，不弹错误（与计费页面
//      对 503 的处理一致）；
//   ② 明文只在创建响应里出现一次。除创建横幅外，页面上任何地方都不该有完整 Key；
//      横幅只活在这个组件实例里，不落任何存储，刷新即消失；
//   ③ can_create: false 时创建入口置灰（灰度期只允许管理员），别等提交后才吃 403。

import { useEffect, useMemo, useState } from "react";
import {
	AlertCircle,
	CheckCircle2,
	Copy,
	KeyRound,
	Plus,
	RefreshCw,
	Search,
	Shield,
	Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
	apiKeyApi,
	type ApiKeyCatalog,
	type ApiKeyCreated,
	type ApiKeyPreset,
	type ApiKeyScopeInfo,
	type ApiKeyView,
} from "@/lib/api";
import {
	ADMIN_ONLY_HINT,
	apiKeyErrorText,
	apiKeyStatus,
	apiKeyStatusLabel,
	apiKeyStatusTone,
	createKeyErrorText,
	formatLastUsed,
	isApiKeyDisabled,
	revokeKeyErrorText,
	scopeDescription,
	scopeTitle,
} from "@/lib/apikey";
import { formatTime } from "@/lib/billing";
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
} from "@/pages/billing/shared";

/** 权限按分组切段；顺序用服务端给的 groups，没登记过的分组排在最后。 */
interface ScopeSection {
	value: string;
	title: string;
	scopes: ApiKeyScopeInfo[];
}

export default function ApiKeysPage() {
	const [catalog, setCatalog] = useState<ApiKeyCatalog | null>(null);
	const [keys, setKeys] = useState<ApiKeyView[]>([]);
	const [loading, setLoading] = useState(true);
	const [disabled, setDisabled] = useState(false);
	const [loadFailed, setLoadFailed] = useState(false);
	const [banner, setBanner] = useState<BannerMessage | null>(null);
	const [reloadKey, setReloadKey] = useState(0);

	const [createOpen, setCreateOpen] = useState(false);
	// createdKey 是刚创建出来的完整明文：只在这个组件实例里活一次，刷新页面就没了。
	const [createdKey, setCreatedKey] = useState<string | null>(null);
	const [copied, setCopied] = useState(false);
	const [revokeTarget, setRevokeTarget] = useState<ApiKeyView | null>(null);
	const [revoking, setRevoking] = useState(false);

	// 目录与列表并行取：表单要目录才画得出来，列表要目录才知道权限怎么显示。
	// 列表一次取满一页（账号最多 100 把 Key，page_size 上限也是 100），表格不分页。
	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		Promise.all([
			apiKeyApi.catalog(),
			apiKeyApi.list({ page: 1, page_size: 100 }),
		])
			.then(([catalogRes, listRes]) => {
				if (cancelled) return;
				setCatalog(catalogRes.data);
				setKeys(listRes.data.data);
				setDisabled(false);
				setLoadFailed(false);
			})
			.catch((err: unknown) => {
				if (cancelled) return;
				if (isApiKeyDisabled(err)) {
					setDisabled(true);
					return;
				}
				setLoadFailed(true);
				setBanner({
					kind: "error",
					text: apiKeyErrorText(err, "加载 Key 列表失败，请重试"),
				});
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, [reloadKey]);

	// 「已复制」只亮两秒；再点一次会重置计时。
	useEffect(() => {
		if (!copied) return;
		const timer = window.setTimeout(() => setCopied(false), 2000);
		return () => window.clearTimeout(timer);
	}, [copied]);

	const refresh = () => setReloadKey((key) => key + 1);

	const canCreate = catalog?.can_create ?? false;
	const scopes = useMemo(() => catalog?.scopes ?? [], [catalog]);

	const handleCreated = (created: ApiKeyCreated) => {
		setCreateOpen(false);
		// 明文（created.key）只进横幅、只活这一屏；列表里放 api_key 元数据。
		setCreatedKey(created.key);
		setCopied(false);
		setBanner(null);
		// 新 Key 排在最前面；明细等下次刷新再对齐服务端。
		setKeys((prev) => [created.api_key, ...prev]);
	};

	const copyCreatedKey = () => {
		if (!createdKey) return;
		navigator.clipboard
			.writeText(createdKey)
			.then(() => setCopied(true))
			.catch(() =>
				setBanner({ kind: "error", text: "复制失败，请手动选中 Key 复制" }),
			);
	};

	const confirmRevoke = async () => {
		if (!revokeTarget) return;
		const target = revokeTarget;
		setRevoking(true);
		setBanner(null);
		try {
			// 幂等：重复提交不会重复撤销，所以这里不需要额外的防重。
			await apiKeyApi.revoke(target.id);
			setRevokeTarget(null);
			// 撤销是软删：刷新后这一行还在，只是状态变成「已撤销」。
			refresh();
			setBanner({ kind: "ok", text: `已撤销「${target.name}」` });
		} catch (err) {
			setBanner({ kind: "error", text: revokeKeyErrorText(err) });
		} finally {
			setRevoking(false);
		}
	};

	const header = (
		<div className="border-b border-zinc-800/80 px-8 py-5">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-lg font-semibold text-white">API Keys</h1>
					<p className="text-sm text-zinc-500 mt-0.5">
						管理用于调用 Orion-X API 的访问密钥
					</p>
				</div>
				{!disabled && (
					<Button
						onClick={() => setCreateOpen(true)}
						disabled={!canCreate}
						title={canCreate ? undefined : ADMIN_ONLY_HINT}
						className="bg-violet-600 hover:bg-violet-500 text-white h-9 px-4 text-sm gap-1.5 shadow-md shadow-violet-600/20"
					>
						<Plus className="w-4 h-4" />
						新建 Key
					</Button>
				)}
			</div>
		</div>
	);

	// 功能没开：整页一个空态。这不是错误，是默认部署形态。
	if (disabled) {
		return (
			<div className="min-h-full">
				{header}
				<div className="px-8 py-6">
					<Panel bodyClassName="p-0">
						<EmptyState
							icon={KeyRound}
							title="API Key 暂未开放"
							hint="当前环境还没有开通 API Key 功能；开通后可以在这里创建、查看与撤销 Key。"
						/>
					</Panel>
				</div>
			</div>
		);
	}

	return (
		<div className="min-h-full">
			{header}

			<div className="px-8 py-6 space-y-4">
				{createdKey && (
					<div className="bg-emerald-400/5 border border-emerald-400/20 rounded-xl px-5 py-4 flex items-center gap-3">
						<CheckCircle2
							className="w-5 h-5 text-emerald-400 shrink-0"
							strokeWidth={1.5}
						/>
						<div className="flex-1 min-w-0">
							<p className="text-sm text-white font-medium mb-1">
								API Key 已创建，请立即复制保存
							</p>
							<p className="text-xs font-mono text-emerald-300 truncate">
								{createdKey}
							</p>
						</div>
						<Button
							size="sm"
							variant="outline"
							onClick={copyCreatedKey}
							className="h-7 px-3 text-xs bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-400 hover:text-emerald-300 border border-emerald-500/30 shrink-0"
						>
							{copied ? (
								<>
									<CheckCircle2 className="w-3 h-3 mr-1" />
									已复制
								</>
							) : (
								<>
									<Copy className="w-3 h-3 mr-1" />
									复制
								</>
							)}
						</Button>
						<button
							onClick={() => setCreatedKey(null)}
							aria-label="关闭提示"
							className="text-zinc-500 hover:text-zinc-300 cursor-pointer ml-1 text-lg leading-none"
						>
							×
						</button>
					</div>
				)}

				{!loading && !canCreate && (
					<Hint icon={Shield}>
						{ADMIN_ONLY_HINT}已有的 Key 不受影响，仍可正常使用与撤销。
					</Hint>
				)}

				<Banner message={banner} onClose={() => setBanner(null)} />

				{loading && keys.length === 0 ? (
					<Panel bodyClassName="p-0">
						<LoadingBlock label="加载 Key 列表中..." />
					</Panel>
				) : loadFailed && keys.length === 0 ? (
					<Panel bodyClassName="p-0">
						<EmptyState
							icon={AlertCircle}
							title="列表加载失败"
							hint="请刷新页面重试；多次失败请联系管理员。"
						/>
					</Panel>
				) : keys.length === 0 ? (
					<Panel bodyClassName="p-0">
						<EmptyState
							icon={KeyRound}
							title="还没有 API Key"
							hint="创建 Key 后，脚本与流水线就能用它访问自己的智能体、设备与数据。"
							action={
								canCreate ? (
									<Button
										onClick={() => setCreateOpen(true)}
										className="bg-violet-600 hover:bg-violet-500 text-white h-8 px-4 text-xs gap-1.5"
									>
										<Plus className="w-3.5 h-3.5" />
										新建 Key
									</Button>
								) : undefined
							}
						/>
					</Panel>
				) : (
					<Panel
						title="我的 Key"
						description={`共 ${keys.length} 个`}
						actions={
							<button
								onClick={refresh}
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
						<TableShell
							head={
								<>
									<Th>名称</Th>
									<Th>Key（脱敏）</Th>
									<Th>权限</Th>
									<Th>状态</Th>
									<Th>调用次数</Th>
									<Th>最后使用</Th>
									<Th>创建时间</Th>
									<Th className="text-right">操作</Th>
								</>
							}
						>
							{keys.map((item, index) => {
								const status = apiKeyStatus(item);
								return (
									<Tr
										key={item.id}
										last={index === keys.length - 1}
										// 已撤销 / 已过期的行保留在列表里，但不该抢注意力。
										className={status === "valid" ? undefined : "opacity-60"}
									>
										<Td>
											<div className="flex items-center gap-2">
												<div className="w-7 h-7 rounded-lg bg-zinc-800 border border-zinc-700/50 flex items-center justify-center shrink-0">
													<KeyRound
														className="w-3.5 h-3.5 text-zinc-500"
														strokeWidth={1.5}
													/>
												</div>
												<span
													className="text-sm text-white font-medium truncate max-w-48"
													title={item.name}
												>
													{item.name}
												</span>
											</div>
										</Td>
										<Td className="text-xs font-mono text-zinc-400 whitespace-nowrap">
											{item.masked_key}
										</Td>
										<Td className="max-w-80">
											{item.scopes.length === 0 ? (
												<span className="text-xs text-zinc-600">—</span>
											) : (
												<div className="flex flex-wrap gap-1">
													{item.scopes.map((value) => (
														// 目录里没有的权限值原样显示（兼容规则），不隐藏。
														<span
															key={value}
															title={scopeDescription(scopes, value)}
														>
															<Pill>{scopeTitle(scopes, value)}</Pill>
														</span>
													))}
												</div>
											)}
										</Td>
										<Td>
											<Pill tone={apiKeyStatusTone(status)}>
												{apiKeyStatusLabel(status)}
											</Pill>
										</Td>
										<Td className="font-mono text-zinc-400">
											{item.call_count.toLocaleString()}
										</Td>
										<Td className="text-xs text-zinc-500 whitespace-nowrap">
											{formatLastUsed(item.last_used_at)}
										</Td>
										<Td className="text-xs text-zinc-500 font-mono whitespace-nowrap">
											{formatTime(item.created_at)}
										</Td>
										<Td className="text-right">
											{status === "revoked" ? (
												<span className="text-xs text-zinc-600">—</span>
											) : (
												<button
													onClick={() => setRevokeTarget(item)}
													aria-label={`撤销 ${item.name}`}
													className="inline-flex items-center gap-1 text-xs text-zinc-500 hover:text-red-400 px-1.5 py-1 rounded hover:bg-red-400/10 transition-colors cursor-pointer"
												>
													<Trash2 className="w-3.5 h-3.5" strokeWidth={1.5} />
													撤销
												</button>
											)}
										</Td>
									</Tr>
								);
							})}
						</TableShell>
					</Panel>
				)}

				<div className="bg-zinc-900/50 border border-zinc-800 rounded-xl p-4 flex items-start gap-3">
					<Shield
						className="w-4 h-4 text-zinc-500 shrink-0 mt-0.5"
						strokeWidth={1.5}
					/>
					<p className="text-xs text-zinc-500 leading-relaxed">
						API Key 创建后仅显示一次，请立即复制保存到安全位置。不要将 Key
						提交到代码库或分享给他人。如发现泄露请立即撤销并新建。
					</p>
				</div>
			</div>

			{createOpen && catalog && (
				<CreateKeyDialog
					catalog={catalog}
					onClose={() => setCreateOpen(false)}
					onCreated={handleCreated}
				/>
			)}

			{revokeTarget && (
				<RevokeKeyDialog
					target={revokeTarget}
					busy={revoking}
					onClose={() => setRevokeTarget(null)}
					onConfirm={confirmRevoke}
				/>
			)}
		</div>
	);
}

/** 创建对话框。挂载时初始化一次，所以每次打开都是干净状态（默认权限已预勾选）。 */
function CreateKeyDialog({
	catalog,
	onClose,
	onCreated,
}: {
	catalog: ApiKeyCatalog;
	onClose: () => void;
	onCreated: (created: ApiKeyCreated) => void;
}) {
	const [name, setName] = useState("");
	const [checked, setChecked] = useState<Set<string>>(() => {
		// 打开就先勾上服务端标了 default 的那批权限。
		return new Set(
			catalog.scopes
				.filter((scope) => scope.default && !scope.deprecated)
				.map((scope) => scope.value),
		);
	});
	const [query, setQuery] = useState("");
	const [submitting, setSubmitting] = useState(false);
	const [formError, setFormError] = useState("");

	// 废弃的权限不再授予新 Key（服务端会直接拒），所以不进勾选列表；
	// 历史 Key 里已经持有的仍会在列表中原样展示。
	const creatable = useMemo(
		() => catalog.scopes.filter((scope) => !scope.deprecated),
		[catalog.scopes],
	);

	const visible = useMemo(() => {
		const keyword = query.trim().toLowerCase();
		if (!keyword) return creatable;
		return creatable.filter(
			(scope) =>
				scope.title.toLowerCase().includes(keyword) ||
				scope.desc.toLowerCase().includes(keyword) ||
				scope.value.toLowerCase().includes(keyword),
		);
	}, [creatable, query]);

	const sections = useMemo<ScopeSection[]>(() => {
		const byGroup = new Map<string, ApiKeyScopeInfo[]>();
		for (const scope of visible) {
			const bucket = byGroup.get(scope.group);
			if (bucket) bucket.push(scope);
			else byGroup.set(scope.group, [scope]);
		}
		const ordered: ScopeSection[] = [];
		for (const group of catalog.groups) {
			const groupScopes = byGroup.get(group.value);
			if (!groupScopes) continue;
			ordered.push({
				value: group.value,
				title: group.title,
				scopes: groupScopes,
			});
			byGroup.delete(group.value);
		}
		// 目录里没登记的分组（服务端先上了新权限）用原始分组名兜底，不丢内容。
		for (const [value, groupScopes] of byGroup) {
			ordered.push({ value, title: value, scopes: groupScopes });
		}
		return ordered;
	}, [visible, catalog.groups]);

	const toggle = (value: string) => {
		setChecked((prev) => {
			const next = new Set(prev);
			if (next.has(value)) next.delete(value);
			else next.add(value);
			return next;
		});
	};

	// 权限组合只是"帮你勾好"：勾完仍然可以自己改，提交的永远是勾选状态。
	const applyPreset = (preset: ApiKeyPreset) => {
		const known = new Set(creatable.map((scope) => scope.value));
		setChecked(new Set(preset.scopes.filter((value) => known.has(value))));
	};

	const selectAllVisible = () =>
		setChecked((prev) => new Set([...prev, ...visible.map((s) => s.value)]));

	const trimmedName = name.trim();
	const canSubmit = trimmedName !== "" && checked.size > 0 && !submitting;

	const submit = async () => {
		if (!canSubmit) return;
		setFormError("");
		setSubmitting(true);
		try {
			const { data } = await apiKeyApi.create({
				name: trimmedName,
				scopes: [...checked],
			});
			onCreated(data);
		} catch (err) {
			setFormError(createKeyErrorText(err));
		} finally {
			setSubmitting(false);
		}
	};

	return (
		<Dialog open onOpenChange={(open: boolean) => !open && onClose()}>
			<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-2xl">
				<DialogHeader>
					<DialogTitle className="text-white flex items-center gap-2">
						<KeyRound className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
						新建 API Key
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4 py-2">
					<div className="space-y-1.5">
						<Label className="text-xs text-zinc-400 uppercase tracking-wide">
							名称
						</Label>
						<Input
							value={name}
							onChange={(e) => setName(e.target.value)}
							placeholder="例：生产环境"
							maxLength={64}
							autoFocus
							onKeyDown={(e) => e.key === "Enter" && submit()}
						/>
						<p className="text-[11px] text-zinc-600">
							用于区分用途，最多 64 个字。
						</p>
					</div>

					<div className="space-y-2">
						<div className="flex items-center justify-between gap-2">
							<Label className="text-xs text-zinc-400 uppercase tracking-wide">
								权限范围
							</Label>
							<div className="flex items-center gap-3 text-[11px]">
								<span className="text-zinc-500">已选 {checked.size} 个权限</span>
								<button
									type="button"
									onClick={selectAllVisible}
									disabled={visible.length === 0}
									className="text-violet-400 hover:text-violet-300 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
								>
									全选
								</button>
								<button
									type="button"
									onClick={() => setChecked(new Set())}
									disabled={checked.size === 0}
									className="text-zinc-400 hover:text-zinc-200 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
								>
									清空
								</button>
							</div>
						</div>

						<div className="flex flex-wrap gap-2">
							{catalog.presets.map((preset) => (
								<button
									key={preset.name}
									type="button"
									onClick={() => applyPreset(preset)}
									className="rounded-lg border border-zinc-700 bg-zinc-800/60 px-2.5 py-1.5 text-left hover:border-violet-500/40 hover:bg-zinc-800 transition-colors cursor-pointer"
								>
									<span className="block text-xs text-zinc-200">
										{preset.title}
									</span>
									<span className="block text-[11px] text-zinc-500 mt-0.5 max-w-72 leading-relaxed">
										{preset.description}
									</span>
								</button>
							))}
						</div>

						<div className="relative">
							<Search
								className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-600"
								strokeWidth={1.5}
							/>
							<Input
								value={query}
								onChange={(e) => setQuery(e.target.value)}
								placeholder="搜索权限"
								className="pl-8"
							/>
						</div>

						<div className="max-h-64 overflow-y-auto rounded-lg border border-zinc-800 divide-y divide-zinc-800/70">
							{sections.length === 0 ? (
								<p className="px-3 py-6 text-center text-xs text-zinc-600">
									没有匹配的权限
								</p>
							) : (
								sections.map((section) => (
									<div key={section.value}>
										<p className="px-3 py-1.5 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider bg-zinc-800/40">
											{section.title}
										</p>
										{section.scopes.map((scope) => (
											<label
												key={scope.value}
												className="flex items-start gap-2.5 px-3 py-2 cursor-pointer hover:bg-zinc-800/30 transition-colors"
											>
												<input
													type="checkbox"
													checked={checked.has(scope.value)}
													onChange={() => toggle(scope.value)}
													className="mt-0.5 h-3.5 w-3.5 accent-violet-500 cursor-pointer"
												/>
												<span className="min-w-0">
													<span className="block text-xs text-zinc-200">
														{scope.title}
													</span>
													<span className="block text-[11px] text-zinc-500 mt-0.5 leading-relaxed">
														{scope.desc}
													</span>
												</span>
											</label>
										))}
									</div>
								))
							)}
						</div>
					</div>

					{formError && (
						<p className="text-xs text-red-400 leading-relaxed">{formError}</p>
					)}
				</div>

				<DialogFooter>
					<Button
						variant="outline"
						onClick={onClose}
						className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
					>
						取消
					</Button>
					<Button
						onClick={submit}
						disabled={!canSubmit}
						className="bg-violet-600 hover:bg-violet-500 text-white"
					>
						{submitting ? "创建中..." : "创建"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

/** 撤销确认。撤销是立即生效的，所以这里必须让人看清撤的是哪一把。 */
function RevokeKeyDialog({
	target,
	busy,
	onClose,
	onConfirm,
}: {
	target: ApiKeyView;
	busy: boolean;
	onClose: () => void;
	onConfirm: () => void;
}) {
	return (
		<Dialog open onOpenChange={(open: boolean) => !open && !busy && onClose()}>
			<DialogContent className="bg-zinc-900 border-zinc-800 text-white sm:max-w-sm">
				<DialogHeader>
					<DialogTitle className="text-white flex items-center gap-2">
						<Trash2 className="w-4 h-4 text-red-400" strokeWidth={1.5} />
						撤销 API Key
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-3 py-2">
					<div className="rounded-lg bg-zinc-800/60 px-3 py-2.5">
						<p className="text-xs text-zinc-200 truncate">{target.name}</p>
						<p className="text-[11px] font-mono text-zinc-500 mt-1 truncate">
							{target.masked_key}
						</p>
					</div>
					<p className="text-sm text-zinc-400 leading-relaxed">
						撤销后使用这把 Key 的服务会立即无法调用，且无法恢复。如果只是担心泄露，撤销后请及时新建一把替换。
					</p>
				</div>

				<DialogFooter>
					<Button
						variant="outline"
						onClick={onClose}
						disabled={busy}
						className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white"
					>
						取消
					</Button>
					<Button
						onClick={onConfirm}
						disabled={busy}
						className="bg-red-600 hover:bg-red-500 text-white"
					>
						{busy ? "撤销中..." : "确认撤销"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
