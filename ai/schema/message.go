package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MorePeanuts/ask/ai/utils"
)

type RoleType string

const (
	Assistant RoleType = "assistant"
	User      RoleType = "user"
	System    RoleType = "system"
	Tool      RoleType = "tool"
)

type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ToolCall struct {
	// Index is used when there are multiple tool calls in a message.
	// In stream mode, it's used to identify the chunk of the tool call for merging.
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type TokenUsage struct {
	PromptTokens         int `json:"prompt_tokens"`
	PromptCacheHitTokens int `json:"prompt_token_hit_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	TotalTokens          int `json:"total_tokens"`
	ReasoningTokens      int `json:"reasoning_tokens,omitempty"`
}

type ResponseMeta struct {
	// FinishReason is the reason why the chat response is finished.
	// It's usually "stop", "length", "tool_calls", "content_filter", "null".
	// This is defined by chat model implementation.
	FinishReason string      `json:"finish_reason,omitempty"`
	Usage        *TokenUsage `json:"usage,omitempty"`
}

type Message struct {
	Role    RoleType `json:"role"`
	Content string   `json:"content"`

	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ResponseMeta     *ResponseMeta `json:"response_meta,omitempty"`

	// only for AssistantMessage
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// only for ToolMessage
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`

	Extra map[string]any `json:"extra,omitempty"`
}

// String returns the string representation of the message.
// e.g.
//
//	msg := schema.UserMessage("hello world")
//	fmt.Println(msg.String()) // Output will be: `user: hello world``
//
//	msg := schema.Message{
//		Role:    schema.Tool,
//		Content: "{...}",
//		ToolCallID: "callxxxx"
//	}
//	fmt.Println(msg.String())
//	Output will be:
//		tool: {...}
//		call_id: callxxxx
func (m *Message) String() string {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "%s: %s", m.Role, m.Content)

	if len(m.ReasoningContent) > 0 {
		sb.WriteString("\nreasoning_content:\n")
		sb.WriteString(m.ReasoningContent)
	}

	if len(m.ToolCalls) > 0 {
		sb.WriteString("\ntool_calls:\n")
		for _, tc := range m.ToolCalls {
			if tc.Index != nil {
				fmt.Fprintf(sb, "index[%d]:", *tc.Index)
			}
			fmt.Fprintf(sb, "%+v\n", tc)
		}
	}

	if m.ToolCallID != "" {
		fmt.Fprintf(sb, "\ntool_call_id: %s", m.ToolCallID)
	}

	if m.ToolName != "" {
		fmt.Fprintf(sb, "\ntool_call_name: %s", m.ToolName)
	}

	if m.ResponseMeta != nil {
		fmt.Fprintf(sb, "\nfinish_reason: %s", m.ResponseMeta.FinishReason)
		if m.ResponseMeta.Usage != nil {
			fmt.Fprintf(sb, "\nusage: %v", m.ResponseMeta.Usage)
		}
	}

	return sb.String()
}

func SystemMessage(content string) *Message {
	return &Message{
		Role:    Assistant,
		Content: content,
	}
}

func AssistantMessage(content string, toolCalls []ToolCall) *Message {
	return &Message{
		Role:      Assistant,
		Content:   content,
		ToolCalls: toolCalls,
	}
}

func UserMessage(content string) *Message {
	return &Message{
		Role:    User,
		Content: content,
	}
}

func ToolMessage(content, ToolName, ToolCallID string) *Message {
	return &Message{
		Role:       Tool,
		Content:    content,
		ToolCallID: ToolCallID,
		ToolName:   ToolName,
	}
}

