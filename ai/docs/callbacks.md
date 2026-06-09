# Callbacks 系统设计

Callbacks 系统为组件调用提供统一的生命周期扩展点，主要用于日志、指标、
Tracing、输入输出记录和 token 用量统计等横切逻辑。

本文介绍 callbacks 系统的公开接口、内部调用流程、流式回调设计、并发约束和
常见问题。

## 核心设计

Callbacks 系统由以下部分组成：

- `Handler`：定义组件生命周期中可以处理的事件。
- `HandlerBuilder`：只注册需要处理的事件，简化 `Handler` 创建。
- `RunInfo`：描述当前触发 callback 的组件。
- `manager`：保存 `RunInfo`、局部 handlers 和全局 handlers，并通过
  `context.Context` 在调用链中传播。
- `OnStart`、`OnEnd`、`OnError`：处理普通输入输出的生命周期事件。
- `OnStartWithStreamInput`、`OnEndWithStreamOutput`：处理流式输入输出的
  生命周期事件。
- `TimingChecker`：让 handler 声明自己需要处理哪些事件。

`Start` 和 `End` 描述的是**组件方法调用边界**，不是流的生命周期：

```text
普通调用：

OnStart -> 组件处理 -> OnEnd
                    \-> OnError

返回流的调用：

OnStart -> 组件创建输出流 -> OnEndWithStreamOutput -> 返回输出流
                                      |
                                      +-> 输出流此时只是可用，尚未到达 EOF
```

## Handler 与事件类型

`Handler` 可以处理五种事件：

| 事件 | 调用时机 | 数据 |
| --- | --- | --- |
| `OnStart` | 组件开始处理普通输入之前 | 普通输入 |
| `OnEnd` | 组件成功产生普通输出之后 | 普通输出 |
| `OnError` | 组件方法返回错误时 | 错误 |
| `OnStartWithStreamInput` | 流式输入在组件入口可用时 | 输入流副本 |
| `OnEndWithStreamOutput` | 流式输出在组件出口可用时 | 输出流副本 |

同一次组件调用应根据输入输出形态选择对应事件：

- 普通输入使用 `OnStart`。
- 流式输入使用 `OnStartWithStreamInput`，不要再调用 `OnStart`。
- 普通成功输出使用 `OnEnd`。
- 流式成功输出使用 `OnEndWithStreamOutput`，不要再调用 `OnEnd`。
- 组件方法返回错误时使用 `OnError`。

`OnError` 只处理组件方法返回前发生的错误。组件已经返回流之后，读取流时发生的
错误会由 `StreamReader.Recv` 返回，不会自动触发 `OnError`。

## 注册 Handler

应用可以使用 `HandlerBuilder` 注册自己关心的事件：

```go
handler := callbacks.NewHandlerBuilder().
    OnStartFn(func(
        ctx context.Context,
        info *callbacks.RunInfo,
        input callbacks.CallbackInput,
    ) context.Context {
        log.Printf("start: %s", info.Name)
        return ctx
    }).
    OnEndFn(func(
        ctx context.Context,
        info *callbacks.RunInfo,
        output callbacks.CallbackOutput,
    ) context.Context {
        log.Printf("end: %s", info.Name)
        return ctx
    }).
    OnErrorFn(func(
        ctx context.Context,
        info *callbacks.RunInfo,
        err error,
    ) context.Context {
        log.Printf("error: %s: %v", info.Name, err)
        return ctx
    }).
    Build()
```

`handlerImpl` 同时实现了 `TimingChecker`。`Needed` 会根据 builder 中是否设置了
对应函数过滤事件，因此没有注册的 callback 不会被调用。

## Manager 与 Context 传播

Callbacks manager 被保存在 `context.Context` 中，包含：

```go
type manager struct {
    globalHandlers []Handler
    handlers       []Handler
    runInfo        *RunInfo
}
```

### InitCallbacks

`InitCallbacks` 创建新的 manager，并完全替换 context 中已有的 `RunInfo` 和
handlers：

```go
ctx = callbacks.InitCallbacks(
    context.Background(),
    &callbacks.RunInfo{
        Name:      "chatbot",
        Type:      "DeepSeek",
        Component: components.ComponentOfChatModel,
    },
    handler,
)
```

创建 manager 时，会复制当时的 `GlobalHandlers`。之后修改 `GlobalHandlers`
不会改变已经创建的 manager。

### ReuseHandlers

当一个组件内部调用另一个组件，并希望复用已有 handlers，但使用内部组件自己的
`RunInfo` 时，应使用 `ReuseHandlers`：

