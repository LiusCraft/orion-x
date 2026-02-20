const TRANSPORT_SET = new Set(['stdio', 'sse', 'stream_http']);
const AUTH_TYPE_SET = new Set(['none', 'bearer', 'api_key']);

export const MCP_TRANSPORT_OPTIONS = Object.freeze([
  { value: 'stdio', label: 'stdio' },
  { value: 'sse', label: 'sse' },
  { value: 'stream_http', label: 'stream_http' }
]);

export const MCP_AUTH_TYPE_OPTIONS = Object.freeze([
  { value: 'none', label: 'none' },
  { value: 'bearer', label: 'bearer' },
  { value: 'api_key', label: 'api_key' }
]);

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

function splitMultilineList(text) {
  return String(text || '')
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function stringifyObject(value) {
  const normalized = normalizeObject(value);
  return JSON.stringify(normalized, null, 2);
}

export function defaultHeaderForAuthType(authType) {
  if (authType === 'api_key') {
    return 'X-API-Key';
  }

  return 'Authorization';
}

export function createDefaultMcpConfigForm() {
  return {
    transport: 'stdio',
    timeoutMs: '30000',
    toolNameListText: '',
    authType: 'none',
    authToken: '',
    authHeader: 'Authorization',
    stdioCommand: '',
    stdioArgsText: '',
    stdioCwd: '',
    stdioEnvText: '{}',
    sseEndpoint: '',
    sseHeadersText: '{}',
    streamHttpEndpoint: '',
    streamHttpHeadersText: '{}'
  };
}

export function buildMcpConfigFormFromConfig(config = {}) {
  const normalized = normalizeObject(config);
  const transport = TRANSPORT_SET.has(normalizeLower(normalized.transport))
    ? normalizeLower(normalized.transport)
    : 'stdio';

  const timeoutMs = Number.isInteger(normalized.timeout_ms) && normalized.timeout_ms > 0
    ? String(normalized.timeout_ms)
    : '30000';

  const toolNameListText = normalizeStringArray(normalized.tool_name_list).join('\n');

  const auth = normalizeObject(normalized.auth);
  const authType = AUTH_TYPE_SET.has(normalizeLower(auth.type))
    ? normalizeLower(auth.type)
    : 'none';
  const authToken = normalizeString(auth.token);
  const authHeader = normalizeString(auth.header) || defaultHeaderForAuthType(authType);

  const stdio = normalizeObject(normalized.stdio);
  const sse = normalizeObject(normalized.sse);
  const streamHttp = normalizeObject(normalized.stream_http);

  return {
    transport,
    timeoutMs,
    toolNameListText,
    authType,
    authToken,
    authHeader,
    stdioCommand: normalizeString(stdio.command),
    stdioArgsText: normalizeStringArray(stdio.args).join('\n'),
    stdioCwd: normalizeString(stdio.cwd),
    stdioEnvText: stringifyObject(stdio.env),
    sseEndpoint: normalizeString(sse.endpoint),
    sseHeadersText: stringifyObject(sse.headers),
    streamHttpEndpoint: normalizeString(streamHttp.endpoint),
    streamHttpHeadersText: stringifyObject(streamHttp.headers)
  };
}

function parsePositiveInteger(text, fieldLabel) {
  const source = normalizeString(String(text || ''));
  if (!source) {
    return {
      ok: false,
      value: 0,
      error: `${fieldLabel} is required.`
    };
  }

  const parsed = Number.parseInt(source, 10);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return {
      ok: false,
      value: 0,
      error: `${fieldLabel} must be a positive integer.`
    };
  }

  return {
    ok: true,
    value: parsed,
    error: ''
  };
}

function parseJsonObjectText(text, fieldLabel) {
  const source = normalizeString(String(text || ''));
  if (!source) {
    return {
      ok: true,
      value: {},
      error: ''
    };
  }

  try {
    const parsed = JSON.parse(source);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {
        ok: false,
        value: {},
        error: `${fieldLabel} must be a JSON object.`
      };
    }

    return {
      ok: true,
      value: parsed,
      error: ''
    };
  } catch {
    return {
      ok: false,
      value: {},
      error: `${fieldLabel} must be valid JSON.`
    };
  }
}

