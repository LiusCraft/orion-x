export const ROLE_ADMIN = 'admin';
export const ROLE_NORMAL_USER = 'normal_user';

const ACCESS_REFRESH_SKEW_MS = 45_000;

function isObject(value) {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function pickFirst(source, keys, validator = () => true) {
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(source, key)) {
      continue;
    }
    const value = source[key];
    if (validator(value)) {
      return value;
    }
  }
  return null;
}

function pickString(source, keys) {
  const value = pickFirst(source, keys, (item) => typeof item === 'string');
  if (value === null) {
    return null;
  }
  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}

function pickNumber(source, keys) {
  const value = pickFirst(source, keys, (item) =>
    typeof item === 'number' && Number.isFinite(item)
  );
  return value === null ? null : value;
}

function pickObject(source, keys) {
  return pickFirst(source, keys, (item) => isObject(item));
}

function parseTimestamp(rawValue) {
  if (rawValue === null || rawValue === undefined) {
    return null;
  }

  if (typeof rawValue === 'number' && Number.isFinite(rawValue)) {
    if (rawValue > 1_000_000_000_000) {
      return Math.floor(rawValue);
    }
    return Math.floor(rawValue * 1000);
  }

  if (typeof rawValue === 'string' && rawValue.trim()) {
    const numeric = Number(rawValue);
    if (Number.isFinite(numeric)) {
      return parseTimestamp(numeric);
    }
    const parsed = Date.parse(rawValue);
    if (!Number.isNaN(parsed)) {
      return parsed;
    }
  }

  return null;
}

function decodeJwtPayload(token) {
  if (!token || typeof token !== 'string') {
    return null;
  }
  const segments = token.split('.');
  if (segments.length < 2) {
    return null;
  }

  try {
    const payload = segments[1]
      .replace(/-/g, '+')
      .replace(/_/g, '/')
      .padEnd(Math.ceil(segments[1].length / 4) * 4, '=');
    const decoded = atob(payload);
    const data = JSON.parse(decoded);
    return isObject(data) ? data : null;
  } catch {
    return null;
  }
}

function normalizeStatus(status) {
  if (typeof status !== 'string') {
    return 'active';
  }
  return status.toLowerCase() === 'disabled' ? 'disabled' : 'active';
}

function resolveExpiryAt({
  tokenContainer,
  payload,
  now,
  directKeys,
  durationKeys,
  claims
}) {
  const directValue =
    pickFirst(tokenContainer, directKeys) ?? pickFirst(payload, directKeys);
  const directAt = parseTimestamp(directValue);
  if (directAt) {
    return directAt;
  }

  const seconds =
    pickNumber(tokenContainer, durationKeys) ?? pickNumber(payload, durationKeys);
  if (seconds !== null) {
    return now + Math.floor(seconds * 1000);
  }

  const claimAt = parseTimestamp(claims?.exp);
  if (claimAt) {
    return claimAt;
  }

  return null;
}

export function normalizeRole(role) {
  if (role === ROLE_ADMIN) {
    return ROLE_ADMIN;
  }
  return ROLE_NORMAL_USER;
}

export function buildSessionFromAuthPayload(payload, previousSession = null) {
  if (!isObject(payload)) {
    throw new Error('Invalid auth payload');
  }

  const data = isObject(payload.data) ? payload.data : payload;
  const tokenContainer = pickObject(data, ['tokens', 'token']) || data;

  let accessToken = pickString(tokenContainer, [
    'access_token',
    'accessToken',
    'token'
  ]);
  if (!accessToken && isObject(tokenContainer.access)) {
    accessToken = pickString(tokenContainer.access, [
      'token',
      'access_token',
      'accessToken',
      'value'
    ]);
  }

  let refreshToken = pickString(tokenContainer, [
    'refresh_token',
    'refreshToken'
  ]);
  if (!refreshToken && isObject(tokenContainer.refresh)) {
    refreshToken = pickString(tokenContainer.refresh, [
      'token',
      'refresh_token',
      'refreshToken',
      'value'
    ]);
  }

  if (!refreshToken) {
    refreshToken = previousSession?.refreshToken || null;
  }

  if (!accessToken) {
    throw new Error('Missing access token in auth response');
  }

  const now = Date.now();
  const accessClaims = decodeJwtPayload(accessToken);
  const refreshClaims = decodeJwtPayload(refreshToken);

  const accessExpiresAt = resolveExpiryAt({
    tokenContainer,
    payload: data,
    now,
    directKeys: ['access_expires_at', 'accessExpiresAt', 'expires_at', 'expiresAt'],
    durationKeys: ['access_expires_in', 'accessExpiresIn', 'expires_in', 'expiresIn'],
    claims: accessClaims
  });

  const refreshExpiresAt = resolveExpiryAt({
    tokenContainer,
    payload: data,
    now,
    directKeys: ['refresh_expires_at', 'refreshExpiresAt'],
    durationKeys: ['refresh_expires_in', 'refreshExpiresIn'],
    claims: refreshClaims
  });

  const userSource =
    pickObject(data, ['user', 'profile']) ||
    pickObject(tokenContainer, ['user', 'profile']) ||
    {};

  const role = normalizeRole(
    pickString(userSource, ['role']) ||
      (typeof accessClaims?.role === 'string' ? accessClaims.role : null) ||
      previousSession?.user?.role
  );

  const user = {
    id:
      pickString(userSource, ['id', 'user_id', 'userId']) ||
      (typeof accessClaims?.sub === 'string' ? accessClaims.sub : null) ||
      previousSession?.user?.id ||
      '',
    email:
      pickString(userSource, ['email', 'username']) ||
      (typeof accessClaims?.email === 'string' ? accessClaims.email : null) ||
      previousSession?.user?.email ||
      '',
    name:
      pickString(userSource, ['name', 'display_name', 'displayName']) ||
      previousSession?.user?.name ||
      '',
    role,
    status: normalizeStatus(
      pickString(userSource, ['status']) || previousSession?.user?.status
    )
  };

  return {
    accessToken,
    refreshToken,
    accessExpiresAt,
    refreshExpiresAt,
    user,
    updatedAt: now
  };
}

export function isTokenExpired(expiresAt, skewMs = 0) {
  if (!expiresAt) {
    return false;
  }
  return Date.now() + skewMs >= expiresAt;
}

export function shouldRefreshAccess(session) {
  if (!session?.accessToken) {
    return false;
  }
  return isTokenExpired(session.accessExpiresAt, ACCESS_REFRESH_SKEW_MS);
}

export function canRefreshSession(session) {
  if (!session?.refreshToken) {
    return false;
  }
  return !isTokenExpired(session.refreshExpiresAt, 5_000);
}

export function roleDisplayName(role) {
  return role === ROLE_ADMIN ? 'admin' : 'normal_user';
}
