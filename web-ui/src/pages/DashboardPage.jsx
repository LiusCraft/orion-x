import * as React from 'react';
import BoltRoundedIcon from '@mui/icons-material/BoltRounded';
import VerifiedUserRoundedIcon from '@mui/icons-material/VerifiedUserRounded';
import VpnKeyRoundedIcon from '@mui/icons-material/VpnKeyRounded';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useAuth } from '../auth/AuthProvider.jsx';

function formatTimestamp(timestamp) {
  if (!timestamp) {
    return 'Not provided by backend';
  }
  return new Date(timestamp).toLocaleString();
}

function SessionMetricCard({ icon, label, value, hint }) {
  const Icon = icon;

  return (
    <Paper sx={{ p: 2.2, borderRadius: 3, height: '100%' }}>
      <Stack spacing={1.1}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Icon color="primary" fontSize="small" />
          <Typography variant="subtitle2">{label}</Typography>
        </Stack>
        <Typography variant="body1" sx={{ fontWeight: 700 }}>
          {value}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {hint}
        </Typography>
      </Stack>
    </Paper>
  );
}

export default function DashboardPage() {
  const { session, refreshSession, user } = useAuth();
  const [refreshing, setRefreshing] = React.useState(false);
  const [refreshError, setRefreshError] = React.useState('');

  const handleRefreshToken = async () => {
    setRefreshing(true);
    setRefreshError('');

    try {
      await refreshSession();
    } catch (error) {
      setRefreshError(error.message || 'Unable to refresh token');
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <Stack spacing={2.25}>
      <Paper sx={{ p: { xs: 2.5, md: 3 }, borderRadius: 3 }}>
        <Stack spacing={1.2}>
          <Typography variant="h4">Manager Console</Typography>
          <Typography color="text.secondary">
            Login state, auth refresh, and role-aware menu are now wired before any
            feature-specific pages.
          </Typography>

          <Box>
            <Button
              variant="contained"
              startIcon={<BoltRoundedIcon />}
              onClick={handleRefreshToken}
              disabled={refreshing}
            >
              {refreshing ? 'Refreshing token...' : 'Refresh token now'}
            </Button>
          </Box>

          {refreshError ? <Alert severity="error">{refreshError}</Alert> : null}
        </Stack>
      </Paper>

      <Grid container spacing={2}>
        <Grid item xs={12} md={4}>
          <SessionMetricCard
            icon={VerifiedUserRoundedIcon}
            label="Current Role"
            value={user?.role || 'unknown'}
            hint="Controls menu and route permissions"
          />
        </Grid>
        <Grid item xs={12} md={4}>
          <SessionMetricCard
            icon={VpnKeyRoundedIcon}
            label="Access Token Expiry"
            value={formatTimestamp(session?.accessExpiresAt)}
            hint="Auto refresh triggers before this timestamp"
          />
        </Grid>
        <Grid item xs={12} md={4}>
          <SessionMetricCard
            icon={BoltRoundedIcon}
            label="Refresh Token Expiry"
            value={formatTimestamp(session?.refreshExpiresAt)}
            hint="If expired, user is redirected back to login"
          />
        </Grid>
      </Grid>
    </Stack>
  );
}
