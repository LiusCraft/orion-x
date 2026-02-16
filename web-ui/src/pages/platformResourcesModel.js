const RESOURCE_CATEGORIES = ['llm', 'asr', 'tts'];
const RESOURCE_STATUSES = ['active', 'inactive'];

const PROVIDER_FIELD_TYPES = new Set(['text', 'number', 'integer', 'select']);
const PROVIDER_FIELD_PATH_PATTERN =
  /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/;
const RESERVED_PROVIDER_FIELD_KEYS = new Set(['base_url', 'access_key']);

function hasReservedProviderPathSegment(path) {
  const segments = String(path || '')
    .split('.')
    .map((segment) => segment.trim().toLowerCase())
    .filter(Boolean);

  return segments.some((segment) => RESERVED_PROVIDER_FIELD_KEYS.has(segment));
}

const PROVIDER_CONFIG_FIELDS = Object.freeze({
  llm: Object.freeze({
    dashscope: Object.freeze([
      Object.freeze({
        key: 'model',
        label: 'Model',
        type: 'text',
        required: true,
        defaultValue: 'qwen-plus',
        helperText: 'Model id used by DashScope compatible API.'
      }),
      Object.freeze({
        key: 'temperature',
        label: 'Temperature',
        type: 'number',
        required: false,
        min: 0,
        max: 2,
        step: 0.1
      }),
      Object.freeze({
        key: 'max_tokens',
        label: 'Max tokens',
        type: 'integer',
        required: false,
        min: 1,
        step: 1
      })
    ]),
    openai: Object.freeze([
      Object.freeze({
        key: 'model',
        label: 'Model',
        type: 'text',
        required: true,
        defaultValue: 'gpt-4o-mini'
      }),
      Object.freeze({
        key: 'temperature',
        label: 'Temperature',
        type: 'number',
        required: false,
        min: 0,
        max: 2,
        step: 0.1
      }),
      Object.freeze({
        key: 'max_tokens',
        label: 'Max tokens',
        type: 'integer',
        required: false,
        min: 1,
        step: 1
      })
    ]),
    zhipu: Object.freeze([
      Object.freeze({
        key: 'model',
        label: 'Model',
        type: 'text',
        required: true,
        defaultValue: 'glm-4-flash'
      }),
      Object.freeze({
        key: 'temperature',
        label: 'Temperature',
        type: 'number',
        required: false,
        min: 0,
        max: 1,
        step: 0.1
      }),
      Object.freeze({
        key: 'max_tokens',
        label: 'Max tokens',
        type: 'integer',
        required: false,
        min: 1,
        step: 1
      })
    ])
  }),
  asr: Object.freeze({
    dashscope: Object.freeze([
      Object.freeze({
        key: 'model',
        label: 'Model',
        type: 'text',
        required: true,
        defaultValue: 'fun-asr-realtime'
      }),
      Object.freeze({
        key: 'sample_rate_hz',
        label: 'Sample Rate (Hz)',
        type: 'integer',
        required: true,
        defaultValue: 16000,
        min: 8000,
        step: 1
      }),
      Object.freeze({
        key: 'format',
        label: 'Format',
        type: 'select',
        required: false,
        defaultValue: 'pcm',
        options: Object.freeze([
          Object.freeze({ value: 'pcm', label: 'pcm' }),
          Object.freeze({ value: 'wav', label: 'wav' })
        ])
      }),
      Object.freeze({
        key: 'language',
        label: 'Language',
        type: 'text',
        required: false,
        placeholder: 'zh-CN'
      })
    ])
  }),
  tts: Object.freeze({
    dashscope: Object.freeze([
      Object.freeze({
        key: 'model',
        label: 'Model',
        type: 'text',
        required: true,
        defaultValue: 'cosyvoice-v3-flash'
      }),
      Object.freeze({
        key: 'voice',
        label: 'Voice',
        type: 'text',
        required: true,
        defaultValue: 'longanyang'
      }),
      Object.freeze({
        key: 'sample_rate_hz',
        label: 'Sample Rate (Hz)',
        type: 'integer',
        required: false,
        min: 8000,
        step: 1
      }),
      Object.freeze({
        key: 'format',
        label: 'Format',
        type: 'select',
        required: false,
        defaultValue: 'pcm',
        options: Object.freeze([
          Object.freeze({ value: 'pcm', label: 'pcm' }),
          Object.freeze({ value: 'mp3', label: 'mp3' }),
          Object.freeze({ value: 'wav', label: 'wav' })
        ])
      })
    ])
  })
});

