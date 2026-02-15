import * as React from 'react';
import LockOpenRoundedIcon from '@mui/icons-material/LockOpenRounded';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { Link as RouterLink, useLocation, useNavigate } from 'react-router-dom';
import { formatAuthError, useAuth } from '../auth/AuthProvider.jsx';
import { managerApiBaseUrl } from '../config.js';

const initialForm = {
  email: '',
  password: ''
};

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { login, isAuthenticated, status } = useAuth();

  const [form, setForm] = React.useState(initialForm);
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState('');

  const apiEndpointLabel = managerApiBaseUrl || '/api (via Vite proxy)';
  const registeredEmail = location.state?.registered
    ? location.state?.email || ''
    : '';

  const targetPath = location.state?.from || '/dashboard';

  React.useEffect(() => {
    if (isAuthenticated) {
      navigate(targetPath, { replace: true });
    }
  }, [isAuthenticated, navigate, targetPath]);

  const handleChange = (field) => (event) => {
    setForm((prev) => ({
      ...prev,
      [field]: event.target.value
    }));
  };

  const handleSubmit = async (event) => {
    event.preventDefault();
    setSubmitting(true);
    setError('');

    try {
      await login(form);
      navigate(targetPath, { replace: true });
    } catch (submitError) {
      setError(formatAuthError(submitError));
    } finally {
      setSubmitting(false);
    }
  };

  if (status === 'loading') {
    return (
      <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'grid',
        placeItems: 'center',
        px: 2,
        py: 4
      }}
    >
      <Paper
        elevation={0}
        sx={{
          width: '100%',
          maxWidth: 468,
          p: { xs: 3, sm: 4 },
          borderRadius: 4,
          border: '1px solid rgba(41, 64, 62, 0.15)'
        }}
      >
        <Stack spacing={2}>
          <Box>
            <Typography variant="h4">Manager Login</Typography>
            <Typography color="text.secondary" sx={{ mt: 1 }}>
              Sign in to access protected routes and role-based menus.
            </Typography>
          </Box>

          <Alert icon={<LockOpenRoundedIcon fontSize="inherit" />} severity="info">
            API base URL: <strong>{apiEndpointLabel}</strong>
          </Alert>

          {registeredEmail ? (
            <Alert severity="success">
              Account created for <strong>{registeredEmail}</strong>. Please sign in.
            </Alert>
          ) : null}

          <Box component="form" onSubmit={handleSubmit} noValidate>
            <Stack spacing={2}>
              <TextField
                required
                fullWidth
                label="Email"
                type="email"
                autoComplete="username"
                value={form.email}
                onChange={handleChange('email')}
              />

              <TextField
                required
                fullWidth
                label="Password"
                type="password"
                autoComplete="current-password"
                value={form.password}
                onChange={handleChange('password')}
              />

              {error ? <Alert severity="error">{error}</Alert> : null}

              <Button
                type="submit"
                fullWidth
                variant="contained"
                size="large"
                disabled={submitting}
              >
                {submitting ? 'Signing in...' : 'Sign in'}
              </Button>

              <Typography variant="body2" color="text.secondary" textAlign="center">
                Need an account?{' '}
                <Link
                  component={RouterLink}
                  to="/register"
                  state={{ from: targetPath }}
                  underline="hover"
                >
                  Create an account
                </Link>
              </Typography>
            </Stack>
          </Box>
        </Stack>
      </Paper>
    </Box>
  );
}
