import assert from 'node:assert/strict';
import test from 'node:test';
import {
  ROLE_ADMIN,
  buildSessionFromAuthPayload,
  canRefreshSession,
  shouldRefreshAccess
} from './sessionModel.js';

function createFakeJwt(payload) {
  const base64Url = Buffer.from(JSON.stringify(payload)).toString('base64url');
  return `header.${base64Url}.signature`;
}

test('buildSessionFromAuthPayload parses tokens and user role', () => {
  const accessToken = createFakeJwt({
    sub: 'u-1',
    email: 'admin@example.com',
    role: ROLE_ADMIN,
    exp: Math.floor(Date.now() / 1000) + 3600
  });

  const refreshToken = createFakeJwt({
    sub: 'u-1',
    exp: Math.floor(Date.now() / 1000) + 7200
  });

  const session = buildSessionFromAuthPayload({
    access_token: accessToken,
    refresh_token: refreshToken,
    user: {
      id: 'u-1',
      email: 'admin@example.com',
      role: ROLE_ADMIN
    }
  });

  assert.equal(session.user.id, 'u-1');
  assert.equal(session.user.role, ROLE_ADMIN);
  assert.equal(session.accessToken, accessToken);
  assert.equal(session.refreshToken, refreshToken);
  assert.equal(typeof session.accessExpiresAt, 'number');
});

test('buildSessionFromAuthPayload keeps previous refresh token', () => {
  const accessToken = createFakeJwt({
    role: 'normal_user',
    exp: Math.floor(Date.now() / 1000) + 3600
  });

  const previousSession = {
    refreshToken: 'legacy-refresh-token',
    user: {
      role: 'normal_user'
    }
  };

  const session = buildSessionFromAuthPayload(
    {
      access_token: accessToken
    },
    previousSession
  );

  assert.equal(session.refreshToken, 'legacy-refresh-token');
});

test('token lifecycle helpers determine refresh behavior', () => {
  const now = Date.now();
  const session = {
    accessToken: 'x',
    accessExpiresAt: now + 10_000,
    refreshToken: 'y',
    refreshExpiresAt: now + 120_000
  };

  assert.equal(shouldRefreshAccess(session), true);
  assert.equal(canRefreshSession(session), true);
  assert.equal(canRefreshSession({ refreshToken: null }), false);
});
