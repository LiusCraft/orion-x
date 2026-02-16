import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildFilterProviderOptions,
  buildProviderCatalogEntries,
  buildPatchPayload,
  buildPlatformResourceListPath,
  buildProviderConfigFormValues,
  buildProviderConfigPayload,
  buildResourceFormFromItem,
  createDefaultProviderTemplates,
  extractProviderConfigExtras,
  extractFieldErrorsFromApiDetails,
  getProviderConfigFields,
  getProviderOptionsForCategory,
  normalizeProviderTemplateFields,
  normalizeProviderTemplates,
  normalizeResourceListResponse,
  pickConfigHighlights,
  validateResourceForm
} from './platformResourcesModel.js';

test('buildPlatformResourceListPath omits all filters', () => {
  const path = buildPlatformResourceListPath({
    category: 'all',
    provider: '',
    status: 'all'
  });

  assert.equal(path, '/api/v1/platform-resources');
});

test('buildPlatformResourceListPath includes selected filters', () => {
  const path = buildPlatformResourceListPath({
    category: 'llm',
    provider: 'DashScope',
    status: 'active'
  });

  assert.equal(
    path,
    '/api/v1/platform-resources?category=llm&provider=dashscope&status=active'
  );
});

test('normalizeResourceListResponse supports object wrapper and sorting', () => {
  const payload = {
    items: [
      {
        id: '2',
        category: 'llm',
        provider: 'zhipu',
        resource_key: 'llm-zhipu',
        schema_version: 2,
        status: 'active',
        updated_at: '2026-02-14T10:00:00Z'
      },
      {
        id: '1',
        category: 'tts',
        provider: 'dashscope',
        resource_key: 'tts-ds',
        schema_version: 1,
        status: 'inactive',
        updated_at: '2026-02-10T10:00:00Z'
      }
    ]
  };

  const normalized = normalizeResourceListResponse(payload);

  assert.equal(normalized.length, 2);
  assert.equal(normalized[0].id, '2');
  assert.equal(normalized[0].schemaVersion, 2);
  assert.equal(normalized[1].id, '1');
});

test('buildResourceFormFromItem converts nested values to editable fields', () => {
  const form = buildResourceFormFromItem({
    category: 'llm',
    provider: 'zhipu',
    resource_key: 'llm-zhipu-prod',
    name: 'Zhipu Prod',
    schema_version: 3,
    status: 'active',
    base_url: 'https://open.bigmodel.cn/api/paas/v4',
    access_key_masked: '****-prod',
    capabilities: { stream: true },
    config: { model: 'glm-4-flash' }
  });

  assert.equal(form.category, 'llm');
  assert.equal(form.schemaVersion, '3');
  assert.equal(form.baseUrl, 'https://open.bigmodel.cn/api/paas/v4');
  assert.equal(form.accessKey, '');
  assert.equal(form.capabilitiesText.includes('"stream": true'), true);
  assert.equal(form.configText.includes('"model": "glm-4-flash"'), true);
});

test('validateResourceForm returns payload when form is valid', () => {
  const result = validateResourceForm({
    category: 'llm',
    provider: 'ZhiPu',
    resourceKey: 'LLM-ZHIPU-PROD',
    name: 'Zhipu Prod',
    schemaVersion: '1',
    status: 'active',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    accessKey: 'zhipu-key-prod',
    capabilitiesText: '{"stream": true}',
    configText: '{"model": "glm-4-flash"}'
  });

  assert.equal(result.ok, true);
  assert.equal(result.payload.resource_key, 'llm-zhipu-prod');
  assert.equal(result.payload.provider, 'zhipu');
  assert.equal(result.payload.schema_version, 1);
  assert.equal(result.payload.base_url, 'https://open.bigmodel.cn/api/paas/v4');
  assert.equal(result.payload.access_key, 'zhipu-key-prod');
  assert.deepEqual(result.payload.capabilities, { stream: true });
});

