package metrics

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ModelResolver resuelve ARNs de inference profiles a model_ids base
type ModelResolver struct {
	db    *pgxpool.Pool
	cache map[string]string // ARN -> model_id
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewModelResolver crea un nuevo resolver de modelos
func NewModelResolver(db *pgxpool.Pool) *ModelResolver {
	return &ModelResolver{
		db:    db,
		cache: make(map[string]string),
		ttl:   5 * time.Minute, // Cache por 5 minutos
	}
}

// ResolveModelID resuelve un ARN o model_id a su model_id base
// Si es un ARN de inference profile, busca en BD el modelo base
// Si ya es un model_id, lo retorna directamente
func (mr *ModelResolver) ResolveModelID(modelIDOrARN string) (string, error) {
	// Si no es un ARN, retornar directamente
	if !strings.HasPrefix(modelIDOrARN, "arn:aws:bedrock:") {
		return modelIDOrARN, nil
	}

	// Verificar cache
	mr.mu.RLock()
	if cachedModelID, exists := mr.cache[modelIDOrARN]; exists {
		mr.mu.RUnlock()
		return cachedModelID, nil
	}
	mr.mu.RUnlock()

	// Buscar en base de datos
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT m.model_id
		FROM "identity-manager-profiles-tbl" p
		JOIN "identity-manager-models-tbl" m ON p.model_id = m.id
		WHERE p.model_arn = $1
		LIMIT 1
	`

	var baseModelID string
	err := mr.db.QueryRow(ctx, query, modelIDOrARN).Scan(&baseModelID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve model ARN %s: %w", modelIDOrARN, err)
	}

	// Guardar en cache
	mr.mu.Lock()
	mr.cache[modelIDOrARN] = baseModelID
	mr.mu.Unlock()

	return baseModelID, nil
}

// ClearCache limpia el cache de resolución
func (mr *ModelResolver) ClearCache() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.cache = make(map[string]string)
}

// GetCacheSize retorna el tamaño actual del cache
func (mr *ModelResolver) GetCacheSize() int {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return len(mr.cache)
}