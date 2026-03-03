package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bedrock-proxy-test/pkg/amslog"
	"bedrock-proxy-test/pkg/database"
)

// Logger es una referencia al logger global que debe ser configurada desde main
var Logger *amslog.Logger

// ContextKey es el tipo para las claves del contexto
type ContextKey string

const (
	// UserContextKey es la clave para almacenar información del usuario en el contexto
	UserContextKey ContextKey = "user"
)

// UserContext contiene la información del usuario autenticado
type UserContext struct {
	UserID                  string
	Email                   string
	IAMUsername             string
	IAMGroups               []string
	DefaultInferenceProfile string
	Team                    string
	Person                  string
	JTI                     string
}

// AuthMiddleware es el middleware de autenticación JWT
type AuthMiddleware struct {
	jwtConfig     JWTConfig
	db            *database.Database
	rateLimiter   *RateLimiter
	metricsWorker interface{
		RecordUsageTracking(data *database.UsageTrackingData) error
	}
}

// NewAuthMiddleware crea una nueva instancia del middleware de autenticación
func NewAuthMiddleware(db *database.Database, jwtConfig JWTConfig) *AuthMiddleware {
	return &AuthMiddleware{
		jwtConfig:   jwtConfig,
		db:          db,
		rateLimiter: NewRateLimiter(),
	}
}

// SetMetricsWorker establece el MetricsWorker para registro de errores tempranos
func (am *AuthMiddleware) SetMetricsWorker(mw interface{
	RecordUsageTracking(data *database.UsageTrackingData) error
}) {
	am.metricsWorker = mw
}

