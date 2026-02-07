# 小智 Web UI（React + MUI）

这是一个基于 React 与 MUI 的终端用户对话界面示例。支持真实 WebSocket 连接与 Opus 语音传输（与 test_page 协议一致），同时保留线程与历史记录的本地 mock 数据。

## 运行

```bash
npm install
npm run dev
```

## 目录

```
web-ui/
  src/
    components/        # UI 组件
    mock/              # 假数据
    utils/             # 工具函数
```

## 说明

- 默认使用 mock 数据渲染 threads、历史、模型与 MCP Server。
- 连接、按住说话、电话通话已直接对接 WebSocket 与 Opus。
- 需确保服务端支持 `listen` 与 Opus 二进制帧协议。
