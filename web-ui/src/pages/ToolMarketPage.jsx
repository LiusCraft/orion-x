import * as React from 'react';
import AddRoundedIcon from '@mui/icons-material/AddRounded';
import AdminPanelSettingsRoundedIcon from '@mui/icons-material/AdminPanelSettingsRounded';
import BoltRoundedIcon from '@mui/icons-material/BoltRounded';
import DeleteOutlineRoundedIcon from '@mui/icons-material/DeleteOutlineRounded';
import EditRoundedIcon from '@mui/icons-material/EditRounded';
import Inventory2RoundedIcon from '@mui/icons-material/Inventory2Rounded';
import RefreshRoundedIcon from '@mui/icons-material/RefreshRounded';
import SettingsRoundedIcon from '@mui/icons-material/SettingsRounded';
import StorefrontRoundedIcon from '@mui/icons-material/StorefrontRounded';
import TimelineRoundedIcon from '@mui/icons-material/TimelineRounded';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Collapse from '@mui/material/Collapse';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { formatAuthError, useAuth } from '../auth/AuthProvider.jsx';
import { ROLE_ADMIN } from '../auth/sessionModel.js';
import {
  buildMcpConfigFormFromConfig,
  createDefaultMcpConfigForm,
  defaultHeaderForAuthType,
  MCP_AUTH_TYPE_OPTIONS,
  MCP_TRANSPORT_OPTIONS,
  validateAndBuildMcpConfigPayload
} from './toolMcpConfigModel.js';
import {
  buildAdminToolItemOffersPath,
  buildAdminToolMarketItemsPath,
  buildAdminToolMarketItemPath,
  buildAdminToolOfferCreatePath,
  buildToolActivatePath,
  buildToolItemOffersPath,
  buildToolMarketItemsPath,
  buildToolRepoToolsCallPath,
  buildToolRepoToolsListPath,
  buildToolRepoPath,
  buildToolRepoUsagePath,
  DEFAULT_MARKET_FILTERS,
  DEFAULT_REPO_FILTERS,
  filterEntitlementsByStatus,
  getEntitlementDisplayStatus,
  MARKET_PROTOCOL_OPTIONS,
  MARKET_STATUS_OPTIONS,
  normalizeToolMarketItemsResponse,
  normalizeToolOffersResponse,
  normalizeToolRepoResponse,
  normalizeToolRuntimeToolsResponse,
  normalizeToolUsageResponse,
  TOOL_MARKET_ITEM_STATUS_OPTIONS,
  TOOL_OFFER_STATUS_OPTIONS,
  TOOL_OFFER_TYPE_OPTIONS,
  REPO_STATUS_OPTIONS
} from './toolMarketModel.js';

const OFFER_TYPE_LABEL_BY_VALUE = Object.freeze({
  free: 'Free',
  trial: 'Trial',
  paid: 'Paid',
  activation_code: 'Activation Code',
  admin_grant: 'Admin Grant',
  usage_pack: 'Usage Pack',
  time_limited: 'Time Limited'
});

const SOURCE_TYPE_LABEL_BY_VALUE = Object.freeze({
  purchase: 'Purchase',
  code: 'Activation Code',
  admin_grant: 'Admin Grant',
  system: 'System'
});

const OFFER_TYPE_SET = new Set(TOOL_OFFER_TYPE_OPTIONS.map((option) => option.value));
const ITEM_STATUS_SET = new Set(TOOL_MARKET_ITEM_STATUS_OPTIONS.map((option) => option.value));
const OFFER_STATUS_SET = new Set(TOOL_OFFER_STATUS_OPTIONS.map((option) => option.value));

const DEFAULT_ADMIN_FILTERS = Object.freeze({
  protocol: '',
  status: 'all'
});

const DEFAULT_ADMIN_ITEM_FORM = Object.freeze({
  toolKey: '',
  name: '',
  provider: '',
  protocol: 'mcp',
  status: 'active'
});

const DEFAULT_ADMIN_OFFER_FORM = Object.freeze({
  offerType: 'activation_code',
  price: '',
  currency: '',
  quotaTotal: '',
  durationSeconds: '',
  status: 'active'
});

function formatDateTime(timestamp) {
  if (!timestamp) {
    return '--';
  }

  const parsed = new Date(timestamp);
  if (Number.isNaN(parsed.getTime())) {
    return '--';
  }

  return parsed.toLocaleString();
}

function formatRefreshLabel(timestamp) {
  if (!timestamp) {
    return 'Not refreshed yet';
  }

  return `Last refreshed ${new Date(timestamp).toLocaleTimeString()}`;
}

function formatOfferType(offerType) {
  return OFFER_TYPE_LABEL_BY_VALUE[offerType] || offerType || 'Unknown';
}

function formatSourceType(sourceType) {
  return SOURCE_TYPE_LABEL_BY_VALUE[sourceType] || sourceType || 'Unknown';
}

function formatOfferPrice(offer) {
  if (offer.price === null || offer.price === undefined) {
    return '--';
  }

  const currency = offer.currency || '';
  const amount = Number.isFinite(offer.price) ? offer.price.toFixed(2) : String(offer.price);
  return currency ? `${currency} ${amount}` : amount;
}

function formatOfferDuration(durationSeconds) {
  if (durationSeconds === null || durationSeconds === undefined) {
    return '--';
  }

  if (durationSeconds < 3600) {
    return `${durationSeconds}s`;
  }

  if (durationSeconds < 86_400) {
    const hours = Math.round((durationSeconds / 3600) * 10) / 10;
    return `${hours}h`;
  }

  const days = Math.round((durationSeconds / 86_400) * 10) / 10;
  return `${days}d`;
}

function formatQuotaSummary(quotaUsed, quotaTotal) {
  if (quotaTotal === null || quotaTotal === undefined) {
    return `${quotaUsed} used / unlimited`;
  }

  return `${quotaUsed} / ${quotaTotal} used`;
}

function resolveUsageLimit(value) {
  const parsed = Number.parseInt(String(value || ''), 10);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return 20;
  }

  return Math.min(parsed, 100);
}

function getItemStatusColor(status) {
  return status === 'active' ? 'success' : 'default';
}

function getEntitlementStatusColor(status) {
  if (status === 'active') {
    return 'success';
  }

  if (status === 'pending' || status === 'expired') {
    return 'warning';
  }

  if (status === 'revoked') {
    return 'error';
  }

  return 'default';
}

function parseOptionalPositiveInteger(text, label) {
  const source = String(text ?? '').trim();
  if (!source) {
    return {
      ok: true,
      value: null,
      error: ''
    };
  }

  const parsed = Number.parseInt(source, 10);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return {
      ok: false,
      value: null,
      error: `${label} must be a positive integer.`
    };
  }

  return {
    ok: true,
    value: parsed,
    error: ''
  };
}

function parseOptionalNonNegativeNumber(text, label) {
  const source = String(text ?? '').trim();
  if (!source) {
    return {
      ok: true,
      value: null,
      error: ''
    };
  }

  const parsed = Number.parseFloat(source);
  if (!Number.isFinite(parsed) || parsed < 0) {
    return {
      ok: false,
      value: null,
      error: `${label} must be a non-negative number.`
    };
  }

  return {
    ok: true,
    value: parsed,
    error: ''
  };
}

function parseToolNameListText(text) {
  return String(text || '')
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function uniqueToolNames(names) {
  const result = [];
  const seen = new Set();

  for (const name of names) {
    const normalized = String(name || '').trim();
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    result.push(normalized);
  }

  return result;
}

function parseJsonObjectField(text, label) {
  const source = String(text ?? '').trim();
  if (!source) {
    return {
      ok: false,
      value: {},
      error: `${label} is required.`
    };
  }

  try {
    const parsed = JSON.parse(source);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {
        ok: false,
        value: {},
        error: `${label} must be a JSON object.`
      };
    }

    return {
      ok: true,
      value: parsed,
      error: ''
    };
  } catch {
    return {
      ok: false,
      value: {},
      error: `${label} must be valid JSON.`
    };
  }
}

function resolveOfferActionType(offer, isAdmin) {
  if (offer.status !== 'active') {
    return 'inactive';
  }

  if (offer.offerType === 'activation_code') {
    return 'activate';
  }

  if (offer.offerType === 'admin_grant') {
    return isAdmin ? 'grant' : 'admin_only';
  }

  return 'unsupported';
}