test('validateResourceForm in edit mode keeps access_key optional', () => {
  const result = validateResourceForm(
    {
      category: 'llm',
      provider: 'zhipu',
      resourceKey: 'llm-zhipu-prod',
      name: 'Zhipu Prod',
      schemaVersion: '2',
      status: 'active',
      baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
      accessKey: '',
      capabilitiesText: '{}',
      configText: '{"model": "glm-4-flash"}'
    },
    {
      mode: 'edit'
    }
  );

  assert.equal(result.ok, true);
  assert.equal(Object.prototype.hasOwnProperty.call(result.payload, 'access_key'), false);
});

test('validateResourceForm strips base_url from config payload', () => {
  const result = validateResourceForm({
    category: 'llm',
    provider: 'zhipu',
    resourceKey: 'llm-zhipu-prod',
    name: 'Zhipu Prod',
    schemaVersion: '1',
    status: 'active',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    accessKey: 'zhipu-key-prod',
    capabilitiesText: '{}',
    configText:
      '{"model":"glm-4-flash","base_url":"https://legacy.example.com","access_key":"bad"}'
  });

  assert.equal(result.ok, true);
  assert.equal(result.payload.base_url, 'https://open.bigmodel.cn/api/paas/v4');
  assert.deepEqual(result.payload.config, {
    model: 'glm-4-flash'
  });
});

test('validateResourceForm enforces provider matrix by category', () => {
  const result = validateResourceForm({
    category: 'asr',
    provider: 'openai',
    resourceKey: 'asr-openai-prod',
    name: 'ASR OpenAI',
    schemaVersion: '1',
    status: 'active',
    baseUrl: 'https://api.openai.com/v1',
    accessKey: 'openai-asr-key',
    capabilitiesText: '{}',
    configText: '{}'
  });

  assert.equal(result.ok, false);
  assert.equal(
    result.fieldErrors.provider,
    'Provider "openai" is not supported for asr.'
  );
});

test('validateResourceForm reports json and required-field errors', () => {
  const result = validateResourceForm({
    category: 'llm',
    provider: '',
    resourceKey: '',
    name: '',
    schemaVersion: '0',
    status: 'active',
    baseUrl: '',
    accessKey: '',
    capabilitiesText: '[]',
    configText: '{bad json}'
  });

  assert.equal(result.ok, false);
  assert.equal(result.fieldErrors.provider, 'Provider is required.');
  assert.equal(result.fieldErrors.resourceKey, 'Resource key is required.');
  assert.equal(
    result.fieldErrors.schemaVersion,
    'Schema version must be a positive integer.'
  );
  assert.equal(result.fieldErrors.baseUrl, 'Base URL is required.');
  assert.equal(result.fieldErrors.accessKey, 'Access key is required.');
  assert.equal(
    result.fieldErrors.capabilitiesText,
    'Capabilities must be a JSON object.'
  );
  assert.equal(result.fieldErrors.providerConfig, 'Config must be valid JSON.');
});

test('extractFieldErrorsFromApiDetails maps snake case fields', () => {
  const fieldErrors = extractFieldErrorsFromApiDetails({
    errors: {
      resource_key: 'resource key already exists',
      schema_version: 'schema version must be >= 1',
      base_url: 'base url is invalid',
      access_key: 'access key is invalid'
    }
  });

  assert.deepEqual(fieldErrors, {
    resourceKey: 'resource key already exists',
    schemaVersion: 'schema version must be >= 1',
    baseUrl: 'base url is invalid',
    accessKey: 'access key is invalid'
  });
});

