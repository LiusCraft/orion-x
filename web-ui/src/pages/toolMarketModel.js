const MARKET_ITEM_STATUS_SET = new Set(['active', 'inactive']);
const OFFER_STATUS_SET = new Set(['active', 'inactive']);
const OFFER_TYPE_SET = new Set([
  'free',
  'trial',
  'paid',
  'activation_code',
  'admin_grant',
  'usage_pack',
  'time_limited'
]);
const ENTITLEMENT_STATUS_SET = new Set([
  'pending',
  'active',
  'expired',
  'revoked'
]);

export const MARKET_STATUS_OPTIONS = Object.freeze([
  { value: 'all', label: 'All statuses' },
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' }
]);

export const TOOL_MARKET_ITEM_STATUS_OPTIONS = Object.freeze([
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' }
]);

export const MARKET_PROTOCOL_OPTIONS = Object.freeze([
  { value: '', label: 'All protocols' },
  { value: 'mcp', label: 'MCP' }
]);

export const REPO_STATUS_OPTIONS = Object.freeze([
  { value: 'all', label: 'All statuses' },
  { value: 'active', label: 'Active' },
  { value: 'pending', label: 'Pending' },
  { value: 'expired', label: 'Expired' },
  { value: 'revoked', label: 'Revoked' }
]);

export const TOOL_OFFER_STATUS_OPTIONS = Object.freeze([
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' }
]);

export const TOOL_OFFER_TYPE_OPTIONS = Object.freeze([
  { value: 'free', label: 'Free' },
  { value: 'trial', label: 'Trial' },
  { value: 'paid', label: 'Paid' },
  { value: 'activation_code', label: 'Activation Code' },
  { value: 'admin_grant', label: 'Admin Grant' },
  { value: 'usage_pack', label: 'Usage Pack' },
  { value: 'time_limited', label: 'Time Limited' }
]);

export const DEFAULT_MARKET_FILTERS = Object.freeze({
  protocol: '',
  status: 'active'
});

export const DEFAULT_REPO_FILTERS = Object.freeze({
  status: 'all'
});

function normalizeString(value) {
  if (typeof value !== 'string') {
    return '';
  }
  return value.trim();
}

function normalizeLower(value) {
  return normalizeString(value).toLowerCase();
}

function normalizeObject(value) {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value;
  }
  return {};
}

function normalizeStringArray(value) {
  if (!Array.isArray(value)) {
    return [];
  }

  return value
    .map((item) => normalizeString(String(item)))
    .filter(Boolean);
}

function normalizeStatus(value, allowedSet, fallback) {
  const normalized = normalizeLower(value);
  if (allowedSet.has(normalized)) {
    return normalized;
  }
  return fallback;
}

function normalizeNullableInteger(value) {
  if (value === null || value === undefined || value === '') {
    return null;
  }

  const parsed = Number.parseInt(String(value), 10);
  if (!Number.isInteger(parsed) || parsed < 0) {
    return null;
  }

  return parsed;
}

function normalizeInteger(value, fallback = 0) {
  const parsed = Number.parseInt(String(value), 10);
  if (!Number.isInteger(parsed) || parsed < 0) {
    return fallback;
  }
  return parsed;
}

function normalizeNullableNumber(value) {
  if (value === null || value === undefined || value === '') {
    return null;
  }

  const parsed = Number.parseFloat(String(value));
  if (!Number.isFinite(parsed)) {
    return null;
  }

  return parsed;
}

function parseTimestamp(rawValue) {
  if (rawValue === null || rawValue === undefined || rawValue === '') {
    return null;
  }

  if (typeof rawValue === 'number' && Number.isFinite(rawValue)) {
    if (rawValue > 1_000_000_000_000) {
      return Math.floor(rawValue);
    }
    return Math.floor(rawValue * 1000);
  }

  const source = normalizeString(String(rawValue));
  if (!source) {
    return null;
  }

  const numeric = Number(source);
  if (Number.isFinite(numeric)) {
    return parseTimestamp(numeric);
  }

  const parsed = Date.parse(source);
  if (Number.isNaN(parsed)) {
    return null;
  }

  return parsed;
}

function toSortTimestamp(rawValue) {
  return parseTimestamp(rawValue) || 0;
}

