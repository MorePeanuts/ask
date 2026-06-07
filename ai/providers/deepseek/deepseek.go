package deepseek

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"time"

	"github.com/MorePeanuts/ask/ai/callbacks"
	"github.com/MorePeanuts/ask/ai/components"
	"github.com/MorePeanuts/ask/ai/components/model"
	"github.com/MorePeanuts/ask/ai/schema"
	ds "github.com/cohesion-org/deepseek-go"
	"github.com/eino-contrib/jsonschema"
)

type ResponseFormatType string

const (
	ResponseFormatTypeText       = "text"
	ResponseFormatTypeJSONObject = "json_object"
	typ                          = "DeepSeek"
)

const (
	toolChoiceNone     = "none"     // none means the model will not call any tool and instead generates a message.
	toolChoiceAuto     = "auto"     // auto means the model can pick between generating a message or calling one or more tools.
	toolChoiceRequired = "required" // required means the model must call one or more tools.
)

const (
	roleAssistant = "assistant"
	roleSystem    = "system"
	roleUser      = "user"
	roleTool      = "tool"
)

type ChatModelConfig struct {
	// APIKey is your authentication key
	// Required
	APIKey string `json:"api_key"`

	// Timeout specifies the maximum duration to wait for API responses
	// Optional. Default: 5 minutes
	Timeout time.Duration `json:"timeout"`

	// BaseURL is your custom deepseek endpoint url
	// Optional. Default: https://api.deepseek.com/
	BaseURL string `json:"base_url"`

	// Path sets the path for the API request. Defaults to "chat/completions", if not set.
	// Example usages would be "/c/chat/" or any http after the baseURL extension
	Path string `json:"path"`

	// The following fields correspond to DeepSeek's chat API parameters
	// Ref: https://api-docs.deepseek.com/api/create-chat-completion

	// Model specifies the ID of the model to use
	// Required
	Model string `json:"model"`

	// MaxTokens limits the maximum number of tokens that can be generated in the chat completion
	// Optional. Default: 4096
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature specifies what sampling temperature to use
	// Generally recommend altering this or TopP but not both.
	// Optional. Default: 1.0
	Temperature float32 `json:"temperature,omitempty"`

	// TopP controls diversity via nucleus sampling
	// Generally recommend altering this or Temperature but not both.
	// Optional. Default: 1.0
	TopP float32 `json:"top_p,omitempty"`

	// StopWords sequences where the API will stop generating further tokens
	// Optional. Example: []string{"\n", "User:"}
	StopWords []string `json:"stop,omitempty"`

	// ResponseFormat specifies the format of the model's response
	// Optional. Use for structured outputs
	ResponseFormatType ResponseFormatType `json:"response_format_type,omitempty"`

	// ThinkingConfig controls the switch between thinking and non-thinking mode.
	// Possible values: enabled, disabled
	ThinkingConfig string `json:"thinking_config,omitempty"`
}

var _ model.ChatModelWithTools = (*ChatModel)(nil)

type ChatModel struct {
	cli  *ds.Client
	conf *ChatModelConfig

	tools      []ds.Tool
	rawTools   []*schema.ToolInfo
	toolChoice *schema.ToolChoice
}

func NewChatModel(config *ChatModelConfig) (*ChatModel, error) {
	var opts []ds.Option
	if config.Timeout > 0 {
		opts = append(opts, ds.WithTimeout(config.Timeout))
	}
	if len(config.BaseURL) > 0 {
		baseURL := config.BaseURL
		if !strings.HasSuffix(baseURL, "/") {
			baseURL = baseURL + "/"
		}
		opts = append(opts, ds.WithBaseURL(baseURL))
	}
	if len(config.Path) > 0 {
		opts = append(opts, ds.WithPath(config.Path))
	}

	cli, err := ds.NewClientWithOptions(config.APIKey, opts...)
	if err != nil {
		return nil, err
	}
	return &ChatModel{cli: cli, conf: config}, nil
}

