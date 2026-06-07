// Package callbacks provide unified lifecycle hooks for the execution process of components, nodes, etc.,
// allowing users to insert custom logic at moments such as start, end, and error occurrences.
//
// Typical use cases include:
//
// 1. Recording input and output of each node;
// 2. Measuring model invocation time;
// 3. Logging token usage;
// 4. Integrating tracing metrics;
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
	// TimingOnEndWithStreamOutput fires after the component returns a
	// streaming output (Stream / Transform paradigms). The handler receives a
	// copy of the output stream and must close it after reading. This is
	// typically where you implement streaming metrics or logging.
	TimingOnEndWithStreamOutput
)

type TimingChecker interface {
	Needed(ctx context.Context, info *RunInfo, timing CallbackTiming) bool
}
