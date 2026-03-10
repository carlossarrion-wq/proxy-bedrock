-- =====================================================
-- Function: check_and_update_quota
-- Description: Verifica y actualiza las cuotas de usuario
-- Version: 2.1.0
-- Last Updated: 2026-03-10
-- Changes: Añadido soporte para guardar campo 'person'
-- =====================================================

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
) AS $$
DECLARE
    v_daily_limit INTEGER;
    v_requests_today INTEGER;
    v_is_blocked BOOLEAN;
    v_blocked_reason TEXT;
    v_administrative_safe BOOLEAN;
    v_blocked_until TIMESTAMP WITH TIME ZONE;
BEGIN
    -- Obtener límite diario por defecto desde configuración
    SELECT COALESCE(
        (SELECT config_value::INTEGER 
         FROM "identity-manager-config-tbl" 
         WHERE config_key = 'default_daily_request_limit'),
        1000
    ) INTO v_daily_limit;

    -- Insertar o actualizar el registro del usuario
    -- IMPORTANTE: Ahora incluye el campo 'person' en INSERT y UPDATE
    INSERT INTO "bedrock-proxy-user-quotas-tbl" (
        cognito_user_id,
        cognito_email,
        team,
        person,  -- ✅ AÑADIDO: Campo person del JWT
        requests_today,
        last_request_at,
        created_at
    ) VALUES (
        p_cognito_user_id,
        p_cognito_email,
        p_team,
        p_person,  -- ✅ AÑADIDO: Valor del parámetro person
        1,
        NOW(),
        NOW()
    )
    ON CONFLICT (cognito_user_id) DO UPDATE SET
        requests_today = "bedrock-proxy-user-quotas-tbl".requests_today + 1,
        last_request_at = NOW(),
        team = p_team,
        person = p_person  -- ✅ AÑADIDO: Actualizar person en cada petición
    RETURNING 
        COALESCE("bedrock-proxy-user-quotas-tbl".daily_request_limit, v_daily_limit),
        "bedrock-proxy-user-quotas-tbl".requests_today,
        "bedrock-proxy-user-quotas-tbl".is_blocked,
        "bedrock-proxy-user-quotas-tbl".block_reason,
        "bedrock-proxy-user-quotas-tbl".administrative_safe,
        "bedrock-proxy-user-quotas-tbl".blocked_until
    INTO 
        v_daily_limit,
        v_requests_today,
        v_is_blocked,
        v_blocked_reason,
        v_administrative_safe,
        v_blocked_until;

    -- Verificar si el usuario está bloqueado administrativamente
    IF v_is_blocked AND v_blocked_until IS NOT NULL THEN
        IF v_blocked_until > NOW() THEN
            -- Bloqueo administrativo activo
            RETURN QUERY SELECT 
                FALSE,
                v_requests_today,
                v_daily_limit,
                TRUE,
                COALESCE(v_blocked_reason, 'User is administratively blocked');
            RETURN;
        ELSE
            -- Bloqueo administrativo expirado - desbloquear
            UPDATE "bedrock-proxy-user-quotas-tbl"
            SET 
                is_blocked = FALSE,
                blocked_at = NULL,
                blocked_until = NULL,
                block_reason = NULL,
                administrative_safe = FALSE
            WHERE cognito_user_id = p_cognito_user_id;
            
            v_is_blocked := FALSE;
        END IF;
    END IF;

    -- Verificar si está en modo "administrative safe"
    IF v_administrative_safe THEN
        -- Permitir hasta medianoche
        RETURN QUERY SELECT 
            TRUE,
            v_requests_today,
            v_daily_limit,
            FALSE,
            NULL::TEXT;
        RETURN;
    END IF;

    -- Verificar si excede el límite diario
    IF v_requests_today > v_daily_limit THEN
        -- Bloquear usuario
        UPDATE "bedrock-proxy-user-quotas-tbl"
        SET 
            is_blocked = TRUE,
            blocked_at = NOW(),
            block_reason = 'Daily request limit exceeded'
        WHERE cognito_user_id = p_cognito_user_id
          AND is_blocked = FALSE;

        RETURN QUERY SELECT 
            FALSE,
            v_requests_today,
            v_daily_limit,
            TRUE,
            'Daily request limit exceeded'::TEXT;
        RETURN;
    END IF;

    -- Usuario dentro del límite
    RETURN QUERY SELECT 
        TRUE,
        v_requests_today,
        v_daily_limit,
        FALSE,
        NULL::TEXT;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- Comentarios sobre el cambio v2.1.0
-- =====================================================
-- ANTES: El campo 'person' no se guardaba en el INSERT inicial
-- AHORA: El campo 'person' se guarda tanto en INSERT como en UPDATE
-- 
-- Esto permite:
-- 1. Trazabilidad completa de quién realizó cada petición
-- 2. Análisis y reportes por persona
-- 3. Mantener la información actualizada si cambia en el JWT
-- =====================================================