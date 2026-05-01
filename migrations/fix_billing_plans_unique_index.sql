-- Миграция: Исправление уникального индекса для billing_plans
-- Удаляем старый глобальный уникальный индекс на name
-- Создаем новый составной индекс на (name, company_id) для изоляции по компаниям

-- Убеждаемся, что мы в схеме public
SET search_path TO public;

-- Удаляем старый уникальный индекс, если он существует
DO $$
BEGIN
    -- Пробуем удалить индекс по имени (если он был создан GORM)
    IF EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'billing_plans' 
        AND indexname LIKE '%name%unique%'
    ) THEN
        DROP INDEX IF EXISTS billing_plans_name_key;
        DROP INDEX IF EXISTS idx_billing_plans_name;
        DROP INDEX IF EXISTS billing_plans_name_idx;
    END IF;
END $$;

-- Удаляем составной индекс, если он уже существует (для пересоздания)
DROP INDEX IF EXISTS idx_billing_plan_company_name;

-- Создаем новый составной уникальный индекс на (name, company_id)
-- Это позволяет каждой компании иметь свои планы с одинаковыми именами
CREATE UNIQUE INDEX idx_billing_plan_company_name 
ON billing_plans(name, company_id) 
WHERE deleted_at IS NULL;

-- Комментарий к индексу
COMMENT ON INDEX idx_billing_plan_company_name IS 
'Уникальный индекс для обеспечения уникальности имени тарифного плана в рамках компании';

