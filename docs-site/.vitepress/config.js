import { defineConfig } from "vitepress";

export default defineConfig({
	title: "Orion-X",
	description: "跨渠道语音聊天智能体",
	base: "/",
	cleanUrls: true,
	themeConfig: {
		socialLinks: [
			{ icon: "github", link: "https://github.com/LiusCraft/orion-x" },
		],
	},
	vite: {
		server: {
			port: 5174,
			strictPort: true,
			proxy: {
				"/manager-api-docs": {
					target: "http://127.0.0.1:9090",
					rewrite: (path) => path.replace(/^\/manager-api-docs/, "/api-docs"),
				},
				"/swagger": "http://127.0.0.1:9090",
				"^/api(?:/|$)": "http://127.0.0.1:9090",
				"^/internal(?:/|$)": "http://127.0.0.1:9090",
			},
		},
	},
});
