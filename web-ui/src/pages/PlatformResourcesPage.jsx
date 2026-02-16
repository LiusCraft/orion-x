import * as React from 'react';
import AddRoundedIcon from '@mui/icons-material/AddRounded';
import AddCircleOutlineRoundedIcon from '@mui/icons-material/AddCircleOutlineRounded';
import BlockRoundedIcon from '@mui/icons-material/BlockRounded';
import CloudQueueRoundedIcon from '@mui/icons-material/CloudQueueRounded';
import DeleteOutlineRoundedIcon from '@mui/icons-material/DeleteOutlineRounded';
import EditRoundedIcon from '@mui/icons-material/EditRounded';
import Inventory2RoundedIcon from '@mui/icons-material/Inventory2Rounded';
import KeyboardArrowDownRoundedIcon from '@mui/icons-material/KeyboardArrowDownRounded';
import KeyboardArrowUpRoundedIcon from '@mui/icons-material/KeyboardArrowUpRounded';
import LaunchRoundedIcon from '@mui/icons-material/LaunchRounded';
import RefreshRoundedIcon from '@mui/icons-material/RefreshRounded';
import RemoveCircleOutlineRoundedIcon from '@mui/icons-material/RemoveCircleOutlineRounded';
import SearchRoundedIcon from '@mui/icons-material/SearchRounded';
import VisibilityOffRoundedIcon from '@mui/icons-material/VisibilityOffRounded';
import VisibilityRoundedIcon from '@mui/icons-material/VisibilityRounded';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Checkbox from '@mui/material/Checkbox';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Collapse from '@mui/material/Collapse';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import FormControlLabel from '@mui/material/FormControlLabel';
import IconButton from '@mui/material/IconButton';
import InputAdornment from '@mui/material/InputAdornment';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import useMediaQuery from '@mui/material/useMediaQuery';
import { useTheme } from '@mui/material/styles';
import { ManagerApiError } from '../api/managerClient.js';
import { formatAuthError, useAuth } from '../auth/AuthProvider.jsx';
import {
  buildProviderTemplatesFromTemplateItems,
  buildPatchPayload,
  buildPlatformResourceListPath,
  buildFilterProviderOptions,
  buildProviderConfigFormValues,
  buildProviderConfigPayload,
  buildResourceFormFromItem,
  DEFAULT_FILTERS,
  DEFAULT_RESOURCE_FORM,
  extractProviderConfigExtras,
  extractFieldErrorsFromApiDetails,
  FILTER_CATEGORY_OPTIONS,
  FILTER_STATUS_OPTIONS,
  FORM_CATEGORY_OPTIONS,
  FORM_STATUS_OPTIONS,
  getDefaultBaseURLForProvider,
  getProviderConfigFields,
  getProviderOptionsForCategory,
  getProviderValuesForCategory,
  normalizeFilters,
  normalizeProviderTemplateListResponse,
  normalizeResourceListResponse,
  pickConfigHighlights,
  serializeProviderTemplateFieldsForApi,
  validateResourceForm
} from './platformResourcesModel.js';

const ADMIN_RESOURCE_ENDPOINT = '/api/v1/admin/platform-resources';
const PROVIDER_TEMPLATES_ENDPOINT = '/api/v1/provider-templates';
const ADMIN_PROVIDER_TEMPLATES_ENDPOINT = '/api/v1/admin/provider-templates';

function buildRevealAccessKeyPath(resourceID) {
  return `${ADMIN_RESOURCE_ENDPOINT}/${resourceID}/access-key/reveal`;
}

const PROVIDER_FIELD_TYPE_OPTIONS = Object.freeze([
  { value: 'text', label: 'Text' },
  { value: 'number', label: 'Number' },
  { value: 'integer', label: 'Integer' },
  { value: 'select', label: 'Select' }
]);

const PROVIDER_FIELD_TYPE_SET = new Set(
  PROVIDER_FIELD_TYPE_OPTIONS.map((option) => option.value)
);

const PROVIDER_FIELD_PATH_PATTERN =
  /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/;
const RESERVED_PROVIDER_TEMPLATE_PATHS = new Set(['base_url', 'access_key']);

function hasReservedTemplatePathSegment(path) {
  const segments = String(path || '')
    .split('.')
    .map((segment) => segment.trim().toLowerCase())
    .filter(Boolean);

  return segments.some((segment) => RESERVED_PROVIDER_TEMPLATE_PATHS.has(segment));
}

function getFieldPathSegments(path) {
  return String(path || '')
    .trim()
    .toLowerCase()
    .split('.')
    .map((segment) => segment.trim())
    .filter(Boolean);
}

function getFieldPathDepth(path) {
  return Math.max(0, getFieldPathSegments(path).length - 1);
}

function getFieldPathParent(path) {
  const segments = getFieldPathSegments(path);
  if (segments.length <= 1) {
    return '';
  }

  return segments.slice(0, -1).join('.');
}

function formatFieldPathBreadcrumb(path) {
  const segments = getFieldPathSegments(path);
  if (segments.length <= 1) {
    return '';
  }

  return segments.join(' > ');
}

function createEditorNodeID(prefix = 'node') {
  return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
}

function createProviderOptionDraft(overrides = {}) {
  return {
    id: createEditorNodeID('opt'),
    value: '',
    label: '',
    ...overrides
  };
}

function createProviderFieldDraft(overrides = {}) {
  return {
    id: createEditorNodeID('fld'),
    key: '',
    label: '',
    type: 'text',
    required: false,
    defaultValue: '',
    helperText: '',
    placeholder: '',
    min: '',
    max: '',
    step: '',
    options: [createProviderOptionDraft()],
    ...overrides
  };
}

function toDraftString(value) {
  if (value === undefined || value === null) {
    return '';
  }
  return String(value);
}

function buildProviderFieldDrafts(fields = []) {
  if (!Array.isArray(fields) || fields.length === 0) {
    return [createProviderFieldDraft()];
  }

  return fields.map((field) => {
    const type = String(field.type || 'text').toLowerCase();
    const options =
      type === 'select' && Array.isArray(field.options) && field.options.length > 0
        ? field.options.map((option) =>
            createProviderOptionDraft({
              value: toDraftString(option.value),
              label: toDraftString(option.label || option.value)
            })
          )
        : [createProviderOptionDraft()];

    return createProviderFieldDraft({
      key: toDraftString(field.key),
      label: toDraftString(field.label),
      type: PROVIDER_FIELD_TYPE_SET.has(type) ? type : 'text',
      required: Boolean(field.required),
      defaultValue: toDraftString(field.defaultValue),
      helperText: toDraftString(field.helperText),
      placeholder: toDraftString(field.placeholder),
      min: toDraftString(field.min),
      max: toDraftString(field.max),
      step: toDraftString(field.step),
      options
    });
  });
}

function createProviderEditorFormDefaults(
  category = 'llm',
  provider = '',
  fields = [],
  options = {}
) {
  return {
    id: options.id || '',
    category,
    provider,
    status: options.status || 'active',
    version: String(options.version || 1),
    fields: buildProviderFieldDrafts(fields)
  };
}

function formatDateTime(timestamp) {
  if (!timestamp) {
    return '--';
  }

  const parsed = new Date(timestamp);
  if (Number.isNaN(parsed.getTime())) {
    return '--';
  }

  return parsed.toLocaleString();
}

function formatRefreshLabel(timestamp) {
  if (!timestamp) {
    return 'Not refreshed yet';
  }

  return `Last refreshed ${new Date(timestamp).toLocaleTimeString()}`;
}

const CATEGORY_LABEL_BY_VALUE = Object.freeze(
  FORM_CATEGORY_OPTIONS.reduce((result, option) => {
    result[option.value] = option.label;
    return result;
  }, {})
);

function formatProviderFieldType(field) {
  if (field.type === 'integer') {
    return 'integer';
  }
  if (field.type === 'number') {
    return 'number';
  }
  if (field.type === 'select') {
    return 'enum';
  }
  return 'string';
}

function formatProviderFieldRule(field) {
  if (field.type === 'select') {
    const options = (field.options || []).map((option) => option.value);
    return options.length > 0 ? options.join(', ') : '--';
  }

  const constraints = [];
  if (typeof field.min === 'number') {
    constraints.push(`>= ${field.min}`);
  }
  if (typeof field.max === 'number') {
    constraints.push(`<= ${field.max}`);
  }

  if (constraints.length > 0) {
    return constraints.join(' and ');
  }

  return '--';
}

function formatProviderDefaultValue(value) {
  if (value === undefined) {
    return '';
  }

  if (typeof value === 'string') {
    if (value.length <= 22) {
      return value;
    }
    return `${value.slice(0, 19)}...`;
  }

  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }

  return '{...}';
}

function parseOptionalNumeric(text, integerOnly = false) {
  const source = String(text ?? '').trim();
  if (!source) {
    return {
      ok: true,
      hasValue: false,
      value: undefined
    };
  }

  if (integerOnly && !/^-?\d+$/.test(source)) {
    return {
      ok: false,
      hasValue: true,
      value: undefined
    };
  }

  const parsed = integerOnly ? Number(source) : Number.parseFloat(source);
  if (!Number.isFinite(parsed)) {
    return {
      ok: false,
      hasValue: true,
      value: undefined
    };
  }

  if (integerOnly && !Number.isInteger(parsed)) {
    return {
      ok: false,
      hasValue: true,
      value: undefined
    };
  }

  return {
    ok: true,
    hasValue: true,
    value: parsed
  };
}