func (cm *ChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (outMsg *schema.Message, err error) {
	ctx = callbacks.EnsureRunInfo(ctx, cm.GetType(), components.ComponentOfChatModel)

	req, cbInput, err := cm.createRequest(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate request: %w", err)
	}

	ctx = callbacks.OnStart(ctx, cbInput)
	defer func() {
		if err != nil {
			callbacks.OnError(ctx, err)
		}
	}()

	resp, err := cm.cli.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("received empty choices from DeepSeek API response")
	}

	for _, choice := range resp.Choices {
		if choice.Index != 0 {
			continue
		}

		outMsg = &schema.Message{
			Role:      toMessageRole(choice.Message.Role),
			Content:   choice.Message.Content,
			ToolCalls: toMessageToolCalls(choice.Message.ToolCalls),
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: choice.FinishReason,
				Usage:        toTokenUsage(&resp.Usage),
			},
		}
		if len(choice.Message.ReasoningContent) > 0 {
			outMsg.ReasoningContent = choice.Message.ReasoningContent
		}

		break
	}

	if outMsg == nil {
		return nil, fmt.Errorf("invalid response format: choice with index 0 not found")
	}

	callbacks.OnEnd(ctx, &model.CallbackOutput{
		Message:    outMsg,
		Config:     cbInput.Config,
		TokenUsage: outMsg.ResponseMeta.Usage,
	})

	return outMsg, nil
}

func (cm *ChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (outStream *schema.StreamReader[*schema.Message], err error) {
	ctx = callbacks.EnsureRunInfo(ctx, cm.GetType(), components.ComponentOfChatModel)

	req, cbInput, err := cm.createStreamRequest(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate request: %w", err)
	}

	ctx = callbacks.OnStart(ctx, cbInput)
	defer func() {
		if err != nil {
			callbacks.OnError(ctx, err)
		}
	}()

	stream, err := cm.cli.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat stream completion: %w", err)
	}

	sr, sw := schema.Pipe[*model.CallbackOutput](1)
	go func() {
		defer func() {
			panicErr := recover()
			_ = stream.Close()

			if panicErr != nil {
				_ = sw.Send(nil, newPanicErr(panicErr, debug.Stack()))
			}

			sw.Close()
		}()

		var lastEmptyMsg *schema.Message

		for {
			chunk, chunkErr := stream.Recv()
			if errors.Is(chunkErr, io.EOF) {
				if lastEmptyMsg != nil {
					sw.Send(&model.CallbackOutput{
						Message:    lastEmptyMsg,
						Config:     cbInput.Config,
						TokenUsage: lastEmptyMsg.ResponseMeta.Usage,
					}, nil)
				}
				return
			}

			if chunkErr != nil {
				_ = sw.Send(nil, fmt.Errorf("failed to receive stream chunk from DeepSeek: %w", chunkErr))
				return
			}

			msg, found, err := resolveStreamResponse(chunk)
			if err != nil {
				_ = sw.Send(nil, fmt.Errorf("failed to receive stream chunk from DeepSeek: %w", chunkErr))
			}
			if !found {
				continue
			}

			if lastEmptyMsg != nil {
				concatMsg, concatErr := schema.ConcatMessages([]*schema.Message{lastEmptyMsg, msg})
				if concatErr != nil {
					_ = sw.Send(nil, fmt.Errorf("failed to concatenate stream messages: %w", &concatErr))
					return
				}

				msg = concatMsg
			}

			if msg.Content == "" && len(msg.ToolCalls) == 0 && msg.ReasoningContent == "" {
				lastEmptyMsg = msg
				continue
			}

			lastEmptyMsg = nil

			closed := sw.Send(&model.CallbackOutput{
				Message:    msg,
				Config:     cbInput.Config,
				TokenUsage: msg.ResponseMeta.Usage,
			}, nil)

			if closed {
				return
			}
		}
	}()

	ctx, newSr := callbacks.OnEndWithStreamOutput(ctx, schema.StreamReaderWithConvert(sr,
		func(src *model.CallbackOutput) (callbacks.CallbackOutput, error) {
			return src, nil
		}))

	outStream = schema.StreamReaderWithConvert(newSr,
		func(src callbacks.CallbackOutput) (*schema.Message, error) {
			s := src.(*model.CallbackOutput)
			if s.Message == nil {
				return nil, schema.ErrNoValue
			}

			return s.Message, nil
		})

	return outStream, nil
}

