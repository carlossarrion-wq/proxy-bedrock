package database

import (
	"context"
	"fmt"
	"time"
)

// QuotaCheckResult contiene el resultado de la verificación de cuota
type QuotaCheckResult struct {
	Allowed       bool
	RequestsToday int
	DailyLimit    int
	IsBlocked     bool
	BlockReason   string
}

// QuotaStatus contiene el estado completo de cuota de un usuario
type QuotaStatus struct {
	CognitoUserID      string
	CognitoEmail       string
	DailyLimit         int
	RequestsToday      int
	RemainingRequests  int
	UsagePercentage    float64
	IsBlocked          bool
	BlockedAt          *time.Time
	AdministrativeSafe bool
	LastRequestAt      *time.Time
}

// UsageTrackingData contiene los datos para registrar el uso de una petición
type UsageTrackingData struct {
	CognitoUserID       string
	CognitoEmail        string
	Team                string    // Team from JWT token
	Person              string    // Person from JWT token
	RequestTimestamp    time.Time
	ModelID             string
	SourceIP            string
	UserAgent           string
	AWSRegion           string
	TokensInput         int
	TokensOutput        int
	TokensCacheRead     int
	TokensCacheCreation int
	CostUSD             float64
	ProcessingTimeMS    int
	ResponseStatus      string
	ErrorMessage        string
}

// CheckAndUpdateQuota verifica la cuota del usuario e incrementa el contador
// Esta es la función principal que debe llamarse en cada petición
func (db *Database) CheckAndUpdateQuota(ctx context.Context, cognitoUserID, cognitoEmail, team string) (*QuotaCheckResult, error) {
	query := `SELECT * FROM check_and_update_quota($1, $2, $3)`
	
	var result QuotaCheckResult
	var blockReason *string
	
	err := db.pool.QueryRow(ctx, query, cognitoUserID, cognitoEmail, team).Scan(
		&result.Allowed,
		&result.RequestsToday,
		&result.DailyLimit,
		&result.IsBlocked,
		&blockReason,
	)
	
	if err != nil {
		return nil, fmt.Errorf("error checking quota: %w", err)
	}
	
	if blockReason != nil {
		result.BlockReason = *blockReason
	}
	
	return &result, nil
}

// GetUserQuotaStatus obtiene el estado completo de cuota de un usuario
func (db *Database) GetUserQuotaStatus(ctx context.Context, cognitoUserID string) (*QuotaStatus, error) {
	query := `SELECT * FROM get_user_quota_status($1)`
	
	var status QuotaStatus
	
	err := db.pool.QueryRow(ctx, query, cognitoUserID).Scan(
		&status.CognitoUserID,
		&status.CognitoEmail,
		&status.DailyLimit,
		&status.RequestsToday,
		&status.RemainingRequests,
		&status.UsagePercentage,
		&status.IsBlocked,
		&status.BlockedAt,
		&status.AdministrativeSafe,
		&status.LastRequestAt,
	)
	
	if err != nil {
		return nil, fmt.Errorf("error getting quota status: %w", err)
	}
	
	return &status, nil
}

// AdministrativeUnblockUser desbloquea un usuario administrativamente
// Activa el flag administrative_safe que permite al usuario continuar hasta medianoche
func (db *Database) AdministrativeUnblockUser(ctx context.Context, cognitoUserID, adminUserID, reason string) error {
	query := `SELECT administrative_unblock_user($1, $2, $3)`
	
	var success bool
	err := db.pool.QueryRow(ctx, query, cognitoUserID, adminUserID, reason).Scan(&success)
	
	if err != nil {
		return fmt.Errorf("error unblocking user: %w", err)
	}
	
	if !success {
		return fmt.Errorf("failed to unblock user %s", cognitoUserID)
	}
	
	return nil
}

// UpdateUserDailyLimit actualiza el límite diario de peticiones de un usuario
func (db *Database) UpdateUserDailyLimit(ctx context.Context, cognitoUserID string, newLimit int) error {
	if newLimit < 0 {
		return fmt.Errorf("daily limit must be >= 0")
	}
	
	query := `SELECT update_user_daily_limit($1, $2)`
	
	var success bool
	err := db.pool.QueryRow(ctx, query, cognitoUserID, newLimit).Scan(&success)
	
	if err != nil {
		return fmt.Errorf("error updating daily limit: %w", err)
	}
	
	if !success {
		return fmt.Errorf("failed to update daily limit for user %s", cognitoUserID)
	}
	
	return nil
}

