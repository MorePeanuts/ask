package callbacks

import (
	"context"

	"github.com/MorePeanuts/ask/ai/schema"
)

type HandlerBuilder struct {
	onStartFn                func(ctx context.Context, info *RunInfo, input CallbackInput) context.Context
	onEndFn                  func(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context
	onErrorFn                func(ctx context.Context, info *RunInfo, err error) context.Context
	onStartWithStreamInputFn func(ctx context.Context, info *RunInfo, input *schema.StreamReader[CallbackInput]) context.Context
	onEndWithStreamOutputFn  func(ctx context.Context, info *RunInfo, output *schema.StreamReader[CallbackOutput]) context.Context
}

// NewHandlerBuilder creates and returns a new HandlerBuilder instance.
// HandlerBuilder is used to construct a Handler with custom callback functions
func NewHandlerBuilder() *HandlerBuilder {
	return &HandlerBuilder{}
}

// OnStartFn sets the handler for the start timing.
func (hb *HandlerBuilder) OnStartFn(
	fn func(ctx context.Context, info *RunInfo, input CallbackInput) context.Context,
) *HandlerBuilder {
	hb.onStartFn = fn
	return hb
}

// OnEndFn sets the handler for the end timing.
func (hb *HandlerBuilder) OnEndFn(
	fn func(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context,
) *HandlerBuilder {
	hb.onEndFn = fn
	return hb
}

// OnErrorFn sets the handler for the error timing.
func (hb *HandlerBuilder) OnErrorFn(
	fn func(ctx context.Context, info *RunInfo, err error) context.Context,
) *HandlerBuilder {
	hb.onErrorFn = fn
	return hb
}

// OnStartWithStreamInputFn sets the handler for the component start timing
// when its input is a stream. The handler owns the stream copy it receives and
// must close it after reading.
func (hb *HandlerBuilder) OnStartWithStreamInputFn(
	fn func(ctx context.Context, info *RunInfo, input *schema.StreamReader[CallbackInput]) context.Context,
) *HandlerBuilder {
	hb.onStartWithStreamInputFn = fn
	return hb
}

// OnEndWithStreamOutputFn sets the handler for the component end timing when
// its output is a stream. The handler owns the stream copy it receives and must
// close it after reading. This timing does not indicate that the stream has
// reached EOF.
func (hb *HandlerBuilder) OnEndWithStreamOutputFn(
	fn func(ctx context.Context, info *RunInfo, output *schema.StreamReader[CallbackOutput]) context.Context,
) *HandlerBuilder {
	hb.onEndWithStreamOutputFn = fn
	return hb
}

// Build returns a Handler with the functions set in the builder.
func (hb *HandlerBuilder) Build() Handler {
	return &handlerImpl{*hb}
}

var (
	_ TimingChecker = (*handlerImpl)(nil)
	_ Handler       = (*handlerImpl)(nil)
)

type handlerImpl struct {
	HandlerBuilder
}

func (h *handlerImpl) OnStart(ctx context.Context, info *RunInfo, input CallbackInput) context.Context {
	return h.onStartFn(ctx, info, input)
}

func (h *handlerImpl) OnEnd(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context {
	return h.onEndFn(ctx, info, output)
}

func (h *handlerImpl) OnError(ctx context.Context, info *RunInfo, err error) context.Context {
	return h.onErrorFn(ctx, info, err)
}

func (h *handlerImpl) OnStartWithStreamInput(ctx context.Context, info *RunInfo,
	input *schema.StreamReader[CallbackInput],
) context.Context {
	return h.onStartWithStreamInputFn(ctx, info, input)
}

func (h *handlerImpl) OnEndWithStreamOutput(ctx context.Context, info *RunInfo,
	output *schema.StreamReader[CallbackOutput],
) context.Context {
	return h.onEndWithStreamOutputFn(ctx, info, output)
}

func (h *handlerImpl) Needed(_ context.Context, _ *RunInfo, timing CallbackTiming) bool {
	switch timing {
	case TimingOnStart:
		return h.onStartFn != nil
	case TimingOnEnd:
		return h.onEndFn != nil
	case TimingOnError:
		return h.onErrorFn != nil
	case TimingOnStartWithStreamInput:
		return h.onStartWithStreamInputFn != nil
	case TimingOnEndWithStreamOutput:
		return h.onEndWithStreamOutputFn != nil
	default:
		return false
	}
}
