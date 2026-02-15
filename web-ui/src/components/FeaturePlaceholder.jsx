import CheckCircleRoundedIcon from '@mui/icons-material/CheckCircleRounded';
import ArrowForwardRoundedIcon from '@mui/icons-material/ArrowForwardRounded';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';

export default function FeaturePlaceholder({
  title,
  description,
  scope = [],
  nextSteps = []
}) {
  return (
    <Paper sx={{ p: { xs: 2.5, md: 3 }, borderRadius: 3 }}>
      <Stack spacing={2}>
        <Box>
          <Typography variant="h5">{title}</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.75 }}>
            {description}
          </Typography>
        </Box>

        <Box>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>
            Scope
          </Typography>
          <List dense disablePadding>
            {scope.map((item) => (
              <ListItem key={item} disableGutters>
                <ListItemIcon sx={{ minWidth: 30 }}>
                  <CheckCircleRoundedIcon color="success" fontSize="small" />
                </ListItemIcon>
                <ListItemText primary={item} primaryTypographyProps={{ variant: 'body2' }} />
              </ListItem>
            ))}
          </List>
        </Box>

        {nextSteps.length > 0 ? (
          <Box>
            <Typography variant="subtitle2" sx={{ mb: 1 }}>
              Next
            </Typography>
            <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap">
              {nextSteps.map((item) => (
                <Chip
                  key={item}
                  icon={<ArrowForwardRoundedIcon fontSize="small" />}
                  label={item}
                  size="small"
                  variant="outlined"
                />
              ))}
            </Stack>
          </Box>
        ) : null}
      </Stack>
    </Paper>
  );
}