test('buildPatchPayload only keeps changed fields', () => {
  const original = {
    category: 'llm',
    provider: 'zhipu',
    resource_key: 'llm-zhipu-prod',
    name: 'Zhipu Prod',
    schema_version: 1,
    status: 'active',
    base_url: 'https://open.bigmodel.cn/api/paas/v4',
    capabilities: { stream: true },
    config: { model: 'glm-4-flash' },
    access_key_masked: '****-prod'
  };

  const patch = buildPatchPayload(original, {
    category: 'llm',
    provider: 'zhipu',
    resource_key: 'llm-zhipu-prod',
    name: 'Zhipu Production',
    schema_version: 1,
    status: 'active',
    base_url: 'https://open.bigmodel.cn/api/paas/v4',
    capabilities: { stream: true },
    config: { model: 'glm-4-air' },
    access_key: 'new-key-token'
  });

  assert.deepEqual(patch, {
    access_key: 'new-key-token',
    name: 'Zhipu Production',
    config: { model: 'glm-4-air' }
  });
});

test('getProviderOptionsForCategory includes unsupported provider label', () => {
  const options = getProviderOptionsForCategory('asr', 'custom-asr');
  assert.equal(options[0].value, 'custom-asr');
  assert.equal(options[0].label, 'custom-asr (unsupported)');
});

test('normalizeProviderTemplateFields sanitizes template fields', () => {
  const fields = normalizeProviderTemplateFields([
    {
      key: ' Model ',
      label: 'Model',
      type: 'text',
      required: true
    },
    {
      key: 'transport.retry.max',
      label: 'Max Retry',
      type: 'integer',
      required: false
    },
    {
      key: 'base_url',
      label: 'Base URL',
      type: 'text',
      required: true
    },
    {
      key: 'transport.access_key',
      label: 'Nested Access Key',
      type: 'text',
      required: false
    },
    {
      key: '',
      label: 'Invalid',
      type: 'text'
    }
  ]);

  assert.deepEqual(fields, [
    {
      key: 'model',
      label: 'Model',
      type: 'text',
      required: true
    },
    {
      key: 'transport.retry.max',
      label: 'Max Retry',
      type: 'integer',
      required: false
    }
  ]);
});

test('normalizeProviderTemplates sanitizes custom registry payload', () => {
  const templates = normalizeProviderTemplates({
    llm: {
      customai: [
        {
          key: 'model',
          label: 'Model',
          type: 'text',
          required: true
        }
      ]
    },
    asr: {},
    tts: {}
  });

  assert.deepEqual(Object.keys(templates.llm), ['customai']);
  assert.equal(templates.llm.customai[0].key, 'model');
});

test('buildFilterProviderOptions respects custom provider templates', () => {
  const templates = createDefaultProviderTemplates();
  templates.llm.customai = [
    {
      key: 'model',
      label: 'Model',
      type: 'text',
      required: true
    }
  ];

  const options = buildFilterProviderOptions(templates);
  assert.equal(options.some((option) => option.value === 'customai'), true);
});

test('buildProviderCatalogEntries flattens category-provider templates', () => {
  const entries = buildProviderCatalogEntries();
  assert.equal(entries.length > 0, true);

  const zhipuLLM = entries.find(
    (entry) => entry.category === 'llm' && entry.provider === 'zhipu'
  );

  assert.equal(Boolean(zhipuLLM), true);
  assert.equal(zhipuLLM.fields.some((field) => field.key === 'model'), true);
  assert.equal(zhipuLLM.requiredFieldCount > 0, true);
});

test('getProviderConfigFields returns template by category and provider', () => {
  const fields = getProviderConfigFields('llm', 'zhipu');
  assert.equal(fields.length > 0, true);
  assert.equal(fields.some((field) => field.key === 'model'), true);
  assert.equal(fields.some((field) => field.key === 'base_url'), false);
});

test('buildProviderConfigFormValues applies defaults and existing values', () => {
  const values = buildProviderConfigFormValues('asr', 'dashscope', {
    model: 'fun-asr-custom'
  });

  assert.equal(values.model, 'fun-asr-custom');
  assert.equal(values.sample_rate_hz, '16000');
  assert.equal(values.format, 'pcm');
});