function buildProviderTemplateFieldsFromDrafts(draftFields = []) {
  if (!Array.isArray(draftFields) || draftFields.length === 0) {
    return {
      ok: false,
      fields: [],
      error: 'At least one template field is required.'
    };
  }

  const fields = [];
  const keySet = new Set();

  for (let index = 0; index < draftFields.length; index += 1) {
    const draft = draftFields[index] || {};
    const rowLabel = `Field #${index + 1}`;
    const key = String(draft.key || '').trim().toLowerCase();
    const label = String(draft.label || '').trim();
    const type = String(draft.type || '').trim().toLowerCase();

    if (!key) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: path is required.`
      };
    }

    if (!PROVIDER_FIELD_PATH_PATTERN.test(key)) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: path must match ^[a-z][a-z0-9_]*(\\.[a-z][a-z0-9_]*)*$.`
      };
    }

    if (hasReservedTemplatePathSegment(key)) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: "${key}" is reserved and managed by dedicated resource fields.`
      };
    }

    if (keySet.has(key)) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: duplicate path "${key}".`
      };
    }

    const conflictKey = Array.from(keySet).find(
      (existing) => existing.startsWith(`${key}.`) || key.startsWith(`${existing}.`)
    );
    if (conflictKey) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: path "${key}" conflicts with "${conflictKey}".`
      };
    }

    keySet.add(key);

    if (!label) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: label is required.`
      };
    }

    if (!PROVIDER_FIELD_TYPE_SET.has(type)) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: unsupported type "${type}".`
      };
    }

    const field = {
      key,
      label,
      type,
      required: Boolean(draft.required)
    };

    const helperText = String(draft.helperText || '').trim();
    if (helperText) {
      field.helperText = helperText;
    }

    const placeholder = String(draft.placeholder || '').trim();
    if (placeholder) {
      field.placeholder = placeholder;
    }

    const defaultValueText = String(draft.defaultValue || '').trim();

    if (type === 'text') {
      if (defaultValueText) {
        field.defaultValue = defaultValueText;
      }
      fields.push(field);
      continue;
    }

    if (type === 'select') {
      const optionDrafts = Array.isArray(draft.options) ? draft.options : [];
      const options = [];
      const optionValueSet = new Set();

      for (const optionDraft of optionDrafts) {
        const value = String(optionDraft?.value || '').trim();
        if (!value) {
          continue;
        }

        if (optionValueSet.has(value)) {
          return {
            ok: false,
            fields: [],
            error: `${rowLabel}: duplicate option value "${value}".`
          };
        }
        optionValueSet.add(value);

        const optionLabel = String(optionDraft?.label || '').trim() || value;
        options.push({
          value,
          label: optionLabel
        });
      }

      if (options.length === 0) {
        return {
          ok: false,
          fields: [],
          error: `${rowLabel}: select field requires at least one option.`
        };
      }

      field.options = options;

      if (defaultValueText) {
        if (!optionValueSet.has(defaultValueText)) {
          return {
            ok: false,
            fields: [],
            error: `${rowLabel}: default value must exist in options.`
          };
        }
        field.defaultValue = defaultValueText;
      }

      fields.push(field);
      continue;
    }

    const integerOnly = type === 'integer';
    const minResult = parseOptionalNumeric(draft.min, integerOnly);
    const maxResult = parseOptionalNumeric(draft.max, integerOnly);
    const stepResult = parseOptionalNumeric(draft.step, integerOnly);
    const defaultResult = parseOptionalNumeric(defaultValueText, integerOnly);

    if (!minResult.ok || !maxResult.ok || !stepResult.ok || !defaultResult.ok) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: numeric constraints/default value are invalid.`
      };
    }

    if (minResult.hasValue) {
      field.min = minResult.value;
    }
    if (maxResult.hasValue) {
      field.max = maxResult.value;
    }
    if (stepResult.hasValue) {
      if (stepResult.value <= 0) {
        return {
          ok: false,
          fields: [],
          error: `${rowLabel}: step must be > 0.`
        };
      }
      field.step = stepResult.value;
    }

    if (
      minResult.hasValue &&
      maxResult.hasValue &&
      minResult.value > maxResult.value
    ) {
      return {
        ok: false,
        fields: [],
        error: `${rowLabel}: min cannot be greater than max.`
      };
    }

    if (defaultResult.hasValue) {
      if (minResult.hasValue && defaultResult.value < minResult.value) {
        return {
          ok: false,
          fields: [],
          error: `${rowLabel}: default value must be >= min.`
        };
      }

      if (maxResult.hasValue && defaultResult.value > maxResult.value) {
        return {
          ok: false,
          fields: [],
          error: `${rowLabel}: default value must be <= max.`
        };
      }

      field.defaultValue = defaultResult.value;
    }

    fields.push(field);
  }

  return {
    ok: true,
    fields,
    error: ''
  };
}

