package amslog

import (
	"context"
	"net/http"
	"time"
)

// HTTPMiddleware crea un middleware HTTP que añade logging automático
func HTTPMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Extraer o generar trace ID
			traceID := r.Header.Get("X-Trace-Id")
			if traceID == "" {
				traceID = r.Header.Get("X-Request-Id")
			}
			if traceID == "" {
				traceID = generateID()
			}

			// Generar request ID
			requestID := generateID()

			// Añadir al contexto
			ctx := WithTraceID(r.Context(), traceID)
			ctx = WithRequestID(ctx, requestID)
			ctx = WithLogger(ctx, logger)

			// Crear response writer que captura el status code
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Añadir headers de trazabilidad a la respuesta
			w.Header().Set("X-Trace-Id", traceID)
			w.Header().Set("X-Request-Id", requestID)

			// Ejecutar handler
			next.ServeHTTP(rw, r.WithContext(ctx))

			// Log de la petición
			duration := time.Since(start).Milliseconds()
			outcome := OutcomeSuccess
			if rw.statusCode >= 400 {
				outcome = OutcomeFailure
			}

			logger.InfoContext(ctx, Event{
				Name:       "HTTP_REQUEST",
				Message:    "HTTP request processed",
				DurationMs: duration,
				Outcome:    outcome,
				Fields: map[string]interface{}{
					"http.request.method":        r.Method,
					"url.path":                   r.URL.Path,
					"http.response.status_code":  rw.statusCode,
					"http.request.body.bytes":    r.ContentLength,
					"http.response.body.bytes":   rw.bytesWritten,
					"user_agent.original":        r.UserAgent(),
					"source.address":             r.RemoteAddr,
				},
			})
		})
	}
}

// responseWriter es un wrapper de http.ResponseWriter que captura el status code
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// LogOperation es un helper para registrar operaciones con duración automática
func LogOperation(ctx context.Context, logger *Logger, eventName string, fn func() error) error {
	start := time.Now()

	err := fn()

	duration := time.Since(start).Milliseconds()
	outcome := OutcomeSuccess
	var errorInfo *ErrorInfo

	if err != nil {
		outcome = OutcomeFailure
		errorInfo = &ErrorInfo{
			Type:    "OperationError",
			Message: err.Error(),
		}
	}

	logger.InfoContext(ctx, Event{
		Name:       eventName,
		Message:    "Operation completed",
		DurationMs: duration,
		Outcome:    outcome,
		Error:      errorInfo,
	})

	return err
}