test('buildProviderConfigFormValues reads nested config via dotted path', () => {
  const templates = createDefaultProviderTemplates();
  templates.llm.customnested = [
    {
      key: 'transport.host',
      label: 'Host',
      type: 'text',
      required: true
    },
    {
      key: 'transport.retry.max',
      label: 'Max Retry',
      type: 'integer',
      required: false,
      defaultValue: 2
    }
  ];

  const values = buildProviderConfigFormValues(
    'llm',
    'customnested',
    {
      transport: {
        host: 'https://api.example.com',
        retry: {
          max: 5
        }
      }
    },
    templates
  );

  assert.equal(values['transport.host'], 'https://api.example.com');
  assert.equal(values['transport.retry.max'], '5');
});

test('extractProviderConfigExtras keeps unknown config keys', () => {
  const extras = extractProviderConfigExtras('tts', 'dashscope', {
    model: 'cosyvoice-v3-flash',
    voice: 'longanyang',
    custom_timeout_ms: 25000
  });

  assert.deepEqual(extras, {
    custom_timeout_ms: 25000
  });
});

test('extractProviderConfigExtras keeps unknown nested config keys', () => {
  const templates = createDefaultProviderTemplates();
  templates.llm.customnested = [
    {
      key: 'transport.host',
      label: 'Host',
      type: 'text',
      required: true
    },
    {
      key: 'transport.retry.max',
      label: 'Max Retry',
      type: 'integer',
      required: false
    }
  ];

  const extras = extractProviderConfigExtras(
    'llm',
    'customnested',
    {
      transport: {
        host: 'https://api.example.com',
        retry: {
          max: 3
        },
        timeout_ms: 2500
      },
      feature_flag: true
    },
    templates
  );

  assert.deepEqual(extras, {
    transport: {
      timeout_ms: 2500
    },
    feature_flag: true
  });
});

test('buildProviderConfigPayload validates required provider fields', () => {
  const payload = buildProviderConfigPayload('tts', 'dashscope', {
    model: '',
    voice: ''
  });

  assert.equal(payload.ok, false);
  assert.equal(payload.fieldErrors.model, 'Model is required.');
  assert.equal(payload.fieldErrors.voice, 'Voice is required.');
});

test('buildProviderConfigPayload parses number fields', () => {
  const payload = buildProviderConfigPayload('asr', 'dashscope', {
    model: 'fun-asr-realtime',
    sample_rate_hz: '16000',
    format: 'wav'
  });

  assert.equal(payload.ok, true);
  assert.deepEqual(payload.config, {
    model: 'fun-asr-realtime',
    sample_rate_hz: 16000,
    format: 'wav'
  });
});

test('buildProviderConfigPayload writes dotted path into nested object', () => {
  const templates = createDefaultProviderTemplates();
  templates.llm.customnested = [
    {
      key: 'transport.host',
      label: 'Host',
      type: 'text',
      required: true
    },
    {
      key: 'transport.retry.max',
      label: 'Max Retry',
      type: 'integer',
      required: true,
      min: 0
    }
  ];

  const payload = buildProviderConfigPayload(
    'llm',
    'customnested',
    {
      'transport.host': 'https://api.example.com',
      'transport.retry.max': '4'
    },
    templates
  );

  assert.equal(payload.ok, true);
  assert.deepEqual(payload.config, {
    transport: {
      host: 'https://api.example.com',
      retry: {
        max: 4
      }
    }
  });
});

test('pickConfigHighlights prioritizes model keys', () => {
  const highlights = pickConfigHighlights({
    endpoint: 'https://example.com',
    model: 'glm-4-flash',
    timeout_ms: 30000
  });

  assert.deepEqual(highlights[0], {
    key: 'model',
    value: 'glm-4-flash'
  });
  assert.deepEqual(highlights[1], {
    key: 'endpoint',
    value: 'https://example.com'
  });
});
