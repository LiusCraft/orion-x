import * as React from 'react';
import AppBar from '@mui/material/AppBar';
import Avatar from '@mui/material/Avatar';
import Box from '@mui/material/Box';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import List from '@mui/material/List';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Stack from '@mui/material/Stack';
import Toolbar from '@mui/material/Toolbar';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import MenuRoundedIcon from '@mui/icons-material/MenuRounded';
import LogoutRoundedIcon from '@mui/icons-material/LogoutRounded';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { useAuth } from '../auth/AuthProvider.jsx';
import { roleDisplayName } from '../auth/sessionModel.js';
import { getNavigationByRole } from './navigation.js';

const DRAWER_WIDTH = 286;

function initialsFromName(user) {
  const source = user?.name || user?.email || 'UX';
  return source
    .split(/[\s@._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((item) => item[0].toUpperCase())
    .join('');
}

function formatRoleLabel(role) {
  return roleDisplayName(role).replace('_', ' ');
}

function NavigationContent({ onNavigate }) {
  const location = useLocation();
  const { user } = useAuth();
  const items = React.useMemo(() => getNavigationByRole(user?.role), [user?.role]);

  return (
    <Stack sx={{ height: '100%' }}>
      <Box sx={{ px: 2.5, py: 3 }}>
        <Typography variant="h6" sx={{ lineHeight: 1.2 }}>
          Orion X Manager
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Web Console Skeleton
        </Typography>
      </Box>

      <Divider />

      <List sx={{ px: 1.5, py: 1.5, gap: 0.5, display: 'grid' }}>
        {items.map((item) => {
          const Icon = item.icon;
          const active =
            location.pathname === item.path ||
            location.pathname.startsWith(`${item.path}/`);

          return (
            <ListItemButton
              key={item.key}
              component={NavLink}
              to={item.path}
              onClick={onNavigate}
              selected={active}
              sx={{
                borderRadius: 2,
                py: 1.1,
                alignItems: 'flex-start',
                '&.Mui-selected': {
                  backgroundColor: 'rgba(15, 107, 78, 0.12)',
                  '&:hover': {
                    backgroundColor: 'rgba(15, 107, 78, 0.18)'
                  }
                }
              }}
            >
              <ListItemIcon sx={{ minWidth: 38, mt: 0.25 }}>
                <Icon fontSize="small" />
              </ListItemIcon>
              <ListItemText
                primary={item.label}
                secondary={item.subtitle}
                primaryTypographyProps={{
                  variant: 'body2',
                  fontWeight: active ? 700 : 600
                }}
                secondaryTypographyProps={{
                  variant: 'caption',
                  color: 'text.secondary'
                }}
              />
            </ListItemButton>
          );
        })}
      </List>

      <Box sx={{ mt: 'auto', px: 2.5, pb: 3 }}>
        <Typography variant="caption" color="text.secondary">
          Role-aware navigation based on JWT / profile role.
        </Typography>
      </Box>
    </Stack>
  );
}

export default function AdminLayout() {
  const [mobileOpen, setMobileOpen] = React.useState(false);
  const { user, logout } = useAuth();
  const roleLabel = formatRoleLabel(user?.role);
  const userName = user?.name || 'Manager User';
  const userEmail = user?.email || '';

  const toggleDrawer = () => {
    setMobileOpen((prev) => !prev);
  };

  const handleNavigate = () => {
    setMobileOpen(false);
  };

  const drawer = (
    <NavigationContent onNavigate={handleNavigate} />
  );

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh' }}>
      <AppBar
        elevation={0}
        color="transparent"
        sx={{
          borderBottom: '1px solid rgba(43, 67, 69, 0.12)',
          width: {
            md: `calc(100% - ${DRAWER_WIDTH}px)`
          },
          ml: {
            md: `${DRAWER_WIDTH}px`
          }
        }}
      >
        <Toolbar sx={{ gap: 1.25 }}>
          <IconButton
            color="inherit"
            edge="start"
            onClick={toggleDrawer}
            sx={{ display: { md: 'none' } }}
          >
            <MenuRoundedIcon />
          </IconButton>

          <Stack spacing={0.25} sx={{ flex: 1 }}>
            <Typography variant="subtitle2" color="text.secondary">
              Manager MVP
            </Typography>
            <Typography variant="h6" sx={{ lineHeight: 1.15 }}>
              Auth + RBAC Shell
            </Typography>
          </Stack>

          <Stack
            direction="row"
            spacing={0.9}
            alignItems="center"
            sx={{
              borderRadius: '999px',
              border: '1px solid rgba(32, 54, 54, 0.16)',
              backgroundColor: 'rgba(255, 255, 255, 0.66)',
              boxShadow: '0 8px 20px rgba(24, 46, 45, 0.08)',
              pl: 0.65,
              pr: 0.35,
              py: 0.4
            }}
          >
            <Tooltip title={userEmail || userName}>
              <Avatar
                sx={{
                  width: 34,
                  height: 34,
                  bgcolor: 'primary.main',
                  color: 'primary.contrastText',
                  fontSize: 14,
                  fontWeight: 700
                }}
              >
                {initialsFromName(user)}
              </Avatar>
            </Tooltip>

            <Box sx={{ display: { xs: 'none', sm: 'block' }, minWidth: 0 }}>
              <Typography
                variant="caption"
                sx={{
                  display: 'block',
                  color: 'text.primary',
                  fontWeight: 700,
                  lineHeight: 1.2,
                  maxWidth: { sm: 120, md: 180 },
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap'
                }}
              >
                {userName}
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{
                  display: 'block',
                  lineHeight: 1.2,
                  maxWidth: { sm: 120, md: 180 },
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap'
                }}
              >
                {userEmail || roleLabel}
              </Typography>
            </Box>

            <Box
              sx={{
                display: { xs: 'none', md: 'inline-flex' },
                px: 1,
                py: 0.25,
                borderRadius: '999px',
                bgcolor: 'rgba(15, 107, 78, 0.14)',
                color: 'primary.dark'
              }}
            >
              <Typography variant="caption" sx={{ fontWeight: 700 }}>
                {roleLabel}
              </Typography>
            </Box>

            <Button
              variant="text"
              size="small"
              startIcon={<LogoutRoundedIcon fontSize="small" />}
              onClick={logout}
              sx={{
                color: 'primary.dark',
                borderRadius: '999px',
                px: { xs: 0.75, sm: 1.1 },
                minWidth: 0,
                '&:hover': {
                  backgroundColor: 'rgba(15, 107, 78, 0.12)'
                }
              }}
            >
              <Box component="span" sx={{ display: { xs: 'none', sm: 'inline' } }}>
                Sign out
              </Box>
            </Button>
          </Stack>
        </Toolbar>
      </AppBar>

      <Box component="nav" sx={{ width: { md: DRAWER_WIDTH }, flexShrink: { md: 0 } }}>
        <Drawer
          variant="temporary"
          open={mobileOpen}
          onClose={toggleDrawer}
          ModalProps={{ keepMounted: true }}
          sx={{
            display: { xs: 'block', md: 'none' },
            '& .MuiDrawer-paper': {
              boxSizing: 'border-box',
              width: DRAWER_WIDTH
            }
          }}
        >
          {drawer}
        </Drawer>

        <Drawer
          variant="permanent"
          sx={{
            display: { xs: 'none', md: 'block' },
            '& .MuiDrawer-paper': {
              boxSizing: 'border-box',
              width: DRAWER_WIDTH,
              borderRight: '1px solid rgba(43, 67, 69, 0.12)',
              background: 'linear-gradient(180deg, rgba(255,255,255,0.9), rgba(255,255,255,0.72))'
            }
          }}
          open
        >
          {drawer}
        </Drawer>
      </Box>

      <Box
        component="main"
        sx={{
          flexGrow: 1,
          px: { xs: 2, sm: 3, md: 4 },
          py: { xs: 10, md: 12 }
        }}
      >
        <Outlet />
      </Box>
    </Box>
  );
}
