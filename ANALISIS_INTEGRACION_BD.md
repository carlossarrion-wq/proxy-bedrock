# 🗄️ Análisis de Integración con Base de Datos

## 📋 Resumen Ejecutivo

Este documento identifica **todos los puntos de integración con la base de datos** en el proyecto Bedrock Proxy, necesarios para migrar a una nueva base de datos o implementar una capa de abstracción.

---

## 🎯 Operaciones de Base de Datos Identificadas

### **Total de Operaciones**: 6 operaciones principales

| # | Operación | Componente | Criticidad | Tipo |
|---|-----------|------------|------------|------|
| 1 | `ValidateToken` | Auth Middleware | 🔴 CRÍTICA | READ |
| 2 | `CheckQuota` | Quota Middleware | 🔴 CRÍTICA | READ |
| 3 | `InsertMetric` | Metrics Worker | 🟡 MEDIA | WRITE |
| 4 | `UpdateQuotaAndCounters` | Bedrock Client / Quota MW | 🔴 CRÍTICA | WRITE (Transaccional) |
| 5 | `CheckAndBlockUser` | Bedrock Client / Quota MW | 🟠 ALTA | WRITE |
| 6 | `ResetDailyCounters` | Scheduler Service | 🟠 ALTA | WRITE (Transaccional) |

---

## 📊 Detalle de Operaciones

### **1. ValidateToken** 🔴 CRÍTICA

**Ubicación**: `pkg/database/queries.go:35`  
**Llamada desde**: `pkg/auth/middleware.go:82`  
**Frecuencia**: Por cada request HTTP (alta frecuencia)

#### Propósito
Valida un token JWT contra la base de datos verificando:
- Token existe y no está revocado
- Token no ha expirado
- Usuario está activo
- Obtiene configuración del usuario (quotas, inference profile)

#### Firma
```go
func (db *Database) ValidateToken(ctx context.Context, tokenHash string) (*TokenInfo, error)
```

#### Input
- `tokenHash` (string): SHA256 hash del token JWT

#### Output
```go
type TokenInfo struct {
    JTI              string    // ID único del token
    UserID           string    // iam_username del usuario
    Email            string    // Email del usuario
    Team             string    // Equipo del usuario
    Person           string    // Nombre completo
    Role             string    // Rol (admin, user, etc.)
    IsRevoked        bool      // Si el token está revocado
    ExpiresAt        time.Time // Fecha de expiración
    MonthlyQuota     float64   // Quota mensual en USD
    DailyLimit       float64   // Límite diario en USD
    DailyReqLimit    int       // Límite diario de requests
    InferenceProfile string    // ARN del inference profile de Bedrock
}
```

#### Query SQL
```sql
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
```

#### Tablas Involucradas
- `tokens` (JOIN principal)
- `users` (JOIN secundario)

#### Índices Requeridos
- `tokens.token_hash` (PRIMARY o UNIQUE INDEX) - **CRÍTICO para performance**
- `users.iam_username` (PRIMARY KEY)
- `users.is_active` (INDEX recomendado)

---

### **2. CheckQuota** 🔴 CRÍTICA

**Ubicación**: `pkg/database/queries.go:75`  
**Llamada desde**: `pkg/quota/middleware.go:28`  
**Frecuencia**: Por cada request HTTP (alta frecuencia)

#### Propósito
Verifica si el usuario tiene quota disponible antes de procesar el request:
- Obtiene límites configurados del usuario
- Calcula uso mensual y diario
- Verifica estado de bloqueo

#### Firma
```go
func (db *Database) CheckQuota(ctx context.Context, userID string) (*QuotaInfo, error)
```

#### Input
- `userID` (string): iam_username del usuario

#### Output
```go
type QuotaInfo struct {
    UserID            string  // iam_username
    MonthlyQuotaUSD   float64 // Límite mensual configurado
    DailyLimitUSD     float64 // Límite diario configurado
    DailyRequestLimit int     // Límite de requests diarios
    MonthlyUsedUSD    float64 // Uso mensual acumulado
    MonthlyRequests   int     // Requests mensuales acumulados
    DailyUsedUSD      float64 // Uso diario acumulado
    DailyRequests     int     // Requests diarios acumulados
    IsBlocked         bool    // Si el usuario está bloqueado
    BlockedReason     string  // Razón del bloqueo
}
```

#### Query SQL
```sql
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
```

#### Tablas Involucradas
- `users` (tabla principal)
- `quota_usage` (LEFT JOIN - agregados mensuales)
- `user_blocking_status` (LEFT JOIN - contadores diarios)