// Middleware es el handler HTTP que valida el JWT
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. RATE LIMITING: Verificar límite de intentos por IP
		clientIP := getClientIP(r)
		allowed, retryAfter := am.rateLimiter.CheckIP(clientIP)
		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			am.respondError(w, r, http.StatusUnauthorized, 
				fmt.Sprintf("too many authentication attempts from IP %s, please try again in %.0f seconds", 
					clientIP, retryAfter.Seconds()), "rate_limit_ip")
			return
		}

		var tokenString string
		var err error

		// Intentar extraer token del header Authorization (formato: Bearer <token>)
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			tokenString, err = ExtractBearerToken(authHeader)
			if err != nil {
				am.rateLimiter.RecordFailedAttempt(clientIP, "")
				am.respondError(w, r, http.StatusUnauthorized, fmt.Sprintf("invalid authorization header: %v", err), "invalid_header")
				return
			}
		} else {
			// Si no hay Authorization, intentar con x-api-key (formato usado por Cline)
			apiKey := r.Header.Get("x-api-key")
			if apiKey == "" {
				am.rateLimiter.RecordFailedAttempt(clientIP, "")
				am.respondError(w, r, http.StatusUnauthorized, "missing authorization header or x-api-key", "missing_auth")
				return
			}
			tokenString = apiKey
		}

		// Calcular hash del token para rate limiting y búsqueda en BD
		tokenHash := HashToken(tokenString)

		// 2. RATE LIMITING: Verificar límite de intentos por token
		allowed, retryAfter = am.rateLimiter.CheckToken(tokenHash)
		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			am.respondError(w, r, http.StatusUnauthorized, 
				fmt.Sprintf("too many authentication attempts with this token, please try again in %.0f seconds", 
					retryAfter.Seconds()), "rate_limit_token", tokenString)
			return
		}

		// Validar firma y estructura del JWT
		claims, err := ValidateToken(tokenString, am.jwtConfig.SecretKey)
		if err != nil {
			am.rateLimiter.RecordFailedAttempt(clientIP, tokenHash)
			
			// Registrar error de token inválido
			am.RecordEarlyError(r, "", "", "token_invalid", fmt.Sprintf("invalid token: %v", err))
			
			am.respondError(w, r, http.StatusUnauthorized, fmt.Sprintf("invalid token: %v", err), "token_invalid", tokenString)
			return
		}

		// Validar token contra base de datos
		tokenInfo, err := am.db.ValidateToken(r.Context(), tokenHash)
		if err != nil {
			am.rateLimiter.RecordFailedAttempt(clientIP, tokenHash)
			am.respondError(w, r, http.StatusUnauthorized, fmt.Sprintf("token validation failed: %v", err), "token_validation_failed", tokenString)
			return
		}

		// Verificar que el token no esté revocado
		if tokenInfo.IsRevoked {
			am.rateLimiter.RecordFailedAttempt(clientIP, tokenHash)
			am.respondError(w, r, http.StatusUnauthorized, "token has been revoked", "token_revoked", tokenString)
			return
		}

		// Verificar que el user_id del token coincida con el de los claims
		if tokenInfo.UserID != claims.UserID {
			am.rateLimiter.RecordFailedAttempt(clientIP, tokenHash)
			am.respondError(w, r, http.StatusUnauthorized, "token user mismatch", "token_user_mismatch", tokenString)
			return
		}

		// 3. AUTENTICACIÓN EXITOSA: Registrar intento exitoso
		am.rateLimiter.RecordSuccessfulAttempt(clientIP)

		// 4. VERIFICACIÓN DE CUOTA DIARIA
		// Verificar y actualizar la cuota del usuario
		quotaResult, err := am.db.CheckAndUpdateQuota(r.Context(), claims.UserID, claims.Email)
		if err != nil {
			am.respondError(w, r, http.StatusInternalServerError, 
				fmt.Sprintf("error checking quota: %v", err), "quota_check_error", tokenString)
			return
		}

		// Si la cuota está excedida, retornar 401 Unauthorized (para compatibilidad con clientes)
		// Nota: Usamos 401 en lugar de 429 porque algunos clientes no interpretan bien 429
		if !quotaResult.Allowed {
			// Añadir headers de rate limit
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", quotaResult.DailyLimit))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", getNextMidnightUTC())
			w.Header().Set("Retry-After", getSecondsUntilMidnightUTC())
			
			// Log del bloqueo por cuota
			if Logger != nil {
				Logger.WarningContext(r.Context(), amslog.Event{
					Name:    "QUOTA_EXCEEDED",
					Message: "Daily quota limit exceeded",
					Outcome: amslog.OutcomeFailure,
					Fields: map[string]interface{}{
						"user.id":           claims.UserID,
						"user.email":        claims.Email,
						"quota.limit":       quotaResult.DailyLimit,
						"quota.used":        quotaResult.RequestsToday,
						"quota.is_blocked":  quotaResult.IsBlocked,
						"quota.block_reason": quotaResult.BlockReason,
						"client.ip":         clientIP,
					},
				})
			}
			
			// Registrar error de cuota excedida
			am.RecordEarlyError(r, claims.UserID, claims.Email, "quota_exceeded", quotaResult.BlockReason)
			
			// Usar 401 para compatibilidad con clientes que no manejan bien 429
			am.respondError(w, r, http.StatusUnauthorized, 
				quotaResult.BlockReason, "quota_exceeded", tokenString)
			return
		}

		// Añadir headers de rate limit para peticiones exitosas
		remaining := quotaResult.DailyLimit - quotaResult.RequestsToday
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", quotaResult.DailyLimit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", getNextMidnightUTC())

		// Crear contexto de usuario
		// IMPORTANTE: Team y Person se extraen de los claims del JWT, no de la BD
		userCtx := UserContext{
			UserID:                  claims.UserID,
			Email:                   claims.Email,
			IAMUsername:             claims.IAMUsername,
			IAMGroups:               claims.IAMGroups,
			DefaultInferenceProfile: tokenInfo.InferenceProfile, // Usar el de la BD (model_arn)
			Team:                    claims.Team,                 // Del JWT
			Person:                  claims.Person,               // Del JWT
			JTI:                     claims.ID,
		}

		// Registrar evento de autenticación exitosa en formato JSON estructurado
		if Logger != nil {
			Logger.InfoContext(r.Context(), amslog.Event{
				Name:    "AUTH_SUCCESS",
				Message: "User authenticated successfully",
				Outcome: amslog.OutcomeSuccess,
				Fields: map[string]interface{}{
					"user.id":                userCtx.UserID,
					"user.email":             userCtx.Email,
					"user.iam_username":      userCtx.IAMUsername,
					"user.team":              userCtx.Team,
					"user.person":            userCtx.Person,
					"user.jti":               userCtx.JTI,
					"user.inference_profile": userCtx.DefaultInferenceProfile,
					"client.ip":              clientIP,
					"http.request.path":      r.URL.Path,
				},
			})
		}

		// Añadir información del usuario al contexto de la request
		ctx := context.WithValue(r.Context(), UserContextKey, userCtx)
		
		// Añadir inference_profile al contexto para que bedrock.go lo use
		if claims.DefaultInferenceProfile != "" {
			ctx = context.WithValue(ctx, "inference_profile", claims.DefaultInferenceProfile)
		}

		// Continuar con el siguiente handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// respondError envía una respuesta de error en formato JSON y registra el evento
