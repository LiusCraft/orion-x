import Box from '@mui/material/Box';
import Fab from '@mui/material/Fab';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import CallEndIcon from '@mui/icons-material/CallEnd';
import PhoneIcon from '@mui/icons-material/Phone';

export default function CallBar({ calling, onToggleCall, disabled }) {
  return (
    <Box
      sx={{
        px: 2,
        py: 1.5,
        borderRadius: 3,
        bgcolor: 'rgba(255,255,255,0.04)',
        border: '1px solid rgba(255,255,255,0.08)'
      }}
    >
      <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between">
        <Stack>
          <Typography variant="subtitle2">电话通话</Typography>
          <Typography variant="caption" color="text.secondary">
            {calling ? '通话中，持续发送语音' : '点击拨打开始持续对话'}
          </Typography>
        </Stack>
        <Fab
          color={calling ? 'error' : 'primary'}
          size="medium"
          onClick={onToggleCall}
          disabled={disabled}
        >
          {calling ? <CallEndIcon /> : <PhoneIcon />}
        </Fab>
      </Stack>
    </Box>
  );
}
