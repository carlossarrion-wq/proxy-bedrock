package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DatabaseConfig contiene la configuración para conectar a PostgreSQL
type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"` // require, verify-full, disable
	MaxConns int32  `json:"max_conns"`
	MinConns int32  `json:"min_conns"`
}

// Database representa la conexión al pool de PostgreSQL
type Database struct {
	pool   *pgxpool.Pool
	config *DatabaseConfig
}

// DBSecret representa la estructura del secreto en AWS Secrets Manager
type DBSecret struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"dbname"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// NewDatabaseFromSecret crea una nueva instancia de Database usando AWS Secrets Manager
func NewDatabaseFromSecret(ctx context.Context, secretARN string, sslMode string, maxConns, minConns int32) (*Database, error) {
	// Cargar configuración de AWS
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading AWS config: %w", err)
	}

	// Crear cliente de Secrets Manager
	client := secretsmanager.NewFromConfig(cfg)

	// Obtener el secreto
	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretARN),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting secret from AWS Secrets Manager: %w", err)
	}

	// Parsear el secreto
	var dbSecret DBSecret
	if err := json.Unmarshal([]byte(*result.SecretString), &dbSecret); err != nil {
		return nil, fmt.Errorf("error parsing database secret: %w", err)
	}

	// Crear DatabaseConfig desde el secreto
	dbConfig := &DatabaseConfig{
		Host:     dbSecret.Host,
		Port:     dbSecret.Port,
		Database: dbSecret.Database,
		User:     dbSecret.Username,
		Password: dbSecret.Password,
		SSLMode:  sslMode,
		MaxConns: maxConns,
		MinConns: minConns,
	}

	// Crear la conexión usando la configuración
	return NewDatabase(dbConfig)
}

// NewDatabase crea una nueva instancia de Database y establece la conexión
func NewDatabase(config *DatabaseConfig) (*Database, error) {
	// Construir connection string
	connString := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s pool_max_conns=%d pool_min_conns=%d",
		config.Host,
		config.Port,
		config.Database,
		config.User,
		config.Password,
		config.SSLMode,
		config.MaxConns,
		config.MinConns,
	)

	// Configurar el pool
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("error parsing database config: %w", err)
	}

	// Configurar timeouts
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	// Crear el pool de conexiones
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating connection pool: %w", err)
	}

	// Verificar la conexión
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("error pinging database: %w", err)
	}

	return &Database{
		pool:   pool,
		config: config,
	}, nil
}

// Close cierra el pool de conexiones
func (db *Database) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// GetPool retorna el pool de conexiones para uso directo
func (db *Database) GetPool() *pgxpool.Pool {
	return db.pool
}

// Ping verifica que la conexión esté activa
func (db *Database) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Stats retorna estadísticas del pool de conexiones
func (db *Database) Stats() *pgxpool.Stat {
	return db.pool.Stat()
}

// GetSecretFromSecretsManager obtiene un secreto de AWS Secrets Manager
// Retorna el valor del secreto como string
func GetSecretFromSecretsManager(ctx context.Context, secretARN string) (string, error) {
	// Crear cliente de Secrets Manager
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}
	
	client := secretsmanager.NewFromConfig(cfg)
	
	// Obtener el secreto
	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretARN),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get secret value: %w", err)
	}
	
	// Retornar el valor del secreto
	if result.SecretString != nil {
		return *result.SecretString, nil
	}
	
	return "", fmt.Errorf("secret does not contain a string value")
}
