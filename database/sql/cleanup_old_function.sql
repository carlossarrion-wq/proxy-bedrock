-- =====================================================
-- Cleanup: Eliminar función antigua con tipos VARCHAR
-- Description: Elimina la versión antigua de check_and_update_quota
-- Date: 2026-03-10
-- =====================================================

-- Verificar qué funciones existen actualmente
SELECT 
    p.proname as function_name,
    pg_get_function_identity_arguments(p.oid) as arguments,
    pg_get_functiondef(p.oid) as definition_preview
FROM pg_proc p
JOIN pg_namespace n ON p.pronamespace = n.oid
WHERE n.nspname = 'public'
  AND p.proname = 'check_and_update_quota';

-- Eliminar la función antigua con tipos VARCHAR
DROP FUNCTION IF EXISTS check_and_update_quota(varchar, varchar, varchar, varchar);

-- Verificar que solo queda la función correcta (con TEXT)
DO $$
DECLARE
    v_function_count INTEGER;
    v_correct_function_exists BOOLEAN;
BEGIN
    -- Contar cuántas funciones check_and_update_quota existen
    SELECT COUNT(*) 
    INTO v_function_count
    FROM pg_proc p
    JOIN pg_namespace n ON p.pronamespace = n.oid
    WHERE n.nspname = 'public'
      AND p.proname = 'check_and_update_quota';
    
    -- Verificar que existe la función correcta (con TEXT)
    SELECT EXISTS (
        SELECT 1 
        FROM pg_proc p
        JOIN pg_namespace n ON p.pronamespace = n.oid
        WHERE n.nspname = 'public'
          AND p.proname = 'check_and_update_quota'
          AND pg_get_function_identity_arguments(p.oid) = 'p_cognito_user_id text, p_cognito_email text, p_team text, p_person text'
    ) INTO v_correct_function_exists;
    
    IF v_function_count = 1 AND v_correct_function_exists THEN
        RAISE NOTICE '✅ Limpieza exitosa: Solo existe la función correcta con tipos TEXT';
    ELSIF v_function_count > 1 THEN
        RAISE WARNING '⚠️  Aún existen % versiones de la función', v_function_count;
    ELSIF NOT v_correct_function_exists THEN
        RAISE EXCEPTION '❌ Error: La función correcta (con TEXT) no existe';
    END IF;
END $$;

-- Mostrar la función final
SELECT 
    'check_and_update_quota' as function_name,
    pg_get_function_identity_arguments(p.oid) as parameters,
    'Función activa' as status
FROM pg_proc p
JOIN pg_namespace n ON p.pronamespace = n.oid
WHERE n.nspname = 'public'
  AND p.proname = 'check_and_update_quota';

-- =====================================================
-- Resultado esperado:
-- =====================================================
-- Debe mostrar solo UNA función:
-- check_and_update_quota(p_cognito_user_id text, p_cognito_email text, p_team text, p_person text)
-- =====================================================