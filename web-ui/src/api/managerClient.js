function trimTrailingSlash(url) {
  return url.replace(/\/+$/, '');
}

function buildUrl(baseUrl, path) {
  if (/^https?:\/\//.test(path)) {
    return path;
  }
  const normalizedBase = trimTrailingSlash(baseUrl || '');
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${normalizedBase}${normalizedPath}`;
}

async function parseJson(response) {
  const text = await response.text();
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function parseResponse(response, payload) {
  if (response.ok) {
    if (payload && typeof payload === 'object' && 'code' in payload) {
      if (payload.code !== 'OK') {
        throw new ManagerApiError(payload.message || 'Request failed', {
          status: response.status,
          code: payload.code || 'ERR_REQUEST',
          details: payload
        });
      }
      return payload.data ?? null;
    }
    return payload;
  }

  const message =
    payload?.message || response.statusText || 'Manager API request failed';
  const code = payload?.code || `HTTP_${response.status}`;
  throw new ManagerApiError(message, {
    status: response.status,
    code,
    details: payload
  });
}

export class ManagerApiError extends Error {
  constructor(message, options = {}) {
    super(message);
    this.name = 'ManagerApiError';
    this.status = options.status || 0;
    this.code = options.code || 'ERR_REQUEST';
    this.details = options.details || null;
  }
}

export function createManagerClient(baseUrl) {
  const request = async (path, options = {}) => {
    const {
      method = 'GET',
      body,
      token,
      headers = {},
      signal
    } = options;
    const url = buildUrl(baseUrl, path);
    const requestHeaders = { ...headers };
    const requestInit = {
      method,
      headers: requestHeaders,
      signal
    };

    if (token) {
      requestHeaders.Authorization = `Bearer ${token}`;
    }

    if (body !== undefined) {
      const isFormData = typeof FormData !== 'undefined' && body instanceof FormData;
      if (typeof body === 'string' || isFormData) {
        requestInit.body = body;
      } else {
        requestHeaders['Content-Type'] = 'application/json';
        requestInit.body = JSON.stringify(body);
      }
    }

    const response = await fetch(url, requestInit);
    const payload = await parseJson(response);
    return parseResponse(response, payload);
  };

  return {
    request,
    register(credentials) {
      return request('/api/v1/auth/register', {
        method: 'POST',
        body: credentials
      });
    },
    login(credentials) {
      return request('/api/v1/auth/login', {
        method: 'POST',
        body: credentials
      });
    },
    refresh(refreshToken) {
      return request('/api/v1/auth/refresh', {
        method: 'POST',
        body: {
          refresh_token: refreshToken
        }
      });
    },
    logout({ accessToken, refreshToken } = {}) {
      return request('/api/v1/auth/logout', {
        method: 'POST',
        token: accessToken,
        body: refreshToken
          ? {
              refresh_token: refreshToken
            }
          : undefined
      });
    }
  };
}
