import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import AccordionSummary from '@mui/material/AccordionSummary';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';

const readyStateLabel = {
  0: 'CONNECTING',
  1: 'OPEN',
  2: 'CLOSING',
  3: 'CLOSED',
  null: 'N/A'
};

function formatBytes(bytes = 0) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
}

export default function DebugPanel({ stats, connState, sessionState, wsReadyState }) {
  const safeStats = stats || {};
  return (
    <Accordion
      defaultExpanded={false}
      sx={{
        bgcolor: 'rgba(255,255,255,0.04)',
        border: '1px solid rgba(255,255,255,0.08)',
        boxShadow: 'none'
      }}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant="subtitle2">调试面板</Typography>
          <Chip
            size="small"
            label={`WS ${readyStateLabel[wsReadyState ?? null] || 'N/A'}`}
          />
          <Chip size="small" label={`Conn ${connState}`} variant="outlined" />
          <Chip size="small" label={`State ${sessionState}`} variant="outlined" />
        </Stack>
      </AccordionSummary>
      <AccordionDetails>
        <Stack spacing={1.5}>
          <Stack direction="row" spacing={1} flexWrap="wrap">
            <Chip size="small" label={`AudioContext: ${safeStats.audioContextState || 'none'}`} />
            <Chip size="small" label={`Buffering: ${safeStats.isAudioBuffering ? 'yes' : 'no'}`} />
            <Chip size="small" label={`Playing: ${safeStats.isAudioPlaying ? 'yes' : 'no'}`} />
            <Chip size="small" label={`Encoder: ${safeStats.opusEncoderReady ? 'ready' : 'no'}`} />
            <Chip size="small" label={`Decoder: ${safeStats.opusDecoderReady ? 'ready' : 'no'}`} />
          </Stack>

          <Divider />

          <Box>
            <Typography variant="caption" color="text.secondary">
              收包
            </Typography>
            <Stack direction="row" spacing={2} sx={{ mt: 0.5 }}>
              <Typography variant="body2">帧数：{safeStats.receivedFrames || 0}</Typography>
              <Typography variant="body2">总量：{formatBytes(safeStats.receivedBytes || 0)}</Typography>
              <Typography variant="body2">最后帧：{formatBytes(safeStats.lastFrameBytes || 0)}</Typography>
            </Stack>
          </Box>

          <Box>
            <Typography variant="caption" color="text.secondary">
              解码/播放
            </Typography>
            <Stack direction="row" spacing={2} sx={{ mt: 0.5 }}>
              <Typography variant="body2">解码样本：{safeStats.decodedSamples || 0}</Typography>
              <Typography variant="body2">最后解码：{safeStats.lastDecodedSamples || 0}</Typography>
              <Typography variant="body2">播放片段：{safeStats.playedChunks || 0}</Typography>
              <Typography variant="body2">缓冲队列：{safeStats.bufferQueueLength || 0}</Typography>
            </Stack>
          </Box>

          {safeStats.lastError && (
            <Box>
              <Typography variant="caption" color="error">
                最近错误：{safeStats.lastError}
              </Typography>
            </Box>
          )}
        </Stack>
      </AccordionDetails>
    </Accordion>
  );
}
