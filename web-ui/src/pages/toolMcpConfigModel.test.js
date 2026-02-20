import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildMcpConfigFormFromConfig,
  createDefaultMcpConfigForm,
  defaultHeaderForAuthType,
  validateAndBuildMcpConfigPayload
} from './toolMcpConfigModel.js';

test('createDefaultMcpConfigForm returns stdio defaults', () => {
  const form = createDefaultMcpConfigForm();

  assert.equal(form.transport, 'stdio');
  assert.equal(form.timeoutMs, '30000');
  assert.equal(form.authType, 'none');
});

test('buildMcpConfigFormFromConfig maps stdio config to form fields', () => {
  const form = buildMcpConfigFormFromConfig({
    transport: 'stdio',
    timeout_ms: 15000,
    tool_name_list: ['get_device_status', 'set_device_status'],
    auth: {
      type: 'bearer',
      token: 'sk-test',
      header: 'Authorization'
    },
    stdio: {
      command: 'python',
      args: ['server.py', '--port', '8080'],
      env: {
        REGION: 'cn-shanghai'
      },
      cwd: '/opt/mcp'
    }
  });

  assert.equal(form.transport, 'stdio');
  assert.equal(form.timeoutMs, '15000');
  assert.equal(form.authType, 'bearer');
  assert.equal(form.authToken, 'sk-test');
  assert.equal(form.stdioCommand, 'python');
  assert.equal(form.stdioArgsText.includes('server.py'), true);
  assert.equal(form.stdioEnvText.includes('REGION'), true);
  assert.equal(form.stdioCwd, '/opt/mcp');
});

test('validateAndBuildMcpConfigPayload builds stdio payload', () => {
  const result = validateAndBuildMcpConfigPayload({
    transport: 'stdio',
    timeoutMs: '30000',
    toolNameListText: 'get_device_status\nset_device_status',
    authType: 'none',
    stdioCommand: 'python',
    stdioArgsText: 'server.py\n--port\n8080',
    stdioCwd: '/opt/mcp',
    stdioEnvText: '{"REGION":"cn-shanghai"}'
  });

  assert.equal(result.ok, true);
  assert.deepEqual(result.config, {
    transport: 'stdio',
    timeout_ms: 30000,
    tool_name_list: ['get_device_status', 'set_device_status'],
    auth: {
      type: 'none'
    },
    stdio: {
      command: 'python',
      args: ['server.py', '--port', '8080'],
      env: {
        REGION: 'cn-shanghai'
      },
      cwd: '/opt/mcp'
    }
  });
});

test('validateAndBuildMcpConfigPayload enforces transport-specific required fields', () => {
  const missingCommand = validateAndBuildMcpConfigPayload({
    transport: 'stdio',
    timeoutMs: '30000',
    authType: 'none',
    stdioCommand: ''
  });

  assert.equal(missingCommand.ok, false);
  assert.equal(
    missingCommand.fieldErrors.stdioCommand,
    'stdio.command is required for stdio transport.'
  );

  const missingSseEndpoint = validateAndBuildMcpConfigPayload({
    transport: 'sse',
    timeoutMs: '30000',
    authType: 'none',
    sseEndpoint: ''
  });

  assert.equal(missingSseEndpoint.ok, false);
  assert.equal(
    missingSseEndpoint.fieldErrors.sseEndpoint,
    'sse.endpoint is required for sse transport.'
  );
});

test('validateAndBuildMcpConfigPayload validates json object fields', () => {
  const result = validateAndBuildMcpConfigPayload({
    transport: 'stdio',
    timeoutMs: '30000',
    authType: 'none',
    stdioCommand: 'python',
    stdioEnvText: '{bad json}'
  });

  assert.equal(result.ok, false);
  assert.equal(result.fieldErrors.stdioEnvText, 'stdio.env must be valid JSON.');
});

test('validateAndBuildMcpConfigPayload requires token for auth and fills default header', () => {
  const missingToken = validateAndBuildMcpConfigPayload({
    transport: 'sse',
    timeoutMs: '30000',
    authType: 'bearer',
    authToken: '',
    sseEndpoint: 'https://host/mcp/sse'
  });

  assert.equal(missingToken.ok, false);
  assert.equal(
    missingToken.fieldErrors.authToken,
    'Auth token is required for bearer/api_key.'
  );

  const withToken = validateAndBuildMcpConfigPayload({
    transport: 'sse',
    timeoutMs: '30000',
    authType: 'api_key',
    authToken: 'abc-123',
    authHeader: '',
    sseEndpoint: 'https://host/mcp/sse'
  });

  assert.equal(withToken.ok, true);
  assert.equal(withToken.config.auth.header, 'X-API-Key');
});

test('defaultHeaderForAuthType returns expected headers', () => {
  assert.equal(defaultHeaderForAuthType('none'), 'Authorization');
  assert.equal(defaultHeaderForAuthType('bearer'), 'Authorization');
  assert.equal(defaultHeaderForAuthType('api_key'), 'X-API-Key');
});
