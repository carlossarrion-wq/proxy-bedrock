-- =====================================================
-- Debug: Verificar por qué person no se actualiza
-- Date: 2026-03-10
-- =====================================================

-- PASO 1: Ver el registro actual
SELECT 
    cognito_user_id,
    cognito_email,
    team,
    person,
    requests_today,
    last_request_at,
    updated_at
FROM "bedrock-proxy-user-quotas-tbl"
WHERE cognito_email = 'carlos.sarrion@es.ibm.com';

-- PASO 2: Simular una llamada a la función con person
-- Esto debería actualizar el campo person
SELECT * FROM check_and_update_quota(
    '62d5f404-90d1-70cc-e0d6-a8cb2d156cbc',  -- cognito_user_id
    'carlos.sarrion@es.ibm.com',              -- cognito_email
    'lcs-sdlc-gen-group',                     -- team
    'Carlos SArrion'                          -- person (CON VALOR)
);

-- PASO 3: Verificar si se actualizó
SELECT 
    cognito_user_id,
    cognito_email,
    team,
    person,  -- ¿Se actualizó?
    requests_today,
    last_request_at,
    updated_at
FROM "bedrock-proxy-user-quotas-tbl"
WHERE cognito_email = 'carlos.sarrion@es.ibm.com';

-- =====================================================
-- DIAGNÓSTICO DEL PROBLEMA
-- =====================================================
-- Si después del PASO 3 el campo person sigue siendo NULL,
-- el problema está en la lógica del ON CONFLICT:
--
-- person = COALESCE(EXCLUDED.person, "bedrock-proxy-user-quotas-tbl".person)
--
-- Esta línea significa:
-- - Si el NUEVO valor (EXCLUDED.person) NO es NULL → usa el nuevo
-- - Si el NUEVO valor ES NULL → mantiene el antiguo
--
-- PERO hay un caso especial:
-- Si el registro YA EXISTE con person=NULL, y llega un UPDATE,
-- el COALESCE compara:
-- - EXCLUDED.person (el nuevo valor que llega)
-- - "bedrock-proxy-user-quotas-tbl".person (el valor actual = NULL)
--
-- Si AMBOS son NULL, entonces person se mantiene NULL.
--
-- SOLUCIÓN: Cambiar a actualización directa sin COALESCE
-- =====================================================