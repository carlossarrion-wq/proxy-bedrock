package auth

import (
	"fmt"
	"sync"
	"time"
)

// RateLimiter gestiona límites de intentos de autenticación
// Implementa protección contra brute force, credential stuffing y DoS
type RateLimiter struct {
	// Intentos por IP
	ipAttempts map[string]*IPAttempts

	// Intentos por token hash
	tokenAttempts map[string]*TokenAttempts

	// Mutex para acceso concurrente seguro
	mu sync.RWMutex

	// Configuración
	maxAttemptsPerIP    int
	maxAttemptsPerToken int
	blockDuration       time.Duration
	cleanupInterval     time.Duration
	windowDuration      time.Duration
}

// IPAttempts rastrea intentos de autenticación de una IP
type IPAttempts struct {
	Count        int
	FirstAttempt time.Time
	LastAttempt  time.Time
	BlockedUntil time.Time
}

// TokenAttempts rastrea intentos con un token específico
type TokenAttempts struct {
	Count        int
	FirstAttempt time.Time
	BlockedUntil time.Time
}

// NewRateLimiter crea un nuevo rate limiter con configuración por defecto
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		ipAttempts:          make(map[string]*IPAttempts),
		tokenAttempts:       make(map[string]*TokenAttempts),
		maxAttemptsPerIP:    10,              // 10 intentos por ventana de tiempo
		maxAttemptsPerToken: 5,               // 5 intentos por token
		blockDuration:       15 * time.Minute, // Bloqueo de 15 minutos
		cleanupInterval:     5 * time.Minute,  // Limpieza cada 5 minutos
		windowDuration:      1 * time.Minute,  // Ventana de 1 minuto
	}

	// Iniciar limpieza periódica en goroutine
	go rl.cleanupLoop()

	return rl
}

// NewRateLimiterWithConfig crea un rate limiter con configuración personalizada
func NewRateLimiterWithConfig(maxPerIP, maxPerToken int, blockDuration, windowDuration time.Duration) *RateLimiter {
	rl := &RateLimiter{
		ipAttempts:          make(map[string]*IPAttempts),
		tokenAttempts:       make(map[string]*TokenAttempts),
		maxAttemptsPerIP:    maxPerIP,
		maxAttemptsPerToken: maxPerToken,
		blockDuration:       blockDuration,
		cleanupInterval:     5 * time.Minute,
		windowDuration:      windowDuration,
	}

	go rl.cleanupLoop()

	return rl
}

// CheckIP verifica si una IP puede intentar autenticación
// Retorna (permitido, tiempo de espera hasta retry)
func (rl *RateLimiter) CheckIP(ip string) (allowed bool, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Obtener o crear registro de intentos
	attempts, exists := rl.ipAttempts[ip]
	if !exists {
		attempts = &IPAttempts{
			Count:        0,
			FirstAttempt: now,
			LastAttempt:  now,
		}
		rl.ipAttempts[ip] = attempts
	}

	// Verificar si está bloqueada
	if now.Before(attempts.BlockedUntil) {
		return false, attempts.BlockedUntil.Sub(now)
	}

	// Resetear contador si ha pasado la ventana de tiempo
	if now.Sub(attempts.FirstAttempt) > rl.windowDuration {
		attempts.Count = 0
		attempts.FirstAttempt = now
	}

	// Verificar límite
	if attempts.Count >= rl.maxAttemptsPerIP {
		// Bloquear por la duración configurada
		attempts.BlockedUntil = now.Add(rl.blockDuration)
		return false, rl.blockDuration
	}

	return true, 0
}

// CheckToken verifica si un token específico puede ser validado
func (rl *RateLimiter) CheckToken(tokenHash string) (allowed bool, retryAfter time.Duration) {
	if tokenHash == "" {
		return true, 0
	}

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()

	attempts, exists := rl.tokenAttempts[tokenHash]
	if !exists {
		return true, 0
	}

	// Verificar si el token está bloqueado
	if now.Before(attempts.BlockedUntil) {
		return false, attempts.BlockedUntil.Sub(now)
	}

	return true, 0
}

