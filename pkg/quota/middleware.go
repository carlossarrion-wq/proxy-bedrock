package quota

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"bedrock-proxy-test/pkg/auth"
	"bedrock-proxy-test/pkg/database"
)

// QuotaMiddleware es el middleware de control de quotas
type QuotaMiddleware struct {
	db *database.Database
}

// NewQuotaMiddleware crea una nueva instancia del middleware de quotas
func NewQuotaMiddleware(db *database.Database) *QuotaMiddleware {
	return &QuotaMiddleware{
		db: db,
	}
}

// Middleware es el handler HTTP que verifica las quotas del usuario
func (qm *QuotaMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Obtener información del usuario del contexto (debe estar autenticado)
		user, err := auth.GetUserFromContext(r.Context())
		if err != nil {
			qm.respondError(w, http.StatusUnauthorized, "user not authenticated")
			return
		}

		// Verificar quotas del usuario
		quotaInfo, err := qm.db.CheckQuota(r.Context(), user.UserID)
		if err != nil {
			qm.respondError(w, http.StatusInternalServerError, fmt.Sprintf("error checking quota: %v", err))
			return
		}

		// Verificar si el usuario está bloqueado
		if quotaInfo.IsBlocked {
			qm.respondError(w, http.StatusForbidden, "user is blocked due to quota limits exceeded")
			return
		}

		// Verificar límite diario de coste
		if quotaInfo.DailyUsedUSD >= quotaInfo.DailyLimitUSD {
			qm.respondError(w, http.StatusTooManyRequests, "daily cost limit exceeded")
			return
		}

		// Verificar límite diario de requests
		if quotaInfo.DailyRequests >= quotaInfo.DailyRequestLimit {
			qm.respondError(w, http.StatusTooManyRequests, "daily request limit exceeded")
			return
		}

		// Verificar límite mensual de coste
		if quotaInfo.MonthlyUsedUSD >= quotaInfo.MonthlyQuotaUSD {
			qm.respondError(w, http.StatusTooManyRequests, "monthly quota exceeded")
			return
		}

		// Añadir información de quota al contexto para uso posterior
		ctx := context.WithValue(r.Context(), QuotaInfoKey, quotaInfo)

		// NO añadir headers para requests de streaming (interfiere con el streaming)
		// Los headers se pueden añadir después si es necesario
		// Para requests no-streaming, añadir headers informativos
		// NOTA: Comentado temporalmente para no interferir con streaming
		// qm.addQuotaHeaders(w, quotaInfo)

		// Continuar con el siguiente handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// addQuotaHeaders añade headers HTTP con información de quotas
func (qm *QuotaMiddleware) addQuotaHeaders(w http.ResponseWriter, quota *database.QuotaInfo) {
	// Headers de quota mensual
	w.Header().Set("X-Quota-Monthly-Limit", fmt.Sprintf("%.2f", quota.MonthlyQuotaUSD))
	w.Header().Set("X-Quota-Monthly-Used", fmt.Sprintf("%.2f", quota.MonthlyUsedUSD))
	w.Header().Set("X-Quota-Monthly-Remaining", fmt.Sprintf("%.2f", quota.MonthlyQuotaUSD-quota.MonthlyUsedUSD))
	w.Header().Set("X-Quota-Monthly-Percent", fmt.Sprintf("%.1f", (quota.MonthlyUsedUSD/quota.MonthlyQuotaUSD)*100))

	// Headers de límite diario de coste
	w.Header().Set("X-Quota-Daily-Limit", fmt.Sprintf("%.2f", quota.DailyLimitUSD))
	w.Header().Set("X-Quota-Daily-Used", fmt.Sprintf("%.2f", quota.DailyUsedUSD))
	w.Header().Set("X-Quota-Daily-Remaining", fmt.Sprintf("%.2f", quota.DailyLimitUSD-quota.DailyUsedUSD))
	w.Header().Set("X-Quota-Daily-Percent", fmt.Sprintf("%.1f", (quota.DailyUsedUSD/quota.DailyLimitUSD)*100))

	// Headers de límite diario de requests
	w.Header().Set("X-Quota-Requests-Limit", strconv.Itoa(quota.DailyRequestLimit))
	w.Header().Set("X-Quota-Requests-Used", strconv.Itoa(quota.DailyRequests))
	w.Header().Set("X-Quota-Requests-Remaining", strconv.Itoa(quota.DailyRequestLimit-quota.DailyRequests))
	w.Header().Set("X-Quota-Requests-Percent", fmt.Sprintf("%.1f", (float64(quota.DailyRequests)/float64(quota.DailyRequestLimit))*100))

	// Header de estado de bloqueo
	if quota.IsBlocked {
		w.Header().Set("X-Quota-Status", "blocked")
	} else {
		w.Header().Set("X-Quota-Status", "active")
	}
}

// respondError envía una respuesta de error en formato JSON
func (qm *QuotaMiddleware) respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"error":"%s"}`, message)
}

// QuotaContextKey es la clave para almacenar información de quota en el contexto
type QuotaContextKey string

const (
	// QuotaInfoKey es la clave para la información de quota en el contexto
	QuotaInfoKey QuotaContextKey = "quota_info"
)

// GetQuotaFromContext extrae la información de quota del contexto
func GetQuotaFromContext(ctx context.Context) (*database.QuotaInfo, error) {
	quota, ok := ctx.Value(QuotaInfoKey).(*database.QuotaInfo)
	if !ok {
		return nil, fmt.Errorf("quota info not found in context")
	}
	return quota, nil
}

// UpdateQuotaAfterRequest actualiza las quotas y contadores después de procesar un request
func (qm *QuotaMiddleware) UpdateQuotaAfterRequest(ctx context.Context, userID string, costUSD float64) error {
	// Actualizar quotas y contadores en transacción
	if err := qm.db.UpdateQuotaAndCounters(ctx, userID, costUSD); err != nil {
		return fmt.Errorf("error updating quota: %w", err)
	}

	// Verificar si el usuario debe ser bloqueado
	if err := qm.db.CheckAndBlockUser(ctx, userID); err != nil {
		return fmt.Errorf("error checking user block status: %w", err)
	}

	return nil
}