func (cm *ChatModel) WithTools(tools []*schema.ToolInfo) (model.ChatModelWithTools, error) {
	return cm, nil
}

func (cm *ChatModel) GetType() string {
	return typ
}

func (cm *ChatModel) createRequest(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*ds.ChatCompletionRequest, *model.CallbackInput, error) {
	options := model.GetCommonOptions(&model.Options{
		ModelName:   &cm.conf.Model,
		Temperature: &cm.conf.Temperature,
		TopP:        &cm.conf.TopP,
		Tools:       nil,
		MaxTokens:   &cm.conf.MaxTokens,
		StopWords:   cm.conf.StopWords,
		ToolChoice:  cm.toolChoice,
	}, opts...)

	specOpts := model.GetImplSpecificOptions(&deepseekOptions{}, opts...)

	req := &ds.ChatCompletionRequest{
		Model:       *options.ModelName,
		MaxTokens:   derefOrZero(options.MaxTokens),
		Temperature: derefOrZero(options.Temperature),
		TopP:        derefOrZero(options.TopP),
		Stop:        options.StopWords,
		ExtraFields: specOpts.extraFields,
	}
	if cm.conf.ThinkingConfig != "" {
		req.Thinking = &ds.ThinkingConfig{Type: cm.conf.ThinkingConfig}
	}

	cbInput := &model.CallbackInput{
		Messages:   input,
		Tools:      cm.rawTools,
		ToolChoice: options.ToolChoice,
		Config: &model.Config{
			ModelName:   req.Model,
			MaxTokens:   req.MaxTokens,
			Temperature: req.Temperature,
			TopP:        req.TopP,
			StopWords:   req.Stop,
		},
	}

	tools := cm.tools
	if options.Tools != nil {
		var err error
		if tools, err = toDeepSeekTools(options.Tools); err != nil {
			return nil, nil, err
		}
		cbInput.Tools = options.Tools
	}

	req.Tools = make([]ds.Tool, len(tools))
	copy(req.Tools, tools)

	err := populateToolChoice(req, options.ToolChoice, options.AllowedToolNames)
	if err != nil {
		return nil, nil, err
	}
	msgs := make([]ds.ChatCompletionMessage, 0, len(input))
	for _, inMsg := range input {
		msg, err := toDeepSeekMessage(inMsg)
		if err != nil {
			return nil, nil, err
		}

		msgs = append(msgs, *msg)
	}

	req.Messages = msgs

	if len(cm.conf.ResponseFormatType) > 0 {
		req.ResponseFormat = &ds.ResponseFormat{
			Type: string(cm.conf.ResponseFormatType),
		}
	}

	return req, cbInput, nil
}

func (cm *ChatModel) createStreamRequest(ctx context.Context, input []*schema.Message, opts ...model.Option) (*ds.StreamChatCompletionRequest, *model.CallbackInput, error) {
	origReq, cbIn, err := cm.createRequest(ctx, input, opts...)
	if err != nil {
		return nil, nil, err
	}
	req := &ds.StreamChatCompletionRequest{
		Stream:         true,
		StreamOptions:  ds.StreamOptions{IncludeUsage: false},
		Model:          origReq.Model,
		Messages:       origReq.Messages,
		MaxTokens:      origReq.MaxTokens,
		Temperature:    origReq.Temperature,
		TopP:           origReq.TopP,
		ResponseFormat: origReq.ResponseFormat,
		Stop:           origReq.Stop,
		Tools:          origReq.Tools,
		ExtraFields:    origReq.ExtraFields,
		Thinking:       origReq.Thinking,
	}
	return req, cbIn, nil
}