func (am *AuthMiddleware) respondError(w http.ResponseWriter, r *http.Request, statusCode int, message string, errorType string, token ...string) {
	// Extraer información del contexto si está disponible
	var requestID string
	var traceID string
	if ctx := r.Context(); ctx != nil {
		if rid := amslog.RequestIDFromContext(ctx); rid != "" {
			requestID = rid
		}
		if tid := amslog.TraceIDFromContext(ctx); tid != "" {
			traceID = tid
		}
	}

	clientIP := getClientIP(r)

	// Registrar evento de autenticación fallida
	event := amslog.Event{
		Name:    "AUTH_FAILURE",
		Message: message,
		Outcome: amslog.OutcomeFailure,
		Fields: map[string]interface{}{
			"client.ip":         clientIP,
			"error.type":        errorType,
			"http.status_code":  statusCode,
			"http.request.path": r.URL.Path,
		},
	}

	if requestID != "" {
		event.Fields["request.id"] = requestID
	}
	if traceID != "" {
		event.Fields["trace.id"] = traceID
	}
	
	// Agregar token si está disponible
	if len(token) > 0 && token[0] != "" {
		event.Fields["auth.token"] = token[0]
	}

	// Log estructurado
	if Logger != nil {
		Logger.WarningContext(r.Context(), event)
	}

	// Construir respuesta de error más detallada
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
			"code":    statusCode,
		},
	}
	
	// Añadir información adicional para errores de cuota
	if errorType == "quota_exceeded" {
		errorResponse["error"].(map[string]interface{})["retry_after"] = getSecondsUntilMidnightUTC()
		errorResponse["error"].(map[string]interface{})["reset_at"] = getNextMidnightUTC()
	}

	// Serializar a JSON
	jsonResponse, err := json.Marshal(errorResponse)
	if err != nil {
		// Fallback a respuesta simple si falla la serialización
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, `{"error":{"message":"%s","type":"%s","code":%d}}`, message, errorType, statusCode)
		return
	}

	// Responder al cliente
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write(jsonResponse)
	
	// Forzar flush si el ResponseWriter lo soporta
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// GetUserFromContext extrae la información del usuario del contexto
func GetUserFromContext(ctx context.Context) (*UserContext, error) {
	user, ok := ctx.Value(UserContextKey).(UserContext)
	if !ok {
		return nil, fmt.Errorf("user not found in context")
	}
	return &user, nil
}

// RequireGroups es un middleware adicional que verifica que el usuario pertenezca a grupos específicos
func RequireGroups(requiredGroups []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := GetUserFromContext(r.Context())
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"user not authenticated"}`)
				return
			}

			// Verificar si el usuario pertenece a alguno de los grupos requeridos
			hasGroup := false
			for _, requiredGroup := range requiredGroups {
				for _, userGroup := range user.IAMGroups {
					if strings.EqualFold(userGroup, requiredGroup) {
						hasGroup = true
						break
					}
				}
				if hasGroup {
					break
				}
			}

			if !hasGroup {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, `{"error":"insufficient permissions"}`)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extrae la IP real del cliente considerando proxies y load balancers
func getClientIP(r *http.Request) string {
	// Intentar obtener IP real detrás de proxies/load balancers
	// X-Forwarded-For es el header estándar usado por proxies
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For puede contener múltiples IPs separadas por comas
		// La primera IP es la del cliente original
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// X-Real-IP es usado por algunos proxies (ej: nginx)
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fallback a RemoteAddr (IP directa sin proxy)
	ip := r.RemoteAddr
	// Remover puerto si existe (formato "IP:puerto")
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// getNextMidnightUTC retorna el timestamp de la próxima medianoche UTC en formato Unix
func getNextMidnightUTC() string {
	now := time.Now().UTC()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return fmt.Sprintf("%d", nextMidnight.Unix())
}

// getSecondsUntilMidnightUTC retorna los segundos hasta la próxima medianoche UTC
func getSecondsUntilMidnightUTC() string {
	now := time.Now().UTC()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	seconds := int(nextMidnight.Sub(now).Seconds())
	return fmt.Sprintf("%d", seconds)
}
