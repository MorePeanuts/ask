# Streaming Chatbot

该示例使用真实 DeepSeek API 演示：

- 流式输出 reasoning 和最终回答；
- 保留上下文的多轮对话；
- 启动时或对话中开启、关闭思考模式；
- 通过流式输出 callback 的独立流副本读取并输出 token 用量。

```bash
export DEEPSEEK_API_KEY="your-api-key"

# 默认开启思考模式
GOWORK=off go run ./examples/chatbot

# 启动时关闭思考模式
GOWORK=off go run ./examples/chatbot -thinking=false
```

对话命令：

- `/thinking on`：开启思考模式；
- `/thinking off`：关闭思考模式；
- `/exit`：退出。

## 流式 callback 设计

`OnEndWithStreamOutput` 中的 `End` 表示模型的 `Stream` 方法已经生成输出流，
到达了组件调用的结束边界。它不表示输出流已经消费完毕，也不表示流已经到达
`io.EOF`。

框架调用 `OnEndWithStreamOutput` 时，会为每个 callback handler 和调用方分别
创建独立的流副本。调用顺序可以简化为：

```text
models.Stream()
  -> 创建输出流
  -> 复制输出流
  -> 调用 OnEndWithStreamOutput handler，传入 handler 的流副本
  -> 返回调用方的流副本
```

### 为什么 handler 中需要启动 goroutine

handler 方法返回后，`models.Stream()` 才能将调用方的流副本返回给主流程。
如果 handler 在方法内同步读取完整个流，它会一直等待 `io.EOF`，此时主流程还
无法开始调用 `consumeResponseStream`。这样会阻止主流程实时输出响应，并可能
造成流生产和分发过程阻塞。

因此，handler 启动 goroutine 消费自己的流副本，然后立即返回：

```go
OnEndWithStreamOutputFn(func(
    ctx context.Context,
    _ *callbacks.RunInfo,
    output *schema.StreamReader[callbacks.CallbackOutput],
) context.Context {
    go func() {
        defer output.Close()
        // 消费 handler 的流副本并记录 token 用量。
    }()
    return ctx
})
```

主流程和 callback goroutine 随后会并发消费各自的流副本。

### 为什么主流程需要调用 wait

goroutine 解决的是流副本必须并发消费的问题，`reporter.wait()` 解决的是本示例
对输出顺序的要求，两者职责不同。

即使主流程已经从自己的流副本读到 `io.EOF`，callback goroutine 仍可能尚未保存
最后一个包含 token 用量的 chunk。主流程在每轮响应后调用 `reporter.wait()`，
可以确保：

- token 用量读取完成后再打印；
- 当前轮的 token 用量不会与下一轮的 `you>` 提示交错；
- 下一轮请求开始前，`usageReporter` 中的状态已经被读取并清理。

`wait()` 不是用于触发 callback；callback 已经由框架自动调用。它只等待 handler
内部启动的异步任务完成。

如果 handler 只是异步上报日志或指标，并且应用不关心完成时间和输出顺序，
主流程可以不等待它。当前示例显式等待，是因为它需要在每轮回答后立即、有序地
展示对应的 token 用量。
