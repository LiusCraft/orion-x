# 修复 Ctrl+C 卡住问题

## 问题

用户按 Ctrl+C 退出程序时，程序会卡住无法退出。

## 根因分析

1. **AudioInPipe 停止流程问题**
   - `Stop()` 方法调用 `cancel()` 取消 context
   - 但音频读取协程可能卡在 `audioSource.Read()` 上
   - PortAudio 的 `Read()` 可能阻塞等待音频数据

2. **MicrophoneSource 关闭阻塞**
   - `stream.Stop()` 可能阻塞等待当前的 Read 操作完成
   - `stream.Close()` 也可能阻塞

3. **goroutine 等待死锁**
   - 主 goroutine 调用 `wg.Wait()` 等待音频读取协程退出
   - 但音频读取协程卡在 `Read()` 上无法退出
   - 形成死锁

## 修复方案

### 1. 改进 Stop 流程

```go
func (p *inPipeImpl) Stop() error {
    // 1. 设置停止状态
    p.state = InPipeStateStopping

    // 2. 先取消 context（让 SendAudio 返回错误）
    if p.cancel != nil {
        p.cancel()
    }

    // 3. 关闭音频源（让 Read() 返回错误，解除阻塞）
    if p.audioSource != nil {
        p.audioSource.Close()
    }

    // 4. 关闭 ASR
    if p.recognizer != nil {
        p.recognizer.Finish(p.ctx)
        p.recognizer.Close()
    }

    // 5. 等待 goroutine 退出
    p.wg.Wait()

    return nil
}
```

**关键点：**
- 先调用 `cancel()` 让正在进行的 `SendAudio()` 立即返回
- 再调用 `audioSource.Close()` 让 `Read()` 返回错误
- 最后等待 goroutine 退出

### 2. 优化 MicrophoneSource.Close()

```go
func (m *MicrophoneSource) Close() error {
    if err := m.stream.Stop(); err != nil {
        log.Printf("MicrophoneSource: error stopping stream: %v", err)
    }
    if err := m.stream.Close(); err != nil {
        log.Printf("MicrophoneSource: error closing stream: %v", err)
    }
    return nil
}
```

**关键点：**
- 移除了 `portaudio.Terminate()` 调用
- PortAudio 可能在多个组件中使用，不能在单一组件中终止
- 由程序在退出时统一终止

### 3. 改进信号处理

```go
go func() {
    <-sigCh
    // 立即停止 orchestrator
    orchestrator.Stop()
    cancel()
    os.Exit(0)
}()
```

**关键点：**
- 在收到信号后立即停止 orchestrator
- 然后调用 `os.Exit(0)` 强制退出
- 不等待优雅关闭

### 4. 添加详细日志

在停止过程中添加了详细日志：
- `AudioInPipe: stopping...`
- `AudioInPipe: canceling context...`
- `AudioInPipe: closing audio source...`
- `AudioInPipe: waiting for goroutines to finish...`
- `MicrophoneSource: closing...`

## 测试

```bash
# 构建程序
go build ./cmd/voicebot

# 运行程序
./voicebot

# 按 Ctrl+C 测试退出
```

预期行为：
- 程序应该在几秒内正常退出
- 不会卡住
- 日志输出完整

## 相关文件

- `internal/audio/inpipe_impl.go` - AudioInPipe 实现
- `internal/audio/microphone.go` - 麦克风音频源
- `cmd/voicebot/main.go` - 主程序
