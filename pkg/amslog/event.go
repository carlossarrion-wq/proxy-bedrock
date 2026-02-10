package amslog

// Event representa un evento de log
type Event struct {
	// Name es el nombre del evento en formato DOMINIO_ACCION
	Name string

	// Message es el mensaje descriptivo del evento
	Message string

	// Outcome es el resultado del evento (SUCCESS/FAILURE)
	Outcome Outcome

	// DurationMs es la duración del evento en milisegundos
	DurationMs int64

	// Error contiene información del error (si aplica)
	Error *ErrorInfo

	// Fields contiene campos adicionales personalizados
	Fields map[string]interface{}
}

// ErrorInfo contiene información de un error
type ErrorInfo struct {
	// Type es el tipo de error
	Type string

	// Message es el mensaje del error
	Message string

	// Code es el código de error
	Code string

	// StackTrace es el stack trace del error
	StackTrace string
}

// LogEntry representa una entrada de log completa
type LogEntry struct {
	Timestamp       string                 `json:"@timestamp"`
	LogLevel        string                 `json:"log.level"`
	ServiceName     string                 `json:"service.name"`
	ServiceVersion  string                 `json:"service.version"`
	Environment     string                 `json:"labels.environment"`
	InstanceID      string                 `json:"service.instance.id,omitempty"`
	EventName       string                 `json:"event.name"`
	EventOutcome    string                 `json:"event.outcome"`
	Message         string                 `json:"message"`
	TraceID         string                 `json:"trace.id,omitempty"`
	RequestID       string                 `json:"request.id,omitempty"`
	DurationMs      int64                  `json:"event.duration_ms,omitempty"`
	ErrorType       string                 `json:"error.type,omitempty"`
	ErrorMessage    string                 `json:"error.message,omitempty"`
	ErrorCode       string                 `json:"error.code,omitempty"`
	ErrorStackTrace string                 `json:"error.stack_trace,omitempty"`
	Fields          map[string]interface{} `json:"-"`
}