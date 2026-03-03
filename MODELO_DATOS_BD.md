# 🗄️ MODELO DE DATOS - BASE DE DATOS POSTGRESQL

**Proyecto**: Proxy-Bedrock  
**Base de Datos**: PostgreSQL 13+  
**Fecha**: 3 de febrero de 2026  
**Versión**: 1.1.0

---

## 📋 ÍNDICE

1. [Resumen Ejecutivo](#resumen-ejecutivo)
2. [Diagrama Entidad-Relación](#diagrama-entidad-relación)
3. [Tablas del Sistema](#tablas-del-sistema)
4. [Relaciones entre Tablas](#relaciones-entre-tablas)
5. [Índices y Optimizaciones](#índices-y-optimizaciones)
6. [Estructuras Go Correspondientes](#estructuras-go-correspondientes)
7. [Queries Principales](#queries-principales)
8. [Consideraciones de Performance](#consideraciones-de-performance)

---

## 📊 RESUMEN EJECUTIVO

El sistema utiliza **5 tablas principales** en PostgreSQL para gestionar:

- ✅ **Usuarios y autenticación** (users, tokens)
- ✅ **Control de cuotas** (quota_usage, user_blocking_status)
- ✅ **Métricas y auditoría** (request_metrics)

**Total de Tablas**: 5  
**Relaciones**: 4 Foreign Keys  
**Índices Críticos**: 8  
**Volumen Estimado**: 
- `users`: ~100-1000 registros
- `tokens`: ~100-1000 registros (activos)
- `quota_usage`: ~1000-10000 registros (mensual)
- `user_blocking_status`: ~100-1000 registros
- `request_metrics`: **ALTO VOLUMEN** (millones de registros)

---

## 🔷 DIAGRAMA ENTIDAD-RELACIÓN

```
┌─────────────────────────────────────────────────────────────────┐
│                         USERS                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ PK: iam_username (VARCHAR 255)                           │  │
│  │ email (VARCHAR 255) NOT NULL                             │  │
│  │ team (VARCHAR 255)                                       │  │
│  │ person (VARCHAR 255)                                     │  │
│  │ role (VARCHAR 50) DEFAULT 'user'                         │  │
│  │ monthly_quota_usd (DECIMAL 10,2) DEFAULT 1000.00         │  │
│  │ daily_limit_usd (DECIMAL 10,2) DEFAULT 50.00             │  │
│  │ daily_request_limit (INT) DEFAULT 1000                   │  │
│  │ default_inference_profile (TEXT)                         │  │
│  │ is_active (BOOLEAN) DEFAULT true                         │  │
│  │ created_at (TIMESTAMP) DEFAULT NOW()                     │  │
│  │ updated_at (TIMESTAMP) DEFAULT NOW()                     │  │
│  └──────────────────────────────────────────────────────────┘  │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     │ 1:N
                     │
        ┌────────────┼────────────┬──────────────┬────────────────┐
        │            │            │              │                │
        ▼            ▼            ▼              ▼                ▼
┌───────────┐  ┌──────────┐  ┌─────────┐  ┌──────────┐  ┌──────────────┐
│  TOKENS   │  │  QUOTA   │  │  USER   │  │ REQUEST  │  │   (FUTURE)   │
│           │  │  USAGE   │  │BLOCKING │  │ METRICS  │  │   TABLES     │
└───────────┘  └──────────┘  └─────────┘  └──────────┘  └──────────────┘
```

---

## 📋 TABLAS DEL SISTEMA

### **1. TABLA: `users`**

**Propósito**: Almacena información de usuarios del sistema con sus configuraciones de cuotas y perfiles de inferencia.

#### Esquema SQL

```sql
CREATE TABLE users (
    -- Identificación
    iam_username VARCHAR(255) PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    
    -- Información organizacional
    team VARCHAR(255),
    person VARCHAR(255),
    role VARCHAR(50) DEFAULT 'user',
    
    -- Configuración de cuotas
    monthly_quota_usd DECIMAL(10,2) DEFAULT 1000.00,
    daily_limit_usd DECIMAL(10,2) DEFAULT 50.00,
    daily_request_limit INT DEFAULT 1000,
    
    -- Configuración de AWS Bedrock
    default_inference_profile TEXT,
    
    -- Estado y auditoría
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Índices
CREATE INDEX idx_users_is_active ON users(is_active);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_team ON users(team);
```

#### Descripción de Campos

| Campo | Tipo | Nulo | Default | Descripción |
|-------|------|------|---------|-------------|
| `iam_username` | VARCHAR(255) | NO | - | **PK**. Username de IAM (ej: "admin-test-user") |
| `email` | VARCHAR(255) | NO | - | Email del usuario |
| `team` | VARCHAR(255) | SÍ | NULL | Equipo al que pertenece (ej: "Platform Team") |
| `person` | VARCHAR(255) | SÍ | NULL | Nombre completo del usuario |
| `role` | VARCHAR(50) | NO | 'user' | Rol del usuario (user, admin, etc.) |
| `monthly_quota_usd` | DECIMAL(10,2) | NO | 1000.00 | Límite mensual en USD |
| `daily_limit_usd` | DECIMAL(10,2) | NO | 50.00 | Límite diario en USD |
| `daily_request_limit` | INT | NO | 1000 | Límite diario de requests |
| `default_inference_profile` | TEXT | SÍ | NULL | ARN del inference profile de Bedrock |
| `is_active` | BOOLEAN | NO | true | Si el usuario está activo |
| `created_at` | TIMESTAMP | NO | NOW() | Fecha de creación |
| `updated_at` | TIMESTAMP | NO | NOW() | Fecha de última actualización |

#### Constraints

- **PRIMARY KEY**: `iam_username`
- **UNIQUE**: `email` (recomendado)
- **CHECK**: `monthly_quota_usd >= 0`
- **CHECK**: `daily_limit_usd >= 0`
- **CHECK**: `daily_request_limit >= 0`

#### Valores de Ejemplo

```sql
INSERT INTO users VALUES (
    'admin-test-user',
    'admin.test@company.com',
    'Platform Team',
    'Admin Test User',
    'admin',
    1000.00,
    50.00,
    1000,
    'eu.anthropic.claude-sonnet-4-5-20250929-v1:0',
    true,
    NOW(),
    NOW()
);
```

---

### **2. TABLA: `tokens`**

**Propósito**: Almacena tokens JWT activos con su hash SHA256 para validación rápida.

#### Esquema SQL

```sql
CREATE TABLE tokens (
    -- Identificación del token
    jti VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES users(iam_username) ON DELETE CASCADE,
    
    -- Hash del token (SHA256)
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    
    -- Validez temporal
    issued_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    
    -- Estado
    is_revoked BOOLEAN DEFAULT false,
    
    -- Auditoría
    created_at TIMESTAMP DEFAULT NOW()
);

-- Índices CRÍTICOS
CREATE UNIQUE INDEX idx_tokens_hash ON tokens(token_hash);
CREATE INDEX idx_tokens_user_id ON tokens(user_id);
CREATE INDEX idx_tokens_expires_at ON tokens(expires_at);
CREATE INDEX idx_tokens_is_revoked ON tokens(is_revoked) WHERE is_revoked = false;
```

#### Descripción de Campos

| Campo | Tipo | Nulo | Default | Descripción |
|-------|------|------|---------|-------------|
| `jti` | VARCHAR(255) | NO | - | **PK**. JWT ID único (claim "jti") |
| `user_id` | VARCHAR(255) | NO | - | **FK** → users.iam_username |
| `token_hash` | VARCHAR(64) | NO | - | **UNIQUE**. SHA256 hash del token JWT completo |
| `issued_at` | TIMESTAMP | NO | - | Fecha de emisión del token (claim "iat") |
| `expires_at` | TIMESTAMP | NO | - | Fecha de expiración (claim "exp") |
| `is_revoked` | BOOLEAN | NO | false | Si el token ha sido revocado manualmente |
| `created_at` | TIMESTAMP | NO | NOW() | Fecha de registro en BD |

#### Constraints

- **PRIMARY KEY**: `jti`
- **FOREIGN KEY**: `user_id` → `users(iam_username)` ON DELETE CASCADE
- **UNIQUE**: `token_hash`
- **CHECK**: `expires_at > issued_at`

#### Cálculo del Token Hash

```go
import "crypto/sha256"
import "encoding/hex"

func HashToken(tokenString string) string {
    hash := sha256.Sum256([]byte(tokenString))
    return hex.EncodeToString(hash[:])
}
```

#### Valores de Ejemplo

```sql
INSERT INTO tokens VALUES (
    'jti-admin-test-002',
    'admin-test-user',
    'eaa0b27882e4124c3a4c2bc1c16c4b96e36af17ef190c9f39f2ae4c684640ab2',
    to_timestamp(1771943010),
    to_timestamp(1779715410),
    false,
    NOW()
);
```

---

### **3. TABLA: `quota_usage`**

**Propósito**: Almacena el uso mensual agregado de cada usuario (costos y número de requests).

#### Esquema SQL

```sql
CREATE TABLE quota_usage (
    -- Identificación
    user_id VARCHAR(255) NOT NULL REFERENCES users(iam_username) ON DELETE CASCADE,
    month DATE NOT NULL,  -- DATE_TRUNC('month', CURRENT_DATE)
    
    -- Métricas agregadas
    total_cost_usd DECIMAL(10,6) DEFAULT 0,
    total_requests INT DEFAULT 0,
    
    -- Auditoría
    last_updated TIMESTAMP DEFAULT NOW(),
    
    -- Constraint
    PRIMARY KEY (user_id, month)
);

-- Índices
CREATE INDEX idx_quota_usage_month ON quota_usage(month);
CREATE INDEX idx_quota_usage_cost ON quota_usage(total_cost_usd);
```

#### Descripción de Campos

| Campo | Tipo | Nulo | Default | Descripción |
|-------|------|------|---------|-------------|
| `user_id` | VARCHAR(255) | NO | - | **PK, FK** → users.iam_username |
| `month` | DATE | NO | - | **PK**. Mes en formato YYYY-MM-01 |
| `total_cost_usd` | DECIMAL(10,6) | NO | 0 | Costo total acumulado en el mes |
| `total_requests` | INT | NO | 0 | Número total de requests en el mes |
| `last_updated` | TIMESTAMP | NO | NOW() | Última actualización |

#### Constraints

- **PRIMARY KEY**: `(user_id, month)`
- **FOREIGN KEY**: `user_id` → `users(iam_username)` ON DELETE CASCADE
- **CHECK**: `total_cost_usd >= 0`
- **CHECK**: `total_requests >= 0`

#### Operación UPSERT

```sql
INSERT INTO quota_usage (user_id, month, total_cost_usd, total_requests, last_updated)
VALUES ($1, DATE_TRUNC('month', CURRENT_DATE), $2, 1, NOW())
ON CONFLICT (user_id, month) 
DO UPDATE SET 
    total_cost_usd = quota_usage.total_cost_usd + $2,
    total_requests = quota_usage.total_requests + 1,
    last_updated = NOW();
```

---

### **4. TABLA: `user_blocking_status`**

**Propósito**: Almacena contadores diarios y estado de bloqueo de usuarios.

#### Esquema SQL

```sql
CREATE TABLE user_blocking_status (
    -- Identificación
    user_id VARCHAR(255) PRIMARY KEY REFERENCES users(iam_username) ON DELETE CASCADE,
    
    -- Contadores diarios
    daily_cost_usd DECIMAL(10,6) DEFAULT 0,
    daily_requests INT DEFAULT 0,
    
    -- Estado de bloqueo
    is_blocked BOOLEAN DEFAULT false,
    blocked_at TIMESTAMP,
    blocked_reason TEXT,
    blocked_until TIMESTAMP,
    blocked_by_admin_id VARCHAR(255),
    requests_at_blocking INT,
    
    -- Auditoría
    last_request_at TIMESTAMP,
    last_reset_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Índices
CREATE INDEX idx_blocking_status_blocked ON user_blocking_status(is_blocked);
CREATE INDEX idx_blocking_last_request ON user_blocking_status(last_request_at);
CREATE INDEX idx_blocking_reset ON user_blocking_status(last_reset_at);
```

#### Descripción de Campos

| Campo | Tipo | Nulo | Default | Descripción |
|-------|------|------|---------|-------------|
| `user_id` | VARCHAR(255) | NO | - | **PK, FK** → users.iam_username |
| `daily_cost_usd` | DECIMAL(10,6) | NO | 0 | Costo acumulado hoy |
| `daily_requests` | INT | NO | 0 | Requests realizados hoy |
| `is_blocked` | BOOLEAN | NO | false | Si el usuario está bloqueado |
| `blocked_at` | TIMESTAMP | SÍ | NULL | Cuándo fue bloqueado |
| `blocked_reason` | TEXT | SÍ | NULL | Razón del bloqueo |
| `blocked_until` | TIMESTAMP | SÍ | NULL | Hasta cuándo está bloqueado |
| `blocked_by_admin_id` | VARCHAR(255) | SÍ | NULL | Admin que bloqueó (NULL = automático) |
| `requests_at_blocking` | INT | SÍ | NULL | Requests cuando fue bloqueado |
| `last_request_at` | TIMESTAMP | SÍ | NULL | Último request realizado |
| `last_reset_at` | TIMESTAMP | SÍ | NULL | Último reset diario |
| `updated_at` | TIMESTAMP | NO | NOW() | Última actualización |

#### Constraints

- **PRIMARY KEY**: `user_id`
- **FOREIGN KEY**: `user_id` → `users(iam_username)` ON DELETE CASCADE
- **CHECK**: `daily_cost_usd >= 0`
- **CHECK**: `daily_requests >= 0`

#### Tipos de Bloqueo

1. **Bloqueo Automático** (`blocked_by_admin_id IS NULL`):
   - Se desbloquea automáticamente en el reset diario
   - Razones: "Daily cost limit exceeded", "Daily request limit exceeded"

2. **Bloqueo Manual** (`blocked_by_admin_id IS NOT NULL`):
   - Requiere desbloqueo manual por admin
   - Persiste después del reset diario

---

### **5. TABLA: `request_metrics`**

**Propósito**: Almacena métricas detalladas de cada request para auditoría, análisis y facturación.

#### Esquema SQL

```sql
CREATE TABLE request_metrics (
    -- Identificación
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    team VARCHAR(255),
    person VARCHAR(255),
    
    -- Información del request
    request_timestamp TIMESTAMP NOT NULL,
    model_id VARCHAR(255) NOT NULL,
    request_id VARCHAR(255) NOT NULL,
    
    -- Información del cliente
    source_ip VARCHAR(45),
    user_agent TEXT,
    
    -- Configuración AWS
    aws_region VARCHAR(50),
    
    -- Métricas de tokens
    tokens_input INT DEFAULT 0,
    tokens_output INT DEFAULT 0,
    tokens_cache_read INT DEFAULT 0,
    tokens_cache_creation INT DEFAULT 0,
    
    -- Métricas de costo
    cost_usd DECIMAL(10,6) DEFAULT 0,
    
    -- Métricas de performance
    processing_time_ms INT,
    
    -- Estado de la respuesta
    response_status VARCHAR(50),
    error_message TEXT,
    
    -- Auditoría
    created_at TIMESTAMP DEFAULT NOW()
);

-- Índices CRÍTICOS para queries
CREATE INDEX idx_metrics_user_id ON request_metrics(user_id);
CREATE INDEX idx_metrics_timestamp ON request_metrics(request_timestamp);
CREATE INDEX idx_metrics_request_id ON request_metrics(request_id);
CREATE INDEX idx_metrics_model_id ON request_metrics(model_id);
CREATE INDEX idx_metrics_team ON request_metrics(team);

-- Índice compuesto para queries de rango temporal por usuario
CREATE INDEX idx_metrics_user_timestamp ON request_metrics(user_id, request_timestamp DESC);
```

#### Descripción de Campos

| Campo | Tipo | Nulo | Default | Descripción |
|-------|------|------|---------|-------------|
| `id` | BIGSERIAL | NO | AUTO | **PK**. ID autoincremental |
| `user_id` | VARCHAR(255) | NO | - | iam_username del usuario |
| `team` | VARCHAR(255) | SÍ | NULL | Equipo del usuario |
| `person` | VARCHAR(255) | SÍ | NULL | Nombre del usuario |
| `request_timestamp` | TIMESTAMP | NO | - | Timestamp del request |
| `model_id` | VARCHAR(255) | NO | - | Modelo/Inference Profile usado |
| `request_id` | VARCHAR(255) | NO | - | UUID del request |
| `source_ip` | VARCHAR(45) | SÍ | NULL | IP del cliente |
| `user_agent` | TEXT | SÍ | NULL | User-Agent del cliente |
| `aws_region` | VARCHAR(50) | SÍ | NULL | Región de AWS Bedrock |
| `tokens_input` | INT | NO | 0 | Tokens de entrada |
| `tokens_output` | INT | NO | 0 | Tokens de salida |
| `tokens_cache_read` | INT | NO | 0 | Tokens leídos de caché |
| `tokens_cache_creation` | INT | NO | 0 | Tokens escritos a caché |
| `cost_usd` | DECIMAL(10,6) | NO | 0 | Costo calculado en USD |
| `processing_time_ms` | INT | SÍ | NULL | Tiempo de procesamiento en ms |
| `response_status` | VARCHAR(50) | SÍ | NULL | Estado (success, error) |
| `error_message` | TEXT | SÍ | NULL | Mensaje de error si aplica |
| `created_at` | TIMESTAMP | NO | NOW() | Fecha de inserción |

#### Particionamiento Recomendado

Para tablas con alto volumen, se recomienda particionamiento por fecha:

```sql
-- Crear tabla particionada
CREATE TABLE request_metrics (
    -- ... campos ...
) PARTITION BY RANGE (request_timestamp);

-- Crear particiones mensuales
CREATE TABLE request_metrics_2026_02 PARTITION OF request_metrics
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE request_metrics_2026_03 PARTITION OF request_metrics
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

-- ... etc
```

#### Política de Retención

```sql
-- Eliminar métricas antiguas (ejemplo: > 6 meses)
DELETE FROM request_metrics 
WHERE request_timestamp < NOW() - INTERVAL '6 months';

-- O archivar a tabla histórica
INSERT INTO request_metrics_archive 
SELECT * FROM request_metrics 
WHERE request_timestamp < NOW() - INTERVAL '6 months';
```

---

## 🔗 RELACIONES ENTRE TABLAS

### Diagrama de Relaciones

```
users (1) ──────< (N) tokens
  │                     [FK: user_id → users.iam_username]
  │
  ├─────────< (N) quota_usage
  │                     [FK: user_id → users.iam_username]
  │
  ├─────────< (1) user_blocking_status
  │                     [FK: user_id → users.iam_username]
  │
  └─────────< (N) request_metrics
                        [NO FK, solo referencia lógica]
```

### Foreign Keys Definidas

```sql
-- tokens → users
ALTER TABLE tokens 
ADD CONSTRAINT fk_tokens_user 
FOREIGN KEY (user_id) REFERENCES users(iam_username) 
ON DELETE CASCADE;

-- quota_usage → users
ALTER TABLE quota_usage 
ADD CONSTRAINT fk_quota_user 
FOREIGN KEY (user_id) REFERENCES users(iam_username) 
ON DELETE CASCADE;

-- user_blocking_status → users
ALTER TABLE user_blocking_status 
ADD CONSTRAINT fk_blocking_user 
FOREIGN KEY (user_id) REFERENCES users(iam_username) 
ON DELETE CASCADE;

-- request_metrics NO tiene FK (por performance)
-- Pero mantiene integridad referencial a nivel de aplicación
```

---

## 🚀 ÍNDICES Y OPTIMIZACIONES

### Índices Críticos (Path Crítico)

Estos índices son **ESENCIALES** para el rendimiento del sistema:

```sql
-- 1. Validación de tokens (cada request)
CREATE UNIQUE INDEX idx_tokens_hash ON tokens(token_hash);

-- 2. Verificación de quotas (cada request)
CREATE INDEX idx_quota_usage_user_month ON quota_usage(user_id, month);

-- 3. Estado de bloqueo (cada request)
CREATE INDEX idx_blocking_status_user ON user_blocking_status(user_id);
```

### Índices Secundarios (Queries de Análisis)

```sql
-- Búsqueda por email
CREATE INDEX idx_users_email ON users(email);

-- Filtrado por equipo
CREATE INDEX idx_users_team ON users(team);
CREATE INDEX idx_metrics_team ON request_metrics(team);

-- Queries temporales
CREATE INDEX idx_metrics_timestamp ON request_metrics(request_timestamp DESC);

-- Búsqueda por request_id (debugging)
CREATE INDEX idx_metrics_request_id ON request_metrics(request_id);
```

### Índices Compuestos

```sql
-- Queries de métricas por usuario y fecha
CREATE INDEX idx_metrics_user_timestamp 
ON request_metrics(user_id, request_timestamp DESC);

-- Tokens activos por usuario
CREATE INDEX idx_tokens_user_active 
ON tokens(user_id, expires_at) 
WHERE is_revoked = false;
```

### Estadísticas y Mantenimiento

```sql
-- Actualizar estadísticas (ejecutar periódicamente)
ANALYZE users;
ANALYZE tokens;
ANALYZE quota_usage;
ANALYZE user_blocking_status;
ANALYZE request_metrics;

-- Vacuum para recuperar espacio
VACUUM ANALYZE request_metrics;
```

---

## 💻 ESTRUCTURAS GO CORRESPONDIENTES

### TokenInfo (pkg/database/queries.go)

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
    InferenceProfile string    // ARN del inference profile
}
```

### QuotaInfo (pkg/database/queries.go)

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

### MetricData (pkg/database/queries.go)

```go
type MetricData struct {
    UserID              string    // iam_username
    Team                string    // Equipo del usuario
    Person              string    // Nombre completo
    RequestTimestamp    time.Time // Timestamp del request
    ModelID             string    // Modelo usado
    RequestID           string    // UUID del request
    SourceIP            string    // IP del cliente
    UserAgent           string    // User-Agent
    AWSRegion           string    // Región de AWS
    TokensInput         int       // Tokens de input
    TokensOutput        int       // Tokens de output
    TokensCacheRead     int       // Tokens de caché leídos
    TokensCacheCreation int       // Tokens de caché escritos
    CostUSD             float64   // Costo en USD
    ProcessingTimeMS    int       // Tiempo de procesamiento
    ResponseStatus      string    // Estado de respuesta
    ErrorMessage        string    // Mensaje de error
}
```

---

## 🔍 QUERIES PRINCIPALES

### 1. Validar Token (Crítico - Path de Request)

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
    AND u.is_active = true;
```

**Frecuencia**: Por cada request HTTP  
**Performance**: < 5ms (con índice en token_hash)

### 2. Verificar Quotas (Crítico - Path de Request)

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
WHERE u.iam_username = $1 AND u.is_active = true;
```

**Frecuencia**: Por cada request HTTP  
**Performance**: < 10ms

### 3. Actualizar Quotas (Transaccional)

```sql
BEGIN;

-- Actualizar quota mensual
INSERT INTO quota_usage (user_id, month, total_cost_usd, total_requests, last_updated)
VALUES ($1, DATE_TRUNC('month', CURRENT_DATE), $2, 1, NOW())
ON CONFLICT (user_id, month) 
DO UPDATE SET 
    total_cost_usd = quota_usage.total_cost_usd + $2,
    total_requests = quota_usage.total_requests + 1,
    last_updated = NOW();

-- Actualizar contadores diarios
INSERT INTO user_blocking_status (user_id, daily_cost_usd, daily_requests, last_request_at, updated_at)
VALUES ($1, $2, 1, NOW(), NOW())
ON CONFLICT (user_id)
DO UPDATE SET
    daily_cost_usd = user_blocking_status.daily_cost_usd + $2,
    daily_requests = user_blocking_status.daily_requests + 1,
    last_request_at = NOW(),
    updated_at = NOW();

COMMIT;
```

**Frecuencia**: Por cada request completado (asíncrono)  
**Performance**: < 20ms

### 4. Verificar y Bloquear Usuario

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
    );
```

**Frecuencia**: Por cada request completado (asíncrono)  
**Performance**: < 10ms

### 5. Reset Diario (Cron 00:00 UTC)

```sql
BEGIN;

-- Resetear contadores y desbloquear usuarios con bloqueo automático
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
  AND blocked_by_admin_id IS NULL;

COMMIT;
```

**Frecuencia**: Una vez al día  
**Performance**: < 1 segundo (depende del número de usuarios)

---

## ⚡ CONSIDERACIONES DE PERFORMANCE

### 1. Connection Pooling

```go
// Configuración recomendada
poolConfig.MaxConns = 25          // Máximo de conexiones
poolConfig.MinConns = 5           // Mínimo de conexiones
poolConfig.MaxConnLifetime = 1h   // Vida máxima de conexión
poolConfig.MaxConnIdleTime = 30m  // Tiempo máximo inactivo
```

### 2. Prepared Statements

El driver `pgx/v5` usa automáticamente prepared statements para queries repetitivas.

### 3. Transacciones

- **ACID**: Usar transacciones para operaciones que modifican múltiples tablas
- **Isolation Level**: READ COMMITTED (default de PostgreSQL)
- **Timeout**: 30 segundos máximo

### 4. Particionamiento

Para `request_metrics` con alto volumen:

```sql
-- Particionamiento mensual
CREATE TABLE request_metrics_2026_02 PARTITION OF request_metrics
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
```

### 5. Archivado

```sql
-- Mover métricas antiguas a tabla de archivo
CREATE TABLE request_metrics_archive (LIKE request_metrics);

INSERT INTO request_metrics_archive 
SELECT * FROM request_metrics 
WHERE request_timestamp < NOW() - INTERVAL '6 months';

DELETE FROM request_metrics 
WHERE request_timestamp < NOW() - INTERVAL '6 months';
```

### 6. Monitoreo

```sql
-- Ver queries lentas
SELECT * FROM pg_stat_statements 
ORDER BY mean_exec_time DESC 
LIMIT 10;

-- Ver tamaño de tablas
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

---

## 📊 ESTIMACIÓN DE VOLUMEN

### Escenario: 100 usuarios activos

| Tabla | Registros | Crecimiento | Tamaño Estimado |
|-------|-----------|-------------|-----------------|
| `users` | 100 | Bajo | < 1 MB |
| `tokens` | 100-200 | Bajo | < 1 MB |
| `quota_usage` | 1,200 | 12/año/usuario | < 10 MB |
| `user_blocking_status` | 100 | Estable | < 1 MB |
| `request_metrics` | **ALTO** | 1M+/mes | **10+ GB/mes** |

### Recomendaciones

1. **Particionamiento**: Implementar para `request_metrics`
2. **Archivado**: Mover datos > 6 meses a tabla histórica
3. **Índices**: Mantener solo los necesarios
4. **Vacuum**: Ejecutar semanalmente
5. **Backup**: Diario incremental, semanal completo

---

## 🔐 SEGURIDAD

### 1. Encriptación

- **En tránsito**: SSL/TLS (sslmode=require)
- **En reposo**: Encriptación de disco (AWS RDS)
- **Tokens**: Solo se almacena el hash SHA256

### 2. Permisos

```sql
-- Usuario de aplicación (solo DML)
CREATE USER proxy_app WITH PASSWORD 'secure_password';
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO proxy_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO proxy_app;

-- Usuario de solo lectura (reportes)
CREATE USER proxy_readonly WITH PASSWORD 'secure_password';
GRANT SELECT ON ALL TABLES IN SCHEMA public TO proxy_readonly;
```

### 3. Auditoría

- Todos los requests se registran en `request_metrics`
- Cambios de estado en `user_blocking_status`
- Tokens revocados mantienen registro histórico

---

## 📝 SCRIPT DE CREACIÓN COMPLETO

```sql
-- ============================================
-- PROXY-BEDROCK DATABASE SCHEMA
-- Version: 1.1.0
-- PostgreSQL 13+
-- ============================================

-- 1. TABLA: users
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
CREATE INDEX idx_users_team ON users(team);

-- 2. TABLA: tokens
CREATE TABLE tokens (
    jti VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES users(iam_username) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    issued_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    is_revoked BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_tokens_hash ON tokens(token_hash);
CREATE INDEX idx_tokens_user_id ON tokens(user_id);
CREATE INDEX idx_tokens_expires_at ON tokens(expires_at);

-- 3. TABLA: quota_usage
CREATE TABLE quota_usage (
    user_id VARCHAR(255) NOT NULL REFERENCES users(iam_username) ON DELETE CASCADE,
    month DATE NOT NULL,
    total_cost_usd DECIMAL(10,6) DEFAULT 0,
    total_requests INT DEFAULT 0,
    last_updated TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (user_id, month)
);

CREATE INDEX idx_quota_usage_month ON quota_usage(month);

-- 4. TABLA: user_blocking_status
CREATE TABLE user_blocking_status (
    user_id VARCHAR(255) PRIMARY KEY REFERENCES users(iam_username) ON DELETE CASCADE,
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

-- 5. TABLA: request_metrics
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
CREATE INDEX idx_metrics_user_timestamp ON request_metrics(user_id, request_timestamp DESC);

-- ============================================
-- FIN DEL SCHEMA
-- ============================================
```

---

## 📞 CONTACTO Y SOPORTE

Para consultas sobre el modelo de datos:
- **Documentación**: `README.md`, `ANALISIS_INTEGRACION_BD.md`
- **Código fuente**: `pkg/database/`
- **Logs**: `logs/bedrock-proxy_*.json`

---

**Documento generado**: 3 de febrero de 2026  
**Versión**: 1.0  
**Autor**: Análisis automatizado del proyecto proxy-bedrock