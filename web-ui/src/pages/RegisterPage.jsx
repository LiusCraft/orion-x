import * as React from 'react';
import PersonAddRoundedIcon from '@mui/icons-material/PersonAddRounded';
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

const initialForm = {
  email: '',
  password: '',
  confirmPassword: ''
};

export default function RegisterPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { register, isAuthenticated, status } = useAuth();

  const [form, setForm] = React.useState(initialForm);
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState('');

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

    if (!form.email.trim()) {
      setError('Email is required.');
      return;
    }

    if (!form.password) {
      setError('Password is required.');
      return;
    }

    if (form.password !== form.confirmPassword) {
      setError('Passwords do not match.');
      return;
    }

    setSubmitting(true);
    setError('');

    try {
      const result = await register({
        email: form.email.trim(),
        password: form.password
      });

      if (result.authenticated) {
        navigate(targetPath, { replace: true });
      } else {
        navigate('/login', {
          replace: true,
          state: {
            from: targetPath,
            registered: true,
            email: form.email.trim().toLowerCase()
          }
        });
      }
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
          maxWidth: 488,
          p: { xs: 3, sm: 4 },
          borderRadius: 4,
          border: '1px solid rgba(41, 64, 62, 0.15)'
        }}
      >
        <Stack spacing={2}>
          <Box>
            <Typography variant="h4">Create account</Typography>
            <Typography color="text.secondary" sx={{ mt: 1 }}>
              Register with your email and sign in immediately.
            </Typography>
          </Box>

          <Alert icon={<PersonAddRoundedIcon fontSize="inherit" />} severity="info">
            Endpoint: <strong>POST /api/v1/auth/register</strong>
          </Alert>

          <Box component="form" onSubmit={handleSubmit} noValidate>
            <Stack spacing={2}>
              <TextField
                required
                fullWidth
                label="Email"
                type="email"
                autoComplete="email"
                value={form.email}
                onChange={handleChange('email')}
              />

              <TextField
                required
                fullWidth
                label="Password"
                type="password"
                autoComplete="new-password"
                value={form.password}
                onChange={handleChange('password')}
                helperText="Current backend rule: password must be non-empty"
              />

              <TextField
                required
                fullWidth
                label="Confirm password"
                type="password"
                autoComplete="new-password"
                value={form.confirmPassword}
                onChange={handleChange('confirmPassword')}
              />

              {error ? <Alert severity="error">{error}</Alert> : null}

              <Button
                type="submit"
                fullWidth
                variant="contained"
                size="large"
                disabled={submitting}
              >
                {submitting ? 'Creating account...' : 'Create account'}
              </Button>

              <Typography variant="body2" color="text.secondary" textAlign="center">
                Already have an account?{' '}
                <Link component={RouterLink} to="/login" underline="hover">
                  Sign in
                </Link>
              </Typography>
            </Stack>
          </Box>
        </Stack>
      </Paper>
    </Box>
  );
}
