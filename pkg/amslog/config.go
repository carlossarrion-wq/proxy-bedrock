package amslog

import (
	"fmt"
	"io"
	"os"
)

// LogLevel representa el nivel de severidad del log
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String devuelve la representación en string del nivel
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Outcome representa el resultado de un evento
type Outcome string

const (
	OutcomeSuccess Outcome = "SUCCESS"
	OutcomeFailure Outcome = "FAILURE"
)

// Servicios oficiales según la política
var OfficialServices = map[string]string{
	"kb-agent":           "Agente de Conocimiento",
	"bedrock-proxy":      "Proxy de Acceso a Amazon Bedrock",
	"capacity-mgmt":      "Gestor de Capacidad",
	"identity-mgmt":      "Gestor de Identidades",
	"bedrock-dashboard":  "Control de Uso Bedrock",
	"kb-agent-dashboard": "Control de Uso Knowledge Base",
}

// Config contiene la configuración del logger
type Config struct {
	// ServiceName es el nombre del servicio (debe estar en OfficialServices)
	ServiceName string

	// ServiceVersion es la versión del servicio
	ServiceVersion string

	// Environment es el entorno (dev, pre, pro)
	Environment string

	// InstanceID es el identificador de la instancia (opcional)
	InstanceID string

	// MinLevel es el nivel mínimo de log a procesar
	MinLevel LogLevel

	// EnableSanitization activa la sanitización de datos sensibles
	EnableSanitization bool

	// Output es el destino de los logs (por defecto os.Stdout)
	Output io.Writer

	// Async activa el modo asíncrono
	Async bool

	// BufferSize es el tamaño del buffer para modo asíncrono
	BufferSize int
}

// Validate valida la configuración
func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}

	if _, ok := OfficialServices[c.ServiceName]; !ok {
		return fmt.Errorf("service_name '%s' is not in official services list", c.ServiceName)
	}

	if c.ServiceVersion == "" {
		return fmt.Errorf("service_version is required")
	}

	if c.Environment == "" {
		return fmt.Errorf("environment is required")
	}

	validEnvs := map[string]bool{"dev": true, "pre": true, "pro": true}
	if !validEnvs[c.Environment] {
		return fmt.Errorf("environment must be one of: dev, pre, pro")
	}

	return nil
}

// SetDefaults establece valores por defecto
func (c *Config) SetDefaults() {
	if c.Output == nil {
		c.Output = os.Stdout
	}

	if c.BufferSize == 0 {
		c.BufferSize = 1000
	}

	// Sanitización habilitada por defecto
	if !c.EnableSanitization {
		c.EnableSanitization = true
	}
}