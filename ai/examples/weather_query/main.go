package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/MorePeanuts/ask/ai/callbacks"
	"github.com/MorePeanuts/ask/ai/components"
	"github.com/MorePeanuts/ask/ai/components/model"
	"github.com/MorePeanuts/ask/ai/providers/deepseek"
	"github.com/MorePeanuts/ask/ai/schema"
)

const (
	weatherEndpoint = "http://wttr.in/%s?format=j1"
	maxAgentRounds  = 5
)

var weatherHTTPClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "请先设置 DEEPSEEK_API_KEY")
		os.Exit(1)
	}

	chatModel, err := deepseek.NewChatModel(&deepseek.ChatModelConfig{
		APIKey:         apiKey,
		Model:          "deepseek-v4-flash",
		ThinkingConfig: "enabled",
	})
	if err != nil {
		exitErr(err)
	}

	weatherTool := &schema.ToolInfo{
		Name: "get_weather",
		Desc: "查询指定城市的当前天气和摄氏温度",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"city": {
				Type:     schema.String,
				Desc:     "需要查询天气的城市名称，例如北京、上海",
				Required: true,
			},
		}),
	}

	ctx := callbacks.InitCallbacks(
		context.Background(),
		&callbacks.RunInfo{
			Name:      "weather-demo",
			Type:      "DeepSeek",
			Component: components.ComponentOfChatModel,
		},
		newLoggingCallback(),
	)

	messages := []*schema.Message{
		schema.UserMessage("请调用天气工具查询北京当前的天气，并用一句话告诉我适合穿什么衣服。"),
	}

	for round := range maxAgentRounds {
		message, err := chatModel.Generate(
			ctx,
			messages,
			model.WithTools([]*schema.ToolInfo{weatherTool}),
			model.WithToolChoice(schema.ToolChoiceAllowed),
		)
		if err != nil {
			exitErr(err)
		}
		messages = append(messages, message)

		if !continueAgentLoop(len(message.ToolCalls), round) {
			if len(message.ToolCalls) > 0 {
				exitErr(errors.New("达到最大工具调用轮数"))
			}
			fmt.Printf("\n[assistant] %s\n", message.Content)
			return
		}

		for _, call := range message.ToolCalls {
			result, err := executeToolCall(call)
			if err != nil {
				result = fmt.Sprintf("工具调用失败：%v", err)
			}
			fmt.Printf("[tool result] %s\n", result)
			messages = append(messages, schema.ToolMessage(result, call.Function.Name, call.ID))
		}
	}
}

func continueAgentLoop(toolCalls int, round int) bool {
	return toolCalls > 0 && round < maxAgentRounds-1
}

func executeToolCall(call schema.ToolCall) (string, error) {
	if call.Function.Name != "get_weather" {
		return "", fmt.Errorf("未知工具 %q", call.Function.Name)
	}

	city, err := parseWeatherArgs(call.Function.Arguments)
	if err != nil {
		return "", err
	}
	return getWeather(city), nil
}

func parseWeatherArgs(arguments string) (string, error) {
	var args struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("解析工具参数失败: %w", err)
	}
	args.City = strings.TrimSpace(args.City)
	if args.City == "" {
		return "", errors.New("工具参数 city 不能为空")
	}
	return args.City, nil
}

func getWeather(city string) string {
	weather, err := getWeatherWithClient(
		context.Background(),
		weatherHTTPClient,
		weatherEndpoint,
		city,
	)
	if err != nil {
		return fmt.Sprintf("查询失败:%v", err)
	}
	return weather
}

func getWeatherWithClient(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	city string,
) (string, error) {
	requestURL := fmt.Sprintf(endpoint, url.PathEscape(city))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("天气服务返回状态码 %d", res.StatusCode)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var obj struct {
		CurrentCondition []struct {
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
			TempC string `json:"temp_C"`
		} `json:"current_condition"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return "", fmt.Errorf("解析天气响应失败: %w", err)
	}
	if len(obj.CurrentCondition) == 0 || len(obj.CurrentCondition[0].WeatherDesc) == 0 {
		return "", errors.New("天气响应缺少当前天气信息")
	}

	condition := obj.CurrentCondition[0]
	return fmt.Sprintf(
		"%s当前天气：%s，气温%s摄氏度",
		city,
		condition.WeatherDesc[0].Value,
		condition.TempC,
	), nil
}

func newLoggingCallback() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			modelInput := model.ConvCallbackInput(input)
			if modelInput == nil {
				fmt.Printf("[callback:start] %s\n", runName(info))
				return ctx
			}
			fmt.Printf(
				"[callback:start] %s messages=%d tools=%d\n",
				runName(info),
				len(modelInput.Messages),
				len(modelInput.Tools),
			)
			fmt.Println(modelInput.Messages[len(modelInput.Messages)-1])
			return ctx
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			modelOutput := model.ConvCallbackOutput(output)
			if modelOutput == nil || modelOutput.TokenUsage == nil {
				fmt.Printf("[callback:end] %s\n", runName(info))
				return ctx
			}
			fmt.Printf(
				"[callback:end] %s tokens=%d reasoning=%t\n",
				runName(info),
				modelOutput.TokenUsage.TotalTokens,
				modelOutput.Message != nil && modelOutput.Message.ReasoningContent != "",
			)
			fmt.Println(modelOutput.Message)
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			fmt.Printf("[callback:error] %s error=%v\n", runName(info), err)
			return ctx
		}).
		Build()
}

func runName(info *callbacks.RunInfo) string {
	if info == nil {
		return "unknown"
	}
	if info.Name != "" {
		return info.Name
	}
	return info.Type
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