const PROVIDERS_BY_CATEGORY = Object.freeze(
  Object.fromEntries(
    Object.entries(PROVIDER_CONFIG_FIELDS).map(([category, providers]) => [
      category,
      Object.keys(providers)
    ])
  )
);

const ALL_PROVIDERS = Object.freeze(
  Array.from(
    new Set(
      Object.values(PROVIDERS_BY_CATEGORY).flatMap((providers) => providers)
    )
  )
);

const PROVIDER_BASE_URL_DEFAULTS = Object.freeze({
  dashscope: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  openai: 'https://api.openai.com/v1',
  zhipu: 'https://open.bigmodel.cn/api/paas/v4'
});

const RESOURCE_CATEGORY_SET = new Set(RESOURCE_CATEGORIES);
const RESOURCE_STATUS_SET = new Set(RESOURCE_STATUSES);
const FILTER_CATEGORY_SET = new Set(['all', ...RESOURCE_CATEGORIES]);
const FILTER_STATUS_SET = new Set(['all', ...RESOURCE_STATUSES]);

const PRIORITY_CONFIG_KEYS = [
  'model',
  'voice',
  'endpoint',
  'region',
  'sample_rate_hz'
];

const API_TO_FORM_FIELD_MAP = Object.freeze({
  resource_key: 'resourceKey',
  schema_version: 'schemaVersion',
  base_url: 'baseUrl',
  credential_ref: 'accessKey',
  access_key: 'accessKey',
  capabilities: 'capabilitiesText',
  config: 'providerConfig'
});

function cloneObject(value) {
  return JSON.parse(JSON.stringify(value));
}

function normalizeProviderTemplateField(rawField) {
  if (!rawField || typeof rawField !== 'object' || Array.isArray(rawField)) {
    return null;
  }

  const key = normalizeLower(rawField.key);
  const label = normalizeString(rawField.label);
  const type = normalizeLower(rawField.type);

  if (
    !key ||
    !PROVIDER_FIELD_PATH_PATTERN.test(key) ||
    hasReservedProviderPathSegment(key) ||
    !label ||
    !PROVIDER_FIELD_TYPES.has(type)
  ) {
    return null;
  }

  const normalizedField = {
    key,
    label,
    type,
    required: Boolean(rawField.required)
  };

  const defaultValue =
    rawField.default_value !== undefined ? rawField.default_value : rawField.defaultValue;
  if (typeof defaultValue === 'string') {
    normalizedField.defaultValue = defaultValue;
  } else if (typeof defaultValue === 'number') {
    normalizedField.defaultValue = defaultValue;
  } else if (typeof defaultValue === 'boolean') {
    normalizedField.defaultValue = defaultValue;
  }

  const helperText =
    rawField.helper_text !== undefined ? rawField.helper_text : rawField.helperText;
  if (typeof helperText === 'string') {
    normalizedField.helperText = helperText;
  }

  if (typeof rawField.placeholder === 'string') {
    normalizedField.placeholder = rawField.placeholder;
  }

  if (typeof rawField.min === 'number') {
    normalizedField.min = rawField.min;
  }

  if (typeof rawField.max === 'number') {
    normalizedField.max = rawField.max;
  }

  if (typeof rawField.step === 'number') {
    normalizedField.step = rawField.step;
  }

  if (type === 'select') {
    const optionCandidates = Array.isArray(rawField.options) ? rawField.options : [];

    const options = optionCandidates
      .map((option) => {
        if (!option || typeof option !== 'object' || Array.isArray(option)) {
          return null;
        }

        const value = normalizeString(option.value);
        const optionLabel = normalizeString(option.label) || value;
        if (!value) {
          return null;
        }

        return {
          value,
          label: optionLabel
        };
      })
      .filter(Boolean);

    if (options.length > 0) {
      normalizedField.options = options;
    }
  }

  return normalizedField;
}

export function normalizeProviderTemplateFields(fields) {
  if (!Array.isArray(fields)) {
    return [];
  }

  return fields
    .map((field) => normalizeProviderTemplateField(field))
    .filter(Boolean);
}

function resolveProviderTemplates(providerTemplates) {
  if (!providerTemplates || typeof providerTemplates !== 'object') {
    return PROVIDER_CONFIG_FIELDS;
  }

  return providerTemplates;
}

export function createDefaultProviderTemplates() {
  return cloneObject(PROVIDER_CONFIG_FIELDS);
}

