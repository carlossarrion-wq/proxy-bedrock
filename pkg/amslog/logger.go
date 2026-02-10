package amslog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Logger es el logger principal que implementa la Política de Logs v1.0
type Logger struct {
	config     Config
	sanitizer  *Sanitizer
	buffer     chan *LogEntry
	wg         sync.WaitGroup
	closed     bool
	closeMutex sync.Mutex
}

// NewLogger crea un nuevo logger con la configuración proporcionada
func NewLogger(config Config) *Logger {
	config.SetDefaults()
	if err := config.Validate(); err != nil {
		panic(fmt.Sprintf("invalid logger configuration: %v", err))
	}

	logger := &Logger{
		config: config,
	}

	if config.EnableSanitization {
		logger.sanitizer = NewSanitizer()
	}

	// Modo asíncrono
	if config.Async {
		logger.buffer = make(chan *LogEntry, config.BufferSize)
		logger.wg.Add(1)
		go logger.worker()
	}

	return logger
}

// worker procesa logs en modo asíncrono
func (l *Logger) worker() {
	defer l.wg.Done()
	for entry := range l.buffer {
		l.writeLog(entry)
	}
}

// Close cierra el logger y espera a que se procesen los logs pendientes
func (l *Logger) Close() error {
	l.closeMutex.Lock()
	defer l.closeMutex.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true

	if l.config.Async && l.buffer != nil {
		close(l.buffer)
		l.wg.Wait()
	}

	return nil
}

// Debug registra un log de nivel DEBUG
func (l *Logger) Debug(event Event) {
	l.log(context.Background(), LevelDebug, event)
}

// DebugContext registra un log de nivel DEBUG con contexto
func (l *Logger) DebugContext(ctx context.Context, event Event) {
	l.log(ctx, LevelDebug, event)
}

// Info registra un log de nivel INFO
func (l *Logger) Info(event Event) {
	l.log(context.Background(), LevelInfo, event)
}

// InfoContext registra un log de nivel INFO con contexto
func (l *Logger) InfoContext(ctx context.Context, event Event) {
	l.log(ctx, LevelInfo, event)
}

// Warning registra un log de nivel WARN
func (l *Logger) Warning(event Event) {
	l.log(context.Background(), LevelWarn, event)
}

// WarningContext registra un log de nivel WARN con contexto
func (l *Logger) WarningContext(ctx context.Context, event Event) {
	l.log(ctx, LevelWarn, event)
}

// Error registra un log de nivel ERROR
func (l *Logger) Error(event Event) {
	l.log(context.Background(), LevelError, event)
}

// ErrorContext registra un log de nivel ERROR con contexto
func (l *Logger) ErrorContext(ctx context.Context, event Event) {
	l.log(ctx, LevelError, event)
}

// Fatal registra un log de nivel FATAL y termina el programa
func (l *Logger) Fatal(event Event) {
	l.log(context.Background(), LevelFatal, event)
	l.Close()
	os.Exit(1)
}

// FatalContext registra un log de nivel FATAL con contexto y termina el programa
func (l *Logger) FatalContext(ctx context.Context, event Event) {
	l.log(ctx, LevelFatal, event)
	l.Close()
	os.Exit(1)
}

// log es el método interno que procesa un log
func (l *Logger) log(ctx context.Context, level LogLevel, event Event) {
	// Filtrar por nivel mínimo
	if level < l.config.MinLevel {
		return
	}

	// Crear entrada de log
	entry := l.createLogEntry(ctx, level, event)

	// Modo asíncrono o síncrono
	if l.config.Async && l.buffer != nil {
		select {
		case l.buffer <- entry:
			// Log encolado
		default:
			// Buffer lleno, escribir directamente
			l.writeLog(entry)
		}
	} else {
		l.writeLog(entry)
	}
}

// createLogEntry crea una entrada de log completa
func (l *Logger) createLogEntry(ctx context.Context, level LogLevel, event Event) *LogEntry {
	entry := &LogEntry{
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		LogLevel:       level.String(),
		ServiceName:    l.config.ServiceName,
		ServiceVersion: l.config.ServiceVersion,
		Environment:    l.config.Environment,
		InstanceID:     l.config.InstanceID,
		EventName:      strings.ToUpper(event.Name),
		Message:        event.Message,
	}

	// Outcome por defecto
	if event.Outcome == "" {
		if level == LevelError || level == LevelFatal {
			entry.EventOutcome = string(OutcomeFailure)
		} else {
			entry.EventOutcome = string(OutcomeSuccess)
		}
	} else {
		entry.EventOutcome = string(event.Outcome)
	}

	// Duración
	if event.DurationMs > 0 {
		entry.DurationMs = event.DurationMs
	}

	// Trazabilidad desde contexto
	if ctx != nil {
		if traceID := TraceIDFromContext(ctx); traceID != "" {
			entry.TraceID = traceID
		}
		if requestID := RequestIDFromContext(ctx); requestID != "" {
			entry.RequestID = requestID
		}
	}

	// Información de error
	if event.Error != nil {
		entry.ErrorType = event.Error.Type
		entry.ErrorMessage = event.Error.Message
		entry.ErrorCode = event.Error.Code
		entry.ErrorStackTrace = event.Error.StackTrace
	}

	// Campos adicionales
	if event.Fields != nil {
		if l.sanitizer != nil {
			entry.Fields = l.sanitizer.Sanitize(event.Fields)
		} else {
			entry.Fields = event.Fields
		}
	}

	return entry
}

// writeLog escribe un log en el output configurado
func (l *Logger) writeLog(entry *LogEntry) {
	// Convertir a mapa para incluir campos adicionales
	data := make(map[string]interface{})

	// Campos obligatorios
	data["@timestamp"] = entry.Timestamp
	data["log.level"] = entry.LogLevel
	data["service.name"] = entry.ServiceName
	data["service.version"] = entry.ServiceVersion
	data["labels.environment"] = entry.Environment

	if entry.InstanceID != "" {
		data["service.instance.id"] = entry.InstanceID
	}

	data["event.name"] = entry.EventName
	data["event.outcome"] = entry.EventOutcome
	data["message"] = entry.Message

	if entry.TraceID != "" {
		data["trace.id"] = entry.TraceID
	}

	if entry.RequestID != "" {
		data["request.id"] = entry.RequestID
	}

	if entry.DurationMs > 0 {
		data["event.duration_ms"] = entry.DurationMs
	}

	// Campos de error
	if entry.ErrorType != "" {
		data["error.type"] = entry.ErrorType
	}
	if entry.ErrorMessage != "" {
		data["error.message"] = entry.ErrorMessage
	}
	if entry.ErrorCode != "" {
		data["error.code"] = entry.ErrorCode
	}
	if entry.ErrorStackTrace != "" {
		data["error.stack_trace"] = entry.ErrorStackTrace
	}

	// Campos adicionales
	for key, value := range entry.Fields {
		data[key] = value
	}

	// Serializar a JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling log: %v\n", err)
		return
	}

	// Escribir al output
	fmt.Fprintln(l.config.Output, string(jsonData))
}