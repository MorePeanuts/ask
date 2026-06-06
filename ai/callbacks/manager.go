package callbacks

import "context"

type (
	CtxManagerKey struct{}
	CtxRunInfoKey struct{}
)

type manager struct {
	globalHandlers []Handler
	handlers       []Handler
	runInfo        *RunInfo
}

func newManager(runInfo *RunInfo, handlers ...Handler) (*manager, bool) {
	if len(handlers)+len(GlobalHandlers) == 0 {
		return nil, false
	}

	hs := make([]Handler, len(GlobalHandlers))
	copy(hs, GlobalHandlers)

	return &manager{
		globalHandlers: hs,
		handlers:       handlers,
		runInfo:        runInfo,
	}, true
}

func (m *manager) withRunInfo(runInfo *RunInfo) *manager {
	if m == nil {
		return nil
	}

	n := *m
	n.runInfo = runInfo
	return &n
}

var GlobalHandlers []Handler

func managerFromCtx(ctx context.Context) (*manager, bool) {
	v := ctx.Value(CtxManagerKey{})
	if m, ok := v.(*manager); ok && m != nil {
		n := *m
		return &n, true
	}
	return nil, false
}

func ctxWithManager(ctx context.Context, manager *manager) context.Context {
	return context.WithValue(ctx, CtxManagerKey{}, manager)
}
