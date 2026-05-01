-- Миграция для заполнения account_id в существующих договорах
-- account_id определяется через company_id объектов, привязанных к договору

-- Обновляем account_id для договоров, у которых есть привязанные объекты
-- Берем company_id из первого привязанного объекта
UPDATE contracts c
SET account_id = (
    SELECT company_id 
    FROM objects o 
    WHERE o.contract_id = c.id 
    LIMIT 1
)
WHERE c.account_id IS NULL 
  AND EXISTS (
      SELECT 1 
      FROM objects o 
      WHERE o.contract_id = c.id
  );

-- Для договоров без объектов оставляем account_id = NULL
-- (они будут использовать company_id договора как fallback)

-- Проверка результата
SELECT 
    id, 
    number, 
    company_id, 
    account_id,
    (SELECT COUNT(*) FROM objects WHERE contract_id = contracts.id) as objects_count
FROM contracts
WHERE account_id IS NOT NULL
ORDER BY id;