function normalizeProviderTemplateItem(item) {
  const normalizedItem = item && typeof item === 'object' ? item : {};
  const category = normalizeLower(normalizedItem.category ?? normalizedItem.Category ?? '');
  const provider = normalizeLower(normalizedItem.provider ?? normalizedItem.Provider ?? '');
  const status = normalizeLower(normalizedItem.status ?? normalizedItem.Status ?? 'active');

  const fields = normalizeProviderTemplateFields(
    normalizedItem.fields ?? normalizedItem.Fields ?? []
  );

  return {
    id: normalizeString(String(normalizedItem.id ?? normalizedItem.ID ?? '')),
    category,
    provider,
    status: status === 'inactive' ? 'inactive' : 'active',
    version: normalizeSchemaVersion(
      normalizedItem.version ?? normalizedItem.Version ?? 1
    ),
    fields,
    createdBy: normalizeString(
      String(normalizedItem.created_by ?? normalizedItem.createdBy ?? '')
    ),
    createdAt: normalizeString(
      normalizedItem.created_at ?? normalizedItem.createdAt ?? ''
    ),
    updatedAt: normalizeString(
      normalizedItem.updated_at ?? normalizedItem.updatedAt ?? ''
    )
  };
}

function extractProviderTemplateItems(payload) {
  if (Array.isArray(payload)) {
    return payload;
  }

  if (!payload || typeof payload !== 'object') {
    return [];
  }

  if (Array.isArray(payload.items)) {
    return payload.items;
  }

  if (Array.isArray(payload.templates)) {
    return payload.templates;
  }

  if (Array.isArray(payload.list)) {
    return payload.list;
  }

  return [];
}

export function normalizeProviderTemplateListResponse(payload) {
  return extractProviderTemplateItems(payload)
    .map((item) => normalizeProviderTemplateItem(item))
    .filter(
      (item) =>
        RESOURCE_CATEGORY_SET.has(item.category) &&
        item.provider &&
        item.fields.length > 0
    )
    .sort((left, right) => {
      const leftTimestamp = toSortTimestamp(left.updatedAt || left.createdAt);
      const rightTimestamp = toSortTimestamp(right.updatedAt || right.createdAt);
      return rightTimestamp - leftTimestamp;
    });
}

export function buildProviderTemplatesFromTemplateItems(items, options = {}) {
  const includeInactive = options.includeInactive === true;
  const templates = {
    llm: {},
    asr: {},
    tts: {}
  };

  for (const item of items) {
    if (!item || (!includeInactive && item.status !== 'active')) {
      continue;
    }

    if (!RESOURCE_CATEGORY_SET.has(item.category) || !item.provider) {
      continue;
    }

    templates[item.category][item.provider] = item.fields;
  }

  return templates;
}

export function serializeProviderTemplateFieldsForApi(fields = []) {
  return fields.map((field) => {
    const payloadField = {
      key: field.key,
      label: field.label,
      type: field.type,
      required: Boolean(field.required)
    };

    if (field.defaultValue !== undefined) {
      payloadField.default_value = field.defaultValue;
    }

    if (field.helperText) {
      payloadField.helper_text = field.helperText;
    }

    if (field.placeholder) {
      payloadField.placeholder = field.placeholder;
    }

    if (typeof field.min === 'number') {
      payloadField.min = field.min;
    }

    if (typeof field.max === 'number') {
      payloadField.max = field.max;
    }

    if (typeof field.step === 'number') {
      payloadField.step = field.step;
    }

    if (field.type === 'select') {
      payloadField.options = (field.options || []).map((option) => ({
        value: option.value,
        label: option.label
      }));
    }

    return payloadField;
  });
}

export function normalizeProviderTemplates(providerTemplates) {
  if (!providerTemplates || typeof providerTemplates !== 'object') {
    return createDefaultProviderTemplates();
  }

  const normalizedTemplates = RESOURCE_CATEGORIES.reduce((result, category) => {
    result[category] = {};
    return result;
  }, {});

  for (const category of RESOURCE_CATEGORIES) {
    const providers = providerTemplates[category];
    if (!providers || typeof providers !== 'object' || Array.isArray(providers)) {
      continue;
    }

    for (const [providerKeyRaw, fieldsRaw] of Object.entries(providers)) {
      const providerKey = normalizeLower(providerKeyRaw);
      if (!providerKey) {
        continue;
      }

      const fields = normalizeProviderTemplateFields(fieldsRaw);

      if (fields.length > 0) {
        normalizedTemplates[category][providerKey] = fields;
      }
    }
  }

  const hasAnyProvider = RESOURCE_CATEGORIES.some(
    (category) => Object.keys(normalizedTemplates[category]).length > 0
  );

  if (!hasAnyProvider) {
    return createDefaultProviderTemplates();
  }

  return normalizedTemplates;
}

export const FILTER_CATEGORY_OPTIONS = Object.freeze([
  { value: 'all', label: 'All categories' },
  { value: 'llm', label: 'LLM' },
  { value: 'asr', label: 'ASR' },
  { value: 'tts', label: 'TTS' }
]);