#### Índices Requeridos
- `users.iam_username` (PRIMARY KEY)
- `quota_usage(user_id, month)` (COMPOSITE INDEX) - **CRÍTICO**
- `user_blocking_status.user_id` (PRIMARY KEY)

---

### **3. InsertMetric** 🟡 MEDIA

**Ubicación**: `pkg/database/queries.go:125`  
**Llamada desde**: `pkg/metrics/worker.go:68`  
**Frecuencia**: Asíncrona, batch processing (50 métricas cada 5 segundos)

#### Propósito
Registra métricas detalladas de cada request para:
- Auditoría y trazabilidad
- Análisis de uso
- Facturación y reportes

#### Firma
```go
func (db *Database) InsertMetric(ctx context.Context, metric *MetricData) error
```

#### Input
```go
type MetricData struct {
    UserID              string    // iam_username
    Team                string    // Equipo del usuario
    Person              string    // Nombre completo
    RequestTimestamp    time.Time // Timestamp del request
    ModelID             string    // Modelo usado (inference profile)
    RequestID           string    // UUID del request
    SourceIP            string    // IP del cliente
    UserAgent           string    // User-Agent del cliente
    AWSRegion           string    // Región de AWS Bedrock
    TokensInput         int       // Tokens de input
    TokensOutput        int       // Tokens de output
    TokensCacheRead     int       // Tokens leídos de caché
    TokensCacheCreation int       // Tokens escritos a caché
    CostUSD             float64   // Costo calculado en USD
    ProcessingTimeMS    int       // Tiempo de procesamiento en ms
    ResponseStatus      string    // Estado de la respuesta (success/error)
    ErrorMessage        string    // Mensaje de error si aplica
}
```

#### Query SQL
```sql
INSERT INTO request_metrics (
    user_id, team, person, request_timestamp, model_id, request_id,
    source_ip, user_agent, aws_region, tokens_input, tokens_output,
    tokens_cache_read, tokens_cache_creation, cost_usd, processing_time_ms,
    response_status, error_message
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
)
```

#### Tablas Involucradas
- `request_metrics` (INSERT único)

#### Índices Requeridos
- `request_metrics.user_id` (INDEX para queries por usuario)
- `request_metrics.request_timestamp` (INDEX para queries temporales)
- `request_metrics.request_id` (UNIQUE INDEX opcional)

#### Consideraciones
- **Volumen alto**: Puede generar millones de registros
- **Particionamiento**: Recomendado por fecha (mensual o semanal)
- **Retención**: Definir política de retención de datos

---

### **4. UpdateQuotaAndCounters** 🔴 CRÍTICA

**Ubicación**: `pkg/database/queries.go:157`  
**Llamada desde**: 
- `pkg/bedrock_metrics.go:48` (post-processing)
- `pkg/quota/middleware.go:103` (UpdateQuotaAfterRequest)

**Frecuencia**: Por cada request completado (alta frecuencia)

#### Propósito
Actualiza contadores de uso después de procesar un request:
- Incrementa uso mensual (quota_usage)
- Incrementa contadores diarios (user_blocking_status)
- **Operación transaccional** (ACID)

#### Firma
```go
func (db *Database) UpdateQuotaAndCounters(ctx context.Context, userID string, costUSD float64) error
```

#### Input
- `userID` (string): iam_username del usuario
- `costUSD` (float64): Costo del request en USD

#### Queries SQL (Transacción)

**Query 1: Actualizar quota_usage**
```sql
INSERT INTO quota_usage (user_id, month, total_cost_usd, total_requests, last_updated)
VALUES ($1, DATE_TRUNC('month', CURRENT_DATE), $2, 1, NOW())
ON CONFLICT (user_id, month) 
DO UPDATE SET 
    total_cost_usd = quota_usage.total_cost_usd + $2,
    total_requests = quota_usage.total_requests + 1,
    last_updated = NOW()
```

**Query 2: Actualizar user_blocking_status**
```sql
INSERT INTO user_blocking_status (user_id, daily_cost_usd, daily_requests, last_request_at, updated_at)
VALUES ($1, $2, 1, NOW(), NOW())
ON CONFLICT (user_id)
DO UPDATE SET
    daily_cost_usd = user_blocking_status.daily_cost_usd + $2,
    daily_requests = user_blocking_status.daily_requests + 1,
    last_request_at = NOW(),
    updated_at = NOW()
```

