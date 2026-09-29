// 浏览器标签页标题：每个页面用 useDocumentTitle 声明自己的名字。
//
// 标题形如「我的智能体 · Orion-X」，页面没声明时（如尚未接入标题的页面）回落到
// 站点默认标题。动态标题直接传当前状态即可，例如智能体详情页传 name。

import { useEffect } from "react";

/** 站点名，拼接在页面标题后面。 */
const SITE_NAME = "Orion-X";

/** 默认标题，index.html 里的初始标题与这里保持一致。 */
const DEFAULT_TITLE = "Orion-X AI语音";

/**
 * useDocumentTitle 把浏览器标签页标题设为「标题 · Orion-X」。
 * title 为空时使用站点默认标题。
 */
export function useDocumentTitle(title?: string) {
	const page = title?.trim();
	useEffect(() => {
		document.title = page ? `${page} · ${SITE_NAME}` : DEFAULT_TITLE;
	}, [page]);
}
