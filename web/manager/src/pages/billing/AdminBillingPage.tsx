// 计费管理（仅 admin）：账户 / 价格版本 / 计费项 / 流水 / 报表。
//
// 这里是控制面：定价、账户余额、流水与报表都只从这一条路径改，数据面只上报事实
// （§19）。非 admin 由路由层拦回 /billing/usage。

import { Coins } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import AccountsTab from "./admin/AccountsTab";
import ItemsTab from "./admin/ItemsTab";
import LedgerTab from "./admin/LedgerTab";
import PricesTab from "./admin/PricesTab";
import RechargeTab from "./admin/RechargeTab";
import StatsTab from "./admin/StatsTab";

const TABS = [
	{ value: "accounts", label: "账户" },
	{ value: "prices", label: "价格版本" },
	{ value: "items", label: "计费项" },
	{ value: "recharge", label: "充值订单" },
	{ value: "ledger", label: "流水" },
	{ value: "stats", label: "报表" },
];

export default function AdminBillingPage() {
	return (
		<div className="min-h-full">
			<div className="border-b border-zinc-800/80 px-8 py-5">
				<div>
					<h1 className="text-lg font-semibold text-white flex items-center gap-2">
						<Coins className="w-4 h-4 text-violet-400" strokeWidth={1.5} />
						计费管理
					</h1>
					<p className="text-sm text-zinc-500 mt-0.5">
						账户余额与人工调整、价格版本、计费项目录、充值订单、流水与报表
					</p>
				</div>
			</div>

			<div className="px-8 py-6">
				<Tabs defaultValue="accounts">
					<TabsList className="bg-zinc-900 border border-zinc-800 h-9 p-0.5">
						{TABS.map((tab) => (
							<TabsTrigger
								key={tab.value}
								value={tab.value}
								className="text-xs data-[state=active]:bg-zinc-800 data-[state=active]:text-white text-zinc-500 h-8 px-4"
							>
								{tab.label}
							</TabsTrigger>
						))}
					</TabsList>

					<TabsContent value="accounts" className="mt-5">
						<AccountsTab />
					</TabsContent>
					<TabsContent value="prices" className="mt-5">
						<PricesTab />
					</TabsContent>
					<TabsContent value="items" className="mt-5">
						<ItemsTab />
					</TabsContent>
					<TabsContent value="recharge" className="mt-5">
						<RechargeTab />
					</TabsContent>
					<TabsContent value="ledger" className="mt-5">
						<LedgerTab />
					</TabsContent>
					<TabsContent value="stats" className="mt-5">
						<StatsTab />
					</TabsContent>
				</Tabs>
			</div>
		</div>
	);
}