#### Tablas Involucradas
- `quota_usage` (UPSERT)
- `user_blocking_status` (UPSERT)

#### Índices Requeridos
- `quota_usage(user_id, month)` (UNIQUE CONSTRAINT) - **CRÍTICO**
- `user_blocking_status.user_id` (PRIMARY KEY)

#### Consideraciones
- **Transaccional**: Ambas operaciones deben completarse o revertirse
- **Concurrencia**: Múltiples requests del mismo usuario pueden ejecutarse simultáneamente
- **Atomicidad**: UPSERT debe ser atómico para evitar race conditions

---

### **5. CheckAndBlockUser** 🟠 ALTA

**Ubicación**: `pkg/database/queries.go:197`  
**Llamada desde**: 
- `pkg/bedrock_metrics.go:54` (post-processing)
- `pkg/quota/middleware.go:109` (UpdateQuotaAfterRequest)

**Frecuencia**: Por cada request completado (alta frecuencia)

#### Propósito
Verifica si el usuario ha excedido límites y lo bloquea automáticamente:
- Compara uso diario vs límites configurados
- Bloquea usuario si excede límites
- Registra razón del bloqueo

#### Firma
```go
func (db *Database) CheckAndBlockUser(ctx context.Context, userID string) error
```

#### Input
- `userID` (string): iam_username del usuario

#### Query SQL
```sql
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
```

#### Tablas Involucradas
- `user_blocking_status` (UPDATE con JOIN)
- `users` (JOIN para obtener límites)

#### Índices Requeridos
- `user_blocking_status.user_id` (PRIMARY KEY)
- `users.iam_username` (PRIMARY KEY)

---

### **6. ResetDailyCounters** 🟠 ALTA

**Ubicación**: `pkg/database/database.go:75`  
**Llamada desde**: `pkg/scheduler/scheduler.go:68`  
**Frecuencia**: Una vez al día (00:00 UTC)

#### Propósito
Reset diario automático de contadores:
- Resetea contadores diarios a cero
- Desbloquea usuarios con bloqueo automático
- Mantiene bloqueos manuales de admin

#### Firma
```go
func (db *Database) ResetDailyCounters(ctx context.Context) (*ResetResult, error)
```

#### Output
```go
type ResetResult struct {
    UsersReset     int // Usuarios con contadores reseteados
    UsersUnblocked int // Usuarios desbloqueados
    CountersReset  int // Total de contadores reseteados
}
```

#### Queries SQL (Transacción)

**Query 1: Contar usuarios a desbloquear**
```sql
SELECT COUNT(*)
FROM user_blocking_status
WHERE is_blocked = true 
  AND last_request_at < CURRENT_DATE
  AND blocked_by_admin_id IS NULL
```

**Query 2: Resetear contadores y desbloquear**
```sql
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
```

**Query 3: Contar usuarios reseteados**
```sql
SELECT COUNT(DISTINCT user_id)
FROM user_blocking_status
WHERE last_reset_at::date = CURRENT_DATE
```

#### Tablas Involucradas
- `user_blocking_status` (UPDATE masivo)

#### Consideraciones
- **Ejecución única diaria**: Scheduler garantiza una sola ejecución
- **Transaccional**: Todas las operaciones deben completarse
- **Performance**: Puede afectar a muchos usuarios simultáneamente

---

## 🏗️ Componentes que Dependen de BD

### **1. AuthMiddleware** (`pkg/auth/middleware.go`)
```go
type AuthMiddleware struct {
    jwtConfig   JWTConfig
    db          *database.Database  // ← Dependencia BD
    rateLimiter *RateLimiter
}
```

**Operaciones BD**:
- `ValidateToken()` - Por cada request HTTP

**Inicialización**: `cmd/main.go:73`
```go
authMiddleware = auth.NewAuthMiddleware(db, authConfig)
```

---

### **2. QuotaMiddleware** (`pkg/quota/middleware.go`)
```go
type QuotaMiddleware struct {
    db *database.Database  // ← Dependencia BD
}
```

**Operaciones BD**:
- `CheckQuota()` - Por cada request HTTP
- `UpdateQuotaAndCounters()` - Post-processing
- `CheckAndBlockUser()` - Post-processing

**Inicialización**: `cmd/main.go:74`
```go
quotaMiddleware = quota.NewQuotaMiddleware(db)
```

---

