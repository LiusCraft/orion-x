import DashboardRoundedIcon from '@mui/icons-material/DashboardRounded';
import ExtensionRoundedIcon from '@mui/icons-material/ExtensionRounded';
import HubRoundedIcon from '@mui/icons-material/HubRounded';
import LayersRoundedIcon from '@mui/icons-material/LayersRounded';
import {
  ROLE_ADMIN,
  ROLE_NORMAL_USER,
  normalizeRole
} from '../auth/sessionModel.js';

const allRoles = [ROLE_ADMIN, ROLE_NORMAL_USER];

const navigationItems = [
  {
    key: 'dashboard',
    label: 'Overview',
    subtitle: 'Session & status',
    path: '/dashboard',
    icon: DashboardRoundedIcon,
    roles: allRoles
  },
  {
    key: 'platform-resources',
    label: 'Platform Resources',
    subtitle: 'LLM / ASR / TTS catalog',
    path: '/platform-resources',
    icon: LayersRoundedIcon,
    roles: [ROLE_ADMIN]
  },
  {
    key: 'tool-market',
    label: 'Tool Market',
    subtitle: 'Offers & entitlements',
    path: '/tool-market',
    icon: ExtensionRoundedIcon,
    roles: allRoles
  },
  {
    key: 'voicebots-devices',
    label: 'Voicebots & Devices',
    subtitle: 'Binding and ownership',
    path: '/voicebots-devices',
    icon: HubRoundedIcon,
    roles: allRoles
  }
];

export function getNavigationByRole(role) {
  const normalizedRole = normalizeRole(role);
  return navigationItems.filter((item) => item.roles.includes(normalizedRole));
}