export function validateAndBuildMcpConfigPayload(form = {}) {
  const fieldErrors = {};
  const normalizedForm = {
    ...createDefaultMcpConfigForm(),
    ...form
  };

  const transport = normalizeLower(normalizedForm.transport);
  if (!TRANSPORT_SET.has(transport)) {
    fieldErrors.transport = 'Transport must be one of stdio, sse, stream_http.';
  }

  const timeoutResult = parsePositiveInteger(normalizedForm.timeoutMs, 'Timeout (ms)');
  if (!timeoutResult.ok) {
    fieldErrors.timeoutMs = timeoutResult.error;
  }

  const authType = normalizeLower(normalizedForm.authType);
  if (!AUTH_TYPE_SET.has(authType)) {
    fieldErrors.authType = 'Auth type must be none, bearer, or api_key.';
  }

  const authToken = normalizeString(normalizedForm.authToken);
  const authHeader = normalizeString(normalizedForm.authHeader);
  if ((authType === 'bearer' || authType === 'api_key') && !authToken) {
    fieldErrors.authToken = 'Auth token is required for bearer/api_key.';
  }

  const toolNameList = splitMultilineList(normalizedForm.toolNameListText);

  const stdioEnvResult = parseJsonObjectText(normalizedForm.stdioEnvText, 'stdio.env');
  if (!stdioEnvResult.ok) {
    fieldErrors.stdioEnvText = stdioEnvResult.error;
  }

  const sseHeadersResult = parseJsonObjectText(normalizedForm.sseHeadersText, 'sse.headers');
  if (!sseHeadersResult.ok) {
    fieldErrors.sseHeadersText = sseHeadersResult.error;
  }

  const streamHeadersResult = parseJsonObjectText(
    normalizedForm.streamHttpHeadersText,
    'stream_http.headers'
  );
  if (!streamHeadersResult.ok) {
    fieldErrors.streamHttpHeadersText = streamHeadersResult.error;
  }

  const stdioCommand = normalizeString(normalizedForm.stdioCommand);
  const stdioCwd = normalizeString(normalizedForm.stdioCwd);
  const stdioArgs = splitMultilineList(normalizedForm.stdioArgsText);
  const sseEndpoint = normalizeString(normalizedForm.sseEndpoint);
  const streamHttpEndpoint = normalizeString(normalizedForm.streamHttpEndpoint);

  if (transport === 'stdio' && !stdioCommand) {
    fieldErrors.stdioCommand = 'stdio.command is required for stdio transport.';
  }

  if (transport === 'sse' && !sseEndpoint) {
    fieldErrors.sseEndpoint = 'sse.endpoint is required for sse transport.';
  }

  if (transport === 'stream_http' && !streamHttpEndpoint) {
    fieldErrors.streamHttpEndpoint =
      'stream_http.endpoint is required for stream_http transport.';
  }

  if (Object.keys(fieldErrors).length > 0) {
    return {
      ok: false,
      config: {},
      fieldErrors
    };
  }

  const config = {
    transport,
    timeout_ms: timeoutResult.value,
    tool_name_list: toolNameList,
    auth: {
      type: authType
    }
  };

  if (authType !== 'none') {
    config.auth.token = authToken;
    config.auth.header = authHeader || defaultHeaderForAuthType(authType);
  }

  if (transport === 'stdio') {
    config.stdio = {
      command: stdioCommand
    };

    if (stdioArgs.length > 0) {
      config.stdio.args = stdioArgs;
    }

    if (Object.keys(stdioEnvResult.value).length > 0) {
      config.stdio.env = stdioEnvResult.value;
    }

    if (stdioCwd) {
      config.stdio.cwd = stdioCwd;
    }
  }

  if (transport === 'sse') {
    config.sse = {
      endpoint: sseEndpoint
    };

    if (Object.keys(sseHeadersResult.value).length > 0) {
      config.sse.headers = sseHeadersResult.value;
    }
  }

  if (transport === 'stream_http') {
    config.stream_http = {
      endpoint: streamHttpEndpoint
    };

    if (Object.keys(streamHeadersResult.value).length > 0) {
      config.stream_http.headers = streamHeadersResult.value;
    }
  }

  return {
    ok: true,
    config,
    fieldErrors: {}
  };
}
