package callbacks

import "context"

type HandlerBuilder struct {
	onStartFn func(ctx context.Context, info *RunInfo, input CallbackInput) context.Context
	onEndFn   func(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context
	onErrorFn func(ctx context.Context, info *RunInfo, err error) context.Context
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

func (h *handlerImpl) Needed(_ context.Context, _ *RunInfo, timing CallbackTiming) bool {
	switch timing {
	case TimingOnStart:
		return h.onStartFn != nil
	case TimingOnEnd:
		return h.onEndFn != nil
	case TimingOnError:
		return h.onErrorFn != nil
	default:
		return false
	}
}
