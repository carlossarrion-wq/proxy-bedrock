package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	
	"bedrock-proxy-test/pkg/database"
)

// JWTConfig contiene la configuración JWT
type JWTConfig struct {
	SecretKey string
	Issuer    string
	Audience  string
}

// LoadJWTConfigWithEnv carga configuración JWT desde AWS Secrets Manager o variables de entorno
// Prioriza AWS Secrets Manager si JWT_SECRET_ARN está configurado
// Retorna error si JWT_SECRET_KEY no cumple requisitos de seguridad OWASP
func LoadJWTConfigWithEnv() (*JWTConfig, error) {
	var secretKey string
	
	// Intentar cargar desde AWS Secrets Manager primero
	jwtSecretARN := os.Getenv("JWT_SECRET_ARN")
	if jwtSecretARN != "" {
		
		secret, err := database.GetSecretFromSecretsManager(context.Background(), jwtSecretARN)
		if err != nil {
			return nil, fmt.Errorf("failed to load JWT secret from Secrets Manager: %w", err)
		}
		
		// El secreto puede ser un JSON o un string simple
		// Intentar parsear como JSON primero
		var secretData map[string]interface{}
		if err := json.Unmarshal([]byte(secret), &secretData); err == nil {
			// Es un JSON, buscar la clave "jwt_secret_key" o "secret_key"
			if key, ok := secretData["jwt_secret_key"].(string); ok {
				secretKey = key
			} else if key, ok := secretData["secret_key"].(string); ok {
				secretKey = key
			} else if key, ok := secretData["key"].(string); ok {
				secretKey = key
			} else {
				return nil, fmt.Errorf("JWT secret JSON does not contain 'jwt_secret_key', 'secret_key', or 'key' field")
			}
		} else {
			// No es JSON, usar el valor directo
			secretKey = secret
		}
		
	} else {
		// Fallback: cargar desde variable de entorno
		secretKey = os.Getenv("JWT_SECRET_KEY")
	}
	
	// Validación crítica de seguridad: JWT secret debe existir
	if secretKey == "" {
		return nil, fmt.Errorf("JWT_SECRET_KEY not found. Set JWT_SECRET_ARN (recommended) or JWT_SECRET_KEY environment variable")
	}
	
	// Validación crítica de seguridad: JWT secret debe tener al menos 32 caracteres
	// Esto previene el uso de claves débiles que podrían ser vulnerables a ataques de fuerza bruta
	// Referencia: OWASP JWT Security Cheat Sheet
	if len(secretKey) < 32 {
		return nil, fmt.Errorf("JWT_SECRET_KEY must be at least 32 characters for security (OWASP recommendation), current length: %d", len(secretKey))
	}
	
	return &JWTConfig{
		SecretKey: secretKey,
		Issuer:    getEnvOrDefault("JWT_ISSUER", "identity-manager"),
		Audience:  getEnvOrDefault("JWT_AUDIENCE", "bedrock-proxy"),
	}, nil
}

// getEnvOrDefault retorna el valor de una variable de entorno o un valor por defecto
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// XMLBufferConfig contiene la configuración del buffer XML
type XMLBufferConfig struct {
	MaxBufferSize int
}

// LoadXMLBufferConfigWithEnv carga configuración del buffer XML desde variables de entorno
func LoadXMLBufferConfigWithEnv() *XMLBufferConfig {
	maxBufferSize := 3 // Valor por defecto: 3 caracteres (EXTREMADAMENTE REDUCIDO PARA TESTING)
	if bufferSizeStr := os.Getenv("XML_BUFFER_MAX_SIZE"); bufferSizeStr != "" {
		if size, err := strconv.Atoi(bufferSizeStr); err == nil && size > 0 {
			maxBufferSize = size
		}
	}

	return &XMLBufferConfig{
		MaxBufferSize: maxBufferSize,
	}
}