function extractList(payload, keys) {
  if (Array.isArray(payload)) {
    return payload;
  }

  if (!payload || typeof payload !== 'object') {
    return [];
  }

  for (const key of keys) {
    if (Array.isArray(payload[key])) {
      return payload[key];
    }
  }

  if (payload.data && typeof payload.data === 'object') {
    for (const key of keys) {
      if (Array.isArray(payload.data[key])) {
        return payload.data[key];
      }
    }
  }

  return [];
}

function normalizeOfferType(offerType) {
  const normalized = normalizeLower(offerType);
  if (OFFER_TYPE_SET.has(normalized)) {
    return normalized;
  }

  if (!normalized) {
    return 'free';
  }

  return normalized;
}

function normalizeToolMarketItem(item) {
  const normalizedItem = normalizeObject(item);

  return {
    id: normalizeString(String(normalizedItem.id ?? normalizedItem.ID ?? '')),
    toolKey: normalizeString(
      normalizedItem.tool_key ?? normalizedItem.toolKey ?? normalizedItem.ToolKey ?? ''
    ),
    name: normalizeString(normalizedItem.name ?? normalizedItem.Name ?? ''),
    provider: normalizeString(normalizedItem.provider ?? normalizedItem.Provider ?? ''),
    protocol: normalizeLower(normalizedItem.protocol ?? normalizedItem.Protocol ?? 'mcp') || 'mcp',
    status: normalizeStatus(
      normalizedItem.status ?? normalizedItem.Status ?? 'inactive',
      MARKET_ITEM_STATUS_SET,
      'inactive'
    ),
    config: normalizeObject(normalizedItem.config ?? normalizedItem.Config ?? {}),
    createdAt: normalizeString(
      normalizedItem.created_at ?? normalizedItem.createdAt ?? normalizedItem.CreatedAt ?? ''
    ),
    updatedAt: normalizeString(
      normalizedItem.updated_at ?? normalizedItem.updatedAt ?? normalizedItem.UpdatedAt ?? ''
    )
  };
}

export function normalizeToolMarketItemsResponse(payload) {
  return extractList(payload, ['items', 'tool_market_items', 'toolMarketItems', 'list'])
    .map((item) => normalizeToolMarketItem(item))
    .filter((item) => item.id)
    .sort((left, right) => {
      const leftSort = toSortTimestamp(left.updatedAt || left.createdAt);
      const rightSort = toSortTimestamp(right.updatedAt || right.createdAt);
      return rightSort - leftSort;
    });
}

function normalizeToolOffer(offer) {
  const normalizedOffer = normalizeObject(offer);

  return {
    id: normalizeString(String(normalizedOffer.id ?? normalizedOffer.ID ?? '')),
    toolItemID: normalizeString(
      String(
        normalizedOffer.tool_item_id ??
          normalizedOffer.toolItemID ??
          normalizedOffer.toolItemId ??
          normalizedOffer.item_id ??
          normalizedOffer.itemID ??
          ''
      )
    ),
    offerType: normalizeOfferType(
      normalizedOffer.offer_type ?? normalizedOffer.offerType ?? normalizedOffer.type ?? 'free'
    ),
    price: normalizeNullableNumber(normalizedOffer.price),
    currency: normalizeString(normalizedOffer.currency).toUpperCase(),
    quotaTotal: normalizeNullableInteger(
      normalizedOffer.quota_total ?? normalizedOffer.quotaTotal
    ),
    durationSeconds: normalizeNullableInteger(
      normalizedOffer.duration_seconds ?? normalizedOffer.durationSeconds
    ),
    status: normalizeStatus(
      normalizedOffer.status ?? normalizedOffer.Status ?? 'inactive',
      OFFER_STATUS_SET,
      'inactive'
    ),
    createdAt: normalizeString(
      normalizedOffer.created_at ?? normalizedOffer.createdAt ?? normalizedOffer.CreatedAt ?? ''
    ),
    updatedAt: normalizeString(
      normalizedOffer.updated_at ?? normalizedOffer.updatedAt ?? normalizedOffer.UpdatedAt ?? ''
    )
  };
}

export function normalizeToolOffersResponse(payload) {
  return extractList(payload, ['offers', 'items', 'tool_offers', 'toolOffers', 'list'])
    .map((offer) => normalizeToolOffer(offer))
    .filter((offer) => offer.id)
    .sort((left, right) => {
      if (left.status !== right.status) {
        return left.status === 'active' ? -1 : 1;
      }

      const leftSort = toSortTimestamp(left.updatedAt || left.createdAt);
      const rightSort = toSortTimestamp(right.updatedAt || right.createdAt);
      return rightSort - leftSort;
    });
}

