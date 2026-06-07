# Weather Tool With Callback

该示例使用真实 DeepSeek API，让模型在思考模式下自动调用 `get_weather` 工具查询北京天气，
并通过 callback 输出每次模型调用的开始、结束、错误、token 用量和是否包含 reasoning。

```bash
export DEEPSEEK_API_KEY="your-api-key"
GOWORK=off go run ./examples/weather_callback
```

示例流程：

1. 模型在思考模式下自主判断是否调用天气工具。
2. 本地调用 `wttr.in` 并将天气结果作为工具消息加入上下文。
3. 持续自动调用模型，直到模型生成最终回答或达到最大轮数。
