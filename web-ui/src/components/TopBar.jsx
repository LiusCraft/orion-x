import AppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import Stack from '@mui/material/Stack';
import Toolbar from '@mui/material/Toolbar';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import PowerSettingsNewIcon from '@mui/icons-material/PowerSettingsNew';
import SettingsIcon from '@mui/icons-material/Settings';

const stateLabel = {
  idle: '空闲',
  listening: '聆听中',
  speaking: '回复中',
  calling: '通话中'
};

const connLabel = {
  disconnected: '未连接',
  connecting: '连接中',
  connected: '已连接'
};

export default function TopBar({
  connState,
  sessionState,
  modelName,
  onToggleConnect,
  onOpenSettings
}) {
  return (
    <AppBar position="sticky" elevation={0} sx={{ bgcolor: 'background.paper' }}>
      <Toolbar sx={{ gap: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box
            sx={{
              width: 10,
              height: 10,
              borderRadius: '50%',
              bgcolor: 'primary.main',
              boxShadow: '0 0 12px rgba(77, 163, 255, 0.8)'
            }}
          />
          <Typography variant="h6">小智 · 终端对话</Typography>
        </Box>

        <Stack direction="row" spacing={1} sx={{ ml: 2, flex: 1 }}>
          <Chip
            size="small"
            label={connLabel[connState]}
            color={connState === 'connected' ? 'success' : 'default'}
            variant={connState === 'connected' ? 'filled' : 'outlined'}
          />
          <Chip
            size="small"
            label={stateLabel[sessionState]}
            color={sessionState === 'calling' ? 'error' : 'primary'}
            variant="outlined"
          />
          <Chip
            size="small"
            label={'模型: ' + modelName}
            variant="outlined"
          />
        </Stack>

        <Stack direction="row" spacing={1} alignItems="center">
          <Tooltip title={connState === 'connected' ? '断开连接' : '连接'}>
            <IconButton onClick={onToggleConnect} color="inherit">
              <PowerSettingsNewIcon />
            </IconButton>
          </Tooltip>
          <Tooltip title="设置">
            <IconButton onClick={onOpenSettings} color="inherit">
              <SettingsIcon />
            </IconButton>
          </Tooltip>
        </Stack>
      </Toolbar>
    </AppBar>
  );
}