```go
innerCtx := callbacks.ReuseHandlers(ctx, innerRunInfo)
```

### EnsureRunInfo

组件实现通常在公开方法入口调用 `EnsureRunInfo`。如果调用方已经提供了
`RunInfo`，它会保持不变；否则会根据组件类型和类别补充默认信息。

```go
ctx = callbacks.EnsureRunInfo(
    ctx,
    model.GetType(),
    components.ComponentOfChatModel,
)
```

### 为什么必须传播返回的 Context

`OnStart` 和 `OnStartWithStreamInput` 会把当前 `RunInfo` 移入返回的 context，
后续 `OnEnd`、`OnEndWithStreamOutput` 和 `OnError` 需要使用该 context 才能将
事件关联到同一次调用。

因此组件必须保存并传播返回值：

```go
ctx = callbacks.OnStart(ctx, input)
callbacks.OnEnd(ctx, output)
```

不要忽略 `OnStart` 返回的 context。

## Handler 调用顺序

Start 事件按注册顺序的逆序执行，End 和 Error 事件按注册顺序执行：

```text
注册顺序：A, B, C

开始事件：C -> B -> A
结束事件：A -> B -> C
错误事件：A -> B -> C
```

这种顺序与中间件的嵌套模型一致：

```text
A(B(C(component)))
```

`OnStartWithStreamInput` 与 `OnStart` 一样逆序执行；
`OnEndWithStreamOutput` 与 `OnEnd` 一样正序执行。

每个 handler 返回的 context 会传给下一个 handler，因此 handler 可以向 context
写入 tracing span 等数据。handler 不应丢弃前一个 handler 返回的 context。

## 普通组件调用

普通同步组件的典型实现：

```go
func (m *Model) Generate(
    ctx context.Context,
    input []*schema.Message,
) (out *schema.Message, err error) {
    ctx = callbacks.EnsureRunInfo(
        ctx,
        m.GetType(),
        components.ComponentOfChatModel,
    )

    cbInput := &model.CallbackInput{Messages: input}
    ctx = callbacks.OnStart(ctx, cbInput)

    defer func() {
        if err != nil {
            callbacks.OnError(ctx, err)
        }
    }()

    out, err = m.generate(ctx, input)
    if err != nil {
        return nil, err
    }

    callbacks.OnEnd(ctx, &model.CallbackOutput{Message: out})
    return out, nil
}
```

普通 handler 是同步调用的。`OnStart`、`OnEnd` 或 `OnError` 返回时，对应 handler
方法已经执行完毕。

## 流式生命周期

### OnStartWithStreamInput

`OnStartWithStreamInput` 用于组件接收流式输入的场景，例如 Collect 或
Transform：

```text
输入流可用
  -> OnStartWithStreamInput
  -> 为 handlers 和组件复制 reader
  -> handlers 获得各自的输入流副本
  -> 组件获得一个新的输入流副本并开始处理
```

它表示组件调用的 Start 边界，不表示：

- 输入流已经产生第一个 chunk；
- handler 或组件已经开始调用 `Recv`；
- 输入流已经消费完成。

### OnEndWithStreamOutput

`OnEndWithStreamOutput` 用于组件产生流式输出的场景，例如 Stream 或
Transform：

```text
组件创建输出流
  -> OnEndWithStreamOutput
  -> 为 handlers 和调用方复制 reader
  -> handlers 获得各自的输出流副本
  -> 调用方获得一个新的输出流副本
```

它表示组件调用的 End 边界，也就是输出流已经可以返回给调用方，但不表示：

- 输出流已经产生第一个 chunk；
- 输出流已经消费完成；
- 输出流已经关闭或到达 `io.EOF`。

## 为什么流式 Handler 需要独立副本

`StreamReader` 是一次性消费的 reader。如果多个消费者直接读取同一个 reader，
它们会争抢 chunk：

```text
原始流：A -> B -> C -> EOF

handler A 读取 A
handler B 读取 B
调用方    读取 C
```

这不能满足日志、指标和调用方都观察完整输出的需求。因此流式 callback 会调用：

```go
copies := stream.Copy(len(handlers) + 1)
```

每个 handler 获得一个副本，最后一个副本返回给组件或调用方：

```text
原始流
  ├─ handler A：A -> B -> C -> EOF
  ├─ handler B：A -> B -> C -> EOF
  └─ 调用方：   A -> B -> C -> EOF
```

## StreamReader.Copy 的实现原理

`Copy` 不是立即把所有数据读入内存并复制多份。它会创建一个共享的
`parentStreamReader` 和多个 `childStreamReader`：

