package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/MorePeanuts/ask/ai/callbacks"
	"github.com/MorePeanuts/ask/ai/components"
	"github.com/MorePeanuts/ask/ai/components/model"
	"github.com/MorePeanuts/ask/ai/providers/deepseek"
	"github.com/MorePeanuts/ask/ai/schema"
)

type inputAction uint8

const (
	actionIgnore inputAction = iota
	actionMessage
	actionSwitchThinking
	actionExit
)

func main() {
	thinking := flag.Bool("thinking", true, "启动时是否开启思考模式")
	flag.Parse()

	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if apiKey == "" {
		exitErr(errors.New("请先设置 DEEPSEEK_API_KEY"))
	}

	models, err := newChatModels(apiKey)
	if err != nil {
		exitErr(err)
	}

	reporter := newUsageReporter(os.Stdout)
	ctx := callbacks.InitCallbacks(
		context.Background(),
		&callbacks.RunInfo{
			Name:      "chatbot",
			Type:      "DeepSeek",
			Component: components.ComponentOfChatModel,
		},
		reporter.handler(),
	)

	fmt.Println("流式多轮 Chatbot")
	fmt.Println("命令：/thinking on、/thinking off、/exit")
	fmt.Printf("当前思考模式：%t\n\n", *thinking)

	var messages []*schema.Message
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("you> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				exitErr(err)
			}
			fmt.Println()
			return
		}

		action, nextThinking, content := handleInput(scanner.Text(), *thinking)
		switch action {
		case actionIgnore:
			continue
		case actionExit:
			return
		case actionSwitchThinking:
			*thinking = nextThinking
			fmt.Printf("思考模式：%t\n", *thinking)
			continue
		case actionMessage:
		}

		messages = append(messages, schema.UserMessage(content))
		stream, err := models[*thinking].Stream(
			ctx,
			messages,
			deepseek.WithExtraFields(map[string]any{
				"stream_options": map[string]any{"include_usage": true},
			}),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "请求失败：%v\n", err)
			messages = messages[:len(messages)-1]
			continue
		}

		response, err := consumeResponseStream(os.Stdout, stream)
		reporter.wait()
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取响应失败：%v\n", err)
			messages = messages[:len(messages)-1]
			continue
		}
		messages = append(messages, response)
	}
}

func newChatModels(apiKey string) (map[bool]*deepseek.ChatModel, error) {
	models := make(map[bool]*deepseek.ChatModel, 2)
	for _, thinking := range []bool{false, true} {
		thinkingConfig := "disabled"
		if thinking {
			thinkingConfig = "enabled"
		}
		chatModel, err := deepseek.NewChatModel(&deepseek.ChatModelConfig{
			APIKey:         apiKey,
			Model:          "deepseek-v4-flash",
			ThinkingConfig: thinkingConfig,
		})
		if err != nil {
			return nil, err
		}
		models[thinking] = chatModel
	}
	return models, nil
}

func handleInput(input string, thinking bool) (inputAction, bool, string) {
	input = strings.TrimSpace(input)
	switch input {
	case "":
		return actionIgnore, thinking, ""
	case "/exit", "/quit":
		return actionExit, thinking, ""
	case "/thinking on":
		return actionSwitchThinking, true, ""
	case "/thinking off":
		return actionSwitchThinking, false, ""
	default:
		return actionMessage, thinking, input
	}
}

func consumeResponseStream(w io.Writer, stream *schema.StreamReader[*schema.Message]) (*schema.Message, error) {
	defer stream.Close()

	var chunks []*schema.Message
	printedReasoning := false
	printedContent := false
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)

		if chunk.ReasoningContent != "" {
			if !printedReasoning {
				fmt.Fprint(w, "thinking> ")
				printedReasoning = true
			}
			fmt.Fprint(w, chunk.ReasoningContent)
		}
		if chunk.Content != "" {
			if !printedContent {
				if printedReasoning {
					fmt.Fprintln(w)
				}
				fmt.Fprint(w, "assistant> ")
				printedContent = true
			}
			fmt.Fprint(w, chunk.Content)
		}
	}
	fmt.Fprintln(w)

	return concatStreamChunks(chunks)
}

func concatStreamChunks(chunks []*schema.Message) (*schema.Message, error) {
	if len(chunks) == 0 {
		return nil, errors.New("模型返回了空响应")
	}
	return schema.ConcatMessages(chunks)
}

type usageReporter struct {
	out   io.Writer
	wg    sync.WaitGroup
	mu    sync.Mutex
	usage *schema.TokenUsage
	err   error
}

func newUsageReporter(out io.Writer) *usageReporter {
	return &usageReporter{out: out}
}

func (r *usageReporter) handler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnEndWithStreamOutputFn(func(
			ctx context.Context,
			_ *callbacks.RunInfo,
			output *schema.StreamReader[callbacks.CallbackOutput],
		) context.Context {
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				defer output.Close()

				var usage *schema.TokenUsage
				for {
					item, err := output.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						r.mu.Lock()
						r.err = err
						r.mu.Unlock()
						return
					}
					modelOutput := model.ConvCallbackOutput(item)
					if modelOutput != nil && modelOutput.TokenUsage != nil {
						usage = modelOutput.TokenUsage
					}
				}
				r.mu.Lock()
				r.usage = usage
				r.mu.Unlock()
			}()
			return ctx
		}).
		Build()
}

func (r *usageReporter) wait() {
	r.wg.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		fmt.Fprintf(r.out, "[callback:end] 读取 token 用量失败：%v\n", r.err)
		r.err = nil
		return
	}
	if r.usage == nil {
		fmt.Fprintln(r.out, "[callback:end] token usage unavailable")
		return
	}
	fmt.Fprintf(
		r.out,
		"[callback:end] prompt=%d completion=%d reasoning=%d total=%d cache_hit=%d\n",
		r.usage.PromptTokens,
		r.usage.CompletionTokens,
		r.usage.ReasoningTokens,
		r.usage.TotalTokens,
		r.usage.PromptCacheHitTokens,
	)
	r.usage = nil
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
