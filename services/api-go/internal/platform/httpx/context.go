package httpx

import "context"

type httpContext = context.Context

func contextWith(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}
