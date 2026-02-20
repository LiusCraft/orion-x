import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildAdminToolItemOffersPath,
  buildAdminToolMarketItemsPath,
  buildAdminToolMarketItemPath,
  buildAdminToolOfferCreatePath,
  buildToolActivatePath,
  buildToolItemOffersPath,
  buildToolMarketItemsPath,
  buildToolOffersPath,
  buildToolRepoToolsCallPath,
  buildToolRepoToolsListPath,
  buildToolRepoUsagePath,
  filterEntitlementsByStatus,
  getEntitlementDisplayStatus,
  normalizeToolMarketItemsResponse,
  normalizeToolOffersResponse,
  normalizeToolRepoResponse,
  normalizeToolRuntimeToolsResponse,
  normalizeToolUsageResponse
} from './toolMarketModel.js';

test('buildToolMarketItemsPath keeps active status by default', () => {
  const path = buildToolMarketItemsPath({
    protocol: '',
    status: 'active'
  });

  assert.equal(path, '/api/v1/tool-market/items?status=active');
});

test('buildToolMarketItemsPath applies protocol and status filters', () => {
  const path = buildToolMarketItemsPath({
    protocol: 'mcp',
    status: 'inactive'
  });

  assert.equal(path, '/api/v1/tool-market/items?protocol=mcp&status=inactive');
});

test('buildToolOffersPath supports item and status query', () => {
  const path = buildToolOffersPath({
    itemID: 'item-1',
    status: 'active'
  });

  assert.equal(path, '/api/v1/tool-market/offers?item_id=item-1&status=active');
});

test('buildAdminToolMarketItemsPath applies protocol and status filters', () => {
  const path = buildAdminToolMarketItemsPath({
    protocol: 'mcp',
    status: 'active'
  });

  assert.equal(path, '/api/v1/admin/tool-market/items?protocol=mcp&status=active');
});

test('buildAdminToolMarketItemPath returns collection and detail paths', () => {
  assert.equal(buildAdminToolMarketItemPath(), '/api/v1/admin/tool-market/items');
  assert.equal(
    buildAdminToolMarketItemPath('item/1'),
    '/api/v1/admin/tool-market/items/item%2F1'
  );
});

test('buildAdminToolOfferCreatePath returns offer create path', () => {
  assert.equal(
    buildAdminToolOfferCreatePath('item/1'),
    '/api/v1/admin/tool-market/items/item%2F1/offers'
  );
});

test('buildAdminToolItemOffersPath returns admin offers path', () => {
  assert.equal(
    buildAdminToolItemOffersPath('item/1'),
    '/api/v1/admin/tool-market/items/item%2F1/offers'
  );
});

test('buildToolItemOffersPath and buildToolActivatePath encode ids', () => {
  assert.equal(
    buildToolItemOffersPath('item/with/slash'),
    '/api/v1/tool-market/items/item%2Fwith%2Fslash/offers'
  );
  assert.equal(
    buildToolActivatePath('item/with/slash'),
    '/api/v1/tool-market/items/item%2Fwith%2Fslash/activate'
  );
});

test('buildToolRepoUsagePath includes pagination query', () => {
  const path = buildToolRepoUsagePath('ent-1', {
    limit: 20,
    offset: 40
  });

  assert.equal(path, '/api/v1/me/tool-repo/ent-1/usage?limit=20&offset=40');
});

test('buildToolRepoToolsListPath and buildToolRepoToolsCallPath encode id', () => {
  assert.equal(
    buildToolRepoToolsListPath('ent/1'),
    '/api/v1/me/tool-repo/ent%2F1/tools/list'
  );
  assert.equal(
    buildToolRepoToolsCallPath('ent/1'),
    '/api/v1/me/tool-repo/ent%2F1/tools/call'
  );
});

test('normalizeToolMarketItemsResponse maps and sorts by recency', () => {
  const items = normalizeToolMarketItemsResponse({
    items: [
      {
        id: 'older',
        tool_key: 'legacy-tool',
        name: 'Legacy Tool',
        protocol: 'mcp',
        status: 'active',
        updated_at: '2026-02-19T08:00:00Z'
      },
      {
        id: 'newer',
        tool_key: 'new-tool',
        name: 'New Tool',
        protocol: 'mcp',
        status: 'inactive',
        updated_at: '2026-02-19T10:00:00Z'
      }
    ]
  });

  assert.equal(items.length, 2);
  assert.equal(items[0].id, 'newer');
  assert.equal(items[0].status, 'inactive');
  assert.equal(items[1].id, 'older');
});

