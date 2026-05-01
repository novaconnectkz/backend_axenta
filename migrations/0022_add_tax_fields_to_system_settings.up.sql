-- Добавление полей налогообложения в system_settings
-- Дата: 2025-11-26

\echo 'Добавление default_tax_rate и tax_included в system_settings...'

-- Добавляем колонки если их нет
ALTER TABLE public.system_settings
  ADD COLUMN IF NOT EXISTS default_tax_rate NUMERIC(5,2) DEFAULT 20.00,
  ADD COLUMN IF NOT EXISTS tax_included BOOLEAN DEFAULT false;

\echo ''
\echo 'Проверка:'
SELECT column_name, data_type, column_default
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'system_settings'
  AND column_name IN ('default_tax_rate', 'tax_included');

\echo ''
\echo 'Готово!'

