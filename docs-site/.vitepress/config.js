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
					target: "http://localhost:9090",
					rewrite: (path) => path.replace(/^\/manager-api-docs/, "/api-docs"),
				},
				"/swagger": "http://localhost:9090",
				"^/api(?:/|$)": "http://localhost:9090",
				"^/internal(?:/|$)": "http://localhost:9090",
			},
		},
	},
});
