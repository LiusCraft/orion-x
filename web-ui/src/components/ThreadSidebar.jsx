import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import AddIcon from '@mui/icons-material/Add';
import PowerSettingsNewIcon from '@mui/icons-material/PowerSettingsNew';
import SettingsIcon from '@mui/icons-material/Settings';
import { formatRelative } from '../utils/time.js';

const drawerWidth = 280;

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

export default function ThreadSidebar({
  threads,
  activeThreadId,
  onSelectThread,
  onCreateThread,
  connState,
  sessionState,
  onToggleConnect,
  onOpenSettings,
  getThreadConnState,
  getThreadSessionState
}) {
  return (
    <Drawer
      variant="permanent"
      sx={{
        width: drawerWidth,
        flexShrink: 0,
        '& .MuiDrawer-paper': {
          width: drawerWidth,
          boxSizing: 'border-box',
          bgcolor: 'background.paper',
          borderRight: '1px solid rgba(255,255,255,0.08)',
          display: 'flex',
          flexDirection: 'column'
        }
      }}
    >
      <Box sx={{ p: 2, flexShrink: 0 }}>
        <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between">
          <Typography variant="subtitle1">对话列表</Typography>
          <Stack direction="row" spacing={0.5}>
            <Tooltip title={connState === 'connected' ? '断开连接' : '连接'}>
              <IconButton size="small" onClick={onToggleConnect}>
                <PowerSettingsNewIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            <Tooltip title="设置">
              <IconButton size="small" onClick={onOpenSettings}>
                <SettingsIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Stack>
        </Stack>
        <Stack direction="row" spacing={0.75} sx={{ mt: 1.5 }} flexWrap="wrap" useFlexGap>
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
        </Stack>
        <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 2 }}>
          <TextField
            size="small"
            placeholder="搜索对话"
            fullWidth
          />
          <Button size="small" startIcon={<AddIcon />} onClick={onCreateThread}>
            新建
          </Button>
        </Stack>
      </Box>
      <Divider />
      <List sx={{ px: 1, flex: 1, overflowY: 'auto' }}>
        {threads.map((thread) => {
          const threadConnState = getThreadConnState?.(thread.id) ?? 'disconnected';
          const threadSessionState = getThreadSessionState?.(thread.id) ?? 'idle';
          const isConnected = threadConnState === 'connected';
          const isActive = thread.id === activeThreadId;

          return (
            <ListItem key={thread.id} disablePadding>
              <ListItemButton
                selected={isActive}
                onClick={() => onSelectThread(thread.id)}
                sx={{
                  borderRadius: 2,
                  my: 0.5,
                  alignItems: 'flex-start',
                  position: 'relative'
                }}
              >
                {/* 连接状态指示器 */}
                <Box
                  sx={{
                    position: 'absolute',
                    left: 6,
                    top: '50%',
                    transform: 'translateY(-50%)',
                    width: 6,
                    height: 6,
                    borderRadius: '50%',
                    bgcolor: isConnected
                      ? threadSessionState === 'calling'
                        ? '#4CAF50'
                        : 'primary.main'
                      : 'text.disabled',
                    boxShadow: isConnected
                      ? threadSessionState === 'calling'
                        ? '0 0 8px rgba(76,175,80,0.6)'
                        : '0 0 8px rgba(77,163,255,0.6)'
                      : 'none',
                    transition: 'all 0.2s'
                  }}
                />
                <ListItemText
                  sx={{ pl: 1 }}
                  primary={thread.title}
                  secondary={
                    <Stack spacing={0.5}>
                      <Typography variant="caption" color="text.secondary">
                        {thread.lastMessage}
                      </Typography>
                      <Stack direction="row" alignItems="center" spacing={0.5}>
                        <Typography variant="caption" color="text.disabled">
                          {formatRelative(thread.updatedAt)}
                        </Typography>
                        {isConnected && (
                          <Chip
                            size="tiny"
                            label={stateLabel[threadSessionState]}
                            sx={{
                              height: 16,
                              fontSize: 10,
                              '& .MuiChip-label': {
                                px: 0.5
                              }
                            }}
                          />
                        )}
                      </Stack>
                    </Stack>
                  }
                />
              </ListItemButton>
            </ListItem>
          );
        })}
      </List>
    </Drawer>
  );
}
