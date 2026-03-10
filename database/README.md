# Database Functions Documentation

## Overview

Este directorio contiene la documentación de las funciones de base de datos utilizadas por el proxy Bedrock.

## Funciones Principales

### `check_and_update_quota()`

Función principal para verificar y actualizar las cuotas de usuario en cada petición.

#### Firma

```sql
CREATE OR REPLACE FUNCTION check_and_update_quota(
    p_cognito_user_id TEXT,
    p_cognito_email TEXT,
    p_team TEXT,
    p_person TEXT
)
RETURNS TABLE(
    allowed BOOLEAN,
    requests_today INTEGER,
    daily_limit INTEGER,
    is_blocked BOOLEAN,
    blocked_reason TEXT
)
```

#### Descripción

Esta función:
1. Inserta o actualiza el registro del usuario en `bedrock-proxy-user-quotas-tbl`
2. Incrementa el contador de peticiones diarias
3. Verifica si el usuario ha excedido su límite diario
4. Gestiona bloqueos administrativos y el modo "administrative safe"
5. **Actualiza los campos `team` y `person` del JWT en cada petición**

#### Parámetros

- `p_cognito_user_id` (TEXT): ID del usuario de Cognito
- `p_cognito_email` (TEXT): Email del usuario
- `p_team` (TEXT): Equipo del usuario (extraído del JWT)
- `p_person` (TEXT): Persona del usuario (extraído del JWT)

#### Retorna

Una tabla con los siguientes campos:
- `allowed` (BOOLEAN): Si la petición está permitida
- `requests_today` (INTEGER): Número de peticiones realizadas hoy
- `daily_limit` (INTEGER): Límite diario de peticiones
- `is_blocked` (BOOLEAN): Si el usuario está bloqueado
- `blocked_reason` (TEXT): Razón del bloqueo (si aplica)

#### Comportamiento

##### Primera Interacción (INSERT)
Cuando un usuario hace su primera petición del día, la función:
- Crea un nuevo registro en `bedrock-proxy-user-quotas-tbl`
- Inicializa `requests_today` en 1
- **Guarda los campos `team` y `person` del JWT**
- Establece `last_request_at` y `created_at` al timestamp actual

##### Interacciones Subsecuentes (UPDATE)
En peticiones posteriores:
- Incrementa `requests_today` en 1
- Actualiza `last_request_at`
- **Actualiza `team` y `person` con los valores actuales del JWT**

##### Verificación de Límites
1. **Bloqueo Administrativo**: Si `is_blocked = TRUE` y `blocked_until > NOW()`, rechaza la petición
2. **Modo Administrative Safe**: Si `administrative_safe = TRUE`, permite la petición hasta medianoche
3. **Límite Diario**: Si `requests_today > daily_limit`, bloquea al usuario automáticamente

#### Ejemplo de Uso desde Go

```go
quotaResult, err := db.CheckAndUpdateQuota(
    ctx, 
    claims.UserID,    // cognito_user_id
    claims.Email,     // cognito_email
    claims.Team,      // team del JWT
    claims.Person     // person del JWT
)

if err != nil {
    return fmt.Errorf("error checking quota: %w", err)
}

if !quotaResult.Allowed {
    return fmt.Errorf("quota exceeded: %s", quotaResult.BlockReason)
}
```

#### Notas Importantes

1. **Campos Team y Person**: Estos campos se extraen del JWT en cada petición y se actualizan en la base de datos. Esto permite:
   - Mantener la información actualizada si cambia en el JWT
   - Tener trazabilidad de qué equipo/persona realizó cada petición
   - Facilitar análisis y reportes por equipo/persona

2. **Reset Diario**: Los contadores `requests_today` se resetean automáticamente a medianoche UTC mediante un job programado (ver `scheduler/scheduler.go`)

3. **Límite por Defecto**: Si un usuario no tiene un `daily_request_limit` personalizado, se usa el valor de la configuración global `default_daily_request_limit` (por defecto: 1000)

4. **Transaccionalidad**: La función es atómica - si falla cualquier parte, no se incrementa el contador

#### Cambios Recientes

**v2.1.0 (2026-03-10)**
- ✅ Corregido: Ahora la función guarda correctamente el campo `person` en el INSERT inicial
- ✅ Mejorado: El campo `person` se actualiza también en el ON CONFLICT UPDATE
- ✅ Añadido: Documentación completa de la función

**Antes del cambio:**
```sql
INSERT INTO "bedrock-proxy-user-quotas-tbl" (
    cognito_user_id,
    cognito_email,
    team,
    -- person NO se guardaba ❌
    requests_today,
    ...
)
```