func resolveStreamResponse(resp *ds.StreamChatCompletionResponse) (msg *schema.Message, found bool, err error) {
	for _, choice := range resp.Choices {
		// take 0 index as response, rewrite if needed
		if choice.Index != 0 {
			continue
		}

		if err != nil {
			return nil, false, fmt.Errorf("failed to extract log probs: %w", err)
		}
		found = true
		msg = &schema.Message{
			Role:      toMessageRole(choice.Delta.Role),
			Content:   choice.Delta.Content,
			ToolCalls: toMessageToolCalls(choice.Delta.ToolCalls),
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: choice.FinishReason,
				Usage:        streamToTokenUsage(resp.Usage),
			},
		}
		if len(choice.Delta.ReasoningContent) > 0 {
			msg.ReasoningContent = choice.Delta.ReasoningContent
		}

		break
	}

	if resp.Usage != nil && !found {
		msg = &schema.Message{
			ResponseMeta: &schema.ResponseMeta{
				Usage: streamToTokenUsage(resp.Usage),
			},
		}
		found = true
	}

	return msg, found, nil
}

func toDeepSeekMessage(m *schema.Message) (*ds.ChatCompletionMessage, error) {
	content := m.Content

	role := toDeepSeekMessageRole(m.Role)
	if len(role) == 0 {
		return nil, fmt.Errorf("unknown role type: %s", m.Role)
	}
	ret := &ds.ChatCompletionMessage{
		Role:    role,
		Content: content,
	}

	if m.ReasoningContent != "" {
		ret.ReasoningContent = m.ReasoningContent
	}

	if ret.Role == roleTool && m.ToolCallID != "" {
		ret.ToolCallID = m.ToolCallID
	}
	if ret.Role == roleAssistant && len(m.ToolCalls) > 0 {
		ret.ToolCalls = make([]ds.ToolCall, len(m.ToolCalls))
		for i, call := range m.ToolCalls {
			var index int
			if call.Index != nil {
				index = *call.Index
			}
			ret.ToolCalls[i] = ds.ToolCall{
				Index: index,
				ID:    call.ID,
				Type:  call.Type,
				Function: ds.ToolCallFunction{
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				},
			}
		}
	}

	return ret, nil
}

func toMessageRole(role string) schema.RoleType {
	switch role {
	case roleUser:
		return schema.User
	case roleAssistant:
		return schema.Assistant
	case roleSystem:
		return schema.System
	case roleTool:
		return schema.Tool
	default:
		return schema.RoleType(role)
	}
}

func toDeepSeekMessageRole(role schema.RoleType) string {
	switch role {
	case schema.User:
		return roleUser
	case schema.Assistant:
		return roleAssistant
	case schema.System:
		return roleSystem
	case schema.Tool:
		return roleTool
	default:
		return ""
	}
}

func toMessageToolCalls(toolCalls []ds.ToolCall) []schema.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	ret := make([]schema.ToolCall, len(toolCalls))
	for i, toolCall := range toolCalls {
		ret[i] = schema.ToolCall{
			Index: &toolCall.Index,
			ID:    toolCall.ID,
			Type:  toolCall.Type,
			Function: schema.FunctionCall{
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			},
		}
	}

	return ret
}

func toTokenUsage(usage *ds.Usage) *schema.TokenUsage {
	if usage == nil {
		return nil
	}
	return &schema.TokenUsage{
		PromptTokens:         usage.PromptTokens,
		PromptCacheHitTokens: usage.PromptCacheHitTokens,
		CompletionTokens:     usage.CompletionTokens,
		TotalTokens:          usage.TotalTokens,
		ReasoningTokens:      usage.CompletionTokensDetails.ReasoningTokens,
	}
}

