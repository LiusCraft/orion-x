import * as React from 'react';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Snackbar from '@mui/material/Snackbar';
import ChatPanel from '../components/ChatPanel.jsx';
import SettingsDrawer from '../components/SettingsDrawer.jsx';
import ThreadSidebar from '../components/ThreadSidebar.jsx';
import {
  mockMcpServers,
  mockMessagesByThread,
  mockModels,
  mockThreads
} from '../mock/data.js';
import { createAudioEngine } from '../utils/audioEngine.js';

function createId(prefix) {
  return `${prefix}-${Math.random().toString(36).slice(2, 8)}`;
}

function createThreadState() {
  return {
    connState: 'disconnected',
    sessionState: 'idle',
    ws: null,
    audioContext: null
  };
}

export default function LegacyVoiceChatPage() {
  const [threads, setThreads] = React.useState(mockThreads);
  const [messagesByThread, setMessagesByThread] = React.useState(
    mockMessagesByThread
  );
  const [activeThreadId, setActiveThreadId] = React.useState(mockThreads[0].id);
  const [inputValue, setInputValue] = React.useState('');
  const [settingsOpen, setSettingsOpen] = React.useState(false);
  const [models] = React.useState(mockModels);
  const [selectedModel, setSelectedModel] = React.useState(mockModels[0].id);
  const [mcpServers, setMcpServers] = React.useState(mockMcpServers);
  const [snackbar, setSnackbar] = React.useState({ open: false, message: '' });
  const [debugStats, setDebugStats] = React.useState({});
  const [connection, setConnection] = React.useState({
    wsUrl: 'ws://127.0.0.1:8000/xiaozhi/v1/',
    deviceMac: 'AA:BB:CC:DD:EE:FF',
    clientId: 'web_user_client',
    token: 'your-token',
    deviceName: 'Web 客户端'
  });

  const threadStatesRef = React.useRef(new Map());
  const audioEngineRef = React.useRef(null);

  const activeThread = threads.find((thread) => thread.id === activeThreadId);
  const messages = messagesByThread[activeThreadId] || [];
  const activeModelId = activeThread ? activeThread.modelId : selectedModel;
  const modelName =
    models.find((model) => model.id === activeModelId)?.name || '默认模型';

  const getThreadState = (threadId) => {
    if (!threadStatesRef.current.has(threadId)) {
      threadStatesRef.current.set(threadId, createThreadState());
    }
    return threadStatesRef.current.get(threadId);
  };

  const currentThreadState = getThreadState(activeThreadId);
  const connState = currentThreadState.connState;
  const sessionState = currentThreadState.sessionState;
  const listening = sessionState === 'listening';
  const calling = sessionState === 'calling';

  const showSnackbar = (message) => {
    setSnackbar({ open: true, message });
  };

  const ensureAudioEngine = React.useCallback(() => {
    if (!audioEngineRef.current) {
      audioEngineRef.current = createAudioEngine({
        onStatus: (message) => showSnackbar(message),
        onError: (message) => showSnackbar(message)
      });
    }
    return audioEngineRef.current;
  }, []);

  const bumpThread = (threadId, lastMessage) => {
    setThreads((previous) => {
      const updated = previous.map((thread) =>
        thread.id === threadId
          ? { ...thread, lastMessage, updatedAt: Date.now() }
          : thread
      );
      const active = updated.find((thread) => thread.id === threadId);
      const rest = updated.filter((thread) => thread.id !== threadId);
      return active ? [active, ...rest] : updated;
    });
  };

  const appendMessage = (threadId, message) => {
    setMessagesByThread((previous) => ({
      ...previous,
      [threadId]: [...(previous[threadId] || []), message]
    }));
    if (message.text) {
      bumpThread(threadId, message.text);
    }
  };

  const buildWsUrl = () => {
    const url = new URL(connection.wsUrl);
    url.searchParams.set('device-id', connection.deviceMac);
    url.searchParams.set('client-id', connection.clientId);
    return url.toString();
  };

  const sendHello = (ws) => {
    const helloMessage = {
      type: 'hello',
      device_id: connection.deviceMac,
      device_name: connection.deviceName,
      device_mac: connection.deviceMac,
      token: connection.token,
      features: {
        mcp: true,
        notify: {
          config_updated: true
        }
      }
    };
    ws.send(JSON.stringify(helloMessage));
  };

  const updateDebugStats = React.useCallback(() => {
    const stats = {};
    for (const [threadId, state] of threadStatesRef.current) {
      stats[threadId] = {
        connState: state.connState,
        sessionState: state.sessionState,
        wsReadyState: state.ws?.readyState ?? null
      };
    }
    setDebugStats(stats);
  }, []);

  const connectThread = (threadId) => {
    const state = getThreadState(threadId);

    if (state.connState === 'connected' || state.connState === 'connecting') {
      return;
    }

    if (!connection.wsUrl) {
      showSnackbar('请先填写 WS 地址');
      return;
    }

    ensureAudioEngine().resume();
    state.connState = 'connecting';
    updateDebugStats();

    const ws = new WebSocket(buildWsUrl());
    ws.binaryType = 'arraybuffer';
    state.ws = ws;

    ws.onopen = () => {
      state.connState = 'connected';
      updateDebugStats();
      if (threadId === activeThreadId) {
        showSnackbar('连接成功');
      }
      sendHello(ws);
    };

    ws.onclose = () => {
      state.connState = 'disconnected';
      state.sessionState = 'idle';
      ensureAudioEngine().stopAll(ws);
      updateDebugStats();
      if (threadId === activeThreadId) {
        showSnackbar('连接已断开');
      }
    };

    ws.onerror = () => {
      if (threadId === activeThreadId) {
        showSnackbar('WebSocket 连接错误');
      }
      state.connState = 'disconnected';
      updateDebugStats();
    };

    ws.onmessage = async (event) => {
      const currentCalling = state.sessionState === 'calling';

      if (typeof event.data === 'string') {
        try {
          const message = JSON.parse(event.data);
          if (message.type === 'hello') {
            if (threadId === activeThreadId) {
              showSnackbar('握手成功');
            }
          } else if (message.type === 'tts') {
            if (message.state === 'start') {
              ensureAudioEngine().onTTSStart();
              if (!currentCalling) {
                state.sessionState = 'speaking';
                updateDebugStats();
              }
            } else if (message.state === 'sentence_start') {
              if (message.text) {
                appendMessage(threadId, {
                  id: createId('m'),
                  threadId,
                  role: 'assistant',
                  text: message.text,
                  tts: true,
                  ts: Date.now()
                });
              }
            } else if (message.state === 'stop') {
              ensureAudioEngine().onTTSStop(Boolean(message.is_aborted));
              if (!currentCalling && state.sessionState !== 'listening') {
                state.sessionState = 'idle';
                updateDebugStats();
              }
            }
          } else if (message.type === 'stt') {
            appendMessage(threadId, {
              id: createId('m'),
              threadId,
              role: 'user',
              text: message.text ? `（语音识别）${message.text}` : '（语音识别）',
              stt: true,
              ts: Date.now()
            });
          } else if (message.type === 'llm') {
            if (message.text && message.text !== '😊') {
              appendMessage(threadId, {
                id: createId('m'),
                threadId,
                role: 'assistant',
                text: message.text,
                ts: Date.now()
              });
            }
          } else if (message.type === 'mcp') {
            appendMessage(threadId, {
              id: createId('m'),
              threadId,
              role: 'system',
              text: `MCP: ${JSON.stringify(message.payload || {})}`,
              ts: Date.now()
            });
          }
        } catch (error) {
          if (threadId === activeThreadId) {
            showSnackbar('收到无法解析的消息');
          }
        }
      } else {
        await ensureAudioEngine().handleIncomingAudio(event.data);
      }
    };
  };

  const disconnectThread = (threadId) => {
    const state = getThreadState(threadId);
    if (state.ws) {
      state.ws.close();
      state.ws = null;
    }
    state.connState = 'disconnected';
    state.sessionState = 'idle';
    updateDebugStats();
  };

  const handleSelectThread = (threadId) => {
    setActiveThreadId(threadId);
    const thread = threads.find((item) => item.id === threadId);
    if (thread) {
      setSelectedModel(thread.modelId);
    }
    updateDebugStats();
  };

  const handleCreateThread = () => {
    const newThread = {
      id: createId('thread'),
      title: '新的对话',
      lastMessage: '开始和小智对话吧',
      updatedAt: Date.now(),
      modelId: selectedModel
    };
    setThreads((previous) => [newThread, ...previous]);
    setMessagesByThread((previous) => ({ ...previous, [newThread.id]: [] }));
    setActiveThreadId(newThread.id);

    threadStatesRef.current.set(newThread.id, createThreadState());
    updateDebugStats();
  };

  const handleToggleConnect = () => {
    const state = getThreadState(activeThreadId);

    if (state.connState === 'connected') {
      disconnectThread(activeThreadId);
    } else {
      connectThread(activeThreadId);
    }
  };

  const handleSend = () => {
    const trimmed = inputValue.trim();
    if (!trimmed) {
      return;
    }

    const state = getThreadState(activeThreadId);
    ensureAudioEngine().resume();

    const newMessage = {
      id: createId('m'),
      threadId: activeThreadId,
      role: 'user',
      text: trimmed,
      ts: Date.now()
    };
    appendMessage(activeThreadId, newMessage);
    setInputValue('');

    if (state.ws && state.ws.readyState === WebSocket.OPEN) {
      state.ws.send(
        JSON.stringify({
          type: 'listen',
          mode: 'manual',
          state: 'detect',
          text: trimmed
        })
      );
    } else {
      showSnackbar('未连接，无法发送');
    }
  };

  const handlePressStart = () => {
    const state = getThreadState(activeThreadId);

    if (state.sessionState === 'calling' || state.connState !== 'connected') {
      return;
    }
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
      showSnackbar('未连接，无法录音');
      return;
    }
    state.sessionState = 'listening';
    updateDebugStats();
    ensureAudioEngine().startRecording(state.ws);
  };

  const handlePressEnd = () => {
    const state = getThreadState(activeThreadId);

    if (state.sessionState !== 'listening') {
      return;
    }
    state.sessionState = 'idle';
    updateDebugStats();
    ensureAudioEngine().stopRecording(state.ws);
  };

  const handleToggleCall = () => {
    const state = getThreadState(activeThreadId);

    if (state.connState !== 'connected') {
      return;
    }
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
      showSnackbar('未连接，无法通话');
      return;
    }

    if (state.sessionState === 'calling') {
      ensureAudioEngine().stopRecording(state.ws);
      state.sessionState = 'idle';
      showSnackbar('已挂断');
    } else {
      state.sessionState = 'calling';
      ensureAudioEngine().startRecording(state.ws);
      showSnackbar('通话中');
    }
    updateDebugStats();
  };

  React.useEffect(() => {
    const timer = setInterval(() => {
      if (!audioEngineRef.current) {
        return;
      }
      const audioStats = audioEngineRef.current.getStats();
      const stats = {};
      for (const [threadId, state] of threadStatesRef.current) {
        stats[threadId] = {
          connState: state.connState,
          sessionState: state.sessionState,
          wsReadyState: state.ws?.readyState ?? null,
          ...audioStats
        };
      }
      setDebugStats(stats);
    }, 500);
    return () => clearInterval(timer);
  }, []);

  React.useEffect(() => {
    return () => {
      for (const [, state] of threadStatesRef.current) {
        if (state.ws) {
          state.ws.close();
        }
      }
      threadStatesRef.current.clear();
    };
  }, []);

  const handleSelectModel = (modelId) => {
    setSelectedModel(modelId);
    setThreads((previous) =>
      previous.map((thread) =>
        thread.id === activeThreadId ? { ...thread, modelId } : thread
      )
    );
  };

  const handleToggleMcp = (serverId) => {
    setMcpServers((previous) =>
      previous.map((server) =>
        server.id === serverId
          ? { ...server, enabled: !server.enabled }
          : server
      )
    );
  };

  const handleUpdateConnection = (field, value) => {
    setConnection((previous) => ({ ...previous, [field]: value }));
  };

  const getThreadConnState = (threadId) => {
    return getThreadState(threadId).connState;
  };

  const getThreadSessionState = (threadId) => {
    return getThreadState(threadId).sessionState;
  };

  if (!activeThread) {
    return null;
  }

  return (
    <>
      <Box sx={{ display: 'flex', height: '100vh' }}>
        <ThreadSidebar
          threads={threads}
          activeThreadId={activeThreadId}
          onSelectThread={handleSelectThread}
          onCreateThread={handleCreateThread}
          connState={connState}
          sessionState={sessionState}
          onToggleConnect={handleToggleConnect}
          onOpenSettings={() => setSettingsOpen(true)}
          getThreadConnState={getThreadConnState}
          getThreadSessionState={getThreadSessionState}
        />
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
          <ChatPanel
            thread={activeThread}
            modelName={modelName}
            messages={messages}
            inputValue={inputValue}
            onInputChange={setInputValue}
            onSend={handleSend}
            onPressStart={handlePressStart}
            onPressEnd={handlePressEnd}
            onToggleCall={handleToggleCall}
            calling={calling}
            listening={listening}
            disabled={connState !== 'connected'}
            debug={debugStats[activeThreadId]}
            connState={connState}
            sessionState={sessionState}
            wsReadyState={debugStats[activeThreadId]?.wsReadyState ?? null}
          />
        </Box>
        <SettingsDrawer
          open={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          models={models}
          selectedModel={selectedModel}
          onSelectModel={handleSelectModel}
          mcpServers={mcpServers}
          onToggleMcp={handleToggleMcp}
          connection={connection}
          onUpdateConnection={handleUpdateConnection}
        />
      </Box>
      <Snackbar
        open={snackbar.open}
        autoHideDuration={2000}
        onClose={() => setSnackbar({ open: false, message: '' })}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert severity="info" variant="filled" sx={{ width: '100%' }}>
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
}
