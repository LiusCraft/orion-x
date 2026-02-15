import BlockRoundedIcon from '@mui/icons-material/BlockRounded';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { Link } from 'react-router-dom';

export default function ForbiddenPage() {
  return (
    <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center', px: 2 }}>
      <Paper sx={{ p: 4, borderRadius: 3, maxWidth: 500, width: '100%' }}>
        <Stack spacing={1.5} alignItems="flex-start">
          <BlockRoundedIcon color="warning" />
          <Typography variant="h5">Permission denied</Typography>
          <Typography color="text.secondary">
            Your current role cannot access this page. Please switch to an account
            with proper permissions.
          </Typography>
          <Button component={Link} to="/dashboard" variant="contained">
            Back to dashboard
          </Button>
        </Stack>
      </Paper>
    </Box>
  );
}