**Después del cambio:**
```sql
INSERT INTO "bedrock-proxy-user-quotas-tbl" (
    cognito_user_id,
    cognito_email,
    team,
    person,  -- ✅ Ahora se guarda correctamente
    requests_today,
    ...
) VALUES (
    p_cognito_user_id,
    p_cognito_email,
    p_team,
    p_person,  -- ✅ Valor del parámetro
    1,
    ...
)
ON CONFLICT (cognito_user_id) DO UPDATE SET
    requests_today = "bedrock-proxy-user-quotas-tbl".requests_today + 1,
    last_request_at = NOW(),
    team = p_team,
    person = p_person  -- ✅ También se actualiza en conflicto
```

## Otras Funciones

### `get_user_quota_status()`

Obtiene el estado completo de cuota de un usuario.

### `administrative_unblock_user()`

Desbloquea un usuario administrativamente y activa el modo "administrative safe".

### `administrative_block_user()`

Bloquea un usuario administrativamente hasta una fecha específica.

### `update_user_daily_limit()`

Actualiza el límite diario de peticiones de un usuario.

## Tablas Relacionadas

### `bedrock-proxy-user-quotas-tbl`

Tabla principal de cuotas de usuario.

**Campos principales:**
- `cognito_user_id` (TEXT, PK): ID del usuario
- `cognito_email` (TEXT): Email del usuario
- `team` (TEXT): Equipo del usuario (del JWT)
- `person` (TEXT): Persona del usuario (del JWT)
- `requests_today` (INTEGER): Contador de peticiones del día
- `daily_request_limit` (INTEGER): Límite personalizado (NULL = usar default)
- `is_blocked` (BOOLEAN): Estado de bloqueo
- `blocked_at` (TIMESTAMP): Cuándo fue bloqueado
- `blocked_until` (TIMESTAMP): Hasta cuándo está bloqueado (bloqueo admin)
- `block_reason` (TEXT): Razón del bloqueo
- `administrative_safe` (BOOLEAN): Modo safe activado por admin
- `last_request_at` (TIMESTAMP): Última petición
- `created_at` (TIMESTAMP): Fecha de creación

### `bedrock-proxy-usage-tracking-tbl`

Tabla de tracking detallado de uso.

**Campos principales:**
- `cognito_user_id` (TEXT): ID del usuario
- `cognito_email` (TEXT): Email del usuario
- `team` (TEXT): Equipo (del JWT)
- `person` (TEXT): Persona (del JWT)
- `request_timestamp` (TIMESTAMP): Timestamp de la petición
- `model_id` (TEXT): ID del modelo usado
- `tokens_input` (INTEGER): Tokens de entrada
- `tokens_output` (INTEGER): Tokens de salida
- `tokens_cache_read` (INTEGER): Tokens leídos de caché
- `tokens_cache_creation` (INTEGER): Tokens escritos en caché
- `cost_usd` (DECIMAL): Costo en USD
- `processing_time_ms` (INTEGER): Tiempo de procesamiento
- `response_status` (TEXT): Estado de la respuesta
- `error_message` (TEXT): Mensaje de error (si aplica)

## Mantenimiento

### Reset Diario de Contadores

Los contadores se resetean automáticamente a medianoche UTC mediante el scheduler:

```go
// En scheduler/scheduler.go
func (s *SchedulerService) resetDailyCounters() {
    query := `
        UPDATE "bedrock-proxy-user-quotas-tbl"
        SET 
            requests_today = 0,
            is_blocked = FALSE,
            blocked_at = NULL,
            block_reason = NULL,
            administrative_safe = FALSE
        WHERE is_blocked = FALSE 
           OR (is_blocked = TRUE AND blocked_until IS NULL)
    `
    // ...
}
```

### Queries de Diagnóstico

```sql
-- Ver usuarios sin person (deberían ser 0 después del fix)
SELECT cognito_user_id, cognito_email, team, person, created_at
FROM "bedrock-proxy-user-quotas-tbl"
WHERE person IS NULL OR person = ''
ORDER BY last_request_at DESC;

-- Ver usuarios cerca del límite
SELECT * FROM "v_users_near_limit";

-- Ver usuarios bloqueados
SELECT cognito_user_id, cognito_email, requests_today, daily_request_limit, 
       block_reason, blocked_at
FROM "bedrock-proxy-user-quotas-tbl"
WHERE is_blocked = TRUE
ORDER BY blocked_at DESC;
```

## Referencias

- Código Go: `pkg/database/quota_queries.go`
- Middleware: `pkg/auth/middleware.go`
- Scheduler: `pkg/scheduler/scheduler.go`