function normalizeMarketFilters(filters = {}) {
  const protocol = normalizeLower(filters.protocol);
  const status = normalizeLower(filters.status);

  return {
    protocol,
    status: status === 'active' || status === 'inactive' ? status : 'all'
  };
}

export function buildToolMarketItemsPath(filters = {}) {
  const normalized = normalizeMarketFilters(filters);
  const query = new URLSearchParams();

  if (normalized.protocol) {
    query.set('protocol', normalized.protocol);
  }

  if (normalized.status !== 'all') {
    query.set('status', normalized.status);
  }

  const search = query.toString();
  if (!search) {
    return '/api/v1/tool-market/items';
  }

  return `/api/v1/tool-market/items?${search}`;
}

export function buildAdminToolMarketItemsPath(filters = {}) {
  const normalized = normalizeMarketFilters(filters);
  const query = new URLSearchParams();

  if (normalized.protocol) {
    query.set('protocol', normalized.protocol);
  }

  if (normalized.status !== 'all') {
    query.set('status', normalized.status);
  }

  const search = query.toString();
  if (!search) {
    return '/api/v1/admin/tool-market/items';
  }

  return `/api/v1/admin/tool-market/items?${search}`;
}

export function buildToolOffersPath(filters = {}) {
  const itemID = normalizeString(filters.itemID ?? filters.item_id);
  const offerType = normalizeLower(filters.offerType ?? filters.offer_type);
  const status = normalizeLower(filters.status);
  const query = new URLSearchParams();

  if (itemID) {
    query.set('item_id', itemID);
  }

  if (offerType) {
    query.set('offer_type', offerType);
  }

  if (status === 'active' || status === 'inactive') {
    query.set('status', status);
  }

  const search = query.toString();
  if (!search) {
    return '/api/v1/tool-market/offers';
  }

  return `/api/v1/tool-market/offers?${search}`;
}

export function buildToolItemOffersPath(itemID) {
  const normalizedItemID = normalizeString(itemID);
  if (!normalizedItemID) {
    return '';
  }

  return `/api/v1/tool-market/items/${encodeURIComponent(normalizedItemID)}/offers`;
}

export function buildToolActivatePath(itemID) {
  const normalizedItemID = normalizeString(itemID);
  if (!normalizedItemID) {
    return '';
  }

  return `/api/v1/tool-market/items/${encodeURIComponent(normalizedItemID)}/activate`;
}

export function buildToolRepoPath() {
  return '/api/v1/me/tool-repo';
}

export function buildAdminToolMarketItemPath(itemID = '') {
  const normalizedItemID = normalizeString(itemID);
  if (!normalizedItemID) {
    return '/api/v1/admin/tool-market/items';
  }

  return `/api/v1/admin/tool-market/items/${encodeURIComponent(normalizedItemID)}`;
}

export function buildAdminToolOfferCreatePath(itemID) {
  const normalizedItemID = normalizeString(itemID);
  if (!normalizedItemID) {
    return '';
  }

  return `/api/v1/admin/tool-market/items/${encodeURIComponent(normalizedItemID)}/offers`;
}

export function buildAdminToolItemOffersPath(itemID) {
  const normalizedItemID = normalizeString(itemID);
  if (!normalizedItemID) {
    return '';
  }

  return `/api/v1/admin/tool-market/items/${encodeURIComponent(normalizedItemID)}/offers`;
}

export function buildToolRepoUsagePath(entitlementID, pagination = {}) {
  const normalizedEntitlementID = normalizeString(entitlementID);
  if (!normalizedEntitlementID) {
    return '';
  }

  const query = new URLSearchParams();
  const limit = normalizeNullableInteger(pagination.limit);
  const offset = normalizeNullableInteger(pagination.offset);

  if (limit !== null) {
    query.set('limit', String(Math.min(limit, 100)));
  }

  if (offset !== null) {
    query.set('offset', String(offset));
  }

  const search = query.toString();
  if (!search) {
    return `/api/v1/me/tool-repo/${encodeURIComponent(normalizedEntitlementID)}/usage`;
  }

  return `/api/v1/me/tool-repo/${encodeURIComponent(normalizedEntitlementID)}/usage?${search}`;
}

