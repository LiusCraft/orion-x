import { useTranslation } from "react-i18next";
import { Check, Languages } from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import i18n, {
	LANGUAGE_LABELS,
	SUPPORTED_LANGUAGES,
	type Language,
} from "@/i18n";

/** 顶栏语言切换：两种语言直接平铺，当前项打勾。 */
export default function LanguageSwitcher({
	className,
}: {
	className?: string;
}) {
	const { t } = useTranslation("layout");
	const current = (i18n.resolvedLanguage ?? "zh") as Language;

	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				aria-label={t("language")}
				className={cn(
					"p-2 text-zinc-500 hover:text-zinc-200 transition-colors cursor-pointer rounded-lg hover:bg-zinc-800/70",
					className,
				)}
			>
				<Languages className="w-5 h-5" strokeWidth={1.5} />
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="w-36">
				{SUPPORTED_LANGUAGES.map((lang) => (
					<DropdownMenuItem
						key={lang}
						onClick={() => i18n.changeLanguage(lang)}
						className="cursor-pointer"
					>
						{LANGUAGE_LABELS[lang]}
						{current === lang && (
							<Check className="w-3.5 h-3.5 ml-auto text-violet-400" />
						)}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
