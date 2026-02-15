import * as React from 'react';
import Alert from '@mui/material/Alert';
import Snackbar from '@mui/material/Snackbar';
import { useLocation, useNavigate } from 'react-router-dom';
import { ManagerApiError, createManagerClient } from '../api/managerClient.js';
import { managerApiBaseUrl } from '../config.js';
import {
  buildSessionFromAuthPayload,
  canRefreshSession,
  shouldRefreshAccess
} from './sessionModel.js';
import {
  clearSession as clearStoredSession,
  loadSession,
  saveSession
} from './sessionStore.js';

const AuthContext = React.createContext(null);

function normalizeErrorMessage(error, fallback) {
  if (!error) {
    return fallback;
  }
  if (error instanceof ManagerApiError) {
    return error.message || fallback;
  }
  if (typeof error.message === 'string' && error.message.trim()) {
    return error.message;
  }
  return fallback;
}

export function AuthProvider({ children }) {
  const navigate = useNavigate();
  const location = useLocation();
  const client = React.useMemo(() => createManagerClient(managerApiBaseUrl), []);

  const [status, setStatus] = React.useState('loading');
  const [session, setSession] = React.useState(null);
  const [notice, setNotice] = React.useState(null);

  const refreshInFlightRef = React.useRef(null);

  const pushNotice = React.useCallback((message, severity = 'info') => {
    if (!message) {
      return;
    }
    setNotice({ message, severity });
  }, []);

  const persistSession = React.useCallback((nextSession) => {
    setSession(nextSession);
    if (nextSession) {
      saveSession(nextSession);
    } else {
      clearStoredSession();
    }
  }, []);

  const redirectToLogin = React.useCallback(() => {
    if (location.pathname !== '/login') {
      navigate('/login', {
        replace: true,
        state: { from: location.pathname }
      });
    }
  }, [location.pathname, navigate]);

  const clearAuthState = React.useCallback(
    (message) => {
      persistSession(null);
      setStatus('anonymous');
      if (message) {
        pushNotice(message, 'warning');
      }
      redirectToLogin();
    },
    [persistSession, pushNotice, redirectToLogin]
  );

  const runRefresh = React.useCallback(
    async (currentSession, options = {}) => {
      if (!canRefreshSession(currentSession)) {
        throw new Error('Missing refresh token');
      }

      if (refreshInFlightRef.current) {
        return refreshInFlightRef.current;
      }

      const { silent = true } = options;
      const task = (async () => {
        const payload = await client.refresh(currentSession.refreshToken);
        const nextSession = buildSessionFromAuthPayload(payload, currentSession);
        persistSession(nextSession);
        setStatus('authenticated');
        if (!silent) {
          pushNotice('Token refreshed successfully.', 'success');
        }
        return nextSession;
      })();

      refreshInFlightRef.current = task;
      try {
        return await task;
      } finally {
        refreshInFlightRef.current = null;
      }
    },
    [client, persistSession, pushNotice]
  );

  React.useEffect(() => {
    let active = true;

    const bootstrap = async () => {
      const storedSession = loadSession();
      if (!storedSession?.accessToken) {
        if (active) {
          setStatus('anonymous');
        }
        return;
      }

      persistSession(storedSession);

      if (!shouldRefreshAccess(storedSession)) {
        if (active) {
          setStatus('authenticated');
        }
        return;
      }

      if (!canRefreshSession(storedSession)) {
        if (active) {
          clearAuthState('Session expired. Please log in again.');
        }
        return;
      }

      try {
        await runRefresh(storedSession, { silent: true });
      } catch {
        if (active) {
          clearAuthState('Session expired. Please log in again.');
        }
      }
    };

    bootstrap();

    return () => {
      active = false;
    };
  }, [clearAuthState, persistSession, runRefresh]);

  React.useEffect(() => {
    if (status !== 'authenticated' || !session?.accessExpiresAt) {
      return undefined;
    }

    const delay = Math.max(session.accessExpiresAt - Date.now() - 45_000, 5_000);
    const timer = window.setTimeout(() => {
      if (!canRefreshSession(session)) {
        clearAuthState('Session expired. Please log in again.');
        return;
      }
      runRefresh(session, { silent: true }).catch(() => {
        clearAuthState('Session expired. Please log in again.');
      });
    }, delay);

    return () => {
      window.clearTimeout(timer);
    };
  }, [clearAuthState, runRefresh, session, status]);

  const login = React.useCallback(
    async ({ email, password }) => {
      const payload = await client.login({ email, password });
      const nextSession = buildSessionFromAuthPayload(payload);
      persistSession(nextSession);
      setStatus('authenticated');
      return nextSession;
    },
    [client, persistSession]
  );

  const register = React.useCallback(
    async ({ email, password }) => {
      const payload = await client.register({ email, password });

      try {
        const nextSession = buildSessionFromAuthPayload(payload);
        persistSession(nextSession);
        setStatus('authenticated');
        pushNotice('Account created. Signed in automatically.', 'success');
        return { authenticated: true, session: nextSession, payload };
      } catch {
        pushNotice('Account created. Please sign in.', 'success');
        return { authenticated: false, payload };
      }
    },
    [client, persistSession, pushNotice]
  );

  const logout = React.useCallback(async () => {
    try {
      await client.logout({
        accessToken: session?.accessToken,
        refreshToken: session?.refreshToken
      });
    } catch {
      // noop
    }

    persistSession(null);
    setStatus('anonymous');
    pushNotice('Signed out.', 'info');
    navigate('/login', { replace: true });
  }, [client, navigate, persistSession, pushNotice, session]);

  const refreshSession = React.useCallback(async () => {
    if (!session) {
      throw new Error('No active session');
    }
    return runRefresh(session, { silent: false });
  }, [runRefresh, session]);

  const authorizedRequest = React.useCallback(
    async (path, options = {}) => {
      if (!session?.accessToken) {
        clearAuthState('Please sign in first.');
        throw new Error('Unauthorized');
      }

      let activeSession = session;
      if (shouldRefreshAccess(activeSession)) {
        if (!canRefreshSession(activeSession)) {
          clearAuthState('Session expired. Please log in again.');
          throw new Error('Session expired');
        }

        try {
          activeSession = await runRefresh(activeSession, { silent: true });
        } catch {
          clearAuthState('Session expired. Please log in again.');
          throw new Error('Session expired');
        }
      }

      try {
        return await client.request(path, {
          ...options,
          token: activeSession.accessToken
        });
      } catch (error) {
        if (error instanceof ManagerApiError && error.status === 403) {
          pushNotice('Permission denied.', 'warning');
          throw error;
        }

        if (
          !(error instanceof ManagerApiError) ||
          error.status !== 401 ||
          !canRefreshSession(activeSession)
        ) {
          throw error;
        }

        try {
          const refreshedSession = await runRefresh(activeSession, { silent: true });
          return await client.request(path, {
            ...options,
            token: refreshedSession.accessToken
          });
        } catch (refreshError) {
          clearAuthState('Session expired. Please log in again.');
          throw refreshError;
        }
      }
    },
    [clearAuthState, client, pushNotice, runRefresh, session]
  );

  const contextValue = React.useMemo(
    () => ({
      status,
      isAuthenticated: status === 'authenticated',
      session,
      user: session?.user || null,
      login,
      register,
      logout,
      refreshSession,
      authorizedRequest
    }),
    [authorizedRequest, login, logout, refreshSession, register, session, status]
  );

  return (
    <AuthContext.Provider value={contextValue}>
      {children}
      <Snackbar
        open={Boolean(notice)}
        autoHideDuration={2600}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
        onClose={() => setNotice(null)}
      >
        <Alert
          severity={notice?.severity || 'info'}
          variant="filled"
          onClose={() => setNotice(null)}
          sx={{ width: '100%' }}
        >
          {notice?.message}
        </Alert>
      </Snackbar>
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = React.useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider.');
  }
  return context;
}

export function formatAuthError(error) {
  return normalizeErrorMessage(error, 'Request failed. Please try again.');
}
