package pkg

import (
	"fmt"
	"os"

	"bedrock-proxy-test/pkg/amslog"
)

var Logger *amslog.Logger

// InitLogger inicializa el logger según la Política de Logs v1.0
// Sección 13.2: En contenedores, los logs se emiten a stdout/stderr
func InitLogger() {
	serviceName := "bedrock-proxy"
	environment := getEnv("ENVIRONMENT", "dev")
	instanceID := getEnv("INSTANCE_ID", "inst-01")

	// Configurar logger para stdout (contenedores ECS → CloudWatch)
	// Según Política v1.0 Sección 13.2: "Los logs se emiten a stdout/stderr"
	config := amslog.Config{
		ServiceName:        serviceName,
		ServiceVersion:     getEnv("SERVICE_VERSION", "1.0.0"),
		Environment:        environment,
		InstanceID:         instanceID,
		MinLevel:           getLogLevel(),
		EnableSanitization: true,
		Output:             os.Stdout, // Escribir a stdout para CloudWatch
		Async:              true,
		BufferSize:         10000,
	}

	Logger = amslog.NewLogger(config)

	// Log de inicialización
	Logger.Info(amslog.Event{
		Name:    EventLoggerInit,
		Message: "Logger initialized for containerized environment (stdout → CloudWatch)",
		Fields: map[string]interface{}{
			"output":      "stdout",
			"environment": environment,
			"version":     config.ServiceVersion,
		},
	})
}

// getLogLevel obtiene el nivel de log desde variable de entorno
func getLogLevel() amslog.LogLevel {
	level := getEnv("LOG_LEVEL", "INFO")
	switch level {
	case "DEBUG":
		return amslog.LevelDebug
	case "INFO":
		return amslog.LevelInfo
	case "WARN":
		return amslog.LevelWarn
	case "ERROR":
		return amslog.LevelError
	default:
		return amslog.LevelInfo
	}
}

// getEnv obtiene una variable de entorno con valor por defecto
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// CloseLogger cierra el logger y espera a que se procesen logs pendientes
func CloseLogger() {
	if Logger != nil {
		Logger.Info(amslog.Event{
			Name:    EventServerShutdown,
			Message: "Logger shutting down",
		})
		Logger.Close()
	}
}

// DEPRECATED: Mantener Log para compatibilidad temporal durante migración
// TODO: Eliminar una vez completada la migración
var Log = &deprecatedLogger{}

type deprecatedLogger struct{}

func (l *deprecatedLogger) Infof(format string, args ...interface{}) {
	if Logger != nil {
		Logger.Info(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprintf(format, args...),
		})
	}
}

func (l *deprecatedLogger) Errorf(format string, args ...interface{}) {
	if Logger != nil {
		Logger.Error(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprintf(format, args...),
			Outcome: amslog.OutcomeFailure,
		})
	}
}

func (l *deprecatedLogger) Warningf(format string, args ...interface{}) {
	if Logger != nil {
		Logger.Warning(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprintf(format, args...),
		})
	}
}

func (l *deprecatedLogger) Debugf(format string, args ...interface{}) {
	if Logger != nil {
		Logger.Debug(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprintf(format, args...),
		})
	}
}

func (l *deprecatedLogger) Info(args ...interface{}) {
	if Logger != nil {
		Logger.Info(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprint(args...),
		})
	}
}

func (l *deprecatedLogger) Error(args ...interface{}) {
	if Logger != nil {
		Logger.Error(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprint(args...),
			Outcome: amslog.OutcomeFailure,
		})
	}
}

func (l *deprecatedLogger) Warning(args ...interface{}) {
	if Logger != nil {
		Logger.Warning(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprint(args...),
		})
	}
}

func (l *deprecatedLogger) Debug(args ...interface{}) {
	if Logger != nil {
		Logger.Debug(amslog.Event{
			Name:    "DEPRECATED_LOG",
			Message: fmt.Sprint(args...),
		})
	}
}