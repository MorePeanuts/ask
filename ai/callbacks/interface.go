// Package callbacks provides unified lifecycle hooks for components, nodes,
// and other executable units.
//
// Typical use cases include:
//
// 1. Recording input and output of each node;
// 2. Measuring model invocation time;
// 3. Logging token usage;
// 4. Integrating tracing metrics;
//
// Start and end refer to the component call boundary, not to the lifetime of a
// stream carried across that boundary. For a non-streaming input or output,
// components use OnStart and OnEnd. When the input or output itself is a
// stream, components use OnStartWithStreamInput or OnEndWithStreamOutput
// instead.
//
// A streaming callback runs when the stream becomes available at the
// corresponding component boundary. It does not run when the first item is
// received or when the stream reaches EOF. Each handler receives an independent
// stream copy and is responsible for consuming and closing it. Handlers should
// normally start a goroutine to consume their copy and return promptly so they
// do not delay the component or its caller from consuming their own copy.
package callbacks

import (
	"context"

	"github.com/MorePeanuts/ask/ai/components"
	"github.com/MorePeanuts/ask/ai/schema"
)

// RunInfo represents the information returned each time a callback is triggered, describing who triggered the callback.
type RunInfo struct {
	// Name is the graph node name for display purposes, not unique.
	Name string
	// Specific implementation types
	Type string
	// Category of the component, for example: ChatModel, Tool
	Component components.Component
}

type CallbackInput any

type CallbackOutput any

type Handler interface {
	OnStart(ctx context.Context, info *RunInfo, input CallbackInput) context.Context
	OnEnd(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context
	OnError(ctx context.Context, info *RunInfo, err error) context.Context
	OnStartWithStreamInput(ctx context.Context, info *RunInfo, input *schema.StreamReader[CallbackInput]) context.Context
	OnEndWithStreamOutput(ctx context.Context, info *RunInfo, output *schema.StreamReader[CallbackOutput]) context.Context
}

type CallbackTiming uint8

// Callback timing constants.
const (
	// TimingOnStart fires just before the component begins processing.
	TimingOnStart CallbackTiming = iota
	// TimingOnEnd fires after the component returns a result successfully.
	TimingOnEnd
	// TimingOnError fires when the component returns a non-nil error.
	TimingOnError
	// TimingOnStartWithStreamInput fires at the component start boundary when
	// its streaming input becomes available (Collect / Transform paradigms).
	// It does not indicate that stream consumption has started. The handler
	// receives a copy of the input stream and must close it after reading.
	TimingOnStartWithStreamInput
	// TimingOnEndWithStreamOutput fires at the component end boundary when its
	// streaming output becomes available (Stream / Transform paradigms). It
	// does not indicate that the stream has reached EOF. The handler receives
	// a copy of the output stream and must close it after reading. This is
	// typically where you implement streaming metrics or logging.
	TimingOnEndWithStreamOutput
)

type TimingChecker interface {
	Needed(ctx context.Context, info *RunInfo, timing CallbackTiming) bool
}
