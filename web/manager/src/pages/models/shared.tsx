// 我的模型 / 厂商管理两页共用的零件：归属范围筛选（全部 / 官方 / 我的）与卡片、表格视图切换。

import { LayoutGrid, List } from "lucide-react";
import { useTranslation } from "react-i18next";

/** 资源的归属：官方 = 平台内置（is_system），我的 = 自己建的。 */
export type ResourceScope = "all" | "system" | "mine";

export type ViewMode = "grid" | "table";

const SCOPE_OPTIONS: Array<{ value: ResourceScope; labelKey: string }> = [
  { value: "all", labelKey: "scope.all" },
  { value: "system", labelKey: "scope.system" },
  { value: "mine", labelKey: "scope.mine" },
];

export function ScopeFilter({
  value,
  onChange,
}: {
  value: ResourceScope;
  onChange: (scope: ResourceScope) => void;
}) {
  const { t } = useTranslation(["models", "common"]);
  return (
    <div className="flex items-center rounded-lg border border-zinc-800 bg-zinc-900 p-0.5">
      {SCOPE_OPTIONS.map((option) => (
        <button
          key={option.value}
          onClick={() => onChange(option.value)}
          className={`h-8 px-3 rounded-md text-xs transition-colors cursor-pointer ${
            value === option.value
              ? "bg-zinc-700 text-white"
              : "text-zinc-500 hover:text-zinc-300"
          }`}
        >
          {t(option.labelKey)}
        </button>
      ))}
    </div>
  );
}

export function ViewToggle({
  value,
  onChange,
}: {
  value: ViewMode;
  onChange: (mode: ViewMode) => void;
}) {
  const { t } = useTranslation(["models", "common"]);
  const buttonClass = (mode: ViewMode) =>
    `flex items-center justify-center w-8 h-8 rounded-md transition-colors cursor-pointer ${
      value === mode ? "bg-zinc-700 text-white" : "text-zinc-500 hover:text-zinc-300"
    }`;
  return (
    <div className="flex items-center rounded-lg border border-zinc-800 bg-zinc-900 p-0.5">
      <button
        onClick={() => onChange("grid")}
        title={t("view.grid")}
        className={buttonClass("grid")}
      >
        <LayoutGrid className="w-4 h-4" />
      </button>
      <button
        onClick={() => onChange("table")}
        title={t("view.table")}
        className={buttonClass("table")}
      >
        <List className="w-4 h-4" />
      </button>
    </div>
  );
}