export const FILTER_STATUS_OPTIONS = Object.freeze([
  { value: 'all', label: 'All statuses' },
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' }
]);

export const FILTER_PROVIDER_OPTIONS = Object.freeze([
  { value: '', label: 'All providers' },
  ...ALL_PROVIDERS.map((provider) => ({
    value: provider,
    label: provider
  }))
]);

export function buildFilterProviderOptions(providerTemplates) {
  const resolvedTemplates = resolveProviderTemplates(providerTemplates);

  const providers = Array.from(
    new Set(
      RESOURCE_CATEGORIES.flatMap((category) =>
        Object.keys(resolvedTemplates[category] || {})
      )
    )
  ).sort((left, right) => left.localeCompare(right));

  return [
    { value: '', label: 'All providers' },
    ...providers.map((provider) => ({
      value: provider,
      label: provider
    }))
  ];
}

export const FORM_CATEGORY_OPTIONS = Object.freeze([
  { value: 'llm', label: 'LLM' },
  { value: 'asr', label: 'ASR' },
  { value: 'tts', label: 'TTS' }
]);

export const FORM_STATUS_OPTIONS = Object.freeze([
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' }
]);

export const DEFAULT_FILTERS = Object.freeze({
  category: 'all',
  provider: '',
  status: 'all'
});

export const DEFAULT_RESOURCE_FORM = Object.freeze({
  category: 'llm',
  provider: '',
  resourceKey: '',
  name: '',
  schemaVersion: '1',
  status: 'active',
  baseUrl: '',
  accessKey: '',
  capabilitiesText: '{}',
  configText: '{}'
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

export function getDefaultBaseURLForProvider(provider) {
  const normalizedProvider = normalizeLower(provider);
  return PROVIDER_BASE_URL_DEFAULTS[normalizedProvider] || '';
}

function getPathSegments(path) {
  return normalizeLower(path)
    .split('.')
    .map((segment) => segment.trim())
    .filter(Boolean);
}

function getValueByPath(source, path) {
  const segments = getPathSegments(path);
  if (segments.length === 0) {
    return undefined;
  }

  let current = source;
  for (const segment of segments) {
    if (!current || typeof current !== 'object' || Array.isArray(current)) {
      return undefined;
    }

    if (!Object.prototype.hasOwnProperty.call(current, segment)) {
      return undefined;
    }

    current = current[segment];
  }

  return current;
}

function setValueByPath(target, path, value) {
  const segments = getPathSegments(path);
  if (segments.length === 0) {
    return;
  }

  let current = target;
  for (let index = 0; index < segments.length - 1; index += 1) {
    const segment = segments[index];
    const next = current[segment];

    if (!next || typeof next !== 'object' || Array.isArray(next)) {
      current[segment] = {};
    }

    current = current[segment];
  }

  const leafKey = segments[segments.length - 1];
  current[leafKey] = value;
}

function pruneEmptyObjects(node) {
  if (!node || typeof node !== 'object' || Array.isArray(node)) {
    return false;
  }

  for (const [key, value] of Object.entries(node)) {
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const shouldDelete = pruneEmptyObjects(value);
      if (shouldDelete) {
        delete node[key];
      }
    }
  }

  return Object.keys(node).length === 0;
}

function deleteValueByPath(target, path) {
  const segments = getPathSegments(path);
  if (segments.length === 0) {
    return;
  }

  const parents = [];
  let current = target;

  for (let index = 0; index < segments.length - 1; index += 1) {
    const segment = segments[index];
    if (!current || typeof current !== 'object' || Array.isArray(current)) {
      return;
    }

    if (!Object.prototype.hasOwnProperty.call(current, segment)) {
      return;
    }

    parents.push({
      parent: current,
      key: segment
    });
    current = current[segment];
  }

  if (!current || typeof current !== 'object' || Array.isArray(current)) {
    return;
  }

  delete current[segments[segments.length - 1]];

  for (let index = parents.length - 1; index >= 0; index -= 1) {
    const { parent, key } = parents[index];
    const child = parent[key];

    if (child && typeof child === 'object' && !Array.isArray(child)) {
      pruneEmptyObjects(child);
      if (Object.keys(child).length === 0) {
        delete parent[key];
      }
    }
  }
}

function sanitizeConfigForPayload(config) {
  const normalized = cloneObject(normalizeJsonObject(config));
  deleteValueByPath(normalized, 'base_url');
  deleteValueByPath(normalized, 'access_key');
  pruneEmptyObjects(normalized);
  return normalized;
}

export function getProviderValuesForCategory(category, providerTemplates) {
  const normalizedCategory = normalizeLower(category);
  const resolvedTemplates = resolveProviderTemplates(providerTemplates);

  return Object.keys(resolvedTemplates[normalizedCategory] || {}).sort((left, right) =>
    left.localeCompare(right)
  );
}

export function getProviderOptionsForCategory(
  category,
  currentProvider = '',
  providerTemplates
) {
  const values = getProviderValuesForCategory(category, providerTemplates);
  const normalizedCurrentProvider = normalizeLower(currentProvider);

  const options = values.map((provider) => ({
    value: provider,
    label: provider
  }));

  if (
    normalizedCurrentProvider &&
    !values.includes(normalizedCurrentProvider)
  ) {
    options.unshift({
      value: normalizedCurrentProvider,
      label: `${normalizedCurrentProvider} (unsupported)`
    });
  }

  return options;
}

export function getProviderConfigFields(category, provider, providerTemplates) {
  const normalizedCategory = normalizeLower(category);
  const normalizedProvider = normalizeLower(provider);

  const resolvedTemplates = resolveProviderTemplates(providerTemplates);
  const byCategory = resolvedTemplates[normalizedCategory];
  if (!byCategory) {
    return [];
  }

  return byCategory[normalizedProvider] || [];
}

export function buildProviderCatalogEntries(providerTemplates) {
  return RESOURCE_CATEGORIES.flatMap((category) =>
    getProviderValuesForCategory(category, providerTemplates).map((provider) => {
      const fields = getProviderConfigFields(category, provider, providerTemplates);
      const requiredFieldCount = fields.filter((field) => field.required).length;

      return {
        id: `${category}:${provider}`,
        category,
        provider,
        fields,
        requiredFieldCount
      };
    })
  );
}

function normalizeProviderFieldValue(value) {
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number') {
    return String(value);
  }
  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }
  return '';
}

