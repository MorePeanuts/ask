package model

import (
	"context"

	"github.com/MorePeanuts/ask/ai/schema"
)

type BaseChatModel interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.Message, error)
	Stream(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.StreamReader[*schema.Message], error)
}

type ChatModelWithTools interface {
	BaseChatModel

	WithTools(tools []*schema.ToolInfo) (ChatModelWithTools, error)
}