// DatabaseConnectionConfig contiene la configuración para conectar a la base de datos
type DatabaseConnectionConfig struct {
	UseSecretsManager bool
	SecretARN         string
	SSLMode           string
	MaxConns          int32
	MinConns          int32
	// Legacy: variables de entorno directas
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// LoadDatabaseConnectionConfig carga la configuración de conexión a BD
// Prioriza AWS Secrets Manager si DB_SECRET_ARN está configurado
func LoadDatabaseConnectionConfig() *DatabaseConnectionConfig {
	config := &DatabaseConnectionConfig{}
	
	// Verificar si se debe usar AWS Secrets Manager
	secretARN := os.Getenv("DB_SECRET_ARN")
	if secretARN != "" {
		config.UseSecretsManager = true
		config.SecretARN = secretARN
	}
	
	// Configuración de SSL
	config.SSLMode = "require"
	if sslModeStr := os.Getenv("DB_SSLMODE"); sslModeStr != "" {
		config.SSLMode = sslModeStr
	}
	
	// Configuración de pool
	config.MaxConns = 25
	if maxConnsStr := os.Getenv("DB_MAX_CONNS"); maxConnsStr != "" {
		if mc, err := strconv.Atoi(maxConnsStr); err == nil {
			config.MaxConns = int32(mc)
		}
	}
	
	config.MinConns = 5
	if minConnsStr := os.Getenv("DB_MIN_CONNS"); minConnsStr != "" {
		if mc, err := strconv.Atoi(minConnsStr); err == nil {
			config.MinConns = int32(mc)
		}
	}
	
	// Legacy: cargar desde variables de entorno si no se usa Secrets Manager
	if !config.UseSecretsManager {
		config.Host = os.Getenv("DB_HOST")
		config.Port = 5432
		if portStr := os.Getenv("DB_PORT"); portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil {
				config.Port = p
			}
		}
		config.Database = os.Getenv("DB_NAME")
		config.User = os.Getenv("DB_USER")
		config.Password = os.Getenv("DB_PASSWORD")
	}
	
	return config
}

// LoadDatabaseConfigWithEnv carga la configuración de base de datos desde variables de entorno (LEGACY)
// Deprecated: Use LoadDatabaseConnectionConfig() instead
func LoadDatabaseConfigWithEnv() *database.DatabaseConfig {
	port := 5432
	if portStr := os.Getenv("DB_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	maxConns := int32(25)
	if maxConnsStr := os.Getenv("DB_MAX_CONNS"); maxConnsStr != "" {
		if mc, err := strconv.Atoi(maxConnsStr); err == nil {
			maxConns = int32(mc)
		}
	}

	minConns := int32(5)
	if minConnsStr := os.Getenv("DB_MIN_CONNS"); minConnsStr != "" {
		if mc, err := strconv.Atoi(minConnsStr); err == nil {
			minConns = int32(mc)
		}
	}

	sslMode := "require"
	if sslModeStr := os.Getenv("DB_SSLMODE"); sslModeStr != "" {
		sslMode = sslModeStr
	}

	return &database.DatabaseConfig{
		Host:     os.Getenv("DB_HOST"),
		Port:     port,
		Database: os.Getenv("DB_NAME"),
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		SSLMode:  sslMode,
		MaxConns: maxConns,
		MinConns: minConns,
	}
}

// InitializeDatabase inicializa la conexión a la base de datos
// Usa AWS Secrets Manager si está configurado, sino usa variables de entorno
func InitializeDatabase(ctx context.Context) (*database.Database, error) {
	config := LoadDatabaseConnectionConfig()
	
	if config.UseSecretsManager {
		return database.NewDatabaseFromSecret(
			ctx,
			config.SecretARN,
			config.SSLMode,
			config.MaxConns,
			config.MinConns,
		)
	}
	
	// Legacy: usar variables de entorno
	if config.Host == "" || config.User == "" || config.Password == "" {
		return nil, fmt.Errorf("database configuration incomplete")
	}
	dbConfig := &database.DatabaseConfig{
		Host:     config.Host,
		Port:     config.Port,
		Database: config.Database,
		User:     config.User,
		Password: config.Password,
		SSLMode:  config.SSLMode,
		MaxConns: config.MaxConns,
		MinConns: config.MinConns,
	}
	
	return database.NewDatabase(dbConfig)
}