export function buildToolRepoToolsListPath(entitlementID) {
  const normalizedEntitlementID = normalizeString(entitlementID);
  if (!normalizedEntitlementID) {
    return '';
  }

  return `/api/v1/me/tool-repo/${encodeURIComponent(normalizedEntitlementID)}/tools/list`;
}

export function buildToolRepoToolsCallPath(entitlementID) {
  const normalizedEntitlementID = normalizeString(entitlementID);
  if (!normalizedEntitlementID) {
    return '';
  }

  return `/api/v1/me/tool-repo/${encodeURIComponent(normalizedEntitlementID)}/tools/call`;
}

function normalizeEntitlement(entitlement) {
  const entry = normalizeObject(entitlement);
  const normalizedEntitlement = normalizeObject(entry.entitlement ?? entry);
  const toolItem = normalizeObject(
    entry.tool_item ??
      entry.toolItem ??
      normalizedEntitlement.tool_item ??
      normalizedEntitlement.toolItem ??
      normalizedEntitlement.item ??
      normalizedEntitlement.market_item
  );
  const offer = normalizeObject(normalizedEntitlement.offer);

  return {
    id: normalizeString(String(normalizedEntitlement.id ?? normalizedEntitlement.ID ?? '')),
    userID: normalizeString(
      String(
        normalizedEntitlement.user_id ??
          normalizedEntitlement.userID ??
          normalizedEntitlement.userId ??
          ''
      )
    ),
    toolItemID: normalizeString(
      String(
        normalizedEntitlement.tool_item_id ??
          normalizedEntitlement.toolItemID ??
          normalizedEntitlement.toolItemId ??
          ''
      )
    ),
    toolName: normalizeString(
      normalizedEntitlement.tool_name ??
        normalizedEntitlement.toolName ??
        normalizedEntitlement.item_name ??
        normalizedEntitlement.itemName ??
        toolItem.name ??
        ''
    ),
    offerID: normalizeString(
      String(
        normalizedEntitlement.offer_id ??
          normalizedEntitlement.offerID ??
          normalizedEntitlement.offerId ??
          ''
      )
    ),
    offerType: normalizeOfferType(
      normalizedEntitlement.offer_type ??
        normalizedEntitlement.offerType ??
        offer.offer_type ??
        offer.offerType ??
        ''
    ),
    sourceType: normalizeLower(
      normalizedEntitlement.source_type ?? normalizedEntitlement.sourceType ?? ''
    ),
    sourceRef: normalizeString(
      normalizedEntitlement.source_ref ?? normalizedEntitlement.sourceRef ?? ''
    ),
    status: normalizeStatus(
      normalizedEntitlement.status ?? normalizedEntitlement.Status ?? 'pending',
      ENTITLEMENT_STATUS_SET,
      'pending'
    ),
    startsAt: normalizeString(
      normalizedEntitlement.starts_at ?? normalizedEntitlement.startsAt ?? ''
    ),
    expiresAt: normalizeString(
      normalizedEntitlement.expires_at ?? normalizedEntitlement.expiresAt ?? ''
    ),
    quotaTotal: normalizeNullableInteger(
      normalizedEntitlement.quota_total ?? normalizedEntitlement.quotaTotal
    ),
    quotaUsed: normalizeInteger(
      normalizedEntitlement.quota_used ?? normalizedEntitlement.quotaUsed,
      0
    ),
    createdAt: normalizeString(
      normalizedEntitlement.created_at ?? normalizedEntitlement.createdAt ?? ''
    ),
    updatedAt: normalizeString(
      normalizedEntitlement.updated_at ?? normalizedEntitlement.updatedAt ?? ''
    ),
    isUsable:
      entry.is_usable === true ||
      entry.isUsable === true ||
      normalizedEntitlement.is_usable === true ||
      normalizedEntitlement.isUsable === true
  };
}

export function normalizeToolRepoResponse(payload) {
  return extractList(payload, ['entitlements', 'items', 'tool_repo', 'toolRepo', 'list'])
    .map((entitlement) => normalizeEntitlement(entitlement))
    .filter((entitlement) => entitlement.id)
    .sort((left, right) => {
      const leftSort = toSortTimestamp(left.updatedAt || left.createdAt);
      const rightSort = toSortTimestamp(right.updatedAt || right.createdAt);
      return rightSort - leftSort;
    });
}

