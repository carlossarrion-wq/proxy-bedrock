package auth

import (
	"net/http"
	"time"

	"bedrock-proxy-test/pkg/amslog"
	"bedrock-proxy-test/pkg/database"
)

// RecordEarlyError registra errores que ocurren antes del streaming (JWT inválido, cuota excedida, etc.)
func (am *AuthMiddleware) RecordEarlyError(r *http.Request, userID, email, errorType, errorMessage string) {
	// Si no hay BD o MetricsWorker, no hacer nada
	if am.db == nil || am.metricsWorker == nil {
		return
	}
	
	// Obtener información del request
	sourceIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		sourceIP = forwarded
	}
	
	// Determinar model_id (puede ser "unknown" si no hay usuario válido)
	modelID := "unknown"
	if userCtx, err := GetUserFromContext(r.Context()); err == nil && userCtx.DefaultInferenceProfile != "" {
		modelID = userCtx.DefaultInferenceProfile
	}
	
	// Crear datos de tracking con error
	usageData := &database.UsageTrackingData{
		CognitoUserID:    userID,
		CognitoEmail:     email,
		RequestTimestamp: time.Now(),
		ModelID:          modelID,
		SourceIP:         sourceIP,
		UserAgent:        r.Header.Get("User-Agent"),
		AWSRegion:        "eu-west-1", // Región por defecto
		TokensInput:      0,
		TokensOutput:     0,
		TokensCacheRead:  0,
		TokensCacheCreation: 0,
		CostUSD:          0.0,
		ProcessingTimeMS: 0,
		ResponseStatus:   errorType, // "token_invalid", "quota_exceeded", etc.
		ErrorMessage:     errorMessage,
	}
	
	// Registrar de forma asíncrona
	go func() {
		if err := am.metricsWorker.RecordUsageTracking(usageData); err != nil {
			if Logger != nil {
				Logger.Error(amslog.Event{
					Name:    "AUTH_ERROR_TRACKING_FAILED",
					Message: "Failed to record early error",
					Error: &amslog.ErrorInfo{
						Type:    "TrackingError",
						Message: err.Error(),
					},
				})
			}
		}
	}()
}
