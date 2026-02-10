package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TokenInfo contiene la información de un token JWT validado
type TokenInfo struct {
	JTI           string
	UserID        string
	Email         string
	Team          string
	Person        string
	Role          string
	IsRevoked     bool
	ExpiresAt     time.Time
	MonthlyQuota  float64
	DailyLimit    float64
	DailyReqLimit int
	InferenceProfile string
}

// QuotaInfo contiene información de uso de quotas
type QuotaInfo struct {
	UserID           string
	MonthlyQuotaUSD  float64
	DailyLimitUSD    float64
	DailyRequestLimit int
	MonthlyUsedUSD   float64
	MonthlyRequests  int
	DailyUsedUSD     float64
	DailyRequests    int
	IsBlocked        bool
	BlockedReason    string
}

// ValidateToken valida un token JWT contra la base de datos
func (db *Database) ValidateToken(ctx context.Context, tokenHash string) (*TokenInfo, error) {
	query := `
		SELECT 
			t.jti,
			t.user_id,
			u.email,
			u.team,
			u.person,
			u.role,
			t.is_revoked,
			t.expires_at,
			u.monthly_quota_usd,
			u.daily_limit_usd,
			u.daily_request_limit,
			u.default_inference_profile
		FROM tokens t
		JOIN users u ON t.user_id = u.iam_username
		WHERE t.token_hash = $1
			AND t.is_revoked = false
			AND t.expires_at > NOW()
			AND u.is_active = true
	`

	var info TokenInfo
	err := db.pool.QueryRow(ctx, query, tokenHash).Scan(
		&info.JTI,
		&info.UserID,
		&info.Email,
		&info.Team,
		&info.Person,
		&info.Role,
		&info.IsRevoked,
		&info.ExpiresAt,
		&info.MonthlyQuota,
		&info.DailyLimit,
		&info.DailyReqLimit,
		&info.InferenceProfile,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("token not found or invalid")
		}
		return nil, fmt.Errorf("error validating token: %w", err)
	}

	return &info, nil
}

// CheckQuota verifica si el usuario tiene quota disponible
func (db *Database) CheckQuota(ctx context.Context, userID string) (*QuotaInfo, error) {
	query := `
		SELECT 
			u.email,
			u.monthly_quota_usd,
			u.daily_limit_usd,
			u.daily_request_limit,
			COALESCE(qu.total_cost_usd, 0) as monthly_used_usd,
			COALESCE(qu.total_requests, 0) as monthly_requests,
			COALESCE(ubs.daily_cost_usd, 0) as daily_used_usd,
			COALESCE(ubs.daily_requests, 0) as daily_requests,
			COALESCE(ubs.is_blocked, false) as is_blocked,
			COALESCE(ubs.blocked_reason, '') as blocked_reason
		FROM users u
		LEFT JOIN quota_usage qu ON u.iam_username = qu.user_id 
			AND qu.month = DATE_TRUNC('month', CURRENT_DATE)
		LEFT JOIN user_blocking_status ubs ON u.iam_username = ubs.user_id
		WHERE u.iam_username = $1 AND u.is_active = true
	`

	var info QuotaInfo
	err := db.pool.QueryRow(ctx, query, userID).Scan(
		&info.UserID,
		&info.MonthlyQuotaUSD,
		&info.DailyLimitUSD,
		&info.DailyRequestLimit,
		&info.MonthlyUsedUSD,
		&info.MonthlyRequests,
		&info.DailyUsedUSD,
		&info.DailyRequests,
		&info.IsBlocked,
		&info.BlockedReason,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("error checking quota: %w", err)
	}

	return &info, nil
}

// MetricData contiene los datos de una métrica de request
type MetricData struct {
	UserID              string
	Team                string
	Person              string
	RequestTimestamp    time.Time
	ModelID             string
	RequestID           string
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

// InsertMetric inserta una métrica de request en la base de datos
func (db *Database) InsertMetric(ctx context.Context, metric *MetricData) error {
	query := `
		INSERT INTO request_metrics (
			user_id, team, person, request_timestamp, model_id, request_id,
			source_ip, user_agent, aws_region, tokens_input, tokens_output,
			tokens_cache_read, tokens_cache_creation, cost_usd, processing_time_ms,
			response_status, error_message
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		)
	`

	_, err := db.pool.Exec(ctx, query,
		metric.UserID,
		metric.Team,
		metric.Person,
		metric.RequestTimestamp,
		metric.ModelID,
		metric.RequestID,
		metric.SourceIP,
		metric.UserAgent,
		metric.AWSRegion,
		metric.TokensInput,
		metric.TokensOutput,
		metric.TokensCacheRead,
		metric.TokensCacheCreation,
		metric.CostUSD,
		metric.ProcessingTimeMS,
		metric.ResponseStatus,
		metric.ErrorMessage,
	)

	if err != nil {
		return fmt.Errorf("error inserting metric: %w", err)
	}

	return nil
}

// UpdateQuotaAndCounters actualiza las quotas y contadores después de un request
func (db *Database) UpdateQuotaAndCounters(ctx context.Context, userID string, costUSD float64) error {
	// Iniciar transacción
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Actualizar quota_usage (agregado mensual)
	queryQuota := `
		INSERT INTO quota_usage (user_id, month, total_cost_usd, total_requests, last_updated)
		VALUES ($1, DATE_TRUNC('month', CURRENT_DATE), $2, 1, NOW())
		ON CONFLICT (user_id, month) 
		DO UPDATE SET 
			total_cost_usd = quota_usage.total_cost_usd + $2,
			total_requests = quota_usage.total_requests + 1,
			last_updated = NOW()
	`
	_, err = tx.Exec(ctx, queryQuota, userID, costUSD)
	if err != nil {
		return fmt.Errorf("error updating quota_usage: %w", err)
	}

	// Actualizar user_blocking_status (contadores diarios)
	queryBlocking := `
		INSERT INTO user_blocking_status (user_id, daily_cost_usd, daily_requests, last_request_at, updated_at)
		VALUES ($1, $2, 1, NOW(), NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET
			daily_cost_usd = user_blocking_status.daily_cost_usd + $2,
			daily_requests = user_blocking_status.daily_requests + 1,
			last_request_at = NOW(),
			updated_at = NOW()
	`
	_, err = tx.Exec(ctx, queryBlocking, userID, costUSD)
	if err != nil {
		return fmt.Errorf("error updating user_blocking_status: %w", err)
	}

	// Commit de la transacción
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

// CheckAndBlockUser verifica si el usuario debe ser bloqueado por exceder límites
func (db *Database) CheckAndBlockUser(ctx context.Context, userID string) error {
	query := `
		UPDATE user_blocking_status ubs
		SET 
			is_blocked = true,
			blocked_at = NOW(),
			blocked_reason = CASE
				WHEN ubs.daily_cost_usd >= u.daily_limit_usd THEN 'Daily cost limit exceeded'
				WHEN ubs.daily_requests >= u.daily_request_limit THEN 'Daily request limit exceeded'
				ELSE 'Limit exceeded'
			END,
			requests_at_blocking = ubs.daily_requests,
			updated_at = NOW()
		FROM users u
		WHERE ubs.user_id = u.iam_username
			AND ubs.user_id = $1
			AND ubs.is_blocked = false
			AND (
				ubs.daily_cost_usd >= u.daily_limit_usd
				OR ubs.daily_requests >= u.daily_request_limit
			)
	`

	_, err := db.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("error checking/blocking user: %w", err)
	}

	return nil
}
