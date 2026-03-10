# Database Functions Changelog

## Version 2.1.0 (2026-03-10)

### `check_and_update_quota()`

**🐛 Bug Fix: Campo 'person' no se guardaba en primera interacción**

#### Problema
El campo `person` del JWT no se estaba guardando cuando se creaba el registro inicial del usuario en `bedrock-proxy-user-quotas-tbl`. Esto causaba que:
- Los registros nuevos tuvieran `person = NULL`
- No se pudiera rastrear qué persona realizó las peticiones
- Los reportes por persona estuvieran incompletos

#### Solución
Se modificó la función `check_and_update_quota()` para:

1. **Incluir `person` en el INSERT inicial:**
   ```sql
   INSERT INTO "bedrock-proxy-user-quotas-tbl" (
       cognito_user_id,
       cognito_email,
       team,
       person,  -- ✅ AÑADIDO
       ...
   ) VALUES (
       p_cognito_user_id,
       p_cognito_email,
       p_team,
       p_person,  -- ✅ AÑADIDO
       ...
   )
   ```

2. **Actualizar `person` en cada petición:**
   ```sql
   ON CONFLICT (cognito_user_id) DO UPDATE SET
       requests_today = ...,
       last_request_at = NOW(),
       team = p_team,
       person = p_person  -- ✅ AÑADIDO
   ```

#### Impacto
- ✅ Todos los nuevos registros tendrán el campo `person` correctamente poblado
- ✅ Los registros existentes se actualizarán en su próxima petición
- ✅ Mejora la trazabilidad y análisis de uso por persona

#### Migración
No se requiere migración de datos. Los registros existentes con `person = NULL` se actualizarán automáticamente en la próxima petición del usuario.

#### Verificación
```sql
-- Ver usuarios sin person (deberían disminuir con el tiempo)
SELECT cognito_user_id, cognito_email, team, person, 
       last_request_at, created_at
FROM "bedrock-proxy-user-quotas-tbl"
WHERE person IS NULL OR person = ''
ORDER BY last_request_at DESC;

-- Ver distribución de personas
SELECT person, COUNT(*) as user_count, 
       SUM(requests_today) as total_requests
FROM "bedrock-proxy-user-quotas-tbl"
WHERE person IS NOT NULL AND person != ''
GROUP BY person
ORDER BY total_requests DESC;
```

---

## Version 2.0.0 (2026-03-01)

### Funciones Iniciales

- `check_and_update_quota()`: Verificación y actualización de cuotas
- `get_user_quota_status()`: Obtener estado de cuota
- `administrative_unblock_user()`: Desbloqueo administrativo
- `administrative_block_user()`: Bloqueo administrativo
- `update_user_daily_limit()`: Actualizar límite diario

### Características
- Sistema de cuotas diarias por usuario
- Bloqueo automático al exceder límites
- Modo "administrative safe" para excepciones
- Reset automático a medianoche UTC