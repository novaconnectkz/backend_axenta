-- Исправление уникального индекса для поддержки партнеров без договоров
-- Дата: 2025-12-02

\echo 'Удаляю старый некорректный индекс...'

SET search_path TO tenant_186;

DROP INDEX IF EXISTS idx_partner_snapshot_unique;

\echo 'Создаю правильный индекс (partner_company_id + snapshot_date)...'

CREATE UNIQUE INDEX idx_partner_snapshot_unique 
ON partner_daily_snapshots(partner_company_id, snapshot_date);

\echo 'Проверка:'
SELECT indexname, indexdef FROM pg_indexes 
WHERE tablename = 'partner_daily_snapshots' 
  AND indexname = 'idx_partner_snapshot_unique';

\echo ''
\echo 'Готово!'

