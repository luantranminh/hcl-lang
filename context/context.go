package context

import (
	"context"
)

type bodyExtCtxKey struct{}

type bodyActiveCountCtxKey struct{}

func WithActiveCount(ctx context.Context) context.Context {
	return context.WithValue(ctx, bodyActiveCountCtxKey{}, true)
}

func ActiveCountFromContext(ctx context.Context) bool {
	return ctx.Value(bodyActiveCountCtxKey{}) != nil
}
