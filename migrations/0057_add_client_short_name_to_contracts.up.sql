-- Добавление поля client_short_name в таблицу contracts
-- Дата: 2025-11-21
-- Описание: Добавляет сокращенное название клиента с ОПФ для организаций

ALTER TABLE contracts 
ADD COLUMN IF NOT EXISTS client_short_name VARCHAR(200) DEFAULT NULL;

-- Добавление комментария к полю
COMMENT ON COLUMN contracts.client_short_name IS 'Сокращенное название с ОПФ (для организаций)';

-- Индекс для быстрого поиска (опционально)
-- CREATE INDEX IF NOT EXISTS idx_contracts_client_short_name ON contracts(client_short_name);