test('normalizeToolOffersResponse normalizes wrappers and values', () => {
  const offers = normalizeToolOffersResponse({
    offers: [
      {
        id: 'off-1',
        tool_item_id: 'item-1',
        offer_type: 'activation_code',
        status: 'active',
        quota_total: '100',
        duration_seconds: 86400,
        updated_at: '2026-02-19T10:00:00Z'
      },
      {
        id: 'off-2',
        tool_item_id: 'item-1',
        offer_type: 'admin_grant',
        status: 'inactive',
        updated_at: '2026-02-19T12:00:00Z'
      }
    ]
  });

  assert.equal(offers.length, 2);
  assert.equal(offers[0].id, 'off-1');
  assert.equal(offers[0].offerType, 'activation_code');
  assert.equal(offers[0].quotaTotal, 100);
  assert.equal(offers[1].id, 'off-2');
});

test('normalizeToolRepoResponse handles nested item and quotas', () => {
  const entitlements = normalizeToolRepoResponse({
    entitlements: [
      {
        id: 'ent-1',
        tool_item_id: 'item-1',
        status: 'active',
        starts_at: '2026-02-19T10:00:00Z',
        expires_at: '2026-02-20T10:00:00Z',
        quota_total: 1000,
        quota_used: 56,
        item: {
          name: 'Device Control'
        },
        offer: {
          offer_type: 'activation_code'
        },
        updated_at: '2026-02-19T12:00:00Z'
      }
    ]
  });

  assert.equal(entitlements.length, 1);
  assert.equal(entitlements[0].toolName, 'Device Control');
  assert.equal(entitlements[0].offerType, 'activation_code');
  assert.equal(entitlements[0].quotaUsed, 56);
  assert.equal(entitlements[0].quotaTotal, 1000);
});

test('normalizeToolRepoResponse supports repo entry wrapper shape', () => {
  const entitlements = normalizeToolRepoResponse({
    items: [
      {
        is_usable: true,
        entitlement: {
          id: 'ent-2',
          tool_item_id: 'item-2',
          status: 'active',
          starts_at: '2026-02-19T10:00:00Z',
          quota_used: 2,
          updated_at: '2026-02-19T13:00:00Z'
        },
        tool_item: {
          name: 'Smart Home Control'
        }
      }
    ]
  });

  assert.equal(entitlements.length, 1);
  assert.equal(entitlements[0].id, 'ent-2');
  assert.equal(entitlements[0].toolItemID, 'item-2');
  assert.equal(entitlements[0].toolName, 'Smart Home Control');
  assert.equal(entitlements[0].isUsable, true);
});

test('getEntitlementDisplayStatus marks active-but-expired as expired', () => {
  const status = getEntitlementDisplayStatus(
    {
      status: 'active',
      expiresAt: '2026-02-19T10:00:00Z'
    },
    Date.parse('2026-02-20T10:00:00Z')
  );

  assert.equal(status, 'expired');
});

test('filterEntitlementsByStatus uses display status resolution', () => {
  const entitlements = [
    {
      id: 'active-1',
      status: 'active',
      expiresAt: '2026-02-22T10:00:00Z'
    },
    {
      id: 'expired-1',
      status: 'active',
      expiresAt: '2026-02-18T10:00:00Z'
    },
    {
      id: 'revoked-1',
      status: 'revoked'
    }
  ];

  const filtered = filterEntitlementsByStatus(
    entitlements,
    'expired',
    Date.parse('2026-02-20T10:00:00Z')
  );

  assert.deepEqual(
    filtered.map((item) => item.id),
    ['expired-1']
  );
});

test('normalizeToolUsageResponse sorts records and parses consumed units', () => {
  const records = normalizeToolUsageResponse({
    records: [
      {
        id: 'r1',
        consumed_units: '1',
        created_at: '2026-02-19T11:00:00Z'
      },
      {
        id: 'r2',
        consumed_units: 3,
        created_at: '2026-02-19T12:00:00Z'
      }
    ]
  });

  assert.equal(records.length, 2);
  assert.equal(records[0].id, 'r2');
  assert.equal(records[0].consumedUnits, 3);
  assert.equal(records[1].id, 'r1');
  assert.equal(records[1].consumedUnits, 1);
});

test('normalizeToolRuntimeToolsResponse supports object and string payloads', () => {
  const tools = normalizeToolRuntimeToolsResponse({
    tools: [
      {
        name: 'set_device_status',
        description: 'Set device state',
        input_schema: {
          type: 'object'
        }
      },
      'get_device_status',
      {
        tool_name: 'get_device_status',
        description: 'duplicated'
      }
    ]
  });

  assert.deepEqual(tools, [
    {
      name: 'get_device_status',
      description: '',
      inputSchema: {}
    },
    {
      name: 'set_device_status',
      description: 'Set device state',
      inputSchema: {
        type: 'object'
      }
    }
  ]);
});

test('normalizeToolRuntimeToolsResponse supports tool_names fallback', () => {
  const tools = normalizeToolRuntimeToolsResponse({
    data: {
      tool_names: ['tool_a', 'tool_b']
    }
  });

  assert.deepEqual(tools, [
    {
      name: 'tool_a',
      description: '',
      inputSchema: {}
    },
    {
      name: 'tool_b',
      description: '',
      inputSchema: {}
    }
  ]);
});
