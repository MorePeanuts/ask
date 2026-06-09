package callbacks

import (
	"context"

	"github.com/MorePeanuts/ask/ai/components"
	"github.com/MorePeanuts/ask/ai/schema"
	"github.com/MorePeanuts/ask/ai/utils"
)

// InitCallbacks creates a new context with the given RunInfo and handlers,
// completely replacing any RunInfo and handlers already in ctx.
//
// Use this when running a component standalone outside a Graph — the Graph
// normally manages RunInfo injection automatically, but standalone callers must
// set it up themselves:
//
//	ctx = callbacks.InitCallbacks(ctx, &callbacks.RunInfo{
//	    Type:      myModel.GetType(),
//	    Component: components.ComponentOfChatModel,
//	    Name:      "my-model",
//	}, myHandler)
func InitCallbacks(ctx context.Context, info *RunInfo, handlers ...Handler) context.Context {
	mgr, ok := newManager(info, handlers...)
	if ok {
		return ctxWithManager(ctx, mgr)
	}
	return ctxWithManager(ctx, nil)
}

// ReuseHandlers creates a new context that inherits all handlers already
// present in ctx and sets a new RunInfo. Global handlers are added if ctx
// carries none yet.
//
// Use this when a component calls another component internally and wants the
// inner component's callbacks to share the same set of handlers as the outer
// component, but with the inner component's own identity in RunInfo:
//
//	innerCtx := callbacks.ReuseHandlers(ctx, &callbacks.RunInfo{
//	    Type:      "InnerChatModel",
//	    Component: components.ComponentOfChatModel,
//	    Name:      "inner-cm",
//	})
func ReuseHandlers(ctx context.Context, info *RunInfo) context.Context {
	cbm, ok := managerFromCtx(ctx)
	if !ok {
		return InitCallbacks(ctx, info)
	}
	return ctxWithManager(ctx, cbm.withRunInfo(info))
}

// EnsureRunInfo ensures the context carries a [RunInfo] for the given type and
// component kind. If the context already has a matching RunInfo, it is
// returned unchanged. Otherwise, a new callback manager is created that
// inherits the global handlers plus any handlers already in ctx.
//
// Component implementations that set IsCallbacksEnabled() = true should call
// this at the start of every public method (Generate, Stream, etc.) before
// calling [OnStart], so that the RunInfo is never missing from callbacks.
func EnsureRunInfo(ctx context.Context, typ string, comp components.Component) context.Context {
	cbm, ok := managerFromCtx(ctx)
	if !ok {
		return InitCallbacks(ctx, &RunInfo{
			Type:      typ,
			Component: comp,
		})
	}
	if cbm.runInfo == nil {
		return ReuseHandlers(ctx, &RunInfo{
			Type:      typ,
			Component: comp,
		})
	}
	return ctx
}

// OnStart invokes the OnStart timing for all registered handlers in the
// context. This is called by component implementations that manage their own
// callbacks (i.e. implement [components.Checker] and return true from
// IsCallbacksEnabled). The returned context must be propagated to subsequent
// OnEnd/OnError calls so handlers can correlate start and end events.
//
// Handlers are invoked in reverse registration order (last registered = first
// called) to match the middleware wrapping convention.
//
// Example — typical usage inside a component's Generate method:
//
//	func (m *myChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
//	    ctx = callbacks.OnStart(ctx, &model.CallbackInput{Messages: input})
//	    resp, err := m.doGenerate(ctx, input, opts...)
//	    if err != nil {
//	        callbacks.OnError(ctx, err)
//	        return nil, err
//	    }
//	    callbacks.OnEnd(ctx, &model.CallbackOutput{Message: resp})
//	    return resp, nil
//	}
func OnStart[T any](ctx context.Context, input T) context.Context {
	ctx, _ = On(ctx, input, OnStartHandle[T], TimingOnStart, true)
	return ctx
}

// OnEnd invokes the OnEnd timing for all registered handlers. Call this after
// the component produces a successful result. Handlers run in registration
// order (first registered = first called).
//
// Do not call both OnEnd and OnError for the same invocation — OnEnd signals
// success; OnError signals failure.
func OnEnd[T any](ctx context.Context, output T) context.Context {
	ctx, _ = On(ctx, output, OnEndHandle[T], TimingOnEnd, false)
	return ctx
}

// OnStartWithStreamInput invokes the start timing for a component whose input
// is a stream (Collect / Transform paradigms). It is the streaming-input
// alternative to [OnStart]; do not call both for the same component invocation.
//
// This function runs when the input stream becomes available at the component
// start boundary, before the component begins consuming it. It does not wait
// for the first item and does not indicate that stream consumption has started.
//
// The framework copies the stream so every selected handler and the component
// receive independent readers. A handler owns its copy and must close it after
// reading. A handler should normally start a goroutine to consume its copy and
// return promptly; consuming the complete stream before returning delays the
// component from consuming its copy and defeats streaming behavior.
//
// OnStartWithStreamInput returns the updated context and the reader the
// component must use from then on. The original reader becomes unusable after
// it is copied.
func OnStartWithStreamInput[T any](ctx context.Context, input *schema.StreamReader[T]) (
	nextCtx context.Context, newStreamReader *schema.StreamReader[T],
) {
	return On(ctx, input, OnStartWithStreamInputHandle[T], TimingOnStartWithStreamInput, true)
}

