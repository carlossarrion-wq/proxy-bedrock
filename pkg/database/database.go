package database

import (
	"context"
	"fmt"
	"time"

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

// ResetResult contiene los resultados del reset diario
type ResetResult struct {
	UsersReset     int
	UsersUnblocked int
	CountersReset  int
}

// ResetDailyCounters resetea los contadores diarios y desbloquea usuarios
func (db *Database) ResetDailyCounters(ctx context.Context) (*ResetResult, error) {
	// Iniciar transacción
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	result := &ResetResult{}

	// 1. Contar usuarios que serán desbloqueados
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM user_blocking_status
		WHERE is_blocked = true 
		  AND last_request_at < CURRENT_DATE
		  AND blocked_by_admin_id IS NULL
	`).Scan(&result.UsersUnblocked)
	if err != nil {
		return nil, fmt.Errorf("error counting users to unblock: %w", err)
	}

	// 2. Resetear contadores y desbloquear usuarios con bloqueo automático
	cmdTag, err := tx.Exec(ctx, `
		UPDATE user_blocking_status
		SET daily_requests = 0,
		    daily_cost_usd = 0.0,
		    is_blocked = false,
		    blocked_reason = NULL,
		    blocked_at = NULL,
		    blocked_until = NULL,
		    last_reset_at = NOW(),
		    updated_at = NOW()
		WHERE last_request_at < CURRENT_DATE
		  AND blocked_by_admin_id IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("error resetting counters: %w", err)
	}
	result.CountersReset = int(cmdTag.RowsAffected())

	// 3. Contar usuarios afectados
	err = tx.QueryRow(ctx, `
		SELECT COUNT(DISTINCT user_id)
		FROM user_blocking_status
		WHERE last_reset_at::date = CURRENT_DATE
	`).Scan(&result.UsersReset)
	if err != nil {
		return nil, fmt.Errorf("error counting reset users: %w", err)
	}

	// Commit de la transacción
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error committing transaction: %w", err)
	}

	return result, nil
}