func streamToTokenUsage(usage *ds.StreamUsage) *schema.TokenUsage {
	if usage == nil {
		return nil
	}
	if usage.PromptTokens == 0 &&
		usage.CompletionTokens == 0 &&
		usage.TotalTokens == 0 {
		return nil
	}
	return toTokenUsage(&ds.Usage{
		PromptTokens:            usage.PromptTokens,
		PromptCacheHitTokens:    usage.PromptCacheHitTokens,
		CompletionTokens:        usage.CompletionTokens,
		TotalTokens:             usage.TotalTokens,
		CompletionTokensDetails: usage.CompletionTokensDetails,
	})
}

func toDeepSeekTools(tis []*schema.ToolInfo) ([]ds.Tool, error) {
	tools := make([]ds.Tool, len(tis))
	for i, ti := range tis {
		if ti == nil {
			return nil, fmt.Errorf("tool info cannot be nil in BindTools")
		}

		paramsJSONSchema, err := ti.ToJSONSchema()
		if err != nil {
			return nil, fmt.Errorf("failed to convert tol parameters to JSONSchema: %w", err)
		}

		tools[i] = ds.Tool{
			Type: "function",
			Function: ds.Function{
				Name:        ti.Name,
				Description: ti.Desc,
				Parameters:  toDeepSeekToolParam(paramsJSONSchema),
			},
		}
	}

	return tools, nil
}

func toDeepSeekToolParam(sc *jsonschema.Schema) *ds.FunctionParameters {
	if sc == nil {
		return nil
	}
	ret := &ds.FunctionParameters{
		Type:       sc.Type,
		Properties: map[string]any{},
	}
	if len(sc.Required) > 0 {
		required := make([]string, len(sc.Required))
		copy(required, sc.Required)
		ret.Required = required
	}
	for pair := sc.Properties.Oldest(); pair != nil; pair = pair.Next() {
		ret.Properties[pair.Key] = pair.Value
	}
	return ret
}

func populateToolChoice(
	req *ds.ChatCompletionRequest,
	tc *schema.ToolChoice,
	allowedToolNames []string,
) error {
	if tc == nil {
		return nil
	}

	switch *tc {
	case schema.ToolChoiceForbidden:
		req.ToolChoice = toolChoiceNone
	case schema.ToolChoiceAllowed:
		req.ToolChoice = toolChoiceAuto
	case schema.ToolChoiceForced:
		if len(req.Tools) == 0 {
			return fmt.Errorf("tool choice is forced but tool is not provided")
		}

		onlyOneToolName := ""
		if len(allowedToolNames) > 0 {
			if len(allowedToolNames) > 1 {
				return fmt.Errorf("only one allowed tool name can be configured")
			}
			allowedToolName := allowedToolNames[0]
			toolsSet := make(map[string]bool, len(req.Tools))
			for _, t := range req.Tools {
				toolsSet[t.Function.Name] = true
			}
			if _, ok := toolsSet[allowedToolName]; !ok {
				return fmt.Errorf("allowed tool name '%s' not found in tools list", allowedToolName)
			}
			onlyOneToolName = allowedToolName
		} else if len(req.Tools) == 1 {
			onlyOneToolName = req.Tools[0].Function.Name
		}
		if onlyOneToolName != "" {
			req.ToolChoice = ds.ToolChoice{
				Type: "function",
				Function: ds.ToolChoiceFunction{
					Name: onlyOneToolName,
				},
			}
		} else {
			req.ToolChoice = toolChoiceRequired
		}

	default:
		return fmt.Errorf("tool choice:%s not support", *tc)
	}

	return nil
}

func derefOrZero[T any](v *T) T {
	if v == nil {
		var t T
		return t
	}

	return *v
}

type panicErr struct {
	info  any
	stack []byte
}

func (p *panicErr) Error() string {
	return fmt.Sprintf("panic error: %v, \nstack: %s", p.info, string(p.stack))
}

func newPanicErr(info any, stack []byte) error {
	return &panicErr{
		info:  info,
		stack: stack,
	}
}