// OnEndWithStreamOutput invokes the end timing for a component whose output is
// a stream (Stream / Transform paradigms). It is the streaming-output
// alternative to [OnEnd]; do not call both for the same successful component
// invocation.
//
// This function runs when the output stream becomes available at the component
// end boundary, before the caller begins consuming it. Despite "End" in its
// name, it does not indicate that the stream has reached EOF or has been
// closed.
//
// The framework copies the stream so every selected handler and the caller
// receive independent readers. A handler owns its copy and must close it after
// reading. A handler should normally start a goroutine to consume its copy and
// return promptly; consuming the complete stream before returning delays the
// component from returning its caller-facing copy and defeats streaming
// behavior.
//
// OnEndWithStreamOutput returns the updated context and the reader the
// component must return to its caller. The original reader becomes unusable
// after it is copied.
func OnEndWithStreamOutput[T any](ctx context.Context, output *schema.StreamReader[T]) (
	nextCtx context.Context, newStreamReader *schema.StreamReader[T],
) {
	return On(ctx, output, OnEndWithStreamOutputHandle[T], TimingOnEndWithStreamOutput, false)
}

// OnError invokes the OnError timing for all registered handlers. Call this
// when the component returns an error. Errors that occur mid-stream (after the
// StreamReader has been returned) are NOT routed through OnError; they surface
// as errors inside Recv.
//
// Handlers run in registration order (same as OnEnd).
func OnError(ctx context.Context, err error) context.Context {
	ctx, _ = On(ctx, err, OnErrorHandle, TimingOnError, false)

	return ctx
}

type Handle[T any] func(context.Context, T, *RunInfo, []Handler) (context.Context, T)

func On[T any](ctx context.Context, inOut T, handle Handle[T], timing CallbackTiming,
	start bool,
) (context.Context, T) {
	mgr, ok := managerFromCtx(ctx)
	if !ok {
		return ctx, inOut
	}
	nMgr := *mgr

	var info *RunInfo
	if start {
		info = nMgr.runInfo
		nMgr.runInfo = nil
		ctx = context.WithValue(ctx, CtxRunInfoKey{}, info)
	} else {
		if nMgr.runInfo != nil {
			info = nMgr.runInfo
		} else {
			info, _ = ctx.Value(CtxRunInfoKey{}).(*RunInfo)
		}
	}

	hs := make([]Handler, 0, len(nMgr.handlers)+len(nMgr.globalHandlers))
	for _, handler := range append(nMgr.handlers, nMgr.globalHandlers...) {
		timingChecker, ok := handler.(TimingChecker)
		if !ok || timingChecker.Needed(ctx, info, timing) {
			hs = append(hs, handler)
		}
	}

	var out T
	ctx, out = handle(ctx, inOut, info, hs)
	return ctxWithManager(ctx, &nMgr), out
}

func OnStartHandle[T any](ctx context.Context, input T, runInfo *RunInfo, handlers []Handler) (context.Context, T) {
	for i := len(handlers) - 1; i >= 0; i-- {
		ctx = handlers[i].OnStart(ctx, runInfo, input)
	}

	return ctx, input
}

func OnEndHandle[T any](ctx context.Context, output T, runInfo *RunInfo, handlers []Handler) (context.Context, T) {
	for _, handler := range handlers {
		ctx = handler.OnEnd(ctx, runInfo, output)
	}

	return ctx, output
}

func OnStartWithStreamInputHandle[T any](ctx context.Context, input *schema.StreamReader[T],
	runInfo *RunInfo, handlers []Handler,
) (context.Context, *schema.StreamReader[T]) {
	handlers = utils.Reverse(handlers)

	cpy := input.Copy

	handle := func(ctx context.Context, handler Handler, in *schema.StreamReader[T]) context.Context {
		in_ := schema.StreamReaderWithConvert(in, func(i T) (CallbackInput, error) {
			return i, nil
		})
		return handler.OnStartWithStreamInput(ctx, runInfo, in_)
	}

	return OnWithStreamHandle(ctx, input, handlers, cpy, handle)
}

func OnEndWithStreamOutputHandle[T any](ctx context.Context, output *schema.StreamReader[T],
	runInfo *RunInfo, handlers []Handler,
) (context.Context, *schema.StreamReader[T]) {
	cpy := output.Copy

	handle := func(ctx context.Context, handler Handler, out *schema.StreamReader[T]) context.Context {
		out_ := schema.StreamReaderWithConvert(out, func(i T) (CallbackOutput, error) {
			return i, nil
		})
		return handler.OnEndWithStreamOutput(ctx, runInfo, out_)
	}

	return OnWithStreamHandle(ctx, output, handlers, cpy, handle)
}

func OnErrorHandle(ctx context.Context, err error, runInfo *RunInfo, handlers []Handler) (context.Context, error) {
	for _, handler := range handlers {
		ctx = handler.OnError(ctx, runInfo, err)
	}

	return ctx, err
}

func OnWithStreamHandle[T any](
	ctx context.Context,
	inOut T,
	handlers []Handler,
	cpy func(int) []T,
	handle func(context.Context, Handler, T) context.Context,
) (context.Context, T) {
	if len(handlers) == 0 {
		return ctx, inOut
	}

	inOuts := cpy(len(handlers) + 1)

	for i, handler := range handlers {
		ctx = handle(ctx, handler, inOuts[i])
	}

	return ctx, inOuts[len(inOuts)-1]
}
