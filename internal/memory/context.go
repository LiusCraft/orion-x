package memory

import "context"

type contextKey struct{}

// WithContext 将记忆上下文写入 context。
func WithContext(ctx context.Context, memCtx Context) context.Context {
	return context.WithValue(ctx, contextKey{}, memCtx)
}

// FromContext 从 context 读取记忆上下文。
func FromContext(ctx context.Context) (Context, bool) {
	value := ctx.Value(contextKey{})
	if value == nil {
		return Context{}, false
	}
	memCtx, ok := value.(Context)
	return memCtx, ok
}