// AdministrativeBlockUser bloquea un usuario administrativamente hasta una fecha específica
// Permite bloqueos de múltiples días
func (db *Database) AdministrativeBlockUser(ctx context.Context, cognitoUserID, adminUserID string, blockUntil time.Time, reason string) error {
	if blockUntil.Before(time.Now()) {
		return fmt.Errorf("block until date must be in the future")
	}
	
	query := `SELECT administrative_block_user($1, $2, $3, $4)`
	
	var success bool
	err := db.pool.QueryRow(ctx, query, cognitoUserID, adminUserID, blockUntil, reason).Scan(&success)
	
	if err != nil {
		return fmt.Errorf("error blocking user: %w", err)
	}
	
	if !success {
		return fmt.Errorf("failed to block user %s", cognitoUserID)
	}
	
	return nil
}

// InsertUsageTracking registra el uso detallado de una petición
// Esta función debe llamarse de manera asíncrona después de procesar la petición
func (db *Database) InsertUsageTracking(ctx context.Context, data *UsageTrackingData) error {
	query := `
		INSERT INTO "bedrock-proxy-usage-tracking-tbl" (
			cognito_user_id,
			cognito_email,
			team,
			person,
			request_timestamp,
			model_id,
			source_ip,
			user_agent,
			aws_region,
			tokens_input,
			tokens_output,
			tokens_cache_read,
			tokens_cache_creation,
			cost_usd,
			processing_time_ms,
			response_status,
			error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	
	_, err := db.pool.Exec(ctx, query,
		data.CognitoUserID,
		data.CognitoEmail,
		data.Team,
		data.Person,
		data.RequestTimestamp,
		data.ModelID,
		data.SourceIP,
		data.UserAgent,
		data.AWSRegion,
		data.TokensInput,
		data.TokensOutput,
		data.TokensCacheRead,
		data.TokensCacheCreation,
		data.CostUSD,
		data.ProcessingTimeMS,
		data.ResponseStatus,
		data.ErrorMessage,
	)
	
	if err != nil {
		return fmt.Errorf("error inserting usage tracking: %w", err)
	}
	
	return nil
}

// GetBlockedUsers obtiene la lista de usuarios actualmente bloqueados
func (db *Database) GetBlockedUsers(ctx context.Context) ([]QuotaStatus, error) {
	query := `
		SELECT 
			cognito_user_id,
			cognito_email,
			COALESCE(daily_request_limit, 
				(SELECT config_value::INTEGER FROM "identity-manager-config-tbl" 
				 WHERE config_key = 'default_daily_request_limit'), 
				1000) as daily_limit,
			requests_today,
			blocked_at,
			administrative_safe
		FROM "bedrock-proxy-user-quotas-tbl"
		WHERE is_blocked = true
		ORDER BY blocked_at DESC
	`
	
	rows, err := db.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error querying blocked users: %w", err)
	}
	defer rows.Close()
	
	var users []QuotaStatus
	for rows.Next() {
		var user QuotaStatus
		err := rows.Scan(
			&user.CognitoUserID,
			&user.CognitoEmail,
			&user.DailyLimit,
			&user.RequestsToday,
			&user.BlockedAt,
			&user.AdministrativeSafe,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning blocked user: %w", err)
		}
		user.IsBlocked = true
		users = append(users, user)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blocked users: %w", err)
	}
	
	return users, nil
}

// GetUsersNearLimit obtiene usuarios que han usado más del 80% de su cuota
func (db *Database) GetUsersNearLimit(ctx context.Context) ([]QuotaStatus, error) {
	query := `SELECT * FROM "v_users_near_limit"`
	
	rows, err := db.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error querying users near limit: %w", err)
	}
	defer rows.Close()
	
	var users []QuotaStatus
	for rows.Next() {
		var user QuotaStatus
		var remaining int
		var usagePct float64
		
		err := rows.Scan(
			&user.CognitoUserID,
			&user.CognitoEmail,
			&user.RequestsToday,
			&user.DailyLimit,
			&remaining,
			&usagePct,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning user near limit: %w", err)
		}
		
		user.RemainingRequests = remaining
		user.UsagePercentage = usagePct
		users = append(users, user)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users near limit: %w", err)
	}
	
	return users, nil
}