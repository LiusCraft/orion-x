import * as React from 'react';
import Box from '@mui/material/Box';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import FormControl from '@mui/material/FormControl';
import FormControlLabel from '@mui/material/FormControlLabel';
import FormGroup from '@mui/material/FormGroup';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';

const drawerWidth = 320;

export default function SettingsDrawer({
  open,
  onClose,
  models,
  selectedModel,
  onSelectModel,
  mcpServers,
  onToggleMcp,
  connection,
  onUpdateConnection
}) {
  const [tab, setTab] = React.useState(0);

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      sx={{
        '& .MuiDrawer-paper': {
          width: drawerWidth,
          bgcolor: 'background.paper',
          borderLeft: '1px solid rgba(255,255,255,0.08)'
        }
      }}
    >
      <Box sx={{ p: 2 }}>
        <Typography variant="subtitle1">设置</Typography>
      </Box>
      <Divider />
      <Tabs
        value={tab}
        onChange={(_, next) => setTab(next)}
        variant="fullWidth"
      >
        <Tab label="模型" />
        <Tab label="工具" />
        <Tab label="连接" />
      </Tabs>
      <Divider />
      <Box sx={{ p: 2 }}>
        {tab === 0 && (
          <Stack spacing={2}>
            <Typography variant="subtitle2">对话模型</Typography>
            <FormControl fullWidth size="small">
              <Select
                value={selectedModel}
                onChange={(e) => onSelectModel(e.target.value)}
              >
                {models.map((model) => (
                  <MenuItem key={model.id} value={model.id}>
                    {model.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <Typography variant="caption" color="text.secondary">
              切换模型后，将对新消息生效
            </Typography>
          </Stack>
        )}

        {tab === 1 && (
          <Stack spacing={2}>
            <Typography variant="subtitle2">MCP Server</Typography>
            <FormGroup>
              {mcpServers.map((server) => (
                <FormControlLabel
                  key={server.id}
                  control={
                    <Switch
                      checked={server.enabled}
                      onChange={() => onToggleMcp(server.id)}
                    />
                  }
                  label={
                    <Stack>
                      <Typography variant="body2">{server.name}</Typography>
                      <Typography variant="caption" color="text.secondary">
                        {server.description} · {server.status}
                      </Typography>
                    </Stack>
                  }
                />
              ))}
            </FormGroup>
          </Stack>
        )}

        {tab === 2 && (
          <Stack spacing={2}>
            <Typography variant="subtitle2">连接信息</Typography>
            <TextField
              size="small"
              label="WS 地址"
              value={connection.wsUrl}
              onChange={(e) => onUpdateConnection('wsUrl', e.target.value)}
            />
            <TextField
              size="small"
              label="设备 MAC"
              value={connection.deviceMac}
              onChange={(e) => onUpdateConnection('deviceMac', e.target.value)}
            />
            <TextField
              size="small"
              label="客户端 ID"
              value={connection.clientId}
              onChange={(e) => onUpdateConnection('clientId', e.target.value)}
            />
            <TextField
              size="small"
              label="Token"
              value={connection.token}
              type="password"
              onChange={(e) => onUpdateConnection('token', e.target.value)}
            />
            <TextField
              size="small"
              label="设备名称"
              value={connection.deviceName}
              onChange={(e) => onUpdateConnection('deviceName', e.target.value)}
            />
            <Typography variant="caption" color="text.secondary">
              修改后在顶部点击连接即可生效
            </Typography>
          </Stack>
        )}
      </Box>
    </Drawer>
  );
}