export function buildProviderConfigFormValues(
  category,
  provider,
  config = {},
  providerTemplates
) {
  const fields = getProviderConfigFields(category, provider, providerTemplates);
  const normalizedConfig = normalizeJsonObject(config);

  return fields.reduce((result, field) => {
    const sourceValue = getValueByPath(normalizedConfig, field.key);
    if (sourceValue === undefined || sourceValue === null || sourceValue === '') {
      result[field.key] = normalizeProviderFieldValue(field.defaultValue);
      return result;
    }

    result[field.key] = normalizeProviderFieldValue(sourceValue);
    return result;
  }, {});
}

export function extractProviderConfigExtras(
  category,
  provider,
  config = {},
  providerTemplates
) {
  const normalizedConfig = cloneObject(normalizeJsonObject(config));
  const knownKeys = getProviderConfigFields(category, provider, providerTemplates).map(
    (field) => field.key
  );

  for (const key of knownKeys) {
    deleteValueByPath(normalizedConfig, key);
  }

  pruneEmptyObjects(normalizedConfig);
  return normalizedConfig;
}

export function buildProviderConfigPayload(
  category,
  provider,
  values = {},
  providerTemplates
) {
  const fields = getProviderConfigFields(category, provider, providerTemplates);
  if (fields.length === 0) {
    return {
      ok: false,
      config: {},
      fieldErrors: {
        _template: 'No config template found for the selected provider.'
      }
    };
  }

  const config = {};
  const fieldErrors = {};

  for (const field of fields) {
    const rawValue = normalizeProviderFieldValue(values[field.key]);
    const textValue = rawValue.trim();

    if (field.type === 'text') {
      if (!textValue) {
        if (field.required) {
          fieldErrors[field.key] = `${field.label} is required.`;
        }
        continue;
      }
      setValueByPath(config, field.key, textValue);
      continue;
    }

    if (field.type === 'select') {
      if (!textValue) {
        if (field.required) {
          fieldErrors[field.key] = `${field.label} is required.`;
        }
        continue;
      }

      const options = field.options || [];
      const optionValues = options.map((option) => option.value);
      if (optionValues.length > 0 && !optionValues.includes(textValue)) {
        fieldErrors[field.key] = `${field.label} must be one of: ${optionValues.join(', ')}.`;
        continue;
      }

      setValueByPath(config, field.key, textValue);
      continue;
    }

    if (field.type === 'integer' || field.type === 'number') {
      if (!textValue) {
        if (field.required) {
          fieldErrors[field.key] = `${field.label} is required.`;
        }
        continue;
      }

      const parsed =
        field.type === 'integer'
          ? Number.parseInt(textValue, 10)
          : Number.parseFloat(textValue);

      if (!Number.isFinite(parsed)) {
        fieldErrors[field.key] = `${field.label} must be a valid number.`;
        continue;
      }

      if (field.type === 'integer' && !Number.isInteger(parsed)) {
        fieldErrors[field.key] = `${field.label} must be an integer.`;
        continue;
      }

      if (typeof field.min === 'number' && parsed < field.min) {
        fieldErrors[field.key] = `${field.label} must be >= ${field.min}.`;
        continue;
      }

      if (typeof field.max === 'number' && parsed > field.max) {
        fieldErrors[field.key] = `${field.label} must be <= ${field.max}.`;
        continue;
      }

      setValueByPath(config, field.key, parsed);
      continue;
    }
  }

  return {
    ok: Object.keys(fieldErrors).length === 0,
    config,
    fieldErrors
  };
}

