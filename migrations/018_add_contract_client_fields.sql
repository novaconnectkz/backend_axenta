-- Добавляем недостающие поля для клиентов в таблицу contracts
-- Эта миграция должна быть применена ко всем tenant-схемам

-- Тип клиента
ALTER TABLE contracts
ADD COLUMN IF NOT EXISTS client_type VARCHAR(20);

-- Дополнительные общие поля
ALTER TABLE contracts
ADD COLUMN IF NOT EXISTS client_short_name VARCHAR(200);

-- Дополнительные поля для организаций
ALTER TABLE contracts
ADD COLUMN IF NOT EXISTS client_legal_address TEXT,
ADD COLUMN IF NOT EXISTS client_postal_address TEXT,
ADD COLUMN IF NOT EXISTS client_ogrn VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_okpo VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_director VARCHAR(200),
ADD COLUMN IF NOT EXISTS client_based_on VARCHAR(200),
ADD COLUMN IF NOT EXISTS client_website VARCHAR(200);

-- Банковские реквизиты
ALTER TABLE contracts
ADD COLUMN IF NOT EXISTS client_bank_name VARCHAR(200),
ADD COLUMN IF NOT EXISTS client_bank_bik VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_bank_correspondent_account VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_bank_account VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_bank_recipient VARCHAR(200);

-- Поля для физических лиц
ALTER TABLE contracts
ADD COLUMN IF NOT EXISTS client_passport_series VARCHAR(10),
ADD COLUMN IF NOT EXISTS client_passport_number VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_passport_issued_by TEXT,
ADD COLUMN IF NOT EXISTS client_passport_issue_date VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_passport_department_code VARCHAR(10),
ADD COLUMN IF NOT EXISTS client_registration_address TEXT,
ADD COLUMN IF NOT EXISTS client_actual_address TEXT,
ADD COLUMN IF NOT EXISTS client_snils VARCHAR(20),
ADD COLUMN IF NOT EXISTS client_ogrn_ip VARCHAR(20);

-- Устанавливаем значение по умолчанию для существующих записей
UPDATE contracts
SET client_type = 'organization'
WHERE client_type IS NULL;