export default function ToolMarketPage() {
  const { authorizedRequest, user } = useAuth();
  const isAdmin = user?.role === ROLE_ADMIN;

  const [activeTab, setActiveTab] = React.useState('market');
  const [notice, setNotice] = React.useState(null);

  const [marketFilters, setMarketFilters] = React.useState(() => ({
    ...DEFAULT_MARKET_FILTERS
  }));
  const [marketItems, setMarketItems] = React.useState([]);
  const [marketLoading, setMarketLoading] = React.useState(true);
  const [marketError, setMarketError] = React.useState('');
  const [marketRefreshedAt, setMarketRefreshedAt] = React.useState(0);

  const [expandedItemID, setExpandedItemID] = React.useState('');
  const [offersByItemID, setOffersByItemID] = React.useState({});
  const [offersLoadingByItemID, setOffersLoadingByItemID] = React.useState({});
  const [offersErrorByItemID, setOffersErrorByItemID] = React.useState({});

  const [repoFilterStatus, setRepoFilterStatus] = React.useState(
    DEFAULT_REPO_FILTERS.status
  );
  const [repoEntitlements, setRepoEntitlements] = React.useState([]);
  const [repoLoading, setRepoLoading] = React.useState(true);
  const [repoError, setRepoError] = React.useState('');
  const [repoRefreshedAt, setRepoRefreshedAt] = React.useState(0);

  const [activationDialogOpen, setActivationDialogOpen] = React.useState(false);
  const [activationTarget, setActivationTarget] = React.useState(null);
  const [activationSubmitting, setActivationSubmitting] = React.useState(false);
  const [activationError, setActivationError] = React.useState('');

  const [grantDialogOpen, setGrantDialogOpen] = React.useState(false);
  const [grantTarget, setGrantTarget] = React.useState(null);
  const [grantForm, setGrantForm] = React.useState({
    userID: '',
    sourceRef: '',
    startsAt: ''
  });
  const [grantSubmitting, setGrantSubmitting] = React.useState(false);
  const [grantError, setGrantError] = React.useState('');

  const [usageDialogOpen, setUsageDialogOpen] = React.useState(false);
  const [usageTarget, setUsageTarget] = React.useState(null);
  const [usageRecords, setUsageRecords] = React.useState([]);
  const [usageLoading, setUsageLoading] = React.useState(false);
  const [usageError, setUsageError] = React.useState('');
  const [usageLimitInput, setUsageLimitInput] = React.useState('20');
  const [usageRefreshedAt, setUsageRefreshedAt] = React.useState(0);

  const [runtimeDialogOpen, setRuntimeDialogOpen] = React.useState(false);
  const [runtimeTargetEntitlement, setRuntimeTargetEntitlement] = React.useState(null);
  const [runtimeTools, setRuntimeTools] = React.useState([]);
  const [runtimeToolsLoading, setRuntimeToolsLoading] = React.useState(false);
  const [runtimeToolsError, setRuntimeToolsError] = React.useState('');
  const [runtimeRefreshedAt, setRuntimeRefreshedAt] = React.useState(0);
  const [runtimeSelectedToolName, setRuntimeSelectedToolName] = React.useState('');
  const [runtimeToolArguments, setRuntimeToolArguments] = React.useState('{}');
  const [runtimeCallSubmitting, setRuntimeCallSubmitting] = React.useState(false);
  const [runtimeCallError, setRuntimeCallError] = React.useState('');
  const [runtimeCallResult, setRuntimeCallResult] = React.useState(null);

  const [adminFilters, setAdminFilters] = React.useState(() => ({
    ...DEFAULT_ADMIN_FILTERS
  }));
  const [adminItems, setAdminItems] = React.useState([]);
  const [adminLoading, setAdminLoading] = React.useState(true);
  const [adminError, setAdminError] = React.useState('');
  const [adminRefreshedAt, setAdminRefreshedAt] = React.useState(0);

  const [adminItemDialogOpen, setAdminItemDialogOpen] = React.useState(false);
  const [adminItemDialogMode, setAdminItemDialogMode] = React.useState('create');
  const [editingAdminItem, setEditingAdminItem] = React.useState(null);
  const [adminItemForm, setAdminItemForm] = React.useState(() => ({
    ...DEFAULT_ADMIN_ITEM_FORM
  }));
  const [adminMcpConfigForm, setAdminMcpConfigForm] = React.useState(() =>
    createDefaultMcpConfigForm()
  );
  const [adminItemFieldErrors, setAdminItemFieldErrors] = React.useState({});
  const [adminMcpConfigFieldErrors, setAdminMcpConfigFieldErrors] = React.useState({});
  const [adminItemSubmitError, setAdminItemSubmitError] = React.useState('');
  const [adminItemSubmitting, setAdminItemSubmitting] = React.useState(false);
  const [adminItemDeletingID, setAdminItemDeletingID] = React.useState('');

  const [adminOfferDialogOpen, setAdminOfferDialogOpen] = React.useState(false);
  const [adminOfferTargetItem, setAdminOfferTargetItem] = React.useState(null);
  const [adminOfferForm, setAdminOfferForm] = React.useState(() => ({
    ...DEFAULT_ADMIN_OFFER_FORM
  }));
  const [adminOfferError, setAdminOfferError] = React.useState('');
  const [adminOfferSubmitting, setAdminOfferSubmitting] = React.useState(false);

  const marketItemNameByID = React.useMemo(
    () =>
      marketItems.reduce((result, item) => {
        if (!item.id) {
          return result;
        }

        result[item.id] = item.name || item.toolKey || item.id;
        return result;
      }, {}),
    [marketItems]
  );

  const visibleEntitlements = React.useMemo(
    () => filterEntitlementsByStatus(repoEntitlements, repoFilterStatus),
    [repoEntitlements, repoFilterStatus]
  );

  const usageConsumedTotal = React.useMemo(
    () => usageRecords.reduce((sum, record) => sum + record.consumedUnits, 0),
    [usageRecords]
  );

  const selectedMcpToolNameList = React.useMemo(
    () => uniqueToolNames(parseToolNameListText(adminMcpConfigForm.toolNameListText)),
    [adminMcpConfigForm.toolNameListText]
  );

  const runtimeSelectedTool = React.useMemo(
    () => runtimeTools.find((tool) => tool.name === runtimeSelectedToolName) || null,
    [runtimeSelectedToolName, runtimeTools]
  );

  const loadMarketItems = React.useCallback(async () => {
    setMarketLoading(true);
    setMarketError('');

    try {
      const payload = await authorizedRequest(buildToolMarketItemsPath(marketFilters));
      setMarketItems(normalizeToolMarketItemsResponse(payload));
      setMarketRefreshedAt(Date.now());
    } catch (error) {
      setMarketItems([]);
      setMarketError(formatAuthError(error));
    } finally {
      setMarketLoading(false);
    }
  }, [authorizedRequest, marketFilters]);

  const loadAdminItems = React.useCallback(async () => {
    if (!isAdmin) {
      setAdminItems([]);
      setAdminLoading(false);
      setAdminError('');
      return;
    }

    setAdminLoading(true);
    setAdminError('');

    try {
      const payload = await authorizedRequest(buildAdminToolMarketItemsPath(adminFilters));
      setAdminItems(normalizeToolMarketItemsResponse(payload));
      setAdminRefreshedAt(Date.now());
    } catch (error) {
      setAdminItems([]);
      setAdminError(formatAuthError(error));
    } finally {
      setAdminLoading(false);
    }
  }, [adminFilters, authorizedRequest, isAdmin]);

  const loadOffersForItem = React.useCallback(
    async (itemID, options = {}) => {
      if (!itemID) {
        return;
      }

      const forceReload = options.forceReload === true;
      const useAdminScope = options.adminScope === true;
      if (
        !forceReload &&
        Array.isArray(offersByItemID[itemID]) &&
        !offersErrorByItemID[itemID]
      ) {
        return;
      }

      setOffersLoadingByItemID((prev) => ({
        ...prev,
        [itemID]: true
      }));
      setOffersErrorByItemID((prev) => ({
        ...prev,
        [itemID]: ''
      }));

      try {
        const payload = await authorizedRequest(
          useAdminScope
            ? buildAdminToolItemOffersPath(itemID)
            : buildToolItemOffersPath(itemID)
        );

        const normalizedOffers = normalizeToolOffersResponse(payload).filter(
          (offer) => !offer.toolItemID || offer.toolItemID === itemID
        );

        setOffersByItemID((prev) => ({
          ...prev,
          [itemID]: normalizedOffers
        }));
      } catch (error) {
        setOffersByItemID((prev) => ({
          ...prev,
          [itemID]: []
        }));
        setOffersErrorByItemID((prev) => ({
          ...prev,
          [itemID]: formatAuthError(error)
        }));
      } finally {
        setOffersLoadingByItemID((prev) => ({
          ...prev,
          [itemID]: false
        }));
      }
    },
    [authorizedRequest, offersByItemID, offersErrorByItemID]
  );

  const loadRepoEntitlements = React.useCallback(async () => {
    setRepoLoading(true);
    setRepoError('');

    try {
      const payload = await authorizedRequest(buildToolRepoPath());
      setRepoEntitlements(normalizeToolRepoResponse(payload));
      setRepoRefreshedAt(Date.now());
    } catch (error) {
      setRepoEntitlements([]);
      setRepoError(formatAuthError(error));
    } finally {
      setRepoLoading(false);
    }
  }, [authorizedRequest]);

  const loadUsageRecords = React.useCallback(
    async (entitlementID) => {
      if (!entitlementID) {
        return;
      }

      setUsageLoading(true);
      setUsageError('');

      try {
        const payload = await authorizedRequest(
          buildToolRepoUsagePath(entitlementID, {
            limit: resolveUsageLimit(usageLimitInput)
          })
        );

        setUsageRecords(normalizeToolUsageResponse(payload));
        setUsageRefreshedAt(Date.now());
      } catch (error) {
        setUsageRecords([]);
        setUsageError(formatAuthError(error));
      } finally {
        setUsageLoading(false);
      }
    },
    [authorizedRequest, usageLimitInput]
  );

  const loadRuntimeTools = React.useCallback(
    async (entitlementID) => {
      if (!entitlementID) {
        return;
      }

      setRuntimeToolsLoading(true);
      setRuntimeToolsError('');

      try {
        const payload = await authorizedRequest(buildToolRepoToolsListPath(entitlementID));
        const tools = normalizeToolRuntimeToolsResponse(payload);
        setRuntimeTools(tools);
        setRuntimeRefreshedAt(Date.now());

        if (tools.length === 0) {
          setRuntimeSelectedToolName('');
          setRuntimeCallResult(null);
        } else {
          setRuntimeSelectedToolName((prev) =>
            prev && tools.some((item) => item.name === prev) ? prev : tools[0].name
          );
        }
      } catch (error) {
        setRuntimeTools([]);
        setRuntimeSelectedToolName('');
        setRuntimeToolsError(formatAuthError(error));
      } finally {
        setRuntimeToolsLoading(false);
      }
    },
    [authorizedRequest]
  );

  React.useEffect(() => {
    loadMarketItems();
  }, [loadMarketItems]);

  React.useEffect(() => {
    loadRepoEntitlements();
  }, [loadRepoEntitlements]);

  React.useEffect(() => {
    loadAdminItems();
  }, [loadAdminItems]);

  React.useEffect(() => {
    if (!isAdmin && activeTab === 'admin') {
      setActiveTab('market');
    }
  }, [activeTab, isAdmin]);

  React.useEffect(() => {
    if (!expandedItemID) {
      return;
    }

    const exists = marketItems.some((item) => item.id === expandedItemID);
    if (!exists) {
      setExpandedItemID('');
    }
  }, [expandedItemID, marketItems]);

  const handleMarketFilterChange = (field) => (event) => {
    const value = event.target.value;
    setMarketFilters((prev) => ({
      ...prev,
      [field]: value
    }));
  };

  const handleToggleOffers = (itemID) => {
    const nextExpanded = expandedItemID === itemID ? '' : itemID;
    setExpandedItemID(nextExpanded);

    if (nextExpanded) {
      loadOffersForItem(nextExpanded);
    }
  };

  const openActivationDialog = (item) => {
    setActivationTarget(item);
    setActivationError('');
    setActivationSubmitting(false);
    setActivationDialogOpen(true);
  };

  const closeActivationDialog = () => {
    if (activationSubmitting) {
      return;
    }

    setActivationDialogOpen(false);
    setActivationTarget(null);
    setActivationError('');
  };

  const handleSubmitActivation = async () => {
    const itemID = activationTarget?.id;

    if (!itemID) {
      setActivationError('Tool item id is missing. Please close and retry.');
      return;
    }

    setActivationSubmitting(true);
    setActivationError('');

    try {
      await authorizedRequest(buildToolActivatePath(itemID), {
        method: 'POST',
        body: {}
      });

      setActivationDialogOpen(false);
      setActivationTarget(null);
      setNotice({
        severity: 'success',
        message: `Activated ${
          activationTarget.name || activationTarget.toolKey || 'tool'
        } successfully.`
      });

      await loadRepoEntitlements();
    } catch (error) {
      setActivationError(formatAuthError(error));
    } finally {
      setActivationSubmitting(false);
    }
  };

  const openGrantDialog = (item) => {
    setGrantTarget({ item });
    setGrantForm({
      userID: '',
      sourceRef: '',
      startsAt: ''
    });
    setGrantError('');
    setGrantSubmitting(false);
    setGrantDialogOpen(true);
  };

  const closeGrantDialog = () => {
    if (grantSubmitting) {
      return;
    }

    setGrantDialogOpen(false);
    setGrantTarget(null);
    setGrantError('');
  };

  const handleGrantFormChange = (field) => (event) => {
    const value = event.target.value;
    setGrantForm((prev) => ({
      ...prev,
      [field]: value
    }));
    setGrantError('');
  };

  const handleSubmitGrant = async () => {
    if (!grantTarget?.item?.id) {
      setGrantError('Grant target is missing item id.');
      return;
    }

    const userID = grantForm.userID.trim();
    if (!userID) {
      setGrantError('Target user id is required.');
      return;
    }

    const sourceRef = grantForm.sourceRef.trim();
    const startsAtInput = grantForm.startsAt.trim();
    let startsAtISO = '';
    if (startsAtInput) {
      const parsedStartsAt = new Date(startsAtInput);
      if (Number.isNaN(parsedStartsAt.getTime())) {
        setGrantError('Starts at must be a valid datetime.');
        return;
      }
      startsAtISO = parsedStartsAt.toISOString();
    }

    setGrantSubmitting(true);
    setGrantError('');

    try {
      const requestBody = {
        user_id: userID,
        item_id: grantTarget.item.id
      };

      if (sourceRef) {
        requestBody.source_ref = sourceRef;
      }

      if (startsAtISO) {
        requestBody.starts_at = startsAtISO;
      }

      await authorizedRequest('/api/v1/admin/tool-entitlements/grant', {
        method: 'POST',
        body: requestBody
      });

      setGrantDialogOpen(false);
      setGrantTarget(null);
      setNotice({
        severity: 'success',
        message: `Granted ${
          grantTarget.item.name || grantTarget.item.toolKey || 'tool'
        } to user ${userID}.`
      });

      await loadRepoEntitlements();
    } catch (error) {
      setGrantError(formatAuthError(error));
    } finally {
      setGrantSubmitting(false);
    }
  };

  const openUsageDialog = (entitlement) => {
    setUsageTarget(entitlement);
    setUsageRecords([]);
    setUsageError('');
    setUsageDialogOpen(true);
    loadUsageRecords(entitlement.id);
  };

  const closeUsageDialog = () => {
    if (usageLoading) {
      return;
    }

    setUsageDialogOpen(false);
    setUsageTarget(null);
    setUsageRecords([]);
    setUsageError('');
  };

  const handleRefreshUsage = () => {
    if (!usageTarget?.id) {
      return;
    }

    loadUsageRecords(usageTarget.id);
  };

  const openRuntimeDialog = (entitlement) => {
    setRuntimeTargetEntitlement(entitlement);
    setRuntimeTools([]);
    setRuntimeToolsError('');
    setRuntimeCallError('');
    setRuntimeCallResult(null);
    setRuntimeToolArguments('{}');
    setRuntimeSelectedToolName('');
    setRuntimeDialogOpen(true);

    loadRuntimeTools(entitlement.id);
  };

  const closeRuntimeDialog = () => {
    if (runtimeToolsLoading || runtimeCallSubmitting) {
      return;
    }

    setRuntimeDialogOpen(false);
    setRuntimeTargetEntitlement(null);
    setRuntimeTools([]);
    setRuntimeToolsError('');
    setRuntimeCallError('');
    setRuntimeCallResult(null);
  };

  const handleRefreshRuntimeTools = () => {
    if (!runtimeTargetEntitlement?.id) {
      return;
    }

    loadRuntimeTools(runtimeTargetEntitlement.id);
  };

  const handleSubmitRuntimeToolCall = async () => {
    if (!runtimeTargetEntitlement?.id) {
      setRuntimeCallError('Missing entitlement id.');
      return;
    }

    const toolName = runtimeSelectedToolName.trim();
    if (!toolName) {
      setRuntimeCallError('Please choose a tool to call.');
      return;
    }

    const argsResult = parseJsonObjectField(runtimeToolArguments, 'Tool arguments');
    if (!argsResult.ok) {
      setRuntimeCallError(argsResult.error);
      return;
    }

    setRuntimeCallSubmitting(true);
    setRuntimeCallError('');

    try {
      const payload = await authorizedRequest(
        buildToolRepoToolsCallPath(runtimeTargetEntitlement.id),
        {
          method: 'POST',
          body: {
            tool_name: toolName,
            arguments: argsResult.value
          }
        }
      );

      setRuntimeCallResult(payload);
    } catch (error) {
      setRuntimeCallResult(null);
      setRuntimeCallError(formatAuthError(error));
    } finally {
      setRuntimeCallSubmitting(false);
    }
  };

  const handleAdminFilterChange = (field) => (event) => {
    const value = event.target.value;
    setAdminFilters((prev) => ({
      ...prev,
      [field]: value
    }));
  };

  const openCreateAdminItemDialog = () => {
    setAdminItemDialogMode('create');
    setEditingAdminItem(null);
    setAdminItemForm({
      ...DEFAULT_ADMIN_ITEM_FORM
    });
    setAdminMcpConfigForm(createDefaultMcpConfigForm());
    setAdminItemFieldErrors({});
    setAdminMcpConfigFieldErrors({});
    setAdminItemSubmitError('');
    setAdminItemSubmitting(false);
    setAdminItemDialogOpen(true);
  };

  const openEditAdminItemDialog = (item) => {
    if (!item?.id) {
      return;
    }

    setAdminItemDialogMode('edit');
    setEditingAdminItem(item);
    setAdminItemForm({
      toolKey: item.toolKey || '',
      name: item.name || '',
      provider: item.provider || '',
      protocol: item.protocol || 'mcp',
      status: item.status || 'active'
    });
    setAdminMcpConfigForm(buildMcpConfigFormFromConfig(item.config || {}));
    setAdminItemFieldErrors({});
    setAdminMcpConfigFieldErrors({});
    setAdminItemSubmitError('');
    setAdminItemSubmitting(false);
    setAdminItemDialogOpen(true);
  };

  const closeAdminItemDialog = () => {
    if (adminItemSubmitting) {
      return;
    }

    setAdminItemDialogOpen(false);
    setEditingAdminItem(null);
    setAdminItemFieldErrors({});
    setAdminMcpConfigFieldErrors({});
    setAdminItemSubmitError('');
  };

  const handleAdminItemFormChange = (field) => (event) => {
    const value = event.target.value;
    setAdminItemForm((prev) => ({
      ...prev,
      [field]: value
    }));

    setAdminItemFieldErrors((prev) => {
      if (!prev[field]) {
        return prev;
      }
      const next = { ...prev };
      delete next[field];
      return next;
    });
    setAdminItemSubmitError('');
  };

  const handleAdminMcpConfigFormChange = (field) => (event) => {
    const value = event.target.value;

    setAdminMcpConfigForm((prev) => {
      const next = {
        ...prev,
        [field]: value
      };

      if (
        field === 'authType' &&
        (value === 'bearer' || value === 'api_key') &&
        !String(next.authHeader || '').trim()
      ) {
        next.authHeader = defaultHeaderForAuthType(value);
      }

      if (field === 'authType' && value === 'none') {
        next.authToken = '';
      }

      return next;
    });

    setAdminMcpConfigFieldErrors((prev) => {
      if (!prev[field]) {
        return prev;
      }
      const next = { ...prev };
      delete next[field];
      return next;
    });

    setAdminItemSubmitError('');
  };

  const handleSubmitAdminItem = async () => {
    const mode = adminItemDialogMode;
    const toolKey = adminItemForm.toolKey.trim().toLowerCase();
    const name = adminItemForm.name.trim();
    const provider = adminItemForm.provider.trim().toLowerCase();
    const protocol = adminItemForm.protocol.trim().toLowerCase() || 'mcp';
    const status = adminItemForm.status;

    const fieldErrors = {};
    if (mode === 'create' && !toolKey) {
      fieldErrors.toolKey = 'Tool key is required.';
    }

    if (mode === 'create' && toolKey && !/^[a-z0-9][a-z0-9-_]*$/.test(toolKey)) {
      fieldErrors.toolKey = 'Tool key must match ^[a-z0-9][a-z0-9-_]*$.';
    }

    if (!name) {
      fieldErrors.name = 'Name is required.';
    }

    if (!provider) {
      fieldErrors.provider = 'Provider is required.';
    }

    if (protocol !== 'mcp') {
      fieldErrors.protocol = 'Only mcp protocol is supported in MVP.';
    }

    if (!ITEM_STATUS_SET.has(status)) {
      fieldErrors.status = 'Status must be active or inactive.';
    }

    if (Object.keys(fieldErrors).length > 0) {
      setAdminItemFieldErrors(fieldErrors);
      setAdminMcpConfigFieldErrors({});
      return;
    }

    const mcpConfigResult = validateAndBuildMcpConfigPayload(adminMcpConfigForm);
    if (!mcpConfigResult.ok) {
      setAdminItemFieldErrors({});
      setAdminMcpConfigFieldErrors(mcpConfigResult.fieldErrors);
      return;
    }

    setAdminItemFieldErrors({});
    setAdminMcpConfigFieldErrors({});

    const createPayload = {
      tool_key: toolKey,
      name,
      provider,
      protocol,
      config: mcpConfigResult.config,
      status
    };

    const originalName = editingAdminItem?.name || '';
    const originalStatus = editingAdminItem?.status || 'active';
    const originalConfigText = JSON.stringify(editingAdminItem?.config || {});
    const nextConfigText = JSON.stringify(mcpConfigResult.config);

    const patchPayload = {};
    if (name !== originalName) {
      patchPayload.name = name;
    }
    if (status !== originalStatus) {
      patchPayload.status = status;
    }
    if (nextConfigText !== originalConfigText) {
      patchPayload.config = mcpConfigResult.config;
    }

    if (mode === 'edit' && !editingAdminItem?.id) {
      setAdminItemSubmitError('Editing target is missing id. Please close and retry.');
      return;
    }

    if (mode === 'edit' && Object.keys(patchPayload).length === 0) {
      setAdminItemSubmitError('No changes detected.');
      return;
    }

    setAdminItemSubmitting(true);
    setAdminItemSubmitError('');

    try {
      if (mode === 'create') {
        await authorizedRequest(buildAdminToolMarketItemPath(), {
          method: 'POST',
          body: createPayload
        });
      } else {
        await authorizedRequest(buildAdminToolMarketItemPath(editingAdminItem.id), {
          method: 'PATCH',
          body: patchPayload
        });
      }

      setAdminItemDialogOpen(false);
      setEditingAdminItem(null);
      setNotice({
        severity: 'success',
        message:
          mode === 'create'
            ? `Created tool item ${toolKey}.`
            : `Updated ${editingAdminItem?.name || editingAdminItem?.toolKey || 'tool item'}.`
      });

      await Promise.all([loadMarketItems(), loadAdminItems()]);
    } catch (error) {
      setAdminItemSubmitError(formatAuthError(error));
    } finally {
      setAdminItemSubmitting(false);
    }
  };

  const handleDeleteAdminItem = async (item) => {
    if (!item?.id) {
      return;
    }

    const confirmed = window.confirm(
      `Delete tool item "${item.name || item.toolKey}"? Existing entitlements may become unavailable.`
    );
    if (!confirmed) {
      return;
    }

    setAdminItemDeletingID(item.id);

    try {
      await authorizedRequest(buildAdminToolMarketItemPath(item.id), {
        method: 'DELETE'
      });

      setNotice({
        severity: 'success',
        message: `Deleted ${item.name || item.toolKey || 'tool item'}.`
      });

      setOffersByItemID((prev) => {
        const next = { ...prev };
        delete next[item.id];
        return next;
      });
      setOffersErrorByItemID((prev) => {
        const next = { ...prev };
        delete next[item.id];
        return next;
      });

      await Promise.all([loadMarketItems(), loadAdminItems()]);
    } catch (error) {
      setNotice({
        severity: 'error',
        message: formatAuthError(error)
      });
    } finally {
      setAdminItemDeletingID('');
    }
  };

  const openAdminOfferDialog = (item) => {
    setAdminOfferTargetItem(item);
    setAdminOfferForm({
      ...DEFAULT_ADMIN_OFFER_FORM
    });
    setAdminOfferError('');
    setAdminOfferSubmitting(false);
    setAdminOfferDialogOpen(true);
  };

  const closeAdminOfferDialog = () => {
    if (adminOfferSubmitting) {
      return;
    }

    setAdminOfferDialogOpen(false);
    setAdminOfferTargetItem(null);
    setAdminOfferError('');
  };

  const handleAdminOfferFormChange = (field) => (event) => {
    const value = event.target.value;
    setAdminOfferForm((prev) => ({
      ...prev,
      [field]: value
    }));
    setAdminOfferError('');
  };

  const handleSubmitAdminOffer = async () => {
    if (!adminOfferTargetItem?.id) {
      setAdminOfferError('Target item id is missing. Please close and retry.');
      return;
    }

    const offerType = adminOfferForm.offerType;
    const status = adminOfferForm.status;
    const currency = adminOfferForm.currency.trim().toUpperCase();

    if (!OFFER_TYPE_SET.has(offerType)) {
      setAdminOfferError('Offer type is invalid.');
      return;
    }

    if (!OFFER_STATUS_SET.has(status)) {
      setAdminOfferError('Offer status is invalid.');
      return;
    }

    const priceResult = parseOptionalNonNegativeNumber(adminOfferForm.price, 'Price');
    if (!priceResult.ok) {
      setAdminOfferError(priceResult.error);
      return;
    }

    if (priceResult.value !== null && !currency) {
      setAdminOfferError('Currency is required when price is provided.');
      return;
    }

    const quotaResult = parseOptionalPositiveInteger(adminOfferForm.quotaTotal, 'Quota total');
    if (!quotaResult.ok) {
      setAdminOfferError(quotaResult.error);
      return;
    }

    const durationResult = parseOptionalPositiveInteger(
      adminOfferForm.durationSeconds,
      'Duration seconds'
    );
    if (!durationResult.ok) {
      setAdminOfferError(durationResult.error);
      return;
    }

    setAdminOfferSubmitting(true);
    setAdminOfferError('');

    try {
      await authorizedRequest(buildAdminToolOfferCreatePath(adminOfferTargetItem.id), {
        method: 'POST',
        body: {
          offer_type: offerType,
          price: priceResult.value,
          currency: priceResult.value === null ? null : currency,
          quota_total: quotaResult.value,
          duration_seconds: durationResult.value,
          status
        }
      });

      setAdminOfferDialogOpen(false);
      setAdminOfferTargetItem(null);
      setNotice({
        severity: 'success',
        message: `Created ${formatOfferType(offerType)} offer for ${
          adminOfferTargetItem.name || adminOfferTargetItem.toolKey || 'tool'
        }.`
      });

      await loadOffersForItem(adminOfferTargetItem.id, {
        forceReload: true
      });
    } catch (error) {
      setAdminOfferError(formatAuthError(error));
    } finally {
      setAdminOfferSubmitting(false);
    }
  };

  return (
    <Stack spacing={2.25}>
      <Paper
        sx={{
          p: { xs: 2.5, md: 3 },
          borderRadius: 2,
          border: '1px solid rgba(38, 59, 60, 0.14)',
          background:
            'linear-gradient(125deg, rgba(255, 255, 255, 0.92), rgba(240, 246, 242, 0.9))'
        }}
      >
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          spacing={1.5}
          alignItems={{ xs: 'flex-start', md: 'center' }}
          justifyContent="space-between"
        >
          <Box>
            <Typography variant="h4">Tool Market &amp; My Repo</Typography>
            <Typography color="text.secondary" sx={{ mt: 1 }}>
              Browse market offers, complete activation flows, and inspect entitlement
              usage from one page.
            </Typography>
          </Box>

          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              startIcon={<RefreshRoundedIcon />}
              onClick={loadMarketItems}
              disabled={marketLoading}
            >
              Refresh market
            </Button>
            <Button
              variant="outlined"
              startIcon={<RefreshRoundedIcon />}
              onClick={loadRepoEntitlements}
              disabled={repoLoading}
            >
              Refresh repo
            </Button>
            {isAdmin ? (
              <Button
                variant="outlined"
                startIcon={<RefreshRoundedIcon />}
                onClick={loadAdminItems}
                disabled={adminLoading}
              >
                Refresh admin
              </Button>
            ) : null}
          </Stack>
        </Stack>
      </Paper>

      <Paper sx={{ borderRadius: 2, overflow: 'hidden' }}>
        <Tabs
          value={activeTab}
          onChange={(_event, value) => setActiveTab(value)}
          variant="fullWidth"
          sx={{
            '& .MuiTab-root': {
              py: 1.5,
              fontWeight: 700
            }
          }}
        >
          <Tab
            value="market"
            icon={<StorefrontRoundedIcon fontSize="small" />}
            iconPosition="start"
            label="Market & Offers"
          />
          <Tab
            value="repo"
            icon={<Inventory2RoundedIcon fontSize="small" />}
            iconPosition="start"
            label="My Tool Repo"
          />
          {isAdmin ? (
            <Tab
              value="admin"
              icon={<SettingsRoundedIcon fontSize="small" />}
              iconPosition="start"
              label="Admin Ops"
            />
          ) : null}
        </Tabs>
      </Paper>

      {notice ? (
        <Alert severity={notice.severity} onClose={() => setNotice(null)}>
          {notice.message}
        </Alert>
      ) : null}

      {activeTab === 'market' ? (
        <>
          <Paper sx={{ p: { xs: 2, md: 2.5 }, borderRadius: 2 }}>
            <Stack
              direction={{ xs: 'column', md: 'row' }}
              spacing={1.25}
              alignItems={{ xs: 'stretch', md: 'center' }}
            >
              <TextField
                select
                size="small"
                label="Protocol"
                value={marketFilters.protocol}
                onChange={handleMarketFilterChange('protocol')}
                sx={{ minWidth: { xs: '100%', md: 170 } }}
              >
                {MARKET_PROTOCOL_OPTIONS.map((option) => (
                  <MenuItem key={option.value || '__all_protocol'} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              <TextField
                select
                size="small"
                label="Status"
                value={marketFilters.status}
                onChange={handleMarketFilterChange('status')}
                sx={{ minWidth: { xs: '100%', md: 170 } }}
              >
                {MARKET_STATUS_OPTIONS.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              <Typography variant="caption" color="text.secondary" sx={{ ml: { md: 'auto' } }}>
                {marketItems.length} item(s). {formatRefreshLabel(marketRefreshedAt)}
              </Typography>
            </Stack>
          </Paper>

          {marketError ? <Alert severity="error">{marketError}</Alert> : null}

          {marketLoading && marketItems.length === 0 ? (
            <Paper sx={{ p: 4, borderRadius: 2 }}>
              <Stack direction="row" spacing={1} alignItems="center" justifyContent="center">
                <CircularProgress size={20} />
                <Typography variant="body2" color="text.secondary">
                  Loading tool market items...
                </Typography>
              </Stack>
            </Paper>
          ) : null}

          {!marketLoading && marketItems.length === 0 ? (
            <Paper sx={{ p: 4, borderRadius: 2, textAlign: 'center' }}>
              <Typography variant="body2" color="text.secondary">
                No tool market items match the current filters.
              </Typography>
            </Paper>
          ) : null}

          {marketItems.map((item) => {
            const isExpanded = expandedItemID === item.id;
            const offers = offersByItemID[item.id] || [];
            const offersLoading = Boolean(offersLoadingByItemID[item.id]);
            const offersError = offersErrorByItemID[item.id] || '';

            return (
              <Paper key={item.id} sx={{ p: { xs: 2, md: 2.4 }, borderRadius: 2 }}>
                <Stack
                  direction={{ xs: 'column', md: 'row' }}
                  spacing={1.2}
                  justifyContent="space-between"
                  alignItems={{ xs: 'flex-start', md: 'center' }}
                >
                  <Stack spacing={0.55}>
                    <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                      <Typography variant="h6" sx={{ lineHeight: 1.2 }}>
                        {item.name || item.toolKey || item.id}
                      </Typography>
                      <Chip
                        size="small"
                        color={getItemStatusColor(item.status)}
                        label={item.status}
                      />
                      <Chip
                        size="small"
                        variant="outlined"
                        label={item.protocol ? item.protocol.toUpperCase() : 'unknown'}
                      />
                    </Stack>

                    <Typography variant="body2" color="text.secondary">
                      tool_key: {item.toolKey || '--'} · provider: {item.provider || '--'}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      Updated {formatDateTime(item.updatedAt || item.createdAt)}
                    </Typography>
                  </Stack>

                  <Stack direction="row" spacing={1}>
                    <Button
                      size="small"
                      variant="contained"
                      startIcon={<BoltRoundedIcon fontSize="small" />}
                      onClick={() => openActivationDialog(item)}
                      disabled={item.status !== 'active'}
                    >
                      Activate
                    </Button>
                    {isAdmin ? (
                      <Button
                        size="small"
                        variant="outlined"
                        color="secondary"
                        startIcon={<AdminPanelSettingsRoundedIcon fontSize="small" />}
                        onClick={() => openGrantDialog(item)}
                        disabled={item.status !== 'active'}
                      >
                        Grant
                      </Button>
                    ) : null}
                  </Stack>
                </Stack>

                <Collapse in={isExpanded} unmountOnExit>
                  <Divider sx={{ my: 1.5 }} />

                  {offersError ? <Alert severity="error">{offersError}</Alert> : null}

                  {offersLoading ? (
                    <Stack
                      direction="row"
                      spacing={1}
                      alignItems="center"
                      justifyContent="center"
                      sx={{ py: 2 }}
                    >
                      <CircularProgress size={20} />
                      <Typography variant="body2" color="text.secondary">
                        Loading offers...
                      </Typography>
                    </Stack>
                  ) : offers.length === 0 ? (
                    <Typography variant="body2" color="text.secondary" sx={{ py: 1.2 }}>
                      No offers available for this tool.
                    </Typography>
                  ) : (
                    <TableContainer>
                      <Table size="small" sx={{ minWidth: 760 }}>
                        <TableHead>
                          <TableRow>
                            <TableCell sx={{ fontWeight: 700 }}>Type</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Price</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Quota</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Duration</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                            <TableCell sx={{ fontWeight: 700, width: 170 }}>Action</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {offers.map((offer) => {
                            const actionType = resolveOfferActionType(offer, isAdmin);

                            return (
                              <TableRow key={offer.id} hover>
                                <TableCell>
                                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                    {formatOfferType(offer.offerType)}
                                  </Typography>
                                </TableCell>
                                <TableCell>{formatOfferPrice(offer)}</TableCell>
                                <TableCell>
                                  {offer.quotaTotal === null ? '--' : offer.quotaTotal}
                                </TableCell>
                                <TableCell>{formatOfferDuration(offer.durationSeconds)}</TableCell>
                                <TableCell>
                                  <Chip
                                    size="small"
                                    label={offer.status}
                                    color={offer.status === 'active' ? 'success' : 'default'}
                                  />
                                </TableCell>
                                <TableCell>
                                  {actionType === 'activate' ? (
                                    <Button
                                      size="small"
                                      variant="contained"
                                      startIcon={<BoltRoundedIcon fontSize="small" />}
                                      onClick={() => openActivationDialog(item)}
                                    >
                                      Activate
                                    </Button>
                                  ) : null}

                                  {actionType === 'grant' ? (
                                    <Button
                                      size="small"
                                      variant="contained"
                                      color="secondary"
                                      startIcon={
                                        <AdminPanelSettingsRoundedIcon fontSize="small" />
                                      }
                                      onClick={() => openGrantDialog(item)}
                                    >
                                      Grant
                                    </Button>
                                  ) : null}

                                  {actionType === 'inactive' ? (
                                    <Typography variant="caption" color="text.secondary">
                                      Offer inactive
                                    </Typography>
                                  ) : null}

                                  {actionType === 'admin_only' ? (
                                    <Typography variant="caption" color="text.secondary">
                                      Admin grant only
                                    </Typography>
                                  ) : null}

                                  {actionType === 'unsupported' ? (
                                    <Typography variant="caption" color="text.secondary">
                                      activation_code only (current backend)
                                    </Typography>
                                  ) : null}
                                </TableCell>
                              </TableRow>
                            );
                          })}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  )}
                </Collapse>
              </Paper>
            );
          })}
        </>
      ) : null}

      {activeTab === 'repo' ? (
        <>
          <Paper sx={{ p: { xs: 2, md: 2.5 }, borderRadius: 2 }}>
            <Stack
              direction={{ xs: 'column', md: 'row' }}
              spacing={1.25}
              alignItems={{ xs: 'stretch', md: 'center' }}
            >
              <TextField
                select
                size="small"
                label="Entitlement status"
                value={repoFilterStatus}
                onChange={(event) => setRepoFilterStatus(event.target.value)}
                sx={{ minWidth: { xs: '100%', md: 200 } }}
              >
                {REPO_STATUS_OPTIONS.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              <Typography variant="caption" color="text.secondary" sx={{ ml: { md: 'auto' } }}>
                {visibleEntitlements.length} visible entitlement(s).{' '}
                {formatRefreshLabel(repoRefreshedAt)}
              </Typography>
            </Stack>
          </Paper>

          {repoError ? <Alert severity="error">{repoError}</Alert> : null}

          <Paper sx={{ borderRadius: 2, overflow: 'hidden' }}>
            <TableContainer>
              <Table size="small" sx={{ minWidth: 980 }}>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 700 }}>Tool</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Source</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Validity</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Quota</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Updated</TableCell>
                    <TableCell sx={{ fontWeight: 700, width: 220 }}>Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {repoLoading ? (
                    <TableRow>
                      <TableCell colSpan={7}>
                        <Stack
                          direction="row"
                          spacing={1}
                          alignItems="center"
                          justifyContent="center"
                          sx={{ py: 3 }}
                        >
                          <CircularProgress size={20} />
                          <Typography variant="body2" color="text.secondary">
                            Loading entitlements...
                          </Typography>
                        </Stack>
                      </TableCell>
                    </TableRow>
                  ) : visibleEntitlements.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7}>
                        <Box sx={{ py: 4, textAlign: 'center' }}>
                          <Typography variant="body2" color="text.secondary">
                            No entitlements match the selected status.
                          </Typography>
                        </Box>
                      </TableCell>
                    </TableRow>
                  ) : (
                    visibleEntitlements.map((entitlement) => {
                      const displayStatus = getEntitlementDisplayStatus(entitlement);
                      const toolName =
                        entitlement.toolName ||
                        marketItemNameByID[entitlement.toolItemID] ||
                        entitlement.toolItemID ||
                        '--';

                      return (
                        <TableRow key={entitlement.id} hover>
                          <TableCell>
                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                              {toolName}
                            </Typography>
                            <Typography variant="caption" color="text.secondary">
                              entitlement: {entitlement.id}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2">
                              {formatSourceType(entitlement.sourceType)}
                            </Typography>
                            <Typography variant="caption" color="text.secondary">
                              {entitlement.sourceRef || '--'}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Chip
                              size="small"
                              label={displayStatus}
                              color={getEntitlementStatusColor(displayStatus)}
                            />
                          </TableCell>
                          <TableCell>
                            <Stack spacing={0.35}>
                              <Typography variant="caption" color="text.secondary">
                                Start: {formatDateTime(entitlement.startsAt)}
                              </Typography>
                              <Typography variant="caption" color="text.secondary">
                                Expire:{' '}
                                {entitlement.expiresAt
                                  ? formatDateTime(entitlement.expiresAt)
                                  : 'No expiry'}
                              </Typography>
                            </Stack>
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2">
                              {formatQuotaSummary(entitlement.quotaUsed, entitlement.quotaTotal)}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2">
                              {formatDateTime(entitlement.updatedAt || entitlement.createdAt)}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Stack direction="row" spacing={0.8} flexWrap="wrap" useFlexGap>
                              <Button
                                size="small"
                                variant="outlined"
                                startIcon={<TimelineRoundedIcon fontSize="small" />}
                                onClick={() => openUsageDialog(entitlement)}
                              >
                                Usage
                              </Button>
                              <Button
                                size="small"
                                variant="outlined"
                                onClick={() => openRuntimeDialog(entitlement)}
                              >
                                Tools
                              </Button>
                            </Stack>
                          </TableCell>
                        </TableRow>
                      );
                    })
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Paper>
        </>
      ) : null}

      {activeTab === 'admin' && isAdmin ? (
        <>
          <Paper sx={{ p: { xs: 2, md: 2.5 }, borderRadius: 2 }}>
            <Stack
              direction={{ xs: 'column', md: 'row' }}
              spacing={1.25}
              alignItems={{ xs: 'stretch', md: 'center' }}
            >
              <TextField
                select
                size="small"
                label="Protocol"
                value={adminFilters.protocol}
                onChange={handleAdminFilterChange('protocol')}
                sx={{ minWidth: { xs: '100%', md: 170 } }}
              >
                {MARKET_PROTOCOL_OPTIONS.map((option) => (
                  <MenuItem key={option.value || '__all_protocol'} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              <TextField
                select
                size="small"
                label="Status"
                value={adminFilters.status}
                onChange={handleAdminFilterChange('status')}
                sx={{ minWidth: { xs: '100%', md: 170 } }}
              >
                {MARKET_STATUS_OPTIONS.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              <Typography variant="caption" color="text.secondary" sx={{ ml: { md: 'auto' } }}>
                {adminItems.length} item(s). {formatRefreshLabel(adminRefreshedAt)}
              </Typography>

              <Button
                variant="contained"
                startIcon={<AddRoundedIcon />}
                onClick={openCreateAdminItemDialog}
              >
                Create item
              </Button>
            </Stack>
          </Paper>

          {adminError ? <Alert severity="error">{adminError}</Alert> : null}

          <Paper sx={{ borderRadius: 2, overflow: 'hidden' }}>
            <TableContainer>
              <Table size="small" sx={{ minWidth: 980 }}>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 700 }}>Tool Key</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Name</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Provider</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Protocol</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Updated</TableCell>
                    <TableCell sx={{ fontWeight: 700, width: 280 }}>Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {adminLoading ? (
                    <TableRow>
                      <TableCell colSpan={7}>
                        <Stack
                          direction="row"
                          spacing={1}
                          alignItems="center"
                          justifyContent="center"
                          sx={{ py: 3 }}
                        >
                          <CircularProgress size={20} />
                          <Typography variant="body2" color="text.secondary">
                            Loading admin tool items...
                          </Typography>
                        </Stack>
                      </TableCell>
                    </TableRow>
                  ) : adminItems.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7}>
                        <Box sx={{ py: 4, textAlign: 'center' }}>
                          <Typography variant="body2" color="text.secondary">
                            No tool market items found for current filters.
                          </Typography>
                        </Box>
                      </TableCell>
                    </TableRow>
                  ) : (
                    adminItems.map((item) => {
                      const deleting = adminItemDeletingID === item.id;

                      return (
                        <TableRow key={item.id} hover>
                          <TableCell>
                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                              {item.toolKey || '--'}
                            </Typography>
                          </TableCell>
                          <TableCell>{item.name || '--'}</TableCell>
                          <TableCell>{item.provider || '--'}</TableCell>
                          <TableCell>
                            {(item.protocol || 'mcp').toUpperCase()}
                          </TableCell>
                          <TableCell>
                            <Chip
                              size="small"
                              label={item.status}
                              color={getItemStatusColor(item.status)}
                            />
                          </TableCell>
                          <TableCell>
                            {formatDateTime(item.updatedAt || item.createdAt)}
                          </TableCell>
                          <TableCell>
                            <Stack direction="row" spacing={0.8} flexWrap="wrap">
                              <Button
                                size="small"
                                variant="outlined"
                                startIcon={<EditRoundedIcon fontSize="small" />}
                                onClick={() => openEditAdminItemDialog(item)}
                              >
                                Edit
                              </Button>
                              <Button
                                size="small"
                                variant="outlined"
                                color="secondary"
                                startIcon={<AdminPanelSettingsRoundedIcon fontSize="small" />}
                                onClick={() => openGrantDialog(item)}
                                disabled={item.status !== 'active'}
                              >
                                Grant
                              </Button>
                              <Button
                                size="small"
                                variant="outlined"
                                color="error"
                                startIcon={<DeleteOutlineRoundedIcon fontSize="small" />}
                                onClick={() => handleDeleteAdminItem(item)}
                                disabled={deleting}
                              >
                                {deleting ? 'Deleting...' : 'Delete'}
                              </Button>
                            </Stack>
                          </TableCell>
                        </TableRow>
                      );
                    })
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Paper>
        </>
      ) : null}

      <Dialog
        open={activationDialogOpen}
        onClose={closeActivationDialog}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>Activate Tool</DialogTitle>
        <DialogContent sx={{ pt: 1.2 }}>
          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              Activate {activationTarget?.name || activationTarget?.toolKey || 'tool'} to My Tool
              Repo.
            </Typography>

            {activationError ? <Alert severity="error">{activationError}</Alert> : null}
          </Stack>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2.5 }}>
          <Button onClick={closeActivationDialog} disabled={activationSubmitting}>
            Cancel
          </Button>
          <Button
            variant="contained"
            startIcon={<BoltRoundedIcon />}
            onClick={handleSubmitActivation}
            disabled={activationSubmitting}
          >
            {activationSubmitting ? 'Activating...' : 'Activate'}
          </Button>
        </DialogActions>
      </Dialog>

      {isAdmin ? (
        <Dialog open={grantDialogOpen} onClose={closeGrantDialog} fullWidth maxWidth="sm">
          <DialogTitle>Admin Grant Entitlement</DialogTitle>
          <DialogContent sx={{ pt: 1.2 }}>
            <Stack spacing={1.5}>
              <Typography variant="body2" color="text.secondary">
                Grant {grantTarget?.item?.name || grantTarget?.item?.toolKey || 'tool'} to a user.
              </Typography>

              <TextField
                label="Target user id"
                value={grantForm.userID}
                onChange={handleGrantFormChange('userID')}
                autoFocus
                required
              />

              <TextField
                label="Source ref (optional)"
                value={grantForm.sourceRef}
                onChange={handleGrantFormChange('sourceRef')}
                placeholder="e.g. admin:manual-grant"
              />

              <TextField
                label="Starts at (optional)"
                type="datetime-local"
                value={grantForm.startsAt}
                onChange={handleGrantFormChange('startsAt')}
                InputLabelProps={{ shrink: true }}
              />

              {grantError ? <Alert severity="error">{grantError}</Alert> : null}
            </Stack>
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 2.5 }}>
            <Button onClick={closeGrantDialog} disabled={grantSubmitting}>
              Cancel
            </Button>
            <Button
              variant="contained"
              color="secondary"
              startIcon={<AdminPanelSettingsRoundedIcon />}
              onClick={handleSubmitGrant}
              disabled={grantSubmitting}
            >
              {grantSubmitting ? 'Granting...' : 'Grant'}
            </Button>
          </DialogActions>
        </Dialog>
      ) : null}

      {isAdmin ? (
        <Dialog
          open={adminItemDialogOpen}
          onClose={closeAdminItemDialog}
          fullWidth
          maxWidth="md"
        >
          <DialogTitle>
            {adminItemDialogMode === 'create' ? 'Create Tool Market Item' : 'Edit Tool Market Item'}
          </DialogTitle>
          <DialogContent sx={{ pt: 1.2 }}>
            <Stack spacing={1.5}>
              <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.2}>
                <TextField
                  label="Tool key"
                  value={adminItemForm.toolKey}
                  onChange={handleAdminItemFormChange('toolKey')}
                  error={Boolean(adminItemFieldErrors.toolKey)}
                  helperText={adminItemFieldErrors.toolKey || 'Unique key, e.g. device-status-tool'}
                  required
                  disabled={adminItemDialogMode === 'edit'}
                  fullWidth
                />

                <TextField
                  label="Provider"
                  value={adminItemForm.provider}
                  onChange={handleAdminItemFormChange('provider')}
                  error={Boolean(adminItemFieldErrors.provider)}
                  helperText={adminItemFieldErrors.provider || 'e.g. custom'}
                  required
                  disabled={adminItemDialogMode === 'edit'}
                  fullWidth
                />
              </Stack>

              <TextField
                label="Name"
                value={adminItemForm.name}
                onChange={handleAdminItemFormChange('name')}
                error={Boolean(adminItemFieldErrors.name)}
                helperText={adminItemFieldErrors.name || ''}
                required
                fullWidth
              />

              <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.2}>
                <TextField
                  select
                  label="Protocol"
                  value={adminItemForm.protocol}
                  onChange={handleAdminItemFormChange('protocol')}
                  error={Boolean(adminItemFieldErrors.protocol)}
                  helperText={adminItemFieldErrors.protocol || ''}
                  disabled={adminItemDialogMode === 'edit'}
                  sx={{ minWidth: { xs: '100%', md: 190 } }}
                >
                  <MenuItem value="mcp">mcp</MenuItem>
                </TextField>

                <TextField
                  select
                  label="Status"
                  value={adminItemForm.status}
                  onChange={handleAdminItemFormChange('status')}
                  error={Boolean(adminItemFieldErrors.status)}
                  helperText={adminItemFieldErrors.status || ''}
                  sx={{ minWidth: { xs: '100%', md: 190 } }}
                >
                  {TOOL_MARKET_ITEM_STATUS_OPTIONS.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>
              </Stack>

              <Divider />
              <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                MCP Config
              </Typography>

              <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.2}>
                <TextField
                  select
                  label="Transport"
                  value={adminMcpConfigForm.transport}
                  onChange={handleAdminMcpConfigFormChange('transport')}
                  error={Boolean(adminMcpConfigFieldErrors.transport)}
                  helperText={adminMcpConfigFieldErrors.transport || ''}
                  sx={{ minWidth: { xs: '100%', md: 200 } }}
                >
                  {MCP_TRANSPORT_OPTIONS.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  label="Timeout (ms)"
                  type="number"
                  value={adminMcpConfigForm.timeoutMs}
                  onChange={handleAdminMcpConfigFormChange('timeoutMs')}
                  error={Boolean(adminMcpConfigFieldErrors.timeoutMs)}
                  helperText={adminMcpConfigFieldErrors.timeoutMs || ''}
                  sx={{ minWidth: { xs: '100%', md: 200 } }}
                />
              </Stack>

              <TextField
                label="tool_name_list (selected tools)"
                value={adminMcpConfigForm.toolNameListText}
                onChange={handleAdminMcpConfigFormChange('toolNameListText')}
                helperText={`${selectedMcpToolNameList.length} selected (empty = load all tools)`}
                multiline
                minRows={2}
                fullWidth
              />

              <Alert severity="info" variant="outlined">
                MCP real connectivity and tool_name_list subset validation are executed by backend
                during create/update.
              </Alert>

              <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.2}>
                <TextField
                  select
                  label="Auth type"
                  value={adminMcpConfigForm.authType}
                  onChange={handleAdminMcpConfigFormChange('authType')}
                  error={Boolean(adminMcpConfigFieldErrors.authType)}
                  helperText={adminMcpConfigFieldErrors.authType || ''}
                  sx={{ minWidth: { xs: '100%', md: 200 } }}
                >
                  {MCP_AUTH_TYPE_OPTIONS.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      {option.label}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  label="Auth token"
                  value={adminMcpConfigForm.authToken}
                  onChange={handleAdminMcpConfigFormChange('authToken')}
                  error={Boolean(adminMcpConfigFieldErrors.authToken)}
                  helperText={
                    adminMcpConfigFieldErrors.authToken ||
                    (adminMcpConfigForm.authType === 'none'
                      ? 'Not required when auth type is none'
                      : '')
                  }
                  fullWidth
                />

                <TextField
                  label="Auth header"
                  value={adminMcpConfigForm.authHeader}
                  onChange={handleAdminMcpConfigFormChange('authHeader')}
                  error={Boolean(adminMcpConfigFieldErrors.authHeader)}
                  helperText={adminMcpConfigFieldErrors.authHeader || ''}
                  fullWidth
                />
              </Stack>

              {adminMcpConfigForm.transport === 'stdio' ? (
                <>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    stdio transport
                  </Typography>
                  <TextField
                    label="stdio.command"
                    value={adminMcpConfigForm.stdioCommand}
                    onChange={handleAdminMcpConfigFormChange('stdioCommand')}
                    error={Boolean(adminMcpConfigFieldErrors.stdioCommand)}
                    helperText={adminMcpConfigFieldErrors.stdioCommand || 'e.g. python'}
                    required
                    fullWidth
                  />

                  <TextField
                    label="stdio.args (optional, one per line)"
                    value={adminMcpConfigForm.stdioArgsText}
                    onChange={handleAdminMcpConfigFormChange('stdioArgsText')}
                    multiline
                    minRows={2}
                    fullWidth
                  />

                  <TextField
                    label="stdio.cwd (optional)"
                    value={adminMcpConfigForm.stdioCwd}
                    onChange={handleAdminMcpConfigFormChange('stdioCwd')}
                    fullWidth
                  />

                  <TextField
                    label="stdio.env (JSON object, optional)"
                    value={adminMcpConfigForm.stdioEnvText}
                    onChange={handleAdminMcpConfigFormChange('stdioEnvText')}
                    error={Boolean(adminMcpConfigFieldErrors.stdioEnvText)}
                    helperText={adminMcpConfigFieldErrors.stdioEnvText || ''}
                    multiline
                    minRows={4}
                    fullWidth
                  />
                </>
              ) : null}

              {adminMcpConfigForm.transport === 'sse' ? (
                <>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    sse transport
                  </Typography>
                  <TextField
                    label="sse.endpoint"
                    value={adminMcpConfigForm.sseEndpoint}
                    onChange={handleAdminMcpConfigFormChange('sseEndpoint')}
                    error={Boolean(adminMcpConfigFieldErrors.sseEndpoint)}
                    helperText={adminMcpConfigFieldErrors.sseEndpoint || ''}
                    required
                    fullWidth
                  />

                  <TextField
                    label="sse.headers (JSON object, optional)"
                    value={adminMcpConfigForm.sseHeadersText}
                    onChange={handleAdminMcpConfigFormChange('sseHeadersText')}
                    error={Boolean(adminMcpConfigFieldErrors.sseHeadersText)}
                    helperText={adminMcpConfigFieldErrors.sseHeadersText || ''}
                    multiline
                    minRows={4}
                    fullWidth
                  />
                </>
              ) : null}

              {adminMcpConfigForm.transport === 'stream_http' ? (
                <>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    stream_http transport
                  </Typography>
                  <TextField
                    label="stream_http.endpoint"
                    value={adminMcpConfigForm.streamHttpEndpoint}
                    onChange={handleAdminMcpConfigFormChange('streamHttpEndpoint')}
                    error={Boolean(adminMcpConfigFieldErrors.streamHttpEndpoint)}
                    helperText={adminMcpConfigFieldErrors.streamHttpEndpoint || ''}
                    required
                    fullWidth
                  />

                  <TextField
                    label="stream_http.headers (JSON object, optional)"
                    value={adminMcpConfigForm.streamHttpHeadersText}
                    onChange={handleAdminMcpConfigFormChange('streamHttpHeadersText')}
                    error={Boolean(adminMcpConfigFieldErrors.streamHttpHeadersText)}
                    helperText={adminMcpConfigFieldErrors.streamHttpHeadersText || ''}
                    multiline
                    minRows={4}
                    fullWidth
                  />
                </>
              ) : null}

              {adminItemSubmitError ? <Alert severity="error">{adminItemSubmitError}</Alert> : null}
            </Stack>
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 2.5 }}>
            <Button onClick={closeAdminItemDialog} disabled={adminItemSubmitting}>
              Cancel
            </Button>
            <Button
              variant="contained"
              startIcon={
                adminItemDialogMode === 'create' ? (
                  <AddRoundedIcon />
                ) : (
                  <EditRoundedIcon />
                )
              }
              onClick={handleSubmitAdminItem}
              disabled={adminItemSubmitting}
            >
              {adminItemSubmitting
                ? adminItemDialogMode === 'create'
                  ? 'Creating...'
                  : 'Saving...'
                : adminItemDialogMode === 'create'
                  ? 'Create item'
                  : 'Save changes'}
            </Button>
          </DialogActions>
        </Dialog>
      ) : null}

      {isAdmin ? (
        <Dialog
          open={adminOfferDialogOpen}
          onClose={closeAdminOfferDialog}
          fullWidth
          maxWidth="sm"
        >
          <DialogTitle>Create Offer</DialogTitle>
          <DialogContent sx={{ pt: 1.2 }}>
            <Stack spacing={1.5}>
              <Typography variant="body2" color="text.secondary">
                {adminOfferTargetItem?.name || adminOfferTargetItem?.toolKey || 'tool'}
              </Typography>

              <TextField
                select
                label="Offer type"
                value={adminOfferForm.offerType}
                onChange={handleAdminOfferFormChange('offerType')}
              >
                {TOOL_OFFER_TYPE_OPTIONS.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.2}>
                <TextField
                  label="Price (optional)"
                  value={adminOfferForm.price}
                  onChange={handleAdminOfferFormChange('price')}
                  placeholder="e.g. 9.99"
                  fullWidth
                />
                <TextField
                  label="Currency (optional)"
                  value={adminOfferForm.currency}
                  onChange={handleAdminOfferFormChange('currency')}
                  placeholder="USD"
                  fullWidth
                />
              </Stack>

              <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.2}>
                <TextField
                  label="Quota total (optional)"
                  value={adminOfferForm.quotaTotal}
                  onChange={handleAdminOfferFormChange('quotaTotal')}
                  placeholder="e.g. 1000"
                  fullWidth
                />
                <TextField
                  label="Duration seconds (optional)"
                  value={adminOfferForm.durationSeconds}
                  onChange={handleAdminOfferFormChange('durationSeconds')}
                  placeholder="e.g. 2592000"
                  fullWidth
                />
              </Stack>

              <TextField
                select
                label="Offer status"
                value={adminOfferForm.status}
                onChange={handleAdminOfferFormChange('status')}
              >
                {TOOL_OFFER_STATUS_OPTIONS.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </TextField>

              {adminOfferError ? <Alert severity="error">{adminOfferError}</Alert> : null}
            </Stack>
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 2.5 }}>
            <Button onClick={closeAdminOfferDialog} disabled={adminOfferSubmitting}>
              Cancel
            </Button>
            <Button
              variant="contained"
              color="secondary"
              startIcon={<AddRoundedIcon />}
              onClick={handleSubmitAdminOffer}
              disabled={adminOfferSubmitting}
            >
              {adminOfferSubmitting ? 'Creating...' : 'Create offer'}
            </Button>
          </DialogActions>
        </Dialog>
      ) : null}

      <Dialog open={runtimeDialogOpen} onClose={closeRuntimeDialog} fullWidth maxWidth="md">
        <DialogTitle>Entitlement Tools Runtime</DialogTitle>
        <DialogContent sx={{ pt: 1.2 }}>
          <Stack spacing={1.5}>
            <Stack
              direction={{ xs: 'column', md: 'row' }}
              spacing={1}
              alignItems={{ xs: 'stretch', md: 'center' }}
            >
              <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
                entitlement: {runtimeTargetEntitlement?.id || '--'}
              </Typography>

              <Button
                size="small"
                variant="outlined"
                startIcon={<RefreshRoundedIcon />}
                onClick={handleRefreshRuntimeTools}
                disabled={runtimeToolsLoading || !runtimeTargetEntitlement?.id}
              >
                Refresh tools
              </Button>
            </Stack>

            <Stack direction="row" spacing={1} flexWrap="wrap">
              <Chip size="small" label={`${runtimeTools.length} tools`} />
              <Chip size="small" variant="outlined" label={formatRefreshLabel(runtimeRefreshedAt)} />
            </Stack>

            {runtimeToolsError ? <Alert severity="error">{runtimeToolsError}</Alert> : null}

            {runtimeToolsLoading ? (
              <Stack
                direction="row"
                spacing={1}
                alignItems="center"
                justifyContent="center"
                sx={{ py: 2 }}
              >
                <CircularProgress size={20} />
                <Typography variant="body2" color="text.secondary">
                  Loading runtime tools list...
                </Typography>
              </Stack>
            ) : runtimeTools.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                No tools returned for this entitlement.
              </Typography>
            ) : (
              <>
                <TextField
                  select
                  label="Tool"
                  value={runtimeSelectedToolName}
                  onChange={(event) => {
                    setRuntimeSelectedToolName(event.target.value);
                    setRuntimeCallError('');
                    setRuntimeCallResult(null);
                  }}
                  fullWidth
                >
                  {runtimeTools.map((tool) => (
                    <MenuItem key={tool.name} value={tool.name}>
                      {tool.name}
                      {tool.description ? ` - ${tool.description}` : ''}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  label="Input schema"
                  value={
                    runtimeSelectedTool
                      ? JSON.stringify(runtimeSelectedTool.inputSchema || {}, null, 2)
                      : '{}'
                  }
                  multiline
                  minRows={5}
                  fullWidth
                  InputProps={{ readOnly: true }}
                />

                <TextField
                  label="Tool arguments (JSON object)"
                  value={runtimeToolArguments}
                  onChange={(event) => {
                    setRuntimeToolArguments(event.target.value);
                    if (runtimeCallError) {
                      setRuntimeCallError('');
                    }
                  }}
                  multiline
                  minRows={5}
                  fullWidth
                />

                {runtimeCallError ? <Alert severity="error">{runtimeCallError}</Alert> : null}

                {runtimeCallResult ? (
                  <TextField
                    label="Call result"
                    value={JSON.stringify(runtimeCallResult, null, 2)}
                    multiline
                    minRows={8}
                    fullWidth
                    InputProps={{ readOnly: true }}
                  />
                ) : null}
              </>
            )}
          </Stack>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2.5 }}>
          <Button onClick={closeRuntimeDialog} disabled={runtimeToolsLoading || runtimeCallSubmitting}>
            Close
          </Button>
          <Button
            variant="contained"
            onClick={handleSubmitRuntimeToolCall}
            disabled={runtimeToolsLoading || runtimeCallSubmitting || runtimeTools.length === 0}
          >
            {runtimeCallSubmitting ? 'Calling...' : 'Run tool'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={usageDialogOpen} onClose={closeUsageDialog} fullWidth maxWidth="md">
        <DialogTitle>Entitlement Usage</DialogTitle>
        <DialogContent sx={{ pt: 1.2 }}>
          <Stack spacing={1.5}>
            <Stack
              direction={{ xs: 'column', md: 'row' }}
              spacing={1}
              alignItems={{ xs: 'stretch', md: 'center' }}
            >
              <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
                entitlement: {usageTarget?.id || '--'}
              </Typography>

              <TextField
                size="small"
                label="Limit"
                value={usageLimitInput}
                onChange={(event) => setUsageLimitInput(event.target.value)}
                sx={{ width: { xs: '100%', md: 120 } }}
              />

              <Button
                size="small"
                variant="outlined"
                startIcon={<RefreshRoundedIcon />}
                onClick={handleRefreshUsage}
                disabled={usageLoading || !usageTarget?.id}
              >
                Refresh
              </Button>
            </Stack>

            <Stack direction="row" spacing={1} flexWrap="wrap">
              <Chip size="small" label={`${usageRecords.length} records`} />
              <Chip size="small" label={`${usageConsumedTotal} units consumed`} />
              <Chip size="small" variant="outlined" label={formatRefreshLabel(usageRefreshedAt)} />
            </Stack>

            {usageError ? <Alert severity="error">{usageError}</Alert> : null}

            {usageLoading ? (
              <Stack
                direction="row"
                spacing={1}
                alignItems="center"
                justifyContent="center"
                sx={{ py: 2 }}
              >
                <CircularProgress size={20} />
                <Typography variant="body2" color="text.secondary">
                  Loading usage records...
                </Typography>
              </Stack>
            ) : usageRecords.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                No usage records.
              </Typography>
            ) : (
              <TableContainer>
                <Table size="small" sx={{ minWidth: 700 }}>
                  <TableHead>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 700 }}>Time</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Consumed</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Voicebot</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Device</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {usageRecords.map((record) => (
                      <TableRow key={record.id} hover>
                        <TableCell>{formatDateTime(record.createdAt)}</TableCell>
                        <TableCell>{record.consumedUnits}</TableCell>
                        <TableCell>{record.voicebotID || '--'}</TableCell>
                        <TableCell>{record.deviceID || '--'}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
          </Stack>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2.5 }}>
          <Button onClick={closeUsageDialog} disabled={usageLoading}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