function normalizeJsonObject(value) {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value;
  }

  if (typeof value !== 'string') {
    return {};
  }

  const source = value.trim();
  if (!source) {
    return {};
  }

  try {
    const parsed = JSON.parse(source);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed;
    }
  } catch {
    return {};
  }

  return {};
}

function parseJsonObjectField(value, label) {
  const source = normalizeString(value);
  if (!source) {
    return { value: {}, error: '' };
  }

  try {
    const parsed = JSON.parse(source);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {
        value: {},
        error: `${label} must be a JSON object.`
      };
    }
    return { value: parsed, error: '' };
  } catch {
    return {
      value: {},
      error: `${label} must be valid JSON.`
    };
  }
}

function normalizeSchemaVersion(value) {
  const parsed = Number.parseInt(String(value), 10);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return 1;
  }
  return parsed;
}

function normalizeResourceItem(item) {
  const normalizedItem = item && typeof item === 'object' ? item : {};
  const config = normalizeJsonObject(normalizedItem.config ?? normalizedItem.Config ?? {});

  const category = normalizeLower(
    normalizedItem.category ?? normalizedItem.Category ?? ''
  );
  const status = normalizeLower(normalizedItem.status ?? normalizedItem.Status ?? '');

  return {
    id: normalizeString(String(normalizedItem.id ?? normalizedItem.ID ?? '')),
    category: RESOURCE_CATEGORY_SET.has(category) ? category : 'llm',
    provider: normalizeString(normalizedItem.provider ?? normalizedItem.Provider ?? ''),
    resourceKey: normalizeString(
      normalizedItem.resource_key ??
        normalizedItem.resourceKey ??
        normalizedItem.ResourceKey ??
        ''
    ),
    name: normalizeString(normalizedItem.name ?? normalizedItem.Name ?? ''),
    schemaVersion: normalizeSchemaVersion(
      normalizedItem.schema_version ??
        normalizedItem.schemaVersion ??
        normalizedItem.SchemaVersion ??
        1
    ),
    status: RESOURCE_STATUS_SET.has(status) ? status : 'inactive',
    capabilities: normalizeJsonObject(
      normalizedItem.capabilities ?? normalizedItem.Capabilities ?? {}
    ),
    config,
    baseUrl: normalizeString(
      normalizedItem.base_url ??
        normalizedItem.baseUrl ??
        normalizedItem.BaseURL ??
        config.base_url ??
        ''
    ),
    hasAccessKey: Boolean(
      normalizedItem.has_access_key ??
        normalizedItem.hasAccessKey ??
        normalizedItem.access_key_masked ??
        normalizedItem.accessKeyMasked ??
        normalizedItem.access_key ??
        normalizedItem.AccessKey ??
        normalizedItem.credential_ref ??
        normalizedItem.CredentialRef ??
        false
    ),
    accessKeyMasked: normalizeString(
      normalizedItem.access_key_masked ??
        normalizedItem.accessKeyMasked ??
        normalizedItem.AccessKeyMasked ??
        ''
    ),
    createdAt: normalizeString(
      normalizedItem.created_at ?? normalizedItem.createdAt ?? normalizedItem.CreatedAt ?? ''
    ),
    updatedAt: normalizeString(
      normalizedItem.updated_at ?? normalizedItem.updatedAt ?? normalizedItem.UpdatedAt ?? ''
    )
  };
}

