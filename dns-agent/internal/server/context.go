package server

import (
	"context"
	"net/http"
)

type requestIDKey struct{}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func requestID(r *http.Request) string {
	if value, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return value
	}
	return ""
}
