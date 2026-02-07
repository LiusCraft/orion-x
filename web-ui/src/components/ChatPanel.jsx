import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import Composer from './Composer.jsx';
import MessageList from './MessageList.jsx';
import DebugPanel from './DebugPanel.jsx';

export default function ChatPanel({
  thread,
  modelName,
  messages,
  inputValue,
  onInputChange,
  onSend,
  onPressStart,
  onPressEnd,
  onToggleCall,
  calling,
  listening,
  disabled,
  debug,
  connState,
  sessionState,
  wsReadyState
}) {
  return (
    <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <Box sx={{ px: 3, pt: 2, flexShrink: 0 }}>
        <Stack direction="row" spacing={2} alignItems="center">
          <Typography variant="h6">{thread.title}</Typography>
          <Chip
            size="small"
            variant="outlined"
            label={`模型: ${modelName}`}
          />
        </Stack>
        <Typography variant="caption" color="text.secondary">
          {thread.lastMessage}
        </Typography>
      </Box>
      <Divider sx={{ my: 2 }} />
      <Box sx={{ flex: 1, px: 3, display: 'flex', minHeight: 0 }}>
        <MessageList messages={messages} />
      </Box>
      <Box sx={{ px: 3, pb: 3, display: 'grid', gap: 2, flexShrink: 0 }}>
        <Composer
          inputValue={inputValue}
          onInputChange={onInputChange}
          onSend={onSend}
          onPressStart={onPressStart}
          onPressEnd={onPressEnd}
          onToggleCall={onToggleCall}
          listening={listening}
          calling={calling}
          disabled={disabled}
        />
        <DebugPanel
          stats={debug}
          connState={connState}
          sessionState={sessionState}
          wsReadyState={wsReadyState}
        />
      </Box>
    </Box>
  );
}