### **3. MetricsWorker** (`pkg/metrics/worker.go`)
```go
type MetricsWorker struct {
    db            *database.Database  // ← Dependencia BD
    metricsChan   chan *database.MetricData
    batchSize     int
    flushInterval time.Duration
    wg            sync.WaitGroup
    stopChan      chan struct{}
}
```

**Operaciones BD**:
- `InsertMetric()` - Asíncrono, batch processing

**Inicialización**: `cmd/main.go:88`
```go
metricsWorker = metrics.NewMetricsWorker(db, metricsConfig)
```

---

### **4. BedrockClient** (`pkg/bedrock.go`)
```go
type BedrockClient struct {
    config        *BedrockConfig
    client        *bedrockRuntime.Client
    db            *database.Database  // ← Dependencia BD
    metricsWorker *metrics.MetricsWorker
    quotaMw       *quota.QuotaMiddleware
}
```

**Operaciones BD** (vía QuotaMiddleware):
- `UpdateQuotaAndCounters()` - Post-processing
- `CheckAndBlockUser()` - Post-processing

**Inicialización**: `cmd/main.go:102`
```go
client.SetDependencies(db, metricsWorker, quotaMiddleware)
```

---

### **5. SchedulerService** (`pkg/scheduler/scheduler.go`)
```go
type SchedulerService struct {
    db     *database.Database  // ← Dependencia BD
    logger Logger
    stopCh chan struct{}
}
```

**Operaciones BD**:
- `ResetDailyCounters()` - Una vez al día (00:00 UTC)

**Inicialización**: `cmd/main.go:96`
```go
schedulerService = scheduler.NewSchedulerService(db, pkg.Log)
```

---

## 📐 Esquema de Tablas Requeridas

### **Tabla 1: `users`**
```sql
CREATE TABLE users (
    iam_username VARCHAR(255) PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    team VARCHAR(255),
    person VARCHAR(255),
    role VARCHAR(50) DEFAULT 'user',
    monthly_quota_usd DECIMAL(10,2) DEFAULT 1000.00,
    daily_limit_usd DECIMAL(10,2) DEFAULT 50.00,
    daily_request_limit INT DEFAULT 1000,
    default_inference_profile TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_users_is_active ON users(is_active);
CREATE INDEX idx_users_email ON users(email);
```

---

### **Tabla 2: `tokens`**
```sql
CREATE TABLE tokens (
    jti VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES users(iam_username),
    token_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA256 hash
    issued_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    is_revoked BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_tokens_hash ON tokens(token_hash);  -- CRÍTICO
CREATE INDEX idx_tokens_user_id ON tokens(user_id);
CREATE INDEX idx_tokens_expires_at ON tokens(expires_at);
```

---

### **Tabla 3: `quota_usage`**
```sql
CREATE TABLE quota_usage (
    user_id VARCHAR(255) NOT NULL REFERENCES users(iam_username),
    month DATE NOT NULL,  -- DATE_TRUNC('month', CURRENT_DATE)
    total_cost_usd DECIMAL(10,6) DEFAULT 0,
    total_requests INT DEFAULT 0,
    last_updated TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (user_id, month)
);

CREATE INDEX idx_quota_usage_month ON quota_usage(month);
```

---

### **Tabla 4: `user_blocking_status`**
```sql
CREATE TABLE user_blocking_status (
    user_id VARCHAR(255) PRIMARY KEY REFERENCES users(iam_username),
    daily_cost_usd DECIMAL(10,6) DEFAULT 0,
    daily_requests INT DEFAULT 0,
    is_blocked BOOLEAN DEFAULT false,
    blocked_at TIMESTAMP,
    blocked_reason TEXT,
    blocked_until TIMESTAMP,
    blocked_by_admin_id VARCHAR(255),
    requests_at_blocking INT,
    last_request_at TIMESTAMP,
    last_reset_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_blocking_status_blocked ON user_blocking_status(is_blocked);
CREATE INDEX idx_blocking_last_request ON user_blocking_status(last_request_at);
```

---

### **Tabla 5: `request_metrics`**
```sql
CREATE TABLE request_metrics (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    team VARCHAR(255),
    person VARCHAR(255),
    request_timestamp TIMESTAMP NOT NULL,
    model_id VARCHAR(255) NOT NULL,
    request_id VARCHAR(255) NOT NULL,
    source_ip VARCHAR(45),
    user_agent TEXT,
    aws_region VARCHAR(50),
    tokens_input INT DEFAULT 0,
    tokens_output INT DEFAULT 0,
    tokens_cache_read INT DEFAULT 0,
    tokens_cache_creation INT DEFAULT 0,
    cost_usd DECIMAL(10,6) DEFAULT 0,
    processing_time_ms INT,
    response_status VARCHAR(50),
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_metrics_user_id ON request_metrics(user_id);
CREATE INDEX idx_metrics_timestamp ON request_metrics(request_timestamp);
CREATE INDEX idx_metrics_request_id ON request_metrics(request_id);

-- Particionamiento recomendado por fecha (mensual)
-- CREATE TABLE request_metrics_2026_03 PARTITION OF request_metrics
-- FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
```