function toSortTimestamp(value) {
  if (!value) {
    return 0;
  }
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function extractListItems(payload) {
  if (Array.isArray(payload)) {
    return payload;
  }

  if (!payload || typeof payload !== 'object') {
    return [];
  }

  if (Array.isArray(payload.items)) {
    return payload.items;
  }

  if (Array.isArray(payload.resources)) {
    return payload.resources;
  }

  if (Array.isArray(payload.list)) {
    return payload.list;
  }

  return [];
}

function mapApiFieldToFormField(field) {
  if (typeof field !== 'string') {
    return '';
  }

  return API_TO_FORM_FIELD_MAP[field] || field;
}

function normalizeFieldErrorMessage(value) {
  if (typeof value === 'string') {
    return value;
  }

  if (value && typeof value === 'object') {
    if (typeof value.message === 'string') {
      return value.message;
    }
    if (typeof value.error === 'string') {
      return value.error;
    }
  }

  return '';
}

function stringifyObject(value) {
  const normalized = normalizeJsonObject(value);
  return JSON.stringify(normalized, null, 2);
}

function summarizeConfigValue(value) {
  if (value === null || value === undefined) {
    return 'null';
  }

  if (Array.isArray(value)) {
    return `[${value.length} items]`;
  }

  if (typeof value === 'object') {
    return '{...}';
  }

  const source = String(value);
  if (source.length <= 36) {
    return source;
  }
  return `${source.slice(0, 33)}...`;
}

function sortObjectDeep(value) {
  if (Array.isArray(value)) {
    return value.map((item) => sortObjectDeep(item));
  }

  if (value && typeof value === 'object') {
    return Object.keys(value)
      .sort((left, right) => left.localeCompare(right))
      .reduce((result, key) => {
        result[key] = sortObjectDeep(value[key]);
        return result;
      }, {});
  }

  return value;
}

function deepEqual(left, right) {
  return JSON.stringify(sortObjectDeep(left)) === JSON.stringify(sortObjectDeep(right));
}

function buildComparablePayload(resource) {
  const normalized = normalizeResourceItem(resource);

  return {
    category: normalized.category,
    provider: normalizeLower(normalized.provider),
    resource_key: normalizeLower(normalized.resourceKey),
    name: normalized.name,
    schema_version: normalizeSchemaVersion(normalized.schemaVersion),
    status: normalized.status,
    capabilities: normalizeJsonObject(normalized.capabilities),
    config: sanitizeConfigForPayload(normalized.config),
    base_url: normalized.baseUrl
  };
}

export function normalizeFilters(filters = {}) {
  const category = normalizeLower(filters.category);
  const status = normalizeLower(filters.status);
  const provider = normalizeLower(filters.provider);

  return {
    category: FILTER_CATEGORY_SET.has(category) ? category : 'all',
    provider,
    status: FILTER_STATUS_SET.has(status) ? status : 'all'
  };
}

export function buildPlatformResourceListPath(filters = {}) {
  const normalized = normalizeFilters(filters);
  const query = new URLSearchParams();

  if (normalized.category !== 'all') {
    query.set('category', normalized.category);
  }
  if (normalized.provider) {
    query.set('provider', normalized.provider);
  }
  if (normalized.status !== 'all') {
    query.set('status', normalized.status);
  }

  const search = query.toString();
  if (!search) {
    return '/api/v1/platform-resources';
  }

  return `/api/v1/platform-resources?${search}`;
}

export function normalizeResourceListResponse(payload) {
  return extractListItems(payload)
    .map((item) => normalizeResourceItem(item))
    .sort((left, right) => {
      const leftTimestamp = toSortTimestamp(left.createdAt || left.updatedAt);
      const rightTimestamp = toSortTimestamp(right.createdAt || right.updatedAt);
      return rightTimestamp - leftTimestamp;
    });
}

export function buildResourceFormFromItem(item) {
  const normalized = normalizeResourceItem(item);

  return {
    category: normalized.category,
    provider: normalized.provider,
    resourceKey: normalized.resourceKey,
    name: normalized.name,
    schemaVersion: String(normalized.schemaVersion),
    status: normalized.status,
    baseUrl: normalized.baseUrl,
    accessKey: '',
    capabilitiesText: stringifyObject(normalized.capabilities),
    configText: stringifyObject(normalized.config)
  };
}

export function validateResourceForm(form, options = {}) {
  const mode = options.mode === 'edit' ? 'edit' : 'create';
  const providerTemplates = options.providerTemplates;
  const fieldErrors = {};

  const category = normalizeLower(form.category);
  const provider = normalizeLower(form.provider);
  const resourceKey = normalizeLower(form.resourceKey);
  const name = normalizeString(form.name);
  const status = normalizeLower(form.status);
  const baseUrl = normalizeString(form.baseUrl);
  const accessKey = normalizeString(form.accessKey);

  const schemaVersionRaw = normalizeString(form.schemaVersion);
  const schemaVersion = Number.parseInt(schemaVersionRaw, 10);

  if (!RESOURCE_CATEGORY_SET.has(category)) {
    fieldErrors.category = 'Category must be llm, asr, or tts.';
  }
  if (!provider) {
    fieldErrors.provider = 'Provider is required.';
  } else if (RESOURCE_CATEGORY_SET.has(category)) {
    const supportedProviders = getProviderValuesForCategory(category, providerTemplates);
    if (!supportedProviders.includes(provider)) {
      fieldErrors.provider = `Provider \"${provider}\" is not supported for ${category}.`;
    }
  }
  if (!resourceKey) {
    fieldErrors.resourceKey = 'Resource key is required.';
  }
  if (!name) {
    fieldErrors.name = 'Display name is required.';
  }
  if (!baseUrl) {
    fieldErrors.baseUrl = 'Base URL is required.';
  }
  if (!Number.isInteger(schemaVersion) || schemaVersion <= 0) {
    fieldErrors.schemaVersion = 'Schema version must be a positive integer.';
  }
  if (!RESOURCE_STATUS_SET.has(status)) {
    fieldErrors.status = 'Status must be active or inactive.';
  }
  if (mode === 'create' && !accessKey) {
    fieldErrors.accessKey = 'Access key is required.';
  }

  const capabilitiesResult = parseJsonObjectField(
    form.capabilitiesText,
    'Capabilities'
  );
  if (capabilitiesResult.error) {
    fieldErrors.capabilitiesText = capabilitiesResult.error;
  }

  const configResult = (() => {
    if (form.config && typeof form.config === 'object' && !Array.isArray(form.config)) {
      return {
        value: normalizeJsonObject(form.config),
        error: ''
      };
    }

    return parseJsonObjectField(form.configText, 'Config');
  })();

  if (configResult.error) {
    fieldErrors.providerConfig = configResult.error;
  }

  if (Object.keys(fieldErrors).length > 0) {
    return {
      ok: false,
      mode,
      fieldErrors,
      payload: null
    };
  }

  const payload = {
    category,
    provider,
    resource_key: resourceKey,
    name,
    schema_version: schemaVersion,
    status,
    base_url: baseUrl,
    capabilities: capabilitiesResult.value,
    config: sanitizeConfigForPayload(configResult.value)
  };

  if (accessKey) {
    payload.access_key = accessKey;
  }

  return {
    ok: true,
    mode,
    fieldErrors: {},
    payload
  };
}

export function buildPatchPayload(originalResource, validatedPayload) {
  const comparableOriginal = buildComparablePayload(originalResource);
  const comparableNext = buildComparablePayload({
    ...originalResource,
    category: validatedPayload.category,
    provider: validatedPayload.provider,
    resource_key: validatedPayload.resource_key,
    name: validatedPayload.name,
    schema_version: validatedPayload.schema_version,
    status: validatedPayload.status,
    base_url: validatedPayload.base_url,
    capabilities: validatedPayload.capabilities,
    config: validatedPayload.config
  });

  const patch = {};

  if (validatedPayload.access_key) {
    patch.access_key = validatedPayload.access_key;
  }

  for (const [field, value] of Object.entries(validatedPayload)) {
    if (field === 'access_key') {
      continue;
    }

    const previousValue = comparableOriginal[field];
    const nextValue = comparableNext[field];

    if (field === 'capabilities' || field === 'config') {
      if (!deepEqual(previousValue, nextValue)) {
        patch[field] = value;
      }
      continue;
    }

    if (previousValue !== nextValue) {
      patch[field] = value;
    }
  }

  return patch;
}

export function extractFieldErrorsFromApiDetails(details) {
  if (!details || typeof details !== 'object') {
    return {};
  }

  const candidates = [
    details.data?.errors,
    details.errors,
    details.data?.field_errors,
    details.field_errors
  ];

  for (const candidate of candidates) {
    if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) {
      continue;
    }

    const mapped = {};
    for (const [field, message] of Object.entries(candidate)) {
      const normalizedField = mapApiFieldToFormField(field);
      const normalizedMessage = normalizeFieldErrorMessage(message);
      if (!normalizedField || !normalizedMessage) {
        continue;
      }
      mapped[normalizedField] = normalizedMessage;
    }

    if (Object.keys(mapped).length > 0) {
      return mapped;
    }
  }

  return {};
}

export function pickConfigHighlights(config, limit = 3) {
  const normalized = normalizeJsonObject(config);
  const entries = [];

  for (const key of PRIORITY_CONFIG_KEYS) {
    if (Object.prototype.hasOwnProperty.call(normalized, key)) {
      entries.push([key, normalized[key]]);
    }
  }

  if (entries.length === 0) {
    entries.push(...Object.entries(normalized));
  }

  return entries.slice(0, limit).map(([key, value]) => ({
    key,
    value: summarizeConfigValue(value)
  }));
}
