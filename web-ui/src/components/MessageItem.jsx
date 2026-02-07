import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { formatTime } from '../utils/time.js';

export default function MessageItem({ message }) {
  const isUser = message.role === 'user';
  const align = isUser ? 'flex-end' : 'flex-start';
  const bgColor = isUser ? 'rgba(77, 163, 255, 0.15)' : 'rgba(255,255,255,0.06)';

  return (
    <Box sx={{ display: 'flex', justifyContent: align, mb: 1.5 }}>
      <Paper
        elevation={0}
        sx={{
          px: 2,
          py: 1.5,
          maxWidth: '70%',
          bgcolor: bgColor,
          border: '1px solid rgba(255,255,255,0.08)'
        }}
      >
        <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
          <Typography variant="caption" color="text.secondary">
            {isUser ? '你' : '小智'}
          </Typography>
          {message.stt && <Chip label="语音识别" size="small" />}
          {message.tts && (
            <Chip
              label="语音回复"
              size="small"
              color="primary"
              variant="outlined"
            />
          )}
          <Typography variant="caption" color="text.disabled">
            {formatTime(message.ts)}
          </Typography>
        </Stack>
        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
          {message.text}
        </Typography>
      </Paper>
    </Box>
  );
}