---

## 🔄 Flujo de Datos Completo

```
┌─────────────────────────────────────────────────────────────┐
│                    REQUEST HTTP ENTRANTE                     │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  1. AuthMiddleware.Middleware()                             │
│     ↓                                                        │
│     db.ValidateToken(tokenHash)  ← BD READ (CRÍTICO)        │
│     ↓                                                        │
│     Extrae: UserContext (user_id, team, inference_profile)  │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  2. QuotaMiddleware.Middleware()                            │
│     ↓                                                        │
│     db.CheckQuota(userID)  ← BD READ (CRÍTICO)              │
│     ↓                                                        │
│     Verifica: Límites diarios/mensuales, estado de bloqueo  │
│     ↓                                                        │
│     Si excede límites → HTTP 429 (Too Many Requests)        │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  3. BedrockClient.HandleProxy()                             │
│     ↓                                                        │
│     Procesa request → AWS Bedrock Converse API              │
│     ↓                                                        │
│     Captura métricas: tokens, costos, timing                │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  4. POST-PROCESSING (Goroutine asíncrona)                   │
│     ↓                                                        │
│     A. db.UpdateQuotaAndCounters(userID, cost)              │
│        ← BD WRITE TRANSACCIONAL (CRÍTICO)                   │
│        - Actualiza quota_usage (mensual)                    │
│        - Actualiza user_blocking_status (diario)            │
│     ↓                                                        │
│     B. db.CheckAndBlockUser(userID)                         │
│        ← BD WRITE (ALTA PRIORIDAD)                          │
│        - Bloquea usuario si excede límites                  │
│     ↓                                                        │
│     C. metricsWorker.RecordMetric(metric)                   │
│        → Encola métrica en canal buffered                   │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  5. MetricsWorker (Background Worker)                       │
│     ↓                                                        │
│     Batch processing (50 métricas cada 5 segundos)          │
│     ↓                                                        │
│     db.InsertMetric(metric)  ← BD WRITE (MEDIA PRIORIDAD)   │
│     - Inserta en request_metrics                            │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│  6. SchedulerService (Cron diario 00:00 UTC)               │
│     ↓                                                        │
│     db.ResetDailyCounters()  ← BD WRITE TRANSACCIONAL       │
│     - Resetea contadores diarios                            │
│     - Desbloquea usuarios                                   │
└─────────────────────────────────────────────────────────────┘
```

---

## 🎯 Puntos de Integración para Nueva BD

### **Opción 1: Reemplazar Implementación Actual**

Mantener la interfaz `*database.Database` pero cambiar la implementación interna:

```go
// pkg/database/database.go
type Database struct {
    // Reemplazar pgxpool por tu nueva BD
    // pool *pgxpool.Pool  ← ELIMINAR
    newDBClient *YourNewDBClient  // ← AÑADIR
    config *DatabaseConfig
}
```

**Ventajas**:
- Cambios mínimos en el código existente
- Mantiene la misma interfaz pública

**Desventajas**:
- Acoplamiento a estructura existente

---

### **Opción 2: Crear Interfaz de Abstracción**

Definir una interfaz para operaciones de BD:

```go
// pkg/database/interface.go
type DatabaseInterface interface {
    // Autenticación
    ValidateToken(ctx context.Context, tokenHash string) (*TokenInfo, error)
    
    // Quotas
    CheckQuota(ctx context.Context, userID string) (*QuotaInfo, error)
    UpdateQuotaAndCounters(ctx context.Context, userID string, costUSD float64) error
    CheckAndBlockUser(ctx context.Context, userID string) error
    
    // Métricas
    InsertMetric(ctx context.Context, metric *MetricData) error
    
    // Scheduler
    ResetDailyCounters(ctx context.Context) (*ResetResult, error)
    
    // Utilidades
    Ping(ctx context.Context) error
    Close()
}
```

Luego implementar para tu nueva BD:

