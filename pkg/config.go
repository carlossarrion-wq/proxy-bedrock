package pkg

import (
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

// LoadJWTConfigWithEnv carga configuración JWT desde variables de entorno
func LoadJWTConfigWithEnv() *JWTConfig {
	return &JWTConfig{
		SecretKey: os.Getenv("JWT_SECRET_KEY"),
		Issuer:    os.Getenv("JWT_ISSUER"),
		Audience:  os.Getenv("JWT_AUDIENCE"),
	}
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

// LoadDatabaseConfigWithEnv carga la configuración de base de datos desde variables de entorno
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
