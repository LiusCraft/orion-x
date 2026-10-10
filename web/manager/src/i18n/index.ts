// i18n 入口：中英双语。语言选择存 localStorage("lang")，默认跟随浏览器语言，
// 非英文浏览器回落到中文（现有用户以中文为主）。
//
// 资源按命名空间拆分，见 locales/{zh,en}/*：common 放通用动作 / 状态 / 字段，
// 其余按页面域（layout / login / agents / billing / models / ...）各一份。
// 组件里用 useTranslation(["<域>", "common"])；非 React 模块 import i18n 后
// i18n.t("<域>:key")。

import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import zh from "./locales/zh";
import en from "./locales/en";

export const LANGUAGE_STORAGE_KEY = "lang";
export const SUPPORTED_LANGUAGES = ["zh", "en"] as const;
export type Language = (typeof SUPPORTED_LANGUAGES)[number];

export const LANGUAGE_LABELS: Record<Language, string> = {
	zh: "简体中文",
	en: "English",
};

/** 保存的语言优先；否则浏览器语言以 en 开头用英文，其余用中文。 */
function detectLanguage(): Language {
	const saved = localStorage.getItem(LANGUAGE_STORAGE_KEY);
	if (saved === "zh" || saved === "en") return saved;
	return navigator.language.toLowerCase().startsWith("en") ? "en" : "zh";
}

function syncDocument(language: string) {
	document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
}

i18n.use(initReactI18next).init({
	resources: { zh, en },
	lng: detectLanguage(),
	fallbackLng: "zh",
	defaultNS: "common",
	interpolation: { escapeValue: false },
});

i18n.on("languageChanged", (language) => {
	localStorage.setItem(LANGUAGE_STORAGE_KEY, language);
	syncDocument(language);
});

syncDocument(i18n.language);

export default i18n;
