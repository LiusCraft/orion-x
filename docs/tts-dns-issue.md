# TTS DNS 查询被取消问题

## 现象

```
AudioOutPipe: starting TTS stream...
AudioOutPipe: TTS start error: dial tcp: lookup dashscope.aliyuncs.com: operation was canceled
```

## 根因分析

从日志可以看出，TTS 在连接阿里云 API 时 DNS 查询被取消了。可能的原因：

1. **Context 被提前取消**
   - AudioOutPipe 的 context 可能被 Orchestrator 或主程序的 context 取消了
   - 需要检查 context 的传递和取消逻辑

2. **Mixer 阻塞问题**
   - Mixer.Start() 可能阻塞了主 goroutine
   - PortAudio 的 stream.Start() 可能不是完全异步的

3. **网络问题**
   - DNS 服务器不可用
   - 网络连接问题
   - 防火墙阻止

## 修复方案

已在 `AudioOutPipe.PlayTTS()` 中增加以下处理：

- 为每次 TTS 连接创建独立的超时上下文（默认 10s），避免 DNS 解析被无意取消
- 对 DNS/网络类错误做一次轻量重试（300ms），减少瞬时抖动影响
- 失败时快速返回并记录 `ctx.Err()`，不阻塞对话流程

## 检查清单

- [x] AudioOutPipe 的 context 生命周期
- [x] Mixer.Start() 是否阻塞
- [ ] 网络 DNS 是否正常
- [ ] DASHSCOPE_API_KEY 是否正确

## 建议

1. 先跳过 TTS，验证其他功能是否正常
2. 检查 Mixer 的 Start() 是否应该放在 goroutine 中
3. 检查 context 的传递链路
4. 联系阿里云检查 API 状态