```go
// pkg/database/newdb_impl.go
type NewDatabaseImpl struct {
    client *YourNewDBClient
}

func (db *NewDatabaseImpl) ValidateToken(ctx context.Context, tokenHash string) (*TokenInfo, error) {
    // Implementación específica para tu nueva BD
}

// ... implementar resto de métodos
```

**Ventajas**:
- Desacoplamiento total
- Fácil testing con mocks
- Permite múltiples implementaciones

**Desventajas**:
- Requiere refactoring de código existente

---

### **Opción 3: Patrón Repository**

Crear repositorios específicos por dominio:

```go
// pkg/repository/token_repository.go
type TokenRepository interface {
    ValidateToken(ctx context.Context, tokenHash string) (*TokenInfo, error)
}

// pkg/repository/quota_repository.go
type QuotaRepository interface {
    CheckQuota(ctx context.Context, userID string) (*QuotaInfo, error)
    UpdateQuotaAndCounters(ctx context.Context, userID string, costUSD float64) error
    CheckAndBlockUser(ctx context.Context, userID string) error
}

// pkg/repository/metrics_repository.go
type MetricsRepository interface {
    InsertMetric(ctx context.Context, metric *MetricData) error
}
```

**Ventajas**:
- Separación de responsabilidades
- Más granular y testeable
- Sigue principios SOLID

**Desventajas**:
- Mayor complejidad inicial
- Más archivos y código

---

## 📋 Checklist de Migración

### **Fase 1: Análisis** ✅
- [x] Identificar todas las operaciones de BD
- [x] Documentar queries SQL
- [x] Mapear dependencias entre componentes
- [x] Definir esquema de tablas

### **Fase 2: Diseño**
- [ ] Elegir estrategia de integración (Opción 1, 2 o 3)
- [ ] Diseñar esquema de BD en nuevo sistema
- [ ] Definir índices y optimizaciones
- [ ] Planificar migración de datos existentes

### **Fase 3: Implementación**
- [ ] Implementar operaciones de BD en nuevo sistema
- [ ] Crear tests unitarios para cada operación
- [ ] Implementar connection pooling
- [ ] Añadir manejo de errores y retry logic

### **Fase 4: Testing**
- [ ] Tests de integración con BD real
- [ ] Tests de carga y performance
- [ ] Validar transaccionalidad (ACID)
- [ ] Verificar concurrencia

### **Fase 5: Despliegue**
- [ ] Configurar variables de entorno
- [ ] Migrar datos de PostgreSQL a nueva BD
- [ ] Desplegar en ambiente de staging
- [ ] Monitorear métricas y logs
- [ ] Rollout gradual a producción

---

## 🔍 Consideraciones Técnicas

### **Transaccionalidad**
Las siguientes operaciones **DEBEN** ser transaccionales:
- `UpdateQuotaAndCounters()` - 2 tablas (quota_usage + user_blocking_status)
- `ResetDailyCounters()` - Múltiples operaciones en user_blocking_status

### **Concurrencia**
- Múltiples requests del mismo usuario pueden ejecutarse simultáneamente
- UPSERT debe ser atómico para evitar race conditions
- Considerar locks optimistas o pesimistas según la BD

### **Performance**
- `ValidateToken()` y `CheckQuota()` están en el path crítico (cada request)
- Índices son **CRÍTICOS** para performance
- Considerar caché L1/L2 para tokens y quotas

### **Escalabilidad**
- `request_metrics` puede crecer indefinidamente
- Considerar particionamiento por fecha
- Definir política de retención y archivado

---

## 📊 Resumen de Criticidad

| Operación | Frecuencia | Criticidad | Transaccional | Path Crítico |
|-----------|------------|------------|---------------|--------------|
| ValidateToken | Alta | 🔴 CRÍTICA | No | ✅ Sí |
| CheckQuota | Alta | 🔴 CRÍTICA | No | ✅ Sí |
| UpdateQuotaAndCounters | Alta | 🔴 CRÍTICA | ✅ Sí | ❌ No (async) |
| CheckAndBlockUser | Alta | 🟠 ALTA | No | ❌ No (async) |
| InsertMetric | Media | 🟡 MEDIA | No | ❌ No (async) |
| ResetDailyCounters | Baja | 🟠 ALTA | ✅ Sí | ❌ No (cron) |

---

## 📞 Contacto y Soporte

Para dudas sobre la integración, consultar:
- Documentación del proyecto: `README.md`
- Código fuente: `pkg/database/`
- Logs estructurados: `logs/bedrock-proxy_*.json`