// RecordFailedAttempt registra un intento fallido de autenticación
func (rl *RateLimiter) RecordFailedAttempt(ip, tokenHash string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Registrar intento de IP
	if attempts, exists := rl.ipAttempts[ip]; exists {
		attempts.Count++
		attempts.LastAttempt = now
	} else {
		rl.ipAttempts[ip] = &IPAttempts{
			Count:        1,
			FirstAttempt: now,
			LastAttempt:  now,
		}
	}

	// Registrar intento de token si se proporciona
	if tokenHash != "" {
		tokenAttempts, exists := rl.tokenAttempts[tokenHash]
		if !exists {
			tokenAttempts = &TokenAttempts{
				Count:        0,
				FirstAttempt: now,
			}
			rl.tokenAttempts[tokenHash] = tokenAttempts
		}

		tokenAttempts.Count++

		// Bloquear token si excede límite
		if tokenAttempts.Count >= rl.maxAttemptsPerToken {
			tokenAttempts.BlockedUntil = now.Add(rl.blockDuration)
		}
	}
}

// RecordSuccessfulAttempt registra un intento exitoso de autenticación
// Resetea el contador de intentos para la IP
func (rl *RateLimiter) RecordSuccessfulAttempt(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Resetear contador de IP en caso de éxito
	if attempts, exists := rl.ipAttempts[ip]; exists {
		attempts.Count = 0
		attempts.FirstAttempt = time.Now()
	}
}

// cleanupLoop ejecuta limpieza periódica de registros antiguos
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup elimina registros antiguos para liberar memoria
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour) // Eliminar registros de más de 1 hora

	// Limpiar intentos de IP
	for ip, attempts := range rl.ipAttempts {
		// Eliminar si es antiguo y no está bloqueado
		if attempts.LastAttempt.Before(cutoff) && now.After(attempts.BlockedUntil) {
			delete(rl.ipAttempts, ip)
		}
	}

	// Limpiar intentos de token
	for tokenHash, attempts := range rl.tokenAttempts {
		// Eliminar si es antiguo y no está bloqueado
		if attempts.FirstAttempt.Before(cutoff) && now.After(attempts.BlockedUntil) {
			delete(rl.tokenAttempts, tokenHash)
		}
	}
}

// GetStats retorna estadísticas del rate limiter para monitoreo
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// Contar IPs bloqueadas
	blockedIPs := 0
	now := time.Now()
	for _, attempts := range rl.ipAttempts {
		if now.Before(attempts.BlockedUntil) {
			blockedIPs++
		}
	}

	// Contar tokens bloqueados
	blockedTokens := 0
	for _, attempts := range rl.tokenAttempts {
		if now.Before(attempts.BlockedUntil) {
			blockedTokens++
		}
	}

	return map[string]interface{}{
		"tracked_ips":      len(rl.ipAttempts),
		"tracked_tokens":   len(rl.tokenAttempts),
		"blocked_ips":      blockedIPs,
		"blocked_tokens":   blockedTokens,
		"max_per_ip":       rl.maxAttemptsPerIP,
		"max_per_token":    rl.maxAttemptsPerToken,
		"block_duration":   rl.blockDuration.String(),
		"window_duration":  rl.windowDuration.String(),
	}
}

// GetStatsString retorna estadísticas en formato string legible
func (rl *RateLimiter) GetStatsString() string {
	stats := rl.GetStats()
	return fmt.Sprintf(
		"RateLimiter Stats: IPs tracked=%d (blocked=%d), Tokens tracked=%d (blocked=%d), Limits: %d/IP, %d/token, Block=%s",
		stats["tracked_ips"],
		stats["blocked_ips"],
		stats["tracked_tokens"],
		stats["blocked_tokens"],
		stats["max_per_ip"],
		stats["max_per_token"],
		stats["block_duration"],
	)
}

// IsIPBlocked verifica si una IP específica está bloqueada
func (rl *RateLimiter) IsIPBlocked(ip string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	attempts, exists := rl.ipAttempts[ip]
	if !exists {
		return false
	}

	return time.Now().Before(attempts.BlockedUntil)
}

// UnblockIP desbloquea manualmente una IP (útil para administración)
func (rl *RateLimiter) UnblockIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if attempts, exists := rl.ipAttempts[ip]; exists {
		attempts.BlockedUntil = time.Time{} // Resetear a tiempo cero
		attempts.Count = 0
	}
}

// Reset limpia todos los registros (útil para testing)
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.ipAttempts = make(map[string]*IPAttempts)
	rl.tokenAttempts = make(map[string]*TokenAttempts)
}