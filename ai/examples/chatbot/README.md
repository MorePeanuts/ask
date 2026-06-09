# Streaming Chatbot

该示例使用真实 DeepSeek API 演示：

- 流式输出 reasoning 和最终回答；
- 保留上下文的多轮对话；
- 启动时或对话中开启、关闭思考模式；
- 通过流式输出 callback 的独立流副本读取并输出 token 用量。

这里的流式输出 callback 在模型返回输出流时触发，而不是在输出流到达
`io.EOF` 时触发。callback 会并发消费自己的流副本；主流程消费完调用方的
流副本后，等待 callback 完成 token 用量统计。

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