export default function PlatformResourcesPage() {
  const { authorizedRequest } = useAuth();
  const theme = useTheme();
  const fullScreenDialog = useMediaQuery(theme.breakpoints.down('md'));

  const resourceRequestIDRef = React.useRef(0);
  const providerRequestIDRef = React.useRef(0);

  const [activePanel, setActivePanel] = React.useState('resources');

  const [filters, setFilters] = React.useState(() => ({ ...DEFAULT_FILTERS }));
  const [filterDraft, setFilterDraft] = React.useState(() => ({
    ...DEFAULT_FILTERS
  }));
  const [providerPanelFilters, setProviderPanelFilters] = React.useState(() => ({
    category: 'all',
    provider: '',
    status: 'all'
  }));
  const [providerTemplateItems, setProviderTemplateItems] = React.useState([]);
  const [providerLoading, setProviderLoading] = React.useState(true);
  const [providerLoadError, setProviderLoadError] = React.useState('');

  const [providerEditorOpen, setProviderEditorOpen] = React.useState(false);
  const [providerEditorMode, setProviderEditorMode] = React.useState('create');
  const [providerEditorForm, setProviderEditorForm] = React.useState(() =>
    createProviderEditorFormDefaults()
  );
  const [providerEditorError, setProviderEditorError] = React.useState('');
  const [expandedProviderEntryID, setExpandedProviderEntryID] = React.useState('');

  const [resources, setResources] = React.useState([]);
  const [loading, setLoading] = React.useState(true);
  const [loadError, setLoadError] = React.useState('');
  const [lastRefreshedAt, setLastRefreshedAt] = React.useState(0);

  const [notice, setNotice] = React.useState(null);
  const [disablingID, setDisablingID] = React.useState('');

  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [dialogMode, setDialogMode] = React.useState('create');
  const [editingResource, setEditingResource] = React.useState(null);
  const [form, setForm] = React.useState(() => ({ ...DEFAULT_RESOURCE_FORM }));
  const [providerConfigValues, setProviderConfigValues] = React.useState(() => ({}));
  const [providerConfigExtras, setProviderConfigExtras] = React.useState(() => ({}));
  const [providerConfigErrors, setProviderConfigErrors] = React.useState(() => ({}));
  const [fieldErrors, setFieldErrors] = React.useState({});
  const [submitError, setSubmitError] = React.useState('');
  const [saving, setSaving] = React.useState(false);
  const [showAccessKey, setShowAccessKey] = React.useState(false);
  const [revealDialogOpen, setRevealDialogOpen] = React.useState(false);
  const [revealTargetResource, setRevealTargetResource] = React.useState(null);
  const [revealPassword, setRevealPassword] = React.useState('');
  const [showRevealPassword, setShowRevealPassword] = React.useState(false);
  const [revealSubmitting, setRevealSubmitting] = React.useState(false);
  const [revealError, setRevealError] = React.useState('');
  const [revealedAccessKey, setRevealedAccessKey] = React.useState('');
  const [showRevealedAccessKey, setShowRevealedAccessKey] = React.useState(false);

  const providerTemplates = React.useMemo(
    () => buildProviderTemplatesFromTemplateItems(providerTemplateItems),
    [providerTemplateItems]
  );

  const providerTemplatesForFilter = React.useMemo(
    () =>
      buildProviderTemplatesFromTemplateItems(providerTemplateItems, {
        includeInactive: true
      }),
    [providerTemplateItems]
  );

  const filterProviderOptions = React.useMemo(
    () => buildFilterProviderOptions(providerTemplatesForFilter),
    [providerTemplatesForFilter]
  );

  const formProviderOptions = React.useMemo(
    () => getProviderOptionsForCategory(form.category, form.provider, providerTemplates),
    [form.category, form.provider, providerTemplates]
  );

  const providerConfigFields = React.useMemo(
    () => getProviderConfigFields(form.category, form.provider, providerTemplates),
    [form.category, form.provider, providerTemplates]
  );

  const providerCatalogEntries = React.useMemo(() => {
    return providerTemplateItems
      .filter((entry) => {
        if (
          providerPanelFilters.category !== 'all' &&
          entry.category !== providerPanelFilters.category
        ) {
          return false;
        }

        if (providerPanelFilters.provider && entry.provider !== providerPanelFilters.provider) {
          return false;
        }

        if (
          providerPanelFilters.status !== 'all' &&
          entry.status !== providerPanelFilters.status
        ) {
          return false;
        }

        return true;
      })
      .map((entry) => ({
        ...entry,
        requiredFieldCount: entry.fields.filter((field) => field.required).length
      }));
  }, [providerPanelFilters, providerTemplateItems]);

  const hasProviderTemplate = providerTemplateItems.length > 0;

  const loadResources = React.useCallback(async () => {
    const requestID = resourceRequestIDRef.current + 1;
    resourceRequestIDRef.current = requestID;

    setLoading(true);
    setLoadError('');

    try {
      const payload = await authorizedRequest(buildPlatformResourceListPath(filters));
      if (requestID !== resourceRequestIDRef.current) {
        return;
      }

      setResources(normalizeResourceListResponse(payload));
      setLastRefreshedAt(Date.now());
    } catch (error) {
      if (requestID !== resourceRequestIDRef.current) {
        return;
      }

      setResources([]);
      setLoadError(formatAuthError(error));
    } finally {
      if (requestID === resourceRequestIDRef.current) {
        setLoading(false);
      }
    }
  }, [authorizedRequest, filters]);

  const loadProviderTemplates = React.useCallback(async () => {
    const requestID = providerRequestIDRef.current + 1;
    providerRequestIDRef.current = requestID;

    setProviderLoading(true);
    setProviderLoadError('');

    try {
      const payload = await authorizedRequest(PROVIDER_TEMPLATES_ENDPOINT);
      if (requestID !== providerRequestIDRef.current) {
        return;
      }

      setProviderTemplateItems(normalizeProviderTemplateListResponse(payload));
    } catch (error) {
      if (requestID !== providerRequestIDRef.current) {
        return;
      }

      setProviderTemplateItems([]);
      setProviderLoadError(formatAuthError(error));
    } finally {
      if (requestID === providerRequestIDRef.current) {
        setProviderLoading(false);
      }
    }
  }, [authorizedRequest]);

  React.useEffect(() => {
    loadResources();
  }, [loadResources]);

  React.useEffect(() => {
    loadProviderTemplates();
  }, [loadProviderTemplates]);

  React.useEffect(() => {
    if (providerPanelFilters.provider === '') {
      return;
    }

    const exists = filterProviderOptions.some(
      (option) => option.value === providerPanelFilters.provider
    );
    if (!exists) {
      setProviderPanelFilters((prev) => ({
        ...prev,
        provider: ''
      }));
    }
  }, [providerPanelFilters.provider, filterProviderOptions]);

  React.useEffect(() => {
    if (filterDraft.provider === '') {
      return;
    }

    const exists = filterProviderOptions.some((option) => option.value === filterDraft.provider);
    if (!exists) {
      setFilterDraft((prev) => ({
        ...prev,
        provider: ''
      }));
    }
  }, [filterDraft.provider, filterProviderOptions]);

  React.useEffect(() => {
    if (!dialogOpen) {
      return;
    }

    const providers = getProviderValuesForCategory(form.category, providerTemplates);
    if (providers.length === 0) {
      if (form.provider !== '') {
        setForm((prev) => ({
          ...prev,
          provider: ''
        }));
        setProviderConfigValues({});
        setProviderConfigExtras({});
        setProviderConfigErrors({});
      }
      return;
    }

    if (providers.includes(form.provider)) {
      return;
    }

    const nextProvider = providers[0];
    setForm((prev) => ({
      ...prev,
      provider: nextProvider
    }));
    setProviderConfigValues(
      buildProviderConfigFormValues(form.category, nextProvider, {}, providerTemplates)
    );
    setProviderConfigExtras({});
    setProviderConfigErrors({});
  }, [
    dialogOpen,
    form.category,
    form.provider,
    providerTemplates
  ]);

  React.useEffect(() => {
    if (!expandedProviderEntryID) {
      return;
    }

    const exists = providerCatalogEntries.some((entry) => entry.id === expandedProviderEntryID);
    if (!exists) {
      setExpandedProviderEntryID('');
    }
  }, [expandedProviderEntryID, providerCatalogEntries]);

  const handleFilterDraftChange = (field) => (event) => {
    const value = event.target.value;
    setFilterDraft((prev) => ({
      ...prev,
      [field]: value
    }));
  };

  const handleApplyFilters = (event) => {
    event.preventDefault();
    setFilters(normalizeFilters(filterDraft));
  };

  const handleResetFilters = () => {
    const nextFilters = { ...DEFAULT_FILTERS };
    setFilterDraft(nextFilters);
    setFilters(nextFilters);
  };

  const handlePanelChange = (_event, value) => {
    setActivePanel(value);
  };

  const handleProviderPanelFilterChange = (field) => (event) => {
    const value = event.target.value;
    setProviderPanelFilters((prev) => ({
      ...prev,
      [field]: value
    }));
  };

  const handleToggleProviderEntry = (entryID) => {
    setExpandedProviderEntryID((prev) => (prev === entryID ? '' : entryID));
  };

  const openCreateProviderTemplateDialog = () => {
    const category =
      providerPanelFilters.category === 'all'
        ? DEFAULT_RESOURCE_FORM.category
        : providerPanelFilters.category;
    const providers = getProviderValuesForCategory(category, providerTemplates);
    const sourceProvider = providers[0] || '';

    const sourceFields = sourceProvider
      ? getProviderConfigFields(category, sourceProvider, providerTemplates)
      : [];

    setProviderEditorMode('create');
    setProviderEditorForm(
      createProviderEditorFormDefaults(category, '', sourceFields, {
        status: 'active',
        version: 1
      })
    );
    setProviderEditorError('');
    setProviderEditorOpen(true);
  };

  const openEditProviderTemplateDialog = (entry) => {
    setProviderEditorMode('edit');
    setProviderEditorForm(
      createProviderEditorFormDefaults(entry.category, entry.provider, entry.fields, {
        id: entry.id,
        status: entry.status,
        version: entry.version
      })
    );
    setProviderEditorError('');
    setProviderEditorOpen(true);
  };

  const closeProviderTemplateDialog = () => {
    setProviderEditorOpen(false);
    setProviderEditorError('');
  };

  const handleProviderEditorChange = (field) => (event) => {
    const value = event.target.value;

    setProviderEditorForm((prev) => {
      if (field === 'category' && providerEditorMode === 'create') {
        const sourceProviders = getProviderValuesForCategory(value, providerTemplates);
        const sourceProvider = sourceProviders[0] || '';
        const sourceFields = sourceProvider
          ? getProviderConfigFields(value, sourceProvider, providerTemplates)
          : [];

        return {
          ...prev,
          category: value,
          fields: buildProviderFieldDrafts(sourceFields)
        };
      }

      return {
        ...prev,
        [field]: field === 'provider' ? value.toLowerCase().trim() : value
      };
    });

    setProviderEditorError('');
  };

  const handleProviderEditorFieldChange = (fieldID, fieldName) => (event) => {
    const value =
      fieldName === 'required' ? event.target.checked : event.target.value;

    setProviderEditorForm((prev) => ({
      ...prev,
      fields: prev.fields.map((field) => {
        if (field.id !== fieldID) {
          return field;
        }

        if (fieldName === 'type') {
          const nextType = String(value || 'text');

          return {
            ...field,
            type: nextType,
            options:
              nextType === 'select' && (!field.options || field.options.length === 0)
                ? [createProviderOptionDraft()]
                : field.options
          };
        }

        return {
          ...field,
          [fieldName]: value
        };
      })
    }));

    setProviderEditorError('');
  };

  const handleAddProviderEditorField = () => {
    setProviderEditorForm((prev) => ({
      ...prev,
      fields: [...prev.fields, createProviderFieldDraft()]
    }));
    setProviderEditorError('');
  };

  const handleRemoveProviderEditorField = (fieldID) => {
    setProviderEditorForm((prev) => {
      const nextFields = prev.fields.filter((field) => field.id !== fieldID);
      return {
        ...prev,
        fields: nextFields.length > 0 ? nextFields : [createProviderFieldDraft()]
      };
    });
    setProviderEditorError('');
  };

  const handleAddProviderEditorOption = (fieldID) => {
    setProviderEditorForm((prev) => ({
      ...prev,
      fields: prev.fields.map((field) => {
        if (field.id !== fieldID) {
          return field;
        }

        return {
          ...field,
          options: [...(field.options || []), createProviderOptionDraft()]
        };
      })
    }));
    setProviderEditorError('');
  };

  const handleProviderEditorOptionChange = (fieldID, optionID, key) => (event) => {
    const value = event.target.value;

    setProviderEditorForm((prev) => ({
      ...prev,
      fields: prev.fields.map((field) => {
        if (field.id !== fieldID) {
          return field;
        }

        return {
          ...field,
          options: (field.options || []).map((option) => {
            if (option.id !== optionID) {
              return option;
            }

            return {
              ...option,
              [key]: value
            };
          })
        };
      })
    }));
    setProviderEditorError('');
  };

  const handleRemoveProviderEditorOption = (fieldID, optionID) => {
    setProviderEditorForm((prev) => ({
      ...prev,
      fields: prev.fields.map((field) => {
        if (field.id !== fieldID) {
          return field;
        }

        const nextOptions = (field.options || []).filter((option) => option.id !== optionID);
        return {
          ...field,
          options: nextOptions.length > 0 ? nextOptions : [createProviderOptionDraft()]
        };
      })
    }));
    setProviderEditorError('');
  };

  const handleSaveProviderTemplate = async () => {
    const category = providerEditorForm.category;
    const provider = providerEditorForm.provider;

    const templateResult = buildProviderTemplateFieldsFromDrafts(providerEditorForm.fields);
    if (!templateResult.ok) {
      setProviderEditorError(templateResult.error);
      return;
    }

    const normalizedCategory = String(category || '').trim().toLowerCase();
    if (!CATEGORY_LABEL_BY_VALUE[normalizedCategory]) {
      setProviderEditorError('Category is invalid.');
      return;
    }

    const normalizedProvider = provider.trim().toLowerCase();
    if (!normalizedProvider) {
      setProviderEditorError('Provider key is required.');
      return;
    }

    if (!/^[a-z][a-z0-9-]*$/.test(normalizedProvider)) {
      setProviderEditorError('Provider key must match ^[a-z][a-z0-9-]*$.');
      return;
    }

    const status =
      providerEditorForm.status === 'inactive' ? 'inactive' : 'active';
    const version = Number.parseInt(providerEditorForm.version, 10);
    if (!Number.isInteger(version) || version <= 0) {
      setProviderEditorError('Version must be a positive integer.');
      return;
    }

    const categoryProviders = providerTemplates[normalizedCategory] || {};
    if (providerEditorMode === 'create' && categoryProviders[normalizedProvider]) {
      setProviderEditorError(
        `Provider "${normalizedProvider}" already exists in ${normalizedCategory}.`
      );
      return;
    }

    const serializedFields = serializeProviderTemplateFieldsForApi(templateResult.fields);

    try {
      if (providerEditorMode === 'create') {
        await authorizedRequest(ADMIN_PROVIDER_TEMPLATES_ENDPOINT, {
          method: 'POST',
          body: {
            category: normalizedCategory,
            provider: normalizedProvider,
            status,
            version,
            fields: serializedFields
          }
        });
      } else {
        if (!providerEditorForm.id) {
          setProviderEditorError('Template id is missing. Please refresh and retry.');
          return;
        }

        await authorizedRequest(
          `${ADMIN_PROVIDER_TEMPLATES_ENDPOINT}/${providerEditorForm.id}`,
          {
            method: 'PATCH',
            body: {
              status,
              fields: serializedFields
            }
          }
        );
      }

      await loadProviderTemplates();
      setProviderEditorOpen(false);
      setProviderEditorError('');
      setNotice({
        severity: 'success',
        message:
          providerEditorMode === 'create'
            ? `Provider template ${normalizedCategory}/${normalizedProvider} created.`
            : `Provider template ${normalizedCategory}/${normalizedProvider} updated.`
      });
    } catch (error) {
      setProviderEditorError(formatAuthError(error));
    }
  };

  const handleDeleteProviderTemplate = async (entry) => {
    if (!entry?.id) {
      setNotice({
        severity: 'error',
        message: 'Template id is missing. Please refresh and retry.'
      });
      return;
    }

    const confirmed = window.confirm(
      `Delete provider template ${entry.category}/${entry.provider}?\nResources using this provider may become hard to edit.`
    );
    if (!confirmed) {
      return;
    }

    try {
      await authorizedRequest(`${ADMIN_PROVIDER_TEMPLATES_ENDPOINT}/${entry.id}`, {
        method: 'DELETE'
      });
      await loadProviderTemplates();
      setNotice({
        severity: 'success',
        message: `Provider template ${entry.category}/${entry.provider} deleted.`
      });
    } catch (error) {
      setNotice({
        severity: 'error',
        message: formatAuthError(error)
      });
    }
  };

  const closeDialog = () => {
    if (saving) {
      return;
    }
    setDialogOpen(false);
    setShowAccessKey(false);
  };

  const openCreateDialog = (preset = {}) => {
    const presetCategory = preset.category || '';
    const presetProvider = preset.provider || '';

    const formCategory =
      presetCategory ||
      (filterDraft.category && filterDraft.category !== 'all'
        ? filterDraft.category
        : DEFAULT_RESOURCE_FORM.category);

    const supportedProviders = getProviderValuesForCategory(
      formCategory,
      providerTemplates
    );
    const providerCandidate = presetProvider || filterDraft.provider;
    const nextProvider = supportedProviders.includes(providerCandidate)
      ? providerCandidate
      : supportedProviders[0] || '';

    setDialogMode('create');
    setEditingResource(null);
    setForm({
      ...DEFAULT_RESOURCE_FORM,
      category: formCategory,
      provider: nextProvider,
      baseUrl: getDefaultBaseURLForProvider(nextProvider)
    });
    setProviderConfigValues(
      buildProviderConfigFormValues(formCategory, nextProvider, {}, providerTemplates)
    );
    setProviderConfigExtras({});
    setProviderConfigErrors({});
    setFieldErrors({});
    setSubmitError('');
    setShowAccessKey(false);
    setDialogOpen(true);
  };

  const openEditDialog = (resource) => {
    const nextForm = buildResourceFormFromItem(resource);

    setDialogMode('edit');
    setEditingResource(resource);
    setForm(nextForm);
    setProviderConfigValues(
      buildProviderConfigFormValues(
        nextForm.category,
        nextForm.provider,
        resource?.config,
        providerTemplates
      )
    );
    setProviderConfigExtras(
      extractProviderConfigExtras(
        nextForm.category,
        nextForm.provider,
        resource?.config,
        providerTemplates
      )
    );
    setProviderConfigErrors({});
    setFieldErrors({});
    setSubmitError('');
    setShowAccessKey(false);
    setDialogOpen(true);
  };

  const handleCreateFromProviderTemplate = (category, provider) => {
    setActivePanel('resources');
    openCreateDialog({ category, provider });
  };

  const handleFormChange = (field) => (event) => {
    const value = event.target.value;
    if (field === 'category') {
      const supportedProviders = getProviderValuesForCategory(value, providerTemplates);
      const nextProvider = supportedProviders.includes(form.provider)
        ? form.provider
        : supportedProviders[0] || '';
      const defaultBaseUrl = getDefaultBaseURLForProvider(nextProvider);

      setForm((prev) => ({
        ...prev,
        category: value,
        provider: nextProvider,
        baseUrl:
          dialogMode === 'create'
            ? defaultBaseUrl
            : prev.baseUrl || defaultBaseUrl
      }));
      setProviderConfigValues(
        buildProviderConfigFormValues(value, nextProvider, {}, providerTemplates)
      );
      setProviderConfigExtras({});
      setProviderConfigErrors({});
    } else if (field === 'provider') {
      const defaultBaseUrl = getDefaultBaseURLForProvider(value);

      setForm((prev) => ({
        ...prev,
        provider: value,
        baseUrl:
          dialogMode === 'create'
            ? defaultBaseUrl
            : prev.baseUrl || defaultBaseUrl
      }));
      setProviderConfigValues(
        buildProviderConfigFormValues(form.category, value, {}, providerTemplates)
      );
      setProviderConfigExtras({});
      setProviderConfigErrors({});
    } else {
      setForm((prev) => ({
        ...prev,
        [field]: value
      }));
    }

    setFieldErrors((prev) => {
      if (
        !prev[field] &&
        !(field === 'category' && prev.provider) &&
        !(field === 'provider' && prev.providerConfig)
      ) {
        return prev;
      }

      const next = { ...prev };
      delete next[field];

      if (field === 'category' || field === 'provider') {
        delete next.provider;
        delete next.providerConfig;
        delete next.baseUrl;
      }

      return next;
    });
  };

  const handleProviderConfigChange = (fieldKey) => (event) => {
    const value = event.target.value;
    setProviderConfigValues((prev) => ({
      ...prev,
      [fieldKey]: value
    }));

    setProviderConfigErrors((prev) => {
      if (!prev[fieldKey] && !prev._template) {
        return prev;
      }

      const next = { ...prev };
      delete next[fieldKey];
      delete next._template;
      return next;
    });

    setFieldErrors((prev) => {
      if (!prev.providerConfig) {
        return prev;
      }

      const next = { ...prev };
      delete next.providerConfig;
      return next;
    });
  };

  const handleSubmitDialog = async (event) => {
    event.preventDefault();

    const providerConfigResult = buildProviderConfigPayload(
      form.category,
      form.provider,
      providerConfigValues,
      providerTemplates
    );
    if (!providerConfigResult.ok) {
      setProviderConfigErrors(providerConfigResult.fieldErrors);
      if (providerConfigResult.fieldErrors._template) {
        setFieldErrors((prev) => ({
          ...prev,
          providerConfig: providerConfigResult.fieldErrors._template
        }));
      }
      return;
    }

    setProviderConfigErrors({});
    setFieldErrors((prev) => {
      if (!prev.providerConfig) {
        return prev;
      }
      const next = { ...prev };
      delete next.providerConfig;
      return next;
    });

    const normalizedConfig = {
      ...providerConfigExtras,
      ...providerConfigResult.config
    };

    const validation = validateResourceForm(
      {
        ...form,
        config: normalizedConfig
      },
      {
        mode: dialogMode,
        providerTemplates
      }
    );
    if (!validation.ok) {
      setFieldErrors(validation.fieldErrors);
      return;
    }

    if (dialogMode === 'edit' && !editingResource?.id) {
      setSubmitError('Selected resource is missing an id.');
      return;
    }

    const requestPath =
      dialogMode === 'create'
        ? ADMIN_RESOURCE_ENDPOINT
        : `${ADMIN_RESOURCE_ENDPOINT}/${editingResource.id}`;
    const method = dialogMode === 'create' ? 'POST' : 'PATCH';
    const requestPayload =
      dialogMode === 'create'
        ? validation.payload
        : buildPatchPayload(editingResource, validation.payload);

    if (dialogMode === 'edit' && Object.keys(requestPayload).length === 0) {
      setSubmitError('No changes detected.');
      return;
    }

    setSaving(true);
    setSubmitError('');
    setNotice(null);

    try {
      await authorizedRequest(requestPath, {
        method,
        body: requestPayload
      });

      const resourceKeyLabel =
        validation.payload.resource_key || editingResource?.resourceKey || 'resource';

      setDialogOpen(false);
      setNotice({
        severity: 'success',
        message:
          dialogMode === 'create'
            ? `Created ${resourceKeyLabel}.`
            : `Updated ${resourceKeyLabel}.`
      });
      await loadResources();
    } catch (error) {
      setSubmitError(formatAuthError(error));

      if (error instanceof ManagerApiError) {
        const backendFieldErrors = extractFieldErrorsFromApiDetails(error.details);

        if (backendFieldErrors.providerConfig) {
          setFieldErrors((prev) => ({
            ...prev,
            providerConfig: backendFieldErrors.providerConfig
          }));
        }

        const filteredFieldErrors = Object.fromEntries(
          Object.entries(backendFieldErrors).filter(
            ([field]) => field !== 'providerConfig'
          )
        );

        if (Object.keys(filteredFieldErrors).length > 0) {
          setFieldErrors((prev) => ({
            ...prev,
            ...filteredFieldErrors
          }));
        }
      }
    } finally {
      setSaving(false);
    }
  };

  const handleDisableResource = async (resource) => {
    if (!resource?.id || resource.status === 'inactive') {
      return;
    }

    const confirmed = window.confirm(
      `Disable resource "${resource.name || resource.resourceKey}"?`
    );
    if (!confirmed) {
      return;
    }

    setDisablingID(resource.id);
    setNotice(null);

    try {
      await authorizedRequest(`${ADMIN_RESOURCE_ENDPOINT}/${resource.id}`, {
        method: 'DELETE'
      });

      setNotice({
        severity: 'success',
        message: `Disabled ${resource.resourceKey}.`
      });
      await loadResources();
    } catch (error) {
      setNotice({
        severity: 'error',
        message: formatAuthError(error)
      });
    } finally {
      setDisablingID('');
    }
  };

  const openRevealDialog = (resource) => {
    if (!resource?.id) {
      return;
    }

    setRevealTargetResource(resource);
    setRevealPassword('');
    setShowRevealPassword(false);
    setRevealSubmitting(false);
    setRevealError('');
    setRevealedAccessKey('');
    setShowRevealedAccessKey(false);
    setRevealDialogOpen(true);
  };

  const closeRevealDialog = () => {
    if (revealSubmitting) {
      return;
    }

    setRevealDialogOpen(false);
    setRevealTargetResource(null);
    setRevealPassword('');
    setShowRevealPassword(false);
    setRevealError('');
    setRevealedAccessKey('');
    setShowRevealedAccessKey(false);
  };

  const handleRevealAccessKey = async () => {
    if (!revealTargetResource?.id) {
      setRevealError('Resource id is missing. Please close and retry.');
      return;
    }

    const password = revealPassword.trim();
    if (!password) {
      setRevealError('Password is required.');
      return;
    }

    setRevealSubmitting(true);
    setRevealError('');

    try {
      const payload = await authorizedRequest(buildRevealAccessKeyPath(revealTargetResource.id), {
        method: 'POST',
        body: {
          password
        }
      });

      const accessKey = String(payload?.access_key || '').trim();
      if (!accessKey) {
        setRevealError('Backend response does not include access_key.');
        setRevealedAccessKey('');
      } else {
        setRevealedAccessKey(accessKey);
      }
    } catch (error) {
      setRevealError(formatAuthError(error));
      setRevealedAccessKey('');
    } finally {
      setRevealSubmitting(false);
    }
  };

  const hasActiveFilter =
    filters.category !== 'all' || filters.provider !== '' || filters.status !== 'all';

  return (
    <Stack spacing={2.25}>
      <Paper
        sx={{
          p: { xs: 2.5, md: 3 },
          borderRadius: 2,
          border: '1px solid rgba(41, 64, 62, 0.14)',
          background:
            'linear-gradient(120deg, rgba(255,255,255,0.9), rgba(239, 246, 241, 0.9))'
        }}
      >
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          spacing={1.5}
          alignItems={{ xs: 'flex-start', md: 'center' }}
          justifyContent="space-between"
        >
          <Box>
            <Typography variant="h4">Platform Resources</Typography>
            <Typography color="text.secondary" sx={{ mt: 1 }}>
              Manage LLM / ASR / TTS providers, config schema versions, and runtime
              availability.
            </Typography>
          </Box>

          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              startIcon={<RefreshRoundedIcon />}
              onClick={loadResources}
              disabled={loading}
            >
              Refresh
            </Button>
            <Button
              variant="contained"
              startIcon={<AddRoundedIcon />}
              onClick={openCreateDialog}
            >
              Create resource
            </Button>
          </Stack>
        </Stack>
      </Paper>

      <Paper sx={{ borderRadius: 2, overflow: 'hidden' }}>
        <Tabs
          value={activePanel}
          onChange={handlePanelChange}
          variant="fullWidth"
          sx={{
            '& .MuiTab-root': {
              py: 1.5,
              fontWeight: 700
            }
          }}
        >
          <Tab
            value="resources"
            icon={<Inventory2RoundedIcon fontSize="small" />}
            iconPosition="start"
            label="Resource Instances"
          />
          <Tab
            value="providers"
            icon={<CloudQueueRoundedIcon fontSize="small" />}
            iconPosition="start"
            label="Provider Catalog"
          />
        </Tabs>
      </Paper>

      {activePanel === 'resources' ? (
        <>
          <Paper sx={{ p: { xs: 2, md: 2.5 }, borderRadius: 2 }}>
            <Stack component="form" onSubmit={handleApplyFilters} spacing={1.5}>
              <Stack
                direction={{ xs: 'column', md: 'row' }}
                spacing={1.25}
                alignItems={{ xs: 'stretch', md: 'center' }}
              >
                <TextField
                  select
                  size="small"
                  label="Category"
                  value={filterDraft.category}
                  onChange={handleFilterDraftChange('category')}
                  sx={{ minWidth: { xs: '100%', md: 165 } }}
                >
                  {FILTER_CATEGORY_OPTIONS.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  select
                  size="small"
                  label="Provider"
                  value={filterDraft.provider}
                  onChange={handleFilterDraftChange('provider')}
                  sx={{ minWidth: { xs: '100%', md: 220 } }}
                >
                  {filterProviderOptions.map((option) => (
                    <MenuItem key={option.value || '__all_provider'} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  select
                  size="small"
                  label="Status"
                  value={filterDraft.status}
                  onChange={handleFilterDraftChange('status')}
                  sx={{ minWidth: { xs: '100%', md: 165 } }}
                >
                  {FILTER_STATUS_OPTIONS.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <Stack direction="row" spacing={1}>
                  <Button
                    type="submit"
                    variant="contained"
                    startIcon={<SearchRoundedIcon />}
                  >
                    Apply
                  </Button>
                  <Button type="button" variant="text" onClick={handleResetFilters}>
                    Reset
                  </Button>
                </Stack>
              </Stack>

              <Typography variant="caption" color="text.secondary">
                {resources.length} item(s) visible. {formatRefreshLabel(lastRefreshedAt)}
              </Typography>
            </Stack>
          </Paper>

          {loadError ? <Alert severity="error">{loadError}</Alert> : null}
          {notice ? <Alert severity={notice.severity}>{notice.message}</Alert> : null}

          <Paper sx={{ borderRadius: 2, overflow: 'hidden' }}>
            <TableContainer>
              <Table size="small" sx={{ minWidth: 980 }}>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 700 }}>Resource</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Category / Provider</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Schema</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Key Config</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Updated</TableCell>
                    <TableCell sx={{ fontWeight: 700, width: 170 }}>Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={7}>
                        <Stack
                          direction="row"
                          spacing={1}
                          alignItems="center"
                          justifyContent="center"
                          sx={{ py: 3 }}
                        >
                          <CircularProgress size={20} />
                          <Typography variant="body2" color="text.secondary">
                            Loading resources...
                          </Typography>
                        </Stack>
                      </TableCell>
                    </TableRow>
                  ) : resources.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7}>
                        <Box sx={{ py: 4, textAlign: 'center' }}>
                          <Typography variant="body2" color="text.secondary">
                            {hasActiveFilter
                              ? 'No resources match the current filters.'
                              : 'No resources yet. Create your first platform resource.'}
                          </Typography>
                        </Box>
                      </TableCell>
                    </TableRow>
                  ) : (
                    resources.map((resource, index) => {
                      const configHighlights = pickConfigHighlights(resource.config);
                      const rowKey =
                        resource.id ||
                        resource.resourceKey ||
                        `${resource.category}-${resource.provider}-${index}`;

                      return (
                        <TableRow key={rowKey} hover>
                          <TableCell>
                            <Typography variant="subtitle2">{resource.name || '--'}</Typography>
                            <Typography variant="caption" color="text.secondary">
                              {resource.resourceKey || '--'}
                            </Typography>
                          </TableCell>

                          <TableCell>
                            <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                              <Chip
                                label={resource.category.toUpperCase()}
                                size="small"
                                color="primary"
                                variant="outlined"
                              />
                              <Chip
                                label={resource.provider || '--'}
                                size="small"
                                variant="outlined"
                              />
                            </Stack>
                          </TableCell>

                          <TableCell>
                            <Chip
                              label={`v${resource.schemaVersion}`}
                              size="small"
                              variant="outlined"
                            />
                          </TableCell>

                          <TableCell>
                            <Stack spacing={0.35}>
                              {configHighlights.length === 0 ? (
                                <Typography variant="caption" color="text.secondary">
                                  No key fields
                                </Typography>
                              ) : (
                                configHighlights.map((entry) => (
                                  <Typography
                                    key={`${rowKey}-${entry.key}`}
                                    variant="caption"
                                    sx={{
                                      color: 'text.secondary',
                                      fontFamily:
                                        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'
                                    }}
                                  >
                                    {entry.key}: {entry.value}
                                  </Typography>
                                ))
                              )}
                            </Stack>
                          </TableCell>

                          <TableCell>
                            <Chip
                              label={resource.status === 'active' ? 'Active' : 'Inactive'}
                              size="small"
                              color={resource.status === 'active' ? 'success' : 'default'}
                            />
                          </TableCell>

                          <TableCell>
                            <Typography variant="body2" color="text.secondary">
                              {formatDateTime(resource.updatedAt || resource.createdAt)}
                            </Typography>
                          </TableCell>

                          <TableCell>
                            <Stack direction="row" spacing={0.75}>
                              <Button
                                size="small"
                                variant="outlined"
                                startIcon={<EditRoundedIcon fontSize="small" />}
                                onClick={() => openEditDialog(resource)}
                              >
                                Edit
                              </Button>
                              <Button
                                size="small"
                                variant="outlined"
                                startIcon={<VisibilityRoundedIcon fontSize="small" />}
                                onClick={() => openRevealDialog(resource)}
                                disabled={!resource.hasAccessKey}
                              >
                                Reveal key
                              </Button>
                              <Button
                                size="small"
                                color="warning"
                                variant="outlined"
                                startIcon={<BlockRoundedIcon fontSize="small" />}
                                onClick={() => handleDisableResource(resource)}
                                disabled={
                                  disablingID === resource.id || resource.status === 'inactive'
                                }
                              >
                                {disablingID === resource.id ? '...' : 'Disable'}
                              </Button>
                            </Stack>
                          </TableCell>
                        </TableRow>
                      );
                    })
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Paper>
        </>
      ) : (
        <>
          <Alert severity="info">
            Provider templates are visual capability profiles. You can add, edit, and
            delete category-provider contracts here before creating resources.
          </Alert>

          <Paper sx={{ p: { xs: 2, md: 2.5 }, borderRadius: 2 }}>
            <Stack spacing={1.5}>
              <Stack
                direction={{ xs: 'column', md: 'row' }}
                spacing={1.25}
                alignItems={{ xs: 'stretch', md: 'center' }}
              >
                <TextField
                  select
                  size="small"
                  label="Category"
                  value={providerPanelFilters.category}
                  onChange={handleProviderPanelFilterChange('category')}
                  sx={{ minWidth: { xs: '100%', md: 200 } }}
                >
                  {FILTER_CATEGORY_OPTIONS.map((option) => (
                    <MenuItem key={`provider-panel-${option.value}`} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  select
                  size="small"
                  label="Provider"
                  value={providerPanelFilters.provider}
                  onChange={handleProviderPanelFilterChange('provider')}
                  sx={{ minWidth: { xs: '100%', md: 220 } }}
                >
                  {filterProviderOptions.map((option) => (
                    <MenuItem key={`provider-panel-opt-${option.value || 'all'}`} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  select
                  size="small"
                  label="Status"
                  value={providerPanelFilters.status}
                  onChange={handleProviderPanelFilterChange('status')}
                  sx={{ minWidth: { xs: '100%', md: 165 } }}
                >
                  {FILTER_STATUS_OPTIONS.map((option) => (
                    <MenuItem key={`provider-panel-status-${option.value}`} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <Stack direction="row" spacing={1}>
                  <Button
                    type="button"
                    variant="outlined"
                    startIcon={<RefreshRoundedIcon />}
                    onClick={loadProviderTemplates}
                    disabled={providerLoading}
                  >
                    Sync
                  </Button>
                  <Button
                    type="button"
                    variant="contained"
                    startIcon={<AddRoundedIcon />}
                    onClick={openCreateProviderTemplateDialog}
                  >
                    Add Template
                  </Button>
                  <Button
                    type="button"
                    variant="text"
                    onClick={() =>
                      setProviderPanelFilters({
                        category: 'all',
                        provider: '',
                        status: 'all'
                      })
                    }
                  >
                    Reset
                  </Button>
                </Stack>
              </Stack>

              <Typography variant="caption" color="text.secondary">
                {providerLoading
                  ? 'Syncing provider templates...'
                  : `${providerCatalogEntries.length} provider capability profile(s) shown.`}
              </Typography>
            </Stack>
          </Paper>

          {providerLoadError ? <Alert severity="error">{providerLoadError}</Alert> : null}

          {providerLoading ? (
            <Paper sx={{ p: 3, borderRadius: 2 }}>
              <Stack direction="row" spacing={1} alignItems="center">
                <CircularProgress size={18} />
                <Typography variant="body2" color="text.secondary">
                  Loading provider templates...
                </Typography>
              </Stack>
            </Paper>
          ) : providerCatalogEntries.length === 0 ? (
            <Paper sx={{ p: 3, borderRadius: 2 }}>
              <Typography variant="body2" color="text.secondary">
                No provider template matches the current filters.
              </Typography>
            </Paper>
          ) : (
            <Paper sx={{ borderRadius: 2, overflow: 'hidden' }}>
              <TableContainer>
                <Table size="small" sx={{ minWidth: 1000 }}>
                  <TableHead>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 700 }}>Provider</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Category</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Template Overview</TableCell>
                      <TableCell sx={{ fontWeight: 700, width: 320 }}>Actions</TableCell>
                      <TableCell sx={{ fontWeight: 700, width: 56 }}>Expand</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {providerCatalogEntries.map((entry) => {
                      const isExpanded = expandedProviderEntryID === entry.id;
                      const nestedFieldCount = entry.fields.filter(
                        (field) => getFieldPathDepth(field.key) > 0
                      ).length;

                      return (
                        <React.Fragment key={entry.id}>
                          <TableRow
                            hover
                            onClick={() => handleToggleProviderEntry(entry.id)}
                            sx={{ cursor: 'pointer' }}
                          >
                            <TableCell>
                              <Stack spacing={0.2}>
                                <Typography variant="subtitle2" sx={{ textTransform: 'lowercase' }}>
                                  {entry.provider}
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                  Click row to {isExpanded ? 'collapse' : 'expand'} template fields
                                </Typography>
                              </Stack>
                            </TableCell>

                            <TableCell>
                              <Chip
                                size="small"
                                color="primary"
                                variant="outlined"
                                label={CATEGORY_LABEL_BY_VALUE[entry.category] || entry.category}
                              />
                            </TableCell>

                            <TableCell>
                              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                                <Chip
                                  size="small"
                                  variant="outlined"
                                  label={`${entry.fields.length} fields`}
                                />
                                <Chip
                                  size="small"
                                  color={entry.status === 'active' ? 'success' : 'default'}
                                  variant={entry.status === 'active' ? 'filled' : 'outlined'}
                                  label={entry.status}
                                />
                                <Chip
                                  size="small"
                                  variant="outlined"
                                  label={`v${entry.version}`}
                                />
                                <Chip
                                  size="small"
                                  variant="outlined"
                                  label={`${entry.requiredFieldCount} required`}
                                />
                                <Chip
                                  size="small"
                                  variant="outlined"
                                  label={`${nestedFieldCount} nested`}
                                />
                              </Stack>
                            </TableCell>

                            <TableCell>
                              <Stack
                                direction={{ xs: 'column', md: 'row' }}
                                spacing={0.8}
                                alignItems={{ xs: 'stretch', md: 'center' }}
                                onClick={(event) => event.stopPropagation()}
                              >
                                <Button
                                  variant="contained"
                                  size="small"
                                  startIcon={<LaunchRoundedIcon fontSize="small" />}
                                  onClick={() =>
                                    handleCreateFromProviderTemplate(entry.category, entry.provider)
                                  }
                                  disabled={entry.status !== 'active'}
                                >
                                  Create
                                </Button>

                                <Button
                                  size="small"
                                  variant="outlined"
                                  startIcon={<EditRoundedIcon fontSize="small" />}
                                  onClick={() => openEditProviderTemplateDialog(entry)}
                                >
                                  Edit
                                </Button>

                                <Button
                                  size="small"
                                  color="error"
                                  variant="outlined"
                                  startIcon={<DeleteOutlineRoundedIcon fontSize="small" />}
                                  onClick={() => handleDeleteProviderTemplate(entry)}
                                >
                                  Delete
                                </Button>
                              </Stack>
                            </TableCell>

                            <TableCell>
                              <IconButton
                                size="small"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  handleToggleProviderEntry(entry.id);
                                }}
                              >
                                {isExpanded ? (
                                  <KeyboardArrowUpRoundedIcon fontSize="small" />
                                ) : (
                                  <KeyboardArrowDownRoundedIcon fontSize="small" />
                                )}
                              </IconButton>
                            </TableCell>
                          </TableRow>

                          <TableRow>
                            <TableCell
                              colSpan={5}
                              sx={{ py: 0, backgroundColor: 'rgba(246, 250, 248, 0.58)' }}
                            >
                              <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                                <Box sx={{ py: 1.5, px: 0.5 }}>
                                  <Typography variant="subtitle2">Template Fields</Typography>
                                  <Typography variant="caption" color="text.secondary">
                                    Field details are collapsed by default for scanning efficiency.
                                  </Typography>

                                  <Box
                                    sx={{
                                      mt: 1,
                                      display: 'grid',
                                      gap: 0.9,
                                      gridTemplateColumns: {
                                        xs: '1fr',
                                        lg: 'repeat(2, minmax(0, 1fr))'
                                      }
                                    }}
                                  >
                                    {entry.fields.map((field) => {
                                      const defaultValueLabel = formatProviderDefaultValue(
                                        field.defaultValue
                                      );

                                      return (
                                        <Box
                                          key={`${entry.id}-${field.key}`}
                                          sx={{
                                            borderRadius: 2,
                                            border: '1px solid rgba(48, 73, 72, 0.12)',
                                            px: 1.2,
                                            py: 0.95,
                                            backgroundColor: 'rgba(255,255,255,0.86)'
                                          }}
                                        >
                                          <Stack spacing={0.5}>
                                            <Stack
                                              direction={{ xs: 'column', sm: 'row' }}
                                              spacing={0.8}
                                              alignItems={{ xs: 'flex-start', sm: 'center' }}
                                              justifyContent="space-between"
                                            >
                                              <Typography variant="body2" sx={{ fontWeight: 700 }}>
                                                {field.label}
                                              </Typography>

                                              <Stack
                                                direction="row"
                                                spacing={0.6}
                                                flexWrap="wrap"
                                                useFlexGap
                                              >
                                                <Chip
                                                  size="small"
                                                  variant="outlined"
                                                  label={formatProviderFieldType(field)}
                                                />
                                                <Chip
                                                  size="small"
                                                  color={field.required ? 'warning' : 'default'}
                                                  variant={
                                                    field.required ? 'filled' : 'outlined'
                                                  }
                                                  label={
                                                    field.required ? 'required' : 'optional'
                                                  }
                                                />
                                                {defaultValueLabel ? (
                                                  <Chip
                                                    size="small"
                                                    variant="outlined"
                                                    label={`default ${defaultValueLabel}`}
                                                  />
                                                ) : null}
                                              </Stack>
                                            </Stack>

                                            <Typography variant="caption" color="text.secondary">
                                              path: {field.key} | rule:{' '}
                                              {formatProviderFieldRule(field)}
                                            </Typography>

                                            {field.helperText ? (
                                              <Typography variant="caption" color="text.secondary">
                                                {field.helperText}
                                              </Typography>
                                            ) : null}
                                          </Stack>
                                        </Box>
                                      );
                                    })}
                                  </Box>
                                </Box>
                              </Collapse>
                            </TableCell>
                          </TableRow>
                        </React.Fragment>
                      );
                    })}
                  </TableBody>
                </Table>
              </TableContainer>
            </Paper>
          )}

          {loadError ? <Alert severity="error">{loadError}</Alert> : null}
          {notice ? <Alert severity={notice.severity}>{notice.message}</Alert> : null}
        </>
      )}

      <Dialog
        open={revealDialogOpen}
        onClose={closeRevealDialog}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>
          Reveal Access Key {revealTargetResource?.resourceKey ? `for ${revealTargetResource.resourceKey}` : ''}
        </DialogTitle>

        <DialogContent dividers>
          <Stack spacing={1.5} sx={{ pt: 0.5 }}>
            <Alert severity="warning">
              This action requires your account password for re-authentication.
            </Alert>

            {revealedAccessKey ? (
              <Alert severity="success">
                Access key revealed. Avoid screenshots and close this dialog when done.
              </Alert>
            ) : null}

            <TextField
              fullWidth
              size="small"
              label="Current account password"
              type={showRevealPassword ? 'text' : 'password'}
              value={revealPassword}
              onChange={(event) => {
                setRevealPassword(event.target.value);
                if (revealError) {
                  setRevealError('');
                }
              }}
              autoComplete="current-password"
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      edge="end"
                      size="small"
                      onClick={() => setShowRevealPassword((prev) => !prev)}
                      aria-label="toggle password visibility"
                    >
                      {showRevealPassword ? (
                        <VisibilityOffRoundedIcon fontSize="small" />
                      ) : (
                        <VisibilityRoundedIcon fontSize="small" />
                      )}
                    </IconButton>
                  </InputAdornment>
                )
              }}
            />

            {revealedAccessKey ? (
              <TextField
                fullWidth
                size="small"
                label="Plaintext access key"
                type={showRevealedAccessKey ? 'text' : 'password'}
                value={revealedAccessKey}
                InputProps={{
                  readOnly: true,
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        edge="end"
                        size="small"
                        onClick={() => setShowRevealedAccessKey((prev) => !prev)}
                        aria-label="toggle revealed key visibility"
                      >
                        {showRevealedAccessKey ? (
                          <VisibilityOffRoundedIcon fontSize="small" />
                        ) : (
                          <VisibilityRoundedIcon fontSize="small" />
                        )}
                      </IconButton>
                    </InputAdornment>
                  )
                }}
              />
            ) : null}

            {revealError ? <Alert severity="error">{revealError}</Alert> : null}
          </Stack>
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2 }}>
          <Button onClick={closeRevealDialog} disabled={revealSubmitting}>
            Close
          </Button>
          <Button
            variant="contained"
            onClick={handleRevealAccessKey}
            disabled={revealSubmitting}
          >
            {revealSubmitting ? 'Re-authenticating...' : 'Reveal access key'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={providerEditorOpen}
        onClose={closeProviderTemplateDialog}
        fullWidth
        maxWidth="md"
        fullScreen={fullScreenDialog}
      >
        <DialogTitle>
          {providerEditorMode === 'create'
            ? 'Add Provider Template'
            : `Edit Provider Template ${providerEditorForm.category}/${providerEditorForm.provider}`}
        </DialogTitle>

        <DialogContent dividers>
          <Stack spacing={1.75} sx={{ pt: 0.75 }}>
            <Alert severity="warning">
              Provider template changes affect resource creation and edit validation.
            </Alert>

            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.25}>
              <TextField
                select
                fullWidth
                size="small"
                label="Category"
                value={providerEditorForm.category}
                onChange={handleProviderEditorChange('category')}
                disabled={providerEditorMode === 'edit'}
              >
                {FORM_CATEGORY_OPTIONS.map((option) => (
                  <MenuItem key={`provider-editor-${option.value}`} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              <TextField
                fullWidth
                size="small"
                label="Provider key"
                placeholder="e.g. mycloud"
                value={providerEditorForm.provider}
                onChange={handleProviderEditorChange('provider')}
                disabled={providerEditorMode === 'edit'}
                helperText="lowercase key, e.g. dashscope/openai/zhipu"
              />

              <TextField
                select
                fullWidth
                size="small"
                label="Status"
                value={providerEditorForm.status}
                onChange={handleProviderEditorChange('status')}
              >
                {FORM_STATUS_OPTIONS.map((option) => (
                  <MenuItem key={`provider-editor-status-${option.value}`} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>
            </Stack>

            <TextField
              fullWidth
              size="small"
              label="Version"
              type="number"
              value={providerEditorForm.version}
              onChange={handleProviderEditorChange('version')}
              disabled={providerEditorMode === 'edit'}
              inputProps={{ min: 1, step: 1 }}
              helperText={
                providerEditorMode === 'create'
                  ? 'Initial template version. Usually starts from 1.'
                  : 'Current server version (display and update context).'
              }
            />

            {providerEditorError ? (
              <Alert severity="error">{providerEditorError}</Alert>
            ) : null}

            <Stack
              direction={{ xs: 'column', sm: 'row' }}
              alignItems={{ xs: 'flex-start', sm: 'center' }}
              justifyContent="space-between"
              spacing={1}
            >
              <Box>
                <Typography variant="subtitle2">Template Fields</Typography>
                <Typography variant="caption" color="text.secondary">
                  Use field cards to configure typed schema for provider config.
                  Nested object path is supported (for example: `audio.codec.sample_rate_hz`).
                </Typography>
              </Box>

              <Button
                size="small"
                variant="contained"
                startIcon={<AddCircleOutlineRoundedIcon fontSize="small" />}
                onClick={handleAddProviderEditorField}
              >
                Add Field
              </Button>
            </Stack>

            <Stack spacing={1.25}>
              {providerEditorForm.fields.map((field, index) => (
                <Paper
                  key={field.id}
                  variant="outlined"
                  sx={{
                    p: 1.25,
                    borderRadius: 2,
                    borderColor: 'rgba(42, 64, 62, 0.2)',
                    backgroundColor: 'rgba(255,255,255,0.72)'
                  }}
                >
                  <Stack spacing={1.1}>
                    <Stack
                      direction={{ xs: 'column', sm: 'row' }}
                      alignItems={{ xs: 'flex-start', sm: 'center' }}
                      justifyContent="space-between"
                      spacing={1}
                    >
                      <Stack direction="row" spacing={0.75} alignItems="center" useFlexGap>
                        <Typography variant="body2" sx={{ fontWeight: 700 }}>
                          Field #{index + 1}
                        </Typography>
                        {field.key ? (
                          <Chip
                            size="small"
                            variant="outlined"
                            label={`depth ${getFieldPathDepth(field.key)}`}
                          />
                        ) : null}
                        {getFieldPathParent(field.key) ? (
                          <Chip
                            size="small"
                            variant="outlined"
                            label={`parent ${getFieldPathParent(field.key)}`}
                          />
                        ) : null}
                      </Stack>

                      <IconButton
                        size="small"
                        color="error"
                        onClick={() => handleRemoveProviderEditorField(field.id)}
                      >
                        <DeleteOutlineRoundedIcon fontSize="small" />
                      </IconButton>
                    </Stack>

                    <Stack direction={{ xs: 'column', md: 'row' }} spacing={1}>
                      <TextField
                        fullWidth
                        size="small"
                        label="Path"
                        value={field.key}
                        onChange={handleProviderEditorFieldChange(field.id, 'key')}
                        placeholder="audio.codec.sample_rate_hz"
                        helperText="supports nested path via dot notation; base_url/access_key are reserved"
                      />

                      <TextField
                        fullWidth
                        size="small"
                        label="Label"
                        value={field.label}
                        onChange={handleProviderEditorFieldChange(field.id, 'label')}
                        placeholder="Model"
                      />

                      <TextField
                        select
                        fullWidth
                        size="small"
                        label="Type"
                        value={field.type}
                        onChange={handleProviderEditorFieldChange(field.id, 'type')}
                      >
                        {PROVIDER_FIELD_TYPE_OPTIONS.map((option) => (
                          <MenuItem key={`${field.id}-type-${option.value}`} value={option.value}>
                            {option.label}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Stack>

                    {formatFieldPathBreadcrumb(field.key) ? (
                      <Typography variant="caption" color="text.secondary">
                        Path preview: {formatFieldPathBreadcrumb(field.key)}
                      </Typography>
                    ) : null}

                    <Stack direction={{ xs: 'column', md: 'row' }} spacing={1}>
                      <TextField
                        fullWidth
                        size="small"
                        label="Default value"
                        value={field.defaultValue}
                        onChange={handleProviderEditorFieldChange(field.id, 'defaultValue')}
                      />

                      <TextField
                        fullWidth
                        size="small"
                        label="Helper text"
                        value={field.helperText}
                        onChange={handleProviderEditorFieldChange(field.id, 'helperText')}
                      />

                      <FormControlLabel
                        sx={{ pl: 0.25 }}
                        control={
                          <Checkbox
                            checked={Boolean(field.required)}
                            onChange={handleProviderEditorFieldChange(field.id, 'required')}
                          />
                        }
                        label="Required"
                      />
                    </Stack>

                    {field.type === 'text' ? (
                      <TextField
                        fullWidth
                        size="small"
                        label="Placeholder"
                        value={field.placeholder}
                        onChange={handleProviderEditorFieldChange(field.id, 'placeholder')}
                      />
                    ) : null}

                    {field.type === 'number' || field.type === 'integer' ? (
                      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                        <TextField
                          fullWidth
                          size="small"
                          label="Min"
                          type="number"
                          value={field.min}
                          onChange={handleProviderEditorFieldChange(field.id, 'min')}
                        />

                        <TextField
                          fullWidth
                          size="small"
                          label="Max"
                          type="number"
                          value={field.max}
                          onChange={handleProviderEditorFieldChange(field.id, 'max')}
                        />

                        <TextField
                          fullWidth
                          size="small"
                          label="Step"
                          type="number"
                          value={field.step}
                          onChange={handleProviderEditorFieldChange(field.id, 'step')}
                        />
                      </Stack>
                    ) : null}

                    {field.type === 'select' ? (
                      <Box
                        sx={{
                          border: '1px dashed rgba(35, 58, 56, 0.24)',
                          borderRadius: 1.5,
                          p: 1
                        }}
                      >
                        <Stack spacing={0.8}>
                          <Stack
                            direction={{ xs: 'column', sm: 'row' }}
                            alignItems={{ xs: 'flex-start', sm: 'center' }}
                            justifyContent="space-between"
                            spacing={1}
                          >
                            <Typography variant="caption" color="text.secondary">
                              Select Options
                            </Typography>

                            <Button
                              size="small"
                              variant="text"
                              startIcon={<AddCircleOutlineRoundedIcon fontSize="small" />}
                              onClick={() => handleAddProviderEditorOption(field.id)}
                            >
                              Add Option
                            </Button>
                          </Stack>

                          {(field.options || []).map((option) => (
                            <Stack
                              key={option.id}
                              direction={{ xs: 'column', sm: 'row' }}
                              spacing={1}
                              alignItems={{ xs: 'stretch', sm: 'center' }}
                            >
                              <TextField
                                fullWidth
                                size="small"
                                label="Value"
                                value={option.value}
                                onChange={handleProviderEditorOptionChange(
                                  field.id,
                                  option.id,
                                  'value'
                                )}
                              />

                              <TextField
                                fullWidth
                                size="small"
                                label="Label"
                                value={option.label}
                                onChange={handleProviderEditorOptionChange(
                                  field.id,
                                  option.id,
                                  'label'
                                )}
                              />

                              <IconButton
                                size="small"
                                color="error"
                                onClick={() =>
                                  handleRemoveProviderEditorOption(field.id, option.id)
                                }
                              >
                                <RemoveCircleOutlineRoundedIcon fontSize="small" />
                              </IconButton>
                            </Stack>
                          ))}
                        </Stack>
                      </Box>
                    ) : null}
                  </Stack>
                </Paper>
              ))}
            </Stack>
          </Stack>
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2 }}>
          <Button onClick={closeProviderTemplateDialog}>Cancel</Button>
          <Button variant="contained" onClick={handleSaveProviderTemplate}>
            {providerEditorMode === 'create' ? 'Create Template' : 'Save Template'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={dialogOpen}
        onClose={closeDialog}
        fullWidth
        maxWidth="md"
        fullScreen={fullScreenDialog}
      >
        <Box component="form" onSubmit={handleSubmitDialog} noValidate>
          <DialogTitle>
            {dialogMode === 'create'
              ? 'Create platform resource'
              : `Edit ${editingResource?.resourceKey || 'resource'}`}
          </DialogTitle>

          <DialogContent dividers>
            <Stack spacing={2} sx={{ pt: 0.5 }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Category"
                  value={form.category}
                  onChange={handleFormChange('category')}
                  error={Boolean(fieldErrors.category)}
                  helperText={fieldErrors.category || ''}
                >
                  {FORM_CATEGORY_OPTIONS.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Provider"
                  value={form.provider}
                  onChange={handleFormChange('provider')}
                  error={Boolean(fieldErrors.provider)}
                  helperText={fieldErrors.provider || 'Provider set is validated by category'}
                >
                  {formProviderOptions.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Status"
                  value={form.status}
                  onChange={handleFormChange('status')}
                  error={Boolean(fieldErrors.status)}
                  helperText={fieldErrors.status || ''}
                >
                  {FORM_STATUS_OPTIONS.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>
              </Stack>

              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                <TextField
                  fullWidth
                  size="small"
                  label="Resource key"
                  value={form.resourceKey}
                  onChange={handleFormChange('resourceKey')}
                  error={Boolean(fieldErrors.resourceKey)}
                  helperText={fieldErrors.resourceKey || 'Unique key for routing'}
                />

                <TextField
                  fullWidth
                  size="small"
                  label="Display name"
                  value={form.name}
                  onChange={handleFormChange('name')}
                  error={Boolean(fieldErrors.name)}
                  helperText={fieldErrors.name || ''}
                />

                <TextField
                  fullWidth
                  size="small"
                  label="Schema version"
                  type="number"
                  value={form.schemaVersion}
                  onChange={handleFormChange('schemaVersion')}
                  error={Boolean(fieldErrors.schemaVersion)}
                  helperText={
                    fieldErrors.schemaVersion || 'Positive integer, starts from 1'
                  }
                  inputProps={{ min: 1, step: 1 }}
                />
              </Stack>

              <TextField
                fullWidth
                size="small"
                label="Base URL"
                value={form.baseUrl}
                onChange={handleFormChange('baseUrl')}
                error={Boolean(fieldErrors.baseUrl)}
                helperText={
                  fieldErrors.baseUrl ||
                  'Required endpoint base URL for provider requests.'
                }
                placeholder="https://api.example.com/v1"
              />

              <TextField
                fullWidth
                size="small"
                label="Access Key"
                type={showAccessKey ? 'text' : 'password'}
                value={form.accessKey}
                onChange={handleFormChange('accessKey')}
                error={Boolean(fieldErrors.accessKey)}
                helperText={
                  fieldErrors.accessKey ||
                  (dialogMode === 'create'
                    ? 'Provider request key. It will be sent to backend as access_key.'
                    : editingResource?.hasAccessKey
                      ? 'Leave empty to keep the existing key. Fill to rotate.'
                      : 'Optional for edit. Fill it if this resource has no key yet.')
                }
                autoComplete="new-password"
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        edge="end"
                        size="small"
                        onClick={() => setShowAccessKey((prev) => !prev)}
                        aria-label="toggle access key visibility"
                      >
                        {showAccessKey ? (
                          <VisibilityOffRoundedIcon fontSize="small" />
                        ) : (
                          <VisibilityRoundedIcon fontSize="small" />
                        )}
                      </IconButton>
                    </InputAdornment>
                  )
                }}
              />

              <TextField
                fullWidth
                multiline
                minRows={4}
                label="Capabilities (JSON)"
                value={form.capabilitiesText}
                onChange={handleFormChange('capabilitiesText')}
                error={Boolean(fieldErrors.capabilitiesText)}
                helperText={
                  fieldErrors.capabilitiesText ||
                  'Capability tags exposed to downstream selectors.'
                }
                inputProps={{ spellCheck: 'false' }}
                InputProps={{
                  sx: {
                    fontFamily:
                      'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                    fontSize: 13
                  }
                }}
              />

              <Box
                sx={{
                  border: '1px solid rgba(34, 55, 54, 0.15)',
                  borderRadius: 2,
                  p: 1.5,
                  backgroundColor: 'rgba(255, 255, 255, 0.55)'
                }}
              >
                <Stack
                  direction={{ xs: 'column', sm: 'row' }}
                  spacing={1}
                  alignItems={{ xs: 'flex-start', sm: 'center' }}
                  justifyContent="space-between"
                >
                  <Typography variant="subtitle2">Provider Config Template</Typography>
                  <Chip
                    size="small"
                    label={`${form.category.toUpperCase()} / ${form.provider || '--'}`}
                    variant="outlined"
                  />
                </Stack>

                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.75 }}>
                  Fields are fixed by selected provider. Nested keys are shown by path
                  depth and indentation. `base_url` and `access_key` are managed above,
                  outside provider template fields.
                </Typography>

                {fieldErrors.providerConfig ? (
                  <Alert severity="error" sx={{ mt: 1.25 }}>
                    {fieldErrors.providerConfig}
                  </Alert>
                ) : null}

                {providerConfigErrors._template ? (
                  <Alert severity="warning" sx={{ mt: 1.25 }}>
                    {providerConfigErrors._template}
                  </Alert>
                ) : null}

                {providerConfigFields.length > 0 ? (
                  <Stack spacing={1.1} sx={{ mt: 1.5 }}>
                    {providerConfigFields.map((field) => {
                      const value = providerConfigValues[field.key] ?? '';
                      const errorText = providerConfigErrors[field.key] || '';
                      const depth = getFieldPathDepth(field.key);
                      const parentPath = getFieldPathParent(field.key);
                      const breadcrumb = formatFieldPathBreadcrumb(field.key);

                      const helperText =
                        errorText ||
                        field.helperText ||
                        (parentPath ? `Parent path: ${parentPath}` : '');

                      if (field.type === 'select') {
                        return (
                          <Box
                            key={field.key}
                            sx={{
                              ml: depth > 0 ? depth * 1.5 : 0,
                              pl: depth > 0 ? 1 : 0,
                              borderLeft:
                                depth > 0
                                  ? '2px solid rgba(72, 106, 104, 0.25)'
                                  : 'none'
                            }}
                          >
                            <Stack spacing={0.6}>
                              <Stack
                                direction={{ xs: 'column', sm: 'row' }}
                                spacing={0.7}
                                alignItems={{ xs: 'flex-start', sm: 'center' }}
                              >
                                <Typography variant="caption" color="text.secondary">
                                  {breadcrumb || field.key}
                                </Typography>
                                <Chip size="small" variant="outlined" label="enum" />
                                {field.required ? (
                                  <Chip size="small" color="warning" label="required" />
                                ) : null}
                              </Stack>

                              <TextField
                                select
                                fullWidth
                                size="small"
                                required={Boolean(field.required)}
                                label={field.label}
                                value={value}
                                onChange={handleProviderConfigChange(field.key)}
                                error={Boolean(errorText)}
                                helperText={helperText}
                              >
                                {(field.options || []).map((option) => (
                                  <MenuItem
                                    key={`${field.key}-${option.value}`}
                                    value={option.value}
                                  >
                                    {option.label}
                                  </MenuItem>
                                ))}
                              </TextField>
                            </Stack>
                          </Box>
                        );
                      }

                      return (
                        <Box
                          key={field.key}
                          sx={{
                            ml: depth > 0 ? depth * 1.5 : 0,
                            pl: depth > 0 ? 1 : 0,
                            borderLeft:
                              depth > 0
                                ? '2px solid rgba(72, 106, 104, 0.25)'
                                : 'none'
                          }}
                        >
                          <Stack spacing={0.6}>
                            <Stack
                              direction={{ xs: 'column', sm: 'row' }}
                              spacing={0.7}
                              alignItems={{ xs: 'flex-start', sm: 'center' }}
                            >
                              <Typography variant="caption" color="text.secondary">
                                {breadcrumb || field.key}
                              </Typography>
                              <Chip
                                size="small"
                                variant="outlined"
                                label={
                                  field.type === 'integer' || field.type === 'number'
                                    ? field.type
                                    : 'text'
                                }
                              />
                              {field.required ? (
                                <Chip size="small" color="warning" label="required" />
                              ) : null}
                            </Stack>

                            <TextField
                              fullWidth
                              size="small"
                              required={Boolean(field.required)}
                              type={
                                field.type === 'integer' || field.type === 'number'
                                  ? 'number'
                                  : 'text'
                              }
                              label={field.label}
                              value={value}
                              onChange={handleProviderConfigChange(field.key)}
                              error={Boolean(errorText)}
                              helperText={helperText}
                              placeholder={field.placeholder || ''}
                              inputProps={{
                                min: field.min,
                                max: field.max,
                                step:
                                  field.step ||
                                  (field.type === 'integer'
                                    ? 1
                                    : field.type === 'number'
                                      ? 0.1
                                      : undefined)
                              }}
                            />
                          </Stack>
                        </Box>
                      );
                    })}
                  </Stack>
                ) : null}
              </Box>

              {submitError ? <Alert severity="error">{submitError}</Alert> : null}
            </Stack>
          </DialogContent>

          <DialogActions sx={{ px: 3, py: 2 }}>
            <Button onClick={closeDialog} disabled={saving}>
              Cancel
            </Button>
            <Button type="submit" variant="contained" disabled={saving}>
              {saving
                ? dialogMode === 'create'
                  ? 'Creating...'
                  : 'Saving...'
                : dialogMode === 'create'
                  ? 'Create resource'
                  : 'Save changes'}
            </Button>
          </DialogActions>
        </Box>
      </Dialog>
    </Stack>
  );
}
