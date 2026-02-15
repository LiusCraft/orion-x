import ExploreOffRoundedIcon from '@mui/icons-material/ExploreOffRounded';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { Link } from 'react-router-dom';

export default function NotFoundPage() {
  return (
    <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center', px: 2 }}>
      <Paper sx={{ p: 4, borderRadius: 3, maxWidth: 480, width: '100%' }}>
        <Stack spacing={1.5}>
          <ExploreOffRoundedIcon color="secondary" />
          <Typography variant="h5">Page not found</Typography>
          <Typography color="text.secondary">
            The path you requested does not exist in this manager skeleton.
          </Typography>
          <Button component={Link} to="/dashboard" variant="contained">
            Go to dashboard
          </Button>
        </Stack>
      </Paper>
    </Box>
  );
}