```text
                   原始 StreamReader
                          |
                 parentStreamReader
                          |
              cpStreamElement 链表
                 /        |        \
             child 0   child 1   child 2
```

每个 `cpStreamElement` 表示一个 chunk，使用 `sync.Once` 保证原始流中对应的
chunk 只读取一次：

```go
elem.once.Do(func() {
    chunk, err := parent.sr.Recv()
    elem.item = streamItem{chunk: chunk, err: err}
    elem.next = &cpStreamElement{}
})
```

第一个到达某个节点的 child 从原始流读取数据，其他 child 到达相同节点时读取
已经保存的结果。因此每个 child 都能看到相同的 chunk 序列，同时可以按照自己的
速度消费。

这种实现有以下性质：

- 原始流中的每个 chunk 只会被读取一次。
- 每个副本都能观察到相同的 chunk 和错误序列。
- 不同副本可以并发消费。
- 慢速副本会使尚未消费的链表节点继续保留，增加内存占用。
- 每个副本必须调用 `Close`；所有 child 关闭后，原始 reader 才会关闭。
- `Copy` 后原始 reader 不应再被使用。

`Copy` 复制的是 chunk 值，不保证对 chunk 做深拷贝。如果 chunk 是指针、map、
slice 或包含共享可变状态，handler 修改内容可能影响其他消费者。Callback
handler 应把收到的数据视为只读数据。

## 流副本不会重新聚合

Callback 的流副本是旁路观察分支，不是转换流水线：

```text
                              ┌─ handler A：日志
原始输出流 -> Copy(N + 1) ----├─ handler B：指标
                              └─ 调用方：真正输出
```

框架不会等待 handlers 处理完各自的副本，也不会把处理结果重新聚合。最终输出
直接来自预留给调用方的最后一个副本：

```go
inOuts := copy(len(handlers) + 1)

for i, handler := range handlers {
    handle(handler, inOuts[i])
}

return inOuts[len(inOuts)-1]
```

如果业务确实需要把多个不同来源的流合并为一个输出，应显式使用
`schema.MergeStreamReaders` 或 `schema.MergeNamedStreamReaders`。这与 callback
的 `Copy` 机制不同。

## 为什么流式 Handler 通常需要启动 Goroutine

框架会同步调用 handler 方法，只有 handler 返回后，组件或调用方才能获得剩余的
流副本：

```text
创建输出流
  -> 调用 handler
  -> handler 返回
  -> 返回调用方流副本
```

如果 handler 在方法内同步读取完整流：

```go
OnEndWithStreamOutputFn(func(
    ctx context.Context,
    info *callbacks.RunInfo,
    output *schema.StreamReader[callbacks.CallbackOutput],
) context.Context {
    defer output.Close()
    for {
        _, err := output.Recv()
        if errors.Is(err, io.EOF) {
            break
        }
    }
    return ctx
})
```

handler 会一直等待 EOF，而调用方此时还拿不到自己的流副本。这会使流式接口
退化为等待完整响应后返回，也可能因上游缓冲或分发过程受限而阻塞。

因此流式 handler 通常应启动 goroutine 消费副本，并立即返回：

```go
OnEndWithStreamOutputFn(func(
    ctx context.Context,
    info *callbacks.RunInfo,
    output *schema.StreamReader[callbacks.CallbackOutput],
) context.Context {
    go func() {
        defer output.Close()
        consume(output)
    }()
    return ctx
})
```

之后 handler goroutine 与组件或调用方并发消费各自的副本。

## 为什么某些应用仍然需要 Wait

框架会自动调用 handler，但不会等待 handler 自己启动的异步任务完成。

例如 chatbot 示例需要在每轮响应后立即、有序地打印该轮 token 用量。即使主流程
已经从自己的副本读到 EOF，handler goroutine 仍可能尚未处理最后一个包含 token
用量的 chunk。因此示例使用 `WaitGroup`：

```text
主流程消费调用方副本到 EOF
  -> reporter.wait()
  -> 确认 handler goroutine 已保存 token 用量
  -> 打印当前轮用量
  -> 显示下一轮输入提示
```

`wait()` 不会触发 handler。它只等待 handler 内部已经启动的异步任务。

以下场景通常不需要调用方等待：

- handler 只异步上传日志、指标或 tracing 数据；
- 应用不关心 handler 的完成时间；
- handler 自己负责输出，并允许输出与主流程交错。

以下场景通常需要应用层同步：

