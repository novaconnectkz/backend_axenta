-- Делаем tariff_plan_id опциональным (nullable) в таблице contracts
-- Тарифный план будет привязан через подписку

ALTER TABLE contracts
    ALTER COLUMN tariff_plan_id DROP NOT NULL;

-- Устанавливаем NULL для существующих записей, где tariff_plan_id = 0
UPDATE contracts
SET tariff_plan_id = NULL
WHERE tariff_plan_id = 0;