export function getEntitlementDisplayStatus(entitlement, nowTimestamp = Date.now()) {
  const normalizedEntitlement = normalizeObject(entitlement);
  const status = normalizeStatus(
    normalizedEntitlement.status,
    ENTITLEMENT_STATUS_SET,
    'pending'
  );

  if (status === 'expired' || status === 'revoked' || status === 'pending') {
    return status;
  }

  const expiresAtTimestamp = parseTimestamp(normalizedEntitlement.expiresAt);
  if (expiresAtTimestamp && expiresAtTimestamp <= nowTimestamp) {
    return 'expired';
  }

  return 'active';
}

export function filterEntitlementsByStatus(
  entitlements,
  status = 'all',
  nowTimestamp = Date.now()
) {
  const expectedStatus = normalizeLower(status);
  if (!Array.isArray(entitlements) || expectedStatus === 'all') {
    return Array.isArray(entitlements) ? entitlements : [];
  }

  return entitlements.filter(
    (entitlement) =>
      getEntitlementDisplayStatus(entitlement, nowTimestamp) === expectedStatus
  );
}

function normalizeUsageRecord(record) {
  const normalizedRecord = normalizeObject(record);

  return {
    id: normalizeString(String(normalizedRecord.id ?? normalizedRecord.ID ?? '')),
    entitlementID: normalizeString(
      String(
        normalizedRecord.entitlement_id ??
          normalizedRecord.entitlementID ??
          normalizedRecord.entitlementId ??
          ''
      )
    ),
    voicebotID: normalizeString(
      String(
        normalizedRecord.voicebot_id ??
          normalizedRecord.voicebotID ??
          normalizedRecord.voicebotId ??
          ''
      )
    ),
    deviceID: normalizeString(
      String(
        normalizedRecord.device_id ??
          normalizedRecord.deviceID ??
          normalizedRecord.deviceId ??
          ''
      )
    ),
    consumedUnits: normalizeInteger(
      normalizedRecord.consumed_units ?? normalizedRecord.consumedUnits,
      0
    ),
    createdAt: normalizeString(
      normalizedRecord.created_at ?? normalizedRecord.createdAt ?? ''
    )
  };
}

export function normalizeToolUsageResponse(payload) {
  return extractList(payload, ['records', 'items', 'usage', 'list'])
    .map((record) => normalizeUsageRecord(record))
    .filter((record) => record.id)
    .sort((left, right) => {
      const leftSort = toSortTimestamp(left.createdAt);
      const rightSort = toSortTimestamp(right.createdAt);
      return rightSort - leftSort;
    });
}

function normalizeRuntimeTool(tool) {
  if (typeof tool === 'string') {
    const normalizedName = normalizeString(tool);
    if (!normalizedName) {
      return null;
    }

    return {
      name: normalizedName,
      description: '',
      inputSchema: {}
    };
  }

  const normalizedTool = normalizeObject(tool);
  const normalizedName = normalizeString(
    normalizedTool.name ?? normalizedTool.tool_name ?? normalizedTool.toolName ?? ''
  );
  if (!normalizedName) {
    return null;
  }

  return {
    name: normalizedName,
    description: normalizeString(
      normalizedTool.description ?? normalizedTool.desc ?? normalizedTool.summary ?? ''
    ),
    inputSchema: normalizeObject(
      normalizedTool.input_schema ?? normalizedTool.inputSchema ?? normalizedTool.schema
    )
  };
}

export function normalizeToolRuntimeToolsResponse(payload) {
  const fromList = extractList(payload, [
    'tools',
    'tool_list',
    'toolList',
    'items',
    'list'
  ]);

  let normalizedList = fromList;
  if (!normalizedList.length) {
    const rootObject = normalizeObject(payload);
    const nestedData = normalizeObject(rootObject.data);
    const byStringList = normalizeStringArray(
      rootObject.tool_names ?? nestedData.tool_names ?? rootObject.toolNames ?? nestedData.toolNames
    );
    normalizedList = byStringList;
  }

  const dedup = new Set();
  const result = [];

  for (const rawItem of normalizedList) {
    const normalized = normalizeRuntimeTool(rawItem);
    if (!normalized || dedup.has(normalized.name)) {
      continue;
    }

    dedup.add(normalized.name);
    result.push(normalized);
  }

  return result.sort((left, right) => left.name.localeCompare(right.name));
}