- 当前请求返回前必须确认指标已经提交；
- 下一轮逻辑需要使用 handler 的计算结果；
- 必须保证 callback 输出和主流程输出的顺序。

当前 callbacks API 没有提供统一的异步任务 completion handle，因此是否等待以及
如何等待由 handler 和应用根据业务需求决定。

## 错误与资源管理

### 每个流副本都必须关闭

Handler 拥有自己收到的流副本，必须保证在所有路径中关闭：

```go
go func() {
    defer output.Close()

    for {
        item, err := output.Recv()
        if errors.Is(err, io.EOF) {
            return
        }
        if err != nil {
            recordError(err)
            return
        }
        record(item)
    }
}()
```

即使读到 EOF，也仍然需要调用 `Close`。

### 慢速或停止消费的 Handler

流复制实现允许每个副本按照不同速度读取，但慢速 handler 会使未消费节点继续
保留。长期不消费且不关闭副本的 handler 可能导致内存持续增长和资源无法释放。

### 不要在 Handler 中修改 Chunk

流副本保证独立的读取位置，不保证 chunk 对象深拷贝。Handler 应观察数据，不应
修改共享对象。如果需要修改，应先自行深拷贝。

### Context 取消

Handler 可以读取 `ctx.Done()` 并在取消后停止异步任务，但无论如何都应关闭自己
持有的流副本。框架不会自动等待或强制终止 handler 启动的 goroutine。

## 常见问题

### 添加 Handler 后，框架会自动调用它吗？

会。Handler 注册到 callbacks context 后，组件调用对应的 `OnStart`、`OnEnd`、
`OnError` 或流式事件时，框架会自动筛选并调用 handler。

应用层的 `wait()` 不是用于调用 handler，而是等待 handler 自己启动的异步工作。

### `OnEndWithStreamOutput` 在什么时候调用？

它在组件已经创建输出流、准备把流返回给调用方时调用。这里的 `End` 表示组件
方法到达返回边界，不表示输出流已经到达 EOF。

### `OnStartWithStreamInput` 在什么时候调用？

它在组件入口已经获得输入流、但组件尚未开始消费自己的流副本时调用。它不表示
输入流已经产生第一个 chunk，也不表示 handler 已经开始消费。

### 为什么不把 `OnEndWithStreamOutput` 设计成真正的 EOF callback？

组件方法返回输出流时，流通常尚未消费。框架必须先把 reader 返回给调用方，
调用方才可以开始读取。真正的 EOF 发生在组件方法返回之后，属于流生命周期，
而不是组件方法调用边界。

如果需要在 EOF 时执行逻辑，handler 应消费自己的流副本并在 `Recv` 返回
`io.EOF` 时处理；也可以在具体流水线中使用支持 EOF hook 的 reader 包装。

### 为什么不能让框架自动等待流式 Handler？

如果框架在返回调用方 reader 前等待 handler 消费完整流，调用方无法并发消费，
流式接口会退化，甚至可能阻塞。框架也无法判断异步日志、指标等 handler 是否
真的需要调用方等待。

### 为什么每个 Handler 都需要一个流副本？

因为 `StreamReader` 是一次性消费的。如果共享同一个 reader，不同消费者会争抢
chunk，无法分别观察完整流。

### 流副本是深拷贝吗？

不是。每个副本有独立的读取进度，但 chunk 值本身不保证深拷贝。Handler 应将
chunk 视为只读。

### Handler 的调用顺序是什么？

Start 类事件逆序调用，End 和 Error 类事件正序调用。这个顺序模拟中间件的嵌套
进入和退出过程。

### 可以同时调用 `OnStart` 和 `OnStartWithStreamInput` 吗？

不应对同一次组件调用同时调用。两者都是 Start 边界事件，只是分别处理普通输入
和流式输入。对应地，`OnEnd` 和 `OnEndWithStreamOutput` 也是二选一。

### 全局 Handler 何时生效？

`GlobalHandlers` 在创建 manager 时被复制到 manager 中。之后添加或删除全局
handler 不会影响已经创建的 callbacks context。

## 设计边界

当前 callbacks 系统有意保持以下边界：

- Callback 观察组件调用，不负责改变组件业务输出。
- 流式 callback 提供旁路流副本，不负责重新聚合。
- 框架负责调用 handler，但不负责管理 handler 启动的异步任务。
- 框架不把流消费阶段的错误自动转换为组件方法的 `OnError`。
- 应用负责根据输出顺序、可靠性和关闭时机决定是否等待异步 handler。

这些边界使 callbacks 可以用于日志、指标和 tracing，而不会把组件业务执行与
观察逻辑强制绑定。
