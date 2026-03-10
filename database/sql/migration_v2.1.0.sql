-- =====================================================
-- Migration: v2.1.0
-- Description: Fix para campo 'person' en check_and_update_quota
-- Date: 2026-03-10
-- =====================================================

-- Este script actualiza la función check_and_update_quota para
-- guardar correctamente el campo 'person' del JWT

-- PASO 1: Backup de la función actual (opcional)
-- Puedes ejecutar esto para ver la versión actual antes de actualizar:
-- SELECT pg_get_functiondef('check_and_update_quota'::regproc);

-- PASO 2: Actualizar la función
\i check_and_update_quota.sql

-- PASO 3: Verificar que la función se actualizó correctamente
DO $$
DECLARE
    v_function_exists BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1 
        FROM pg_proc p
        JOIN pg_namespace n ON p.pronamespace = n.oid
        WHERE n.nspname = 'public'
          AND p.proname = 'check_and_update_quota'
          AND p.pronargs = 4  -- Debe tener 4 parámetros
    ) INTO v_function_exists;
    
    IF v_function_exists THEN
        RAISE NOTICE '✅ Función check_and_update_quota actualizada correctamente';
    ELSE
        RAISE EXCEPTION '❌ Error: La función check_and_update_quota no existe o tiene parámetros incorrectos';
    END IF;
END $$;

-- PASO 4: Verificar registros sin 'person'
DO $$
DECLARE
    v_null_person_count INTEGER;
BEGIN
    SELECT COUNT(*) 
    INTO v_null_person_count
    FROM "bedrock-proxy-user-quotas-tbl"
    WHERE person IS NULL OR person = '';
    
    RAISE NOTICE 'ℹ️  Registros con person NULL o vacío: %', v_null_person_count;
    RAISE NOTICE 'ℹ️  Estos registros se actualizarán automáticamente en su próxima petición';
END $$;

-- PASO 5: (Opcional) Actualizar registros existentes si tienes los datos
-- Si tienes una tabla con la información de person por usuario, puedes ejecutar:
-- UPDATE "bedrock-proxy-user-quotas-tbl" q
-- SET person = u.person
-- FROM tu_tabla_usuarios u
-- WHERE q.cognito_user_id = u.cognito_user_id
--   AND (q.person IS NULL OR q.person = '');

-- =====================================================
-- Notas de la migración
-- =====================================================
-- 1. Esta migración es NO DESTRUCTIVA - solo actualiza la función
-- 2. No se requiere downtime - la función se actualiza atómicamente
-- 3. Los registros existentes con person=NULL se actualizarán automáticamente
-- 4. No hay cambios en el esquema de la tabla
-- 5. Compatible con versiones anteriores del código Go
-- =====================================================

-- =====================================================
-- Rollback (si es necesario)
-- =====================================================
-- Si necesitas hacer rollback, ejecuta la versión anterior de la función
-- que NO incluía el campo person en INSERT/UPDATE
-- =====================================================