export const mockModels = [
  { id: 'biker_girlfriend', name: '台味女友' },
  { id: 'english_teacher', name: '英语老师' },
  { id: 'default_assistant', name: '默认助手' }
];

export const mockMcpServers = [
  {
    id: 'filesystem',
    name: 'filesystem',
    description: '文件系统访问与检索',
    enabled: true,
    status: 'online'
  },
  {
    id: 'home_assistant',
    name: 'Home Assistant',
    description: '智能家居控制',
    enabled: false,
    status: 'offline'
  },
  {
    id: 'playwright',
    name: 'playwright',
    description: '网页自动化与抓取',
    enabled: false,
    status: 'online'
  }
];

export const mockThreads = [
  {
    id: 'thread-1',
    title: '今天的计划',
    lastMessage: '帮我整理一下今天的任务',
    updatedAt: Date.now() - 1000 * 60 * 4,
    modelId: 'biker_girlfriend'
  },
  {
    id: 'thread-2',
    title: '旅行助手',
    lastMessage: '三天两晚杭州行程',
    updatedAt: Date.now() - 1000 * 60 * 60,
    modelId: 'default_assistant'
  },
  {
    id: 'thread-3',
    title: '英语口语',
    lastMessage: '今天的口语练习',
    updatedAt: Date.now() - 1000 * 60 * 120,
    modelId: 'english_teacher'
  }
];

export const mockMessagesByThread = {
  'thread-1': [
    {
      id: 'm-1',
      threadId: 'thread-1',
      role: 'assistant',
      text: '早安，我可以帮你梳理今天的计划。先告诉我今天最重要的三件事？',
      ts: Date.now() - 1000 * 60 * 10
    },
    {
      id: 'm-2',
      threadId: 'thread-1',
      role: 'user',
      text: '上午评审，下午做汇报，晚上健身。',
      ts: Date.now() - 1000 * 60 * 8
    },
    {
      id: 'm-3',
      threadId: 'thread-1',
      role: 'assistant',
      text: '收到。我建议上午评审前预留30分钟准备，下午汇报前复盘关键结论，晚上健身安排在晚饭后1小时。',
      ts: Date.now() - 1000 * 60 * 6
    }
  ],
  'thread-2': [
    {
      id: 'm-4',
      threadId: 'thread-2',
      role: 'user',
      text: '帮我做个杭州三天两晚行程。',
      ts: Date.now() - 1000 * 60 * 80
    },
    {
      id: 'm-5',
      threadId: 'thread-2',
      role: 'assistant',
      text: '可以。你更偏好人文景点还是自然风光？',
      ts: Date.now() - 1000 * 60 * 78
    }
  ],
  'thread-3': [
    {
      id: 'm-6',
      threadId: 'thread-3',
      role: 'assistant',
      text: 'Hello! Today we will practice short self-introductions. Ready?',
      ts: Date.now() - 1000 * 60 * 200
    }
  ]
};
