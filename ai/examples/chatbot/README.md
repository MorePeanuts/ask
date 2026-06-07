# Streaming Chatbot

该示例使用真实 DeepSeek API 演示：

- 流式输出 reasoning 和最终回答；
- 保留上下文的多轮对话；
- 启动时或对话中开启、关闭思考模式；
- 在流结束 callback 中输出 token 用量。

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