// ConcatMessages concat messages with the same role and name.
// It will concat tool calls with the same index.
// It will return an error if the messages have different roles or names.
// It's useful for concatenating messages from a stream.
// e.g.
//
//	msgs := []*Message{}
//	for {
//		msg, err := stream.Recv()
//		if errors.Is(err, io.EOF) {
//			break
//		}
//		if err != nil {...}
//		msgs = append(msgs, msg)
//	}
//
// concatedMsg, err := ConcatMessages(msgs) // concatedMsg.Content will be full content of all messages
func ConcatMessages(msgs []*Message) (*Message, error) {
	var (
		contents            []string
		contentLen          int
		reasoningContents   []string
		reasoningContentLen int
		toolCalls           []ToolCall
		ret                 = Message{}
		extraList           = make([]map[string]any, 0, len(msgs))
	)

	for idx, msg := range msgs {
		if msg == nil {
			return nil, fmt.Errorf("unexpected nil chunk in message stream, index: %d", idx)
		}

		if msg.Role != "" {
			if ret.Role == "" {
				ret.Role = msg.Role
			} else if ret.Role != msg.Role {
				return nil, fmt.Errorf("cannot concat messages with "+
					"different roles: '%s' '%s'", ret.Role, msg.Role)
			}
		}

		if msg.ToolCallID != "" {
			if ret.ToolCallID == "" {
				ret.ToolCallID = msg.ToolCallID
			} else if ret.ToolCallID != msg.ToolCallID {
				return nil, fmt.Errorf("cannot concat messages with"+
					" different toolCallIDs: '%s' '%s'", ret.ToolCallID, msg.ToolCallID)
			}
		}
		if msg.ToolName != "" {
			if ret.ToolName == "" {
				ret.ToolName = msg.ToolName
			} else if ret.ToolName != msg.ToolName {
				return nil, fmt.Errorf("cannot concat messages with"+
					" different toolNames: '%s' '%s'", ret.ToolCallID, msg.ToolCallID)
			}
		}

		if msg.Content != "" {
			contents = append(contents, msg.Content)
			contentLen += len(msg.Content)
		}
		if msg.ReasoningContent != "" {
			reasoningContents = append(reasoningContents, msg.ReasoningContent)
			reasoningContentLen += len(msg.ReasoningContent)
		}

		if len(msg.ToolCalls) > 0 {
			toolCalls = append(toolCalls, msg.ToolCalls...)
		}

		if len(msg.Extra) > 0 {
			extraList = append(extraList, msg.Extra)
		}

		if msg.ResponseMeta != nil && ret.ResponseMeta == nil {
			ret.ResponseMeta = &ResponseMeta{}
		}

		if msg.ResponseMeta != nil && ret.ResponseMeta != nil {
			// keep the last FinishReason with a valid value.
			if msg.ResponseMeta.FinishReason != "" {
				ret.ResponseMeta.FinishReason = msg.ResponseMeta.FinishReason
			}

			if msg.ResponseMeta.Usage != nil {
				if ret.ResponseMeta.Usage == nil {
					ret.ResponseMeta.Usage = &TokenUsage{}
				}

				if msg.ResponseMeta.Usage.PromptTokens > ret.ResponseMeta.Usage.PromptTokens {
					ret.ResponseMeta.Usage.PromptTokens = msg.ResponseMeta.Usage.PromptTokens
				}
				if msg.ResponseMeta.Usage.CompletionTokens > ret.ResponseMeta.Usage.CompletionTokens {
					ret.ResponseMeta.Usage.CompletionTokens = msg.ResponseMeta.Usage.CompletionTokens
				}

				if msg.ResponseMeta.Usage.TotalTokens > ret.ResponseMeta.Usage.TotalTokens {
					ret.ResponseMeta.Usage.TotalTokens = msg.ResponseMeta.Usage.TotalTokens
				}

				if msg.ResponseMeta.Usage.PromptCacheHitTokens > ret.ResponseMeta.Usage.PromptCacheHitTokens {
					ret.ResponseMeta.Usage.PromptCacheHitTokens = msg.ResponseMeta.Usage.PromptCacheHitTokens
				}

				if msg.ResponseMeta.Usage.ReasoningTokens > ret.ResponseMeta.Usage.ReasoningTokens {
					ret.ResponseMeta.Usage.ReasoningTokens = msg.ResponseMeta.Usage.ReasoningTokens
				}
			}
		}
	}

	if len(contents) > 0 {
		var sb strings.Builder
		sb.Grow(contentLen)
		for _, content := range contents {
			_, err := sb.WriteString(content)
			if err != nil {
				return nil, err
			}
		}

		ret.Content = sb.String()
	}
	if len(reasoningContents) > 0 {
		var sb strings.Builder
		sb.Grow(reasoningContentLen)
		for _, rc := range reasoningContents {
			_, err := sb.WriteString(rc)
			if err != nil {
				return nil, err
			}
		}

		ret.ReasoningContent = sb.String()
	}

	if len(toolCalls) > 0 {
		merged, err := concatToolCalls(toolCalls)
		if err != nil {
			return nil, err
		}

		ret.ToolCalls = merged
	}

	if len(extraList) > 0 {
		extra, err := concatExtra(extraList)
		if err != nil {
			return nil, fmt.Errorf("failed to concat message's extra: %w", err)
		}

		if len(extra) > 0 {
			ret.Extra = extra
		}
	}

	return &ret, nil
}

func concatToolCalls(chunks []ToolCall) ([]ToolCall, error) {
	var merged []ToolCall
	m := make(map[int][]int)
	for i := range chunks {
		index := chunks[i].Index
		if index == nil {
			merged = append(merged, chunks[i])
		} else {
			m[*index] = append(m[*index], i)
		}
	}

	var args strings.Builder
	for k, v := range m {
		index := k
		toolCall := ToolCall{Index: &index}
		if len(v) > 0 {
			toolCall = chunks[v[0]]
		}

		args.Reset()
		toolID, toolType, toolName := "", "", "" // these field will output atomically in any chunk

		for _, n := range v {
			chunk := chunks[n]
			if chunk.ID != "" {
				if toolID == "" {
					toolID = chunk.ID
				} else if toolID != chunk.ID {
					return nil, fmt.Errorf("cannot concat ToolCalls with different tool id: '%s' '%s'", toolID, chunk.ID)
				}
			}

			if chunk.Type != "" {
				if toolType == "" {
					toolType = chunk.Type
				} else if toolType != chunk.Type {
					return nil, fmt.Errorf("cannot concat ToolCalls with different tool type: '%s' '%s'", toolType, chunk.Type)
				}
			}

			if chunk.Function.Name != "" {
				if toolName == "" {
					toolName = chunk.Function.Name
				} else if toolName != chunk.Function.Name {
					return nil, fmt.Errorf("cannot concat ToolCalls with different tool name: '%s' '%s'", toolName, chunk.Function.Name)
				}
			}

			if chunk.Function.Arguments != "" {
				_, err := args.WriteString(chunk.Function.Arguments)
				if err != nil {
					return nil, err
				}
			}
		}

		toolCall.ID = toolID
		toolCall.Type = toolType
		toolCall.Function.Name = toolName
		toolCall.Function.Arguments = args.String()

		merged = append(merged, toolCall)
	}

	if len(merged) > 1 {
		sort.SliceStable(merged, func(i, j int) bool {
			iVal, jVal := merged[i].Index, merged[j].Index
			if iVal == nil && jVal == nil {
				return false
			} else if iVal == nil && jVal != nil {
				return true
			} else if iVal != nil && jVal == nil {
				return false
			}

			return *iVal < *jVal
		})
	}

	return merged, nil
}

func concatExtra(extraList []map[string]any) (map[string]any, error) {
	if len(extraList) == 1 {
		return utils.CopyMap(extraList[0]), nil
	}

	return utils.ConcatItems(extraList)
}
