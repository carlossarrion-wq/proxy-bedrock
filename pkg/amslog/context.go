package amslog

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	traceIDKey   contextKey = "trace_id"
	requestIDKey contextKey = "request_id"
	loggerKey    contextKey = "logger"
)

// WithTraceID añade un trace ID al contexto
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		traceID = generateID()
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

// WithRequestID añade un request ID al contexto
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = generateID()
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// WithLogger añade un logger al contexto
func WithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// TraceIDFromContext extrae el trace ID del contexto
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// RequestIDFromContext extrae el request ID del contexto
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// FromContext extrae el logger del contexto
func FromContext(ctx context.Context) *Logger {
	if ctx == nil {
		return nil
	}
	if logger, ok := ctx.Value(loggerKey).(*Logger); ok {
		return logger
	}
	return nil
}

// NewTrace crea un nuevo contexto con trace ID y request ID
func NewTrace(ctx context.Context) context.Context {
	ctx = WithTraceID(ctx, "")
	ctx = WithRequestID(ctx, "")
	return ctx
}

// generateID genera un ID único
func generateID() string {
	return uuid.New().String()
}