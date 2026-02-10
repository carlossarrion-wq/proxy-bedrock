package pkg

import (
	"sync"
	"time"
)

// RequestContext mantiene el contexto y timing de una request
type RequestContext struct {
	RequestID    string
	StartTime    time.Time
	PhaseTimings map[string]time.Duration
	mu           sync.RWMutex
}

// NewRequestContext crea un nuevo contexto de request
func NewRequestContext(requestID string) *RequestContext {
	return &RequestContext{
		RequestID:    requestID,
		StartTime:    time.Now(),
		PhaseTimings: make(map[string]time.Duration),
	}
}

// StartPhase inicia el tracking de una fase y retorna una función para finalizarla
func (rc *RequestContext) StartPhase(phase string) func() {
	start := time.Now()
	return func() {
		rc.mu.Lock()
		defer rc.mu.Unlock()
		rc.PhaseTimings[phase] = time.Since(start)
	}
}

// GetTotalDuration retorna la duración total desde el inicio
func (rc *RequestContext) GetTotalDuration() time.Duration {
	return time.Since(rc.StartTime)
}

// LogSummary loguea un resumen de todos los timings
func (rc *RequestContext) LogSummary() {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	
	totalMs := rc.GetTotalDuration().Milliseconds()
	
	// Construir string con timings de cada fase
	phaseStr := ""
	for phase, duration := range rc.PhaseTimings {
		if phaseStr != "" {
			phaseStr += ", "
		}
		phaseStr += phase + "=" + duration.String()
	}
	
	Log.Infof("[REQUEST_SUMMARY] id=%s total=%dms phases=[%s]",
		rc.RequestID, totalMs, phaseStr)
}