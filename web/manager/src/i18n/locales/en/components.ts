// Component pages (MCP / SKILL / Plugins) copy.
export default {
	install: "Install",
	installed: "Installed",
	market: "Market",
	pagination: {
		prev: "Previous",
		next: "Next",
	},
	mcp: {
		title: "MCP Management",
		subtitle:
			"Manage Model Context Protocol services that agents can reference once installed",
		custom: "Custom MCP",
		tab: {
			installed: "Enabled ({{total}})",
		},
		marketEmpty: "No official MCP servers yet",
		marketEmptyHint: "Stay tuned",
		mineEmpty: "No MCP servers enabled yet",
		mineEmptyHint: "Install from the market or add a custom MCP",
		addTitle: "Add Custom MCP Server",
		editTitle: "Edit MCP Server",
		officialHint: "Official MCP · Some settings are managed by the system",
		testConnection: "Test Connection",
		preview: "Preview",
		descriptionPlaceholder:
			"Supports Markdown\nE.g. **bold**, `code`\n- list item",
		tagPlaceholder: "Type a tag, then press Enter or comma",
		tagAddPlaceholder: "Add tag...",
		tools: {
			title: "Try Tools",
			refreshTitle: "Refresh tool list",
			invoke: "Invoke",
			empty: "No tools found",
			selectedAll: "All tools selected",
			selectedCount: "{{selected}}/{{total}} tools selected",
			selectAll: "Select all",
			deselectAll: "Deselect all",
			noParams: "No parameters",
			testFirst: "Test the connection first to load tools",
			result: {
				error: "Execution error",
				success: "Execution succeeded",
				requestFailed: "Request failed",
				copyTitle: "Copy result",
			},
		},
		fields: {
			command: "Command",
			endpoint: "Endpoint",
			tags: "Tags",
			toolAllowlist: "Tool allowlist",
			toolAllowlistEdit: "Tool allowlist (empty = all tools)",
			connectionConfig: "Connection settings",
			autoConfig: "Auto-configured (no input needed)",
			iconUrl: "Icon URL",
			transport: "Transport",
			args: "Arguments (space-separated)",
			cwd: "Working directory (optional)",
			envVars: "Environment variables",
			httpHeaders: "HTTP Headers",
			timeout: "Timeout (ms)",
			workingDir: "Working directory: {{path}}",
			systemConfigHidden: "System config · Hidden",
			headerInputPlaceholder: "Enter {{label}}",
		},
	},
	skills: {
		title: "SKILL Management",
		subtitle: "Preset capability modules reusable across multiple agents",
		custom: "Custom SKILL",
		tab: {
			installed: "Installed ({{total}})",
		},
		mineEmpty: "No SKILLs installed yet",
		mineEmptyHint: "Browse the market to pick the modules you need",
		configure: "Configure",
		uninstall: "Uninstall",
		tags: {
			official: "Official",
			nlp: "NLP",
			programming: "Programming",
			multimodal: "Multimodal",
			language: "Language",
			database: "Database",
		},
		market: {
			sentiment: {
				name: "Sentiment Analysis",
				desc: "Analyze the sentiment of user input and return emotion type and confidence for emotional awareness in conversations.",
			},
			summary: {
				name: "Text Summarization",
				desc: "Compress long text into concise summaries with configurable length, supporting Chinese and English.",
			},
			entities: {
				name: "Entity Recognition",
				desc: "Extract people, places, organizations, and dates from text and return structured data.",
			},
			keywords: {
				name: "Keyword Extraction",
				desc: "Automatically extract keywords and topics from documents with TF-IDF and TextRank.",
			},
			codeReview: {
				name: "Code Review",
				desc: "Check submitted code for security, performance, and best practices, and return issues with suggestions.",
			},
			imageCaption: {
				name: "Image Captioning",
				desc: "Generate natural-language descriptions for images, covering scenes, objects, and text recognition.",
			},
			translation: {
				name: "Translation Expert",
				desc: "High-quality multilingual translation for 50+ languages, preserving formatting and terminology.",
			},
			sqlGen: {
				name: "SQL Generation",
				desc: "Generate SQL queries from natural-language descriptions across multiple database dialects.",
			},
		},
	},
	plugins: {
		title: "Plugin Management",
		subtitle:
			"Plugins are tools that agents invoke automatically during conversations",
		addHttp: "Add HTTP Plugin",
		tab: {
			mine: "My Plugins ({{total}})",
		},
		mineEmpty: "No custom plugins yet",
		mineEmptyHint: "Connect your own tools over an HTTP API",
		method: "Method",
		apiUrl: "API URL",
		authType: "Authentication",
		auth: {
			none: "None",
			bearer: "Bearer Token",
			apiKey: "API Key",
			basic: "Basic Auth",
			tokenLabel: "Token",
			passwordLabel: "Password",
		},
		dialog: {
			title: "Add HTTP API Plugin",
			submit: "Add Plugin",
			namePlaceholder: "Plugin name",
			descLabel: "Description (tells the agent when to use it)",
			descPlaceholder: "E.g. Call this API when querying internal CRM data",
		},
		tags: {
			tools: "Tools",
			free: "Free",
			finance: "Finance",
			knowledge: "Knowledge",
			academic: "Academic",
		},
		market: {
			weather: {
				name: "Weather Lookup",
				desc: "Get real-time weather for any city worldwide, including temperature, humidity, wind speed, and forecast.",
			},
			exchange: {
				name: "Currency Conversion",
				desc: "Real-time exchange rates and currency conversion for 180+ currencies, powered by ExchangeRate-API.",
			},
			wikipedia: {
				name: "Wikipedia Search",
				desc: "Search Wikipedia entries and return summaries and related links in multiple languages.",
			},
			arxiv: {
				name: "Arxiv Papers",
				desc: "Search Arxiv academic papers and return titles, abstracts, authors, and links.",
			},
		},
		my: {
			companyApi: {
				name: "Internal Company API",
				desc: "Query internal CRM data",
			},
			stockQuery: {
				name: "Product Stock Query",
				desc: "Query product stock in real time",
			},
		},
	},
};
