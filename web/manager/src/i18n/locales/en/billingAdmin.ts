// Billing admin page wording: accounts / prices / items / recharge / ledger / reports tabs.
export default {
	shared: {
		item: "Billing item",
		account: "Account",
		amount: "Amount",
		note: "Note",
		time: "Time",
		allItems: "All items",
		allAccounts: "All accounts",
		allStatuses: "All statuses",
		forever: "Open-ended",
		deactivateFailed: "Failed to retire",
	},
	accounts: {
		loadFailed: "Failed to load accounts",
		disabledHint:
			"The billing module (billing.enabled) is not enabled on the server; accounts, ledger, and pricing are unavailable.",
		subjectPlaceholder: "All subjects",
		searchPlaceholder: "Account ID / subject ID",
		panelTitle: "Accounts",
		panelDescription:
			"Balance can be negative (postpaid arrears); frozen is the amount reserved for session pre-authorization",
		emptyTitle: "No matching accounts",
		emptyHint:
			"Accounts are created automatically when a user first generates usage; try a different filter.",
		columnSubject: "Subject",
		columnBalance: "Available balance",
		columnFrozen: "Frozen",
		columnCreditLimit: "Credit limit",
		overdrawn: "Over credit limit",
		arrears: "In arrears",
		adjust: "Adjust",
		grant: "Grant",
	},
	adjust: {
		grantHint:
			"Grants are item-scoped credit and are not added to the available balance: settlement draws down grant → balance → credit_limit in that order. ref_id is the idempotency source; the same ref_id is issued only once—repeat submissions will not issue twice.",
		loadDisabled: "Billing is disabled; cannot load billing items",
		loadFailed: "Failed to load billing item catalog",
		amountHint: "yuan, up to 6 decimal places",
		amountInvalid: "Invalid amount",
		microHint: "{{amount}} micro-yuan",
		amountLabel: "Amount (yuan)",
		amountPositiveHint: "must be positive",
		amountSignedHint: "may be positive or negative",
		itemGrantHint: "required; the grant is limited to this item",
		itemOptionalHint: "optional; recorded in the ledger",
		itemPlaceholder: "No item restriction",
		refIdHint: "idempotency source; the same ref_id is issued only once",
		notePlaceholder: "e.g. signup bonus / refund of mistaken charge (ticket #123)",
		available: "Available {{amount}}",
		frozen: "Frozen {{amount}}",
		errAmount: "Invalid amount: numbers only, up to 6 decimal places.",
		errGrantPositive: "Grant amount must be positive.",
		errAdjustZero: "Adjustment amount cannot be 0.",
		errNote: "A note is required: this adjustment must be understandable later.",
		errItem: "A grant must specify a billing item.",
		errRefId:
			"A grant must include ref_id (idempotency source), otherwise retries will issue twice.",
		fail: "Adjustment failed, please retry",
		grantTitle: "Issue grant",
		adjustTitle: "Adjust balance",
		submitAdjust: "Confirm adjustment",
		successGrant:
			"Issued a grant of {{amount}} to account {{account}} (limited to {{item}}), ref_id={{refId}}",
		successIncrease: "Added {{amount}} to account {{account}}",
		successDecrease: "Deducted {{amount}} from account {{account}}",
	},
	items: {
		loadFailed: "Failed to load billing item catalog",
		toggleEnabled: "Enabled \"{{item}}\" ({{code}})",
		toggleDisabled: "Disabled \"{{item}}\" ({{code}})",
		updateFailed: "Failed to update billing item",
		disabledHint:
			"The billing module (billing.enabled) is not enabled on the server; the billing item catalog is unavailable.",
		conflict: "Conflicting billing modes are both enabled: {{note}}",
		panelTitle: "Billing item catalog",
		panelDescription:
			"Built-in items are synced from the server seed and can only be enabled or disabled here; after an item is disabled its events are still recorded but not billed",
		emptyTitle: "Billing item catalog is empty",
		emptyHint:
			"The server seeds the catalog from internal/billing at startup; an empty table usually means the seed did not run.",
		columnMeterSource: "Metering point",
		columnChargeMode: "Charge mode",
		columnUnit: "Unit",
		columnHints: "Notes",
		columnSource: "Source",
		columnEnabled: "Enabled",
		builtin: "Built-in",
		custom: "Custom",
		saving: "Saving",
		enableAria: "Enable {{code}}",
	},
	ledger: {
		loadFailed: "Failed to load ledger",
		disabledHint:
			"The billing module (billing.enabled) is not enabled on the server; the ledger is unavailable.",
		kindPlaceholder: "All types",
		clear: "Clear",
		panelTitle: "Ledger",
		panelDescription:
			"append-only: entries are never modified. reserve / release only move frozen funds; balance_after stays unchanged",
		emptyTitle: "No matching ledger entries",
		emptyHint:
			"Try another account or time range; usage settlement writes charge entries and manual operations write adjust entries.",
		columnDirection: "Direction",
		columnBalanceAfter: "Balance after",
		columnKind: "Type",
		columnRef: "Reference",
		creator: "Operator {{name}}",
	},
	recharge: {
		loadFailed: "Failed to load recharge orders",
		disabledTitle: "Online recharge is not supported",
		disabledHint:
			"Online recharge is not enabled in this environment; there are no orders to show.",
		channelPlaceholder: "All channels",
		subjectPlaceholder: "Subject ID (user)",
		keywordPlaceholder: "Order no. / transaction no.",
		statusPaid: "Received",
		panelTitle: "Recharge orders",
		panelDescription:
			"Refunds are full-order only and available only for credited orders",
		emptyTitle: "No matching recharge orders",
		emptyHint:
			"Records appear after users place orders; try a different filter.",
		columnOrderNo: "Order no.",
		columnSubject: "Subject",
		columnChannel: "Channel",
		columnCreditedAt: "Credited at",
		refundTitle: "Full refund",
		refundView: "View refund info",
		refundUnavailable: "Only credited orders can be refunded",
		details: "Details",
		refund: "Refund",
	},
	refund: {
		titleDetails: "Refund details",
		orderLabel: "Order",
		amountLabel: "Amount {{amount}}",
		tradeNo: "Transaction no. {{no}}",
		creditedAt: "Credited at {{time}}",
		refundedAt: "Refunded at {{time}}",
		warning:
			"Submitting will return the payment to the payer and deduct the amount from the account balance. Accounts whose balance has been spent will go into arrears. Refunds are full-order only; partial refunds are not supported.",
		noteRequired:
			"A note is required: this refund must be understandable later (ticket number, reason).",
		notePlaceholder: "e.g. user requested a refund (ticket #123)",
		noteHint: "Recorded in the refund log for future reference.",
		disabledError:
			"Online recharge is not enabled in this environment; refunds are unavailable.",
		fail: "Refund failed, please retry",
		success: "Refunded {{amount}} (order {{order}})",
		submitting: "Refunding...",
		submit: "Confirm refund",
	},
	prices: {
		loadItemsAccountsFailed: "Failed to load billing items / accounts",
		loadFailed: "Failed to load price versions",
		deactivateConfirm:
			"Retire this version of the price for \"{{item}}\" ({{scope}})?\n\n" +
			"Retiring sets effective_to to now; it will no longer match new usage events.\n" +
			"Historical bills are unaffected—settlement uses the price at the event time, so historical bills never change when prices are adjusted.",
		deactivateSuccess:
			"Retired this version of the price for \"{{item}}\" (historical bills unaffected)",
		deleteConfirm:
			"Delete this not-yet-effective price version ({{scope}}, effective from {{date}})?\n\n" +
			"It has never matched any usage event and is not referenced by any ledger entry; deletion cannot be undone.",
		deleteSuccess: "Deleted the not-yet-effective price version ({{item}})",
		disabledHint:
			"The billing module (billing.enabled) is not enabled on the server; price versions are unavailable.",
		resourceTypePlaceholder: "All levels",
		scopePlaceholder: "All scopes",
		scopePlatform: "Platform price (filtered on this page)",
		accountPlaceholder: "All accounts (filtered on this page)",
		activeOnly: "Active only",
		newPrice: "New price",
		markupHint:
			"Repricing = add a version with a later effective_from; history is never overwritten. Only one price version can exist per scope (item + account + resource level) at any moment; match priority is account price → voice → model → provider → item fallback. Not-yet-effective versions can be edited directly; for effective versions, \"Adjust\" adds a new version and retires the old one in one step.",
		missingResources:
			"The {{types}} list failed to load; scope of these prices shows internal IDs for now. Refresh the page to retry.",
		panelTitle: "Price versions",
		panelDescription:
			"Includes not-yet-effective and retired versions; effective_from / effective_to are the only source of status",
		emptyTitle: "No matching price versions",
		emptyHint:
			"When the price table is empty, every session is rejected with price_missing; remember to enter real prices before launch.",
		columnScope: "Scope",
		columnUnitPrice: "Unit price",
		columnRounding: "Rounding",
		columnMinCharge: "Min charge",
		columnEffectiveRange: "Effective range",
		tiersSummary: "{{n}} tiers:",
		adjust: "Adjust",
		deactivate: "Retire",
		retiredHint: "Retired; referenced by historical bills only",
	},
	price: {
		catalogEmpty:
			"The billing item catalog is empty: confirm the server seed has run before entering prices (when the price table is empty, every session is rejected with price_missing).",
		titleCreate: "New price version",
		titleEdit: "Edit not-yet-effective price version",
		titleAdjust: "Adjust price (add a price version)",
		adjustNotice:
			"Effective versions are never edited in place: the values below are saved as a new price version, and the old version's effective_to is set to the new version's effective time. Settlement uses the price at the event time, so historical bills stay unchanged.",
		scopeLabel: "Scope",
		currentEffective: "Currently effective",
		noteEdit:
			"This version is not yet effective and has never matched any usage event; the item and scope are fixed at creation, so only the price itself can be changed here.",
		noteAdjust:
			"The values below are saved as a new version; the old version is retired from the effective time above. The item and scope are copied from the current version.",
		itemPlaceholder: "Select an item",
		effectiveFrom: "Effective from",
		effectiveHintAdjust:
			"Leave empty to use \"now\"; this moment also retires the old version.",
		effectiveHintEdit:
			"Editing a not-yet-effective version; changing the effective time does not affect bills already generated.",
		effectiveHintCreate:
			"Leave empty to use \"now\"; only one price version per scope can exist at any moment.",
		unitLabel: "Unit",
		modeLabel: "Mode",
		defaultRoundingLabel: "Default rounding",
		roundingHint: "duration items default to rounding up; usage items default to exact",
		unitPrice: "Unit price",
		unitPricePer: "X yuan per N {{unit}}",
		per: "per",
		countUnit: "units",
		charge: "Charge",
		currencyUnit: "yuan",
		unitSizeHint: "unit_size must be a positive integer",
		maxDecimals: "up to 6 decimal places",
		invalidFormat: "Invalid format",
		microEquals: "unit_price_micro = {{amount}}",
		converted: "Converted",
		microNote:
			"Unit prices must be integer micro-yuan (1e-6 yuan); if the precision is insufficient, raise unit_size instead of rounding the price to 0: for example, \"¥0.0000008 per token\" becomes \"¥8 per 10 million tokens\" (unit_price_micro = 8,000,000, unit_size = 10,000,000). At settlement, qty × unit price ÷ unit_size is rounded only once.",
		minCharge: "Min charge (yuan)",
		minChargePlaceholder: "empty = 0",
		minChargeHint: "after rounding, takes max(qty × unit price, min charge)",
		roundingLabel: "Rounding",
		currency: "Currency",
		currencyHint: "Single-currency deployment; no FX in P1",
		resourceTypeLabel: "Resource level",
		resourceTypeItem: "Billing item (platform-wide fallback price)",
		priorityHint: "Match priority: account price → voice → model → provider → item",
		resourceLabel: "Resource",
		idHintItem: "(leave empty for the item level)",
		idHintOther: "(required at the {{type}} level; enter the internal record ID)",
		manualToggle: "Enter ID manually",
		listToggle: "Pick from list",
		lockedPlaceholder: "item-level prices have no resource_id",
		manualPlaceholder:
			"Record ID from the database (not the provider's model / voice name)",
		selectPlaceholder: "Select {{type}}",
		resourceHintEdit: "Changing the resource requires a new version; it cannot be edited in place.",
		resourceHintItem:
			"item-level prices have no resource_id and apply to all resources (fallback price).",
		resourceHintManual:
			"Enter the database record ID (ai_models.id / providers.id / model_voices.id), not the provider-side model or voice name—a typo will not error, it simply never matches.",
		resourceHintSelect:
			"The dropdown lists resources visible to this account; for other users' resources or when the list fails to load, switch to \"Enter ID manually\".",
		resourceMissing:
			"The {{type}} list failed to load; enter the ID manually. Refresh the page to retry.",
		tiers: "Tiered pricing",
		tiersHint: "optional; progressive tiers by cumulative usage in the period",
		addTier: "Add tier",
		noTiers: "Leave empty for no tiers; everything is billed at the unit price above.",
		tierN: "Tier {{n}}",
		tierUpToPlaceholder: "Cumulative cap (leave the last tier empty = uncapped)",
		tierAmountPlaceholder: "Unit price (yuan / {{size}} units)",
		removeTier: "Remove this tier",
		tiersNote:
			"Tiers share the same unit_size; intermediate tiers must have a cap and strictly increase; only the last tier may be empty. Tiers are progressive (like income tax), not a flat new price once reached.",
		tierUpToInteger: "Tier {{n}} cap must be a positive integer",
		tierUpToRequired:
			"Tier {{n}} has no cap yet: only the last tier may be uncapped",
		tierUpToIncreasing:
			"Tier {{n}} cap must be greater than the previous tier (currently {{previous}})",
		tierAmountInvalid: "Tier {{n}} has an invalid unit price",
		notInList: "{{id}} (not in list)",
		submitCreate: "Add price",
		submitEdit: "Save changes",
		submitAdjust: "Create new version",
		failCreate: "Failed to add price",
		failEdit: "Failed to save",
		failAdjust: "Failed to adjust price",
		errUnitSize:
			"The unit size must be a positive integer: e.g. X yuan per 1,000,000 tokens.",
		errUnitPrice: "Invalid unit price: numbers only, up to 6 decimal places.",
		errMinCharge: "Invalid min charge: numbers only, up to 6 decimal places.",
		errResourceRequired:
			"A resource ID is required at the {{type}} level; use the item level for a platform-wide fallback price.",
		errEffectiveOrder:
			"The new version's effective time must be later than the current version (effective from {{date}}), otherwise the old version cannot be closed off.",
		successEdit:
			"Saved changes to the not-yet-effective price version for \"{{item}}\" ({{scope}})",
		adjustPartialFail:
			"New version created (effective from {{date}}), but auto-retiring the old version failed: {{error}}. Please retire the old version manually in the list.",
		successAdjust:
			"Repriced: \"{{item}}\" ({{scope}}); the new version is effective from {{date}}, the old version is retired at the same time, and historical bills are unchanged",
		successCreate: "Added price version: {{item}} · {{price}}",
	},
	stats: {
		loadAccountsFailed: "Failed to load accounts",
		loadFailed: "Failed to load report",
		disabledHint:
			"The billing module (billing.enabled) is not enabled on the server; reports are unavailable.",
		accountsLoading: "Loading accounts...",
		noAccounts: "No accounts available",
		selectAccount: "Select an account",
		clearPeriod: "Clear (this period)",
		pickAccountTitle: "Select an account first",
		pickAccountHint:
			"Reports aggregate by account; account_id is required for this endpoint. Leave the time range empty for the current period to date.",
		cardBalance: "Available balance",
		cardBalanceHint: "Can be negative = postpaid arrears",
		cardFrozen: "Frozen",
		cardFrozenHint: "Reserved for session pre-authorization",
		cardSpend: "Period spend",
		topItem: "Top: {{item}}",
		noSpend: "No spend in this period",
		cardCreditLimit: "Credit limit",
		cardCreditLimitHint: "0 = prepaid only",
		panelTitle: "Usage by item",
		panelDescription:
			"Based on settled usage events; quantity semantics follow each item's unit",
		emptyTitle: "No settled usage in this period",
		emptyHint:
			"No usage has been generated yet, or events are still pending settlement.",
		columnQuantity: "Quantity",
		columnShare: "Share",
		total: "Total",
		noData: "No report data",
		footer:
			"Reports count settled usage only. The data plane reports at turn boundaries and the control-plane worker runs every 1–5 seconds, so pending events make reports lag the ledger by one settlement delay (check the pending count first if the numbers disagree).",
	},
};
