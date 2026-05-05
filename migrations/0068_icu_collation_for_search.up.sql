-- ICU case-insensitive collation для колонок поиска axenta_account_snapshots.
--
-- Проблема: PostgreSQL ILIKE без ICU не делает Unicode case-folding кириллицы —
-- "служе" ILIKE "%Служебные%" возвращало false на проде, хотя для латиницы
-- работало корректно (ILIKE содержит ASCII case-folding встроенным).
--
-- Решение: ICU collation `acrm_und_ci` с deterministic=false применяется
-- к колонкам поиска. Все операции = и LIKE автоматически становятся
-- case-insensitive для любого языка (ru/kz/uz и др.) — без ILIKE.
--
-- Требования: PostgreSQL ≥ 12 + ICU. Проверено на pg17 (Debian 13).

CREATE COLLATION IF NOT EXISTS acrm_und_ci (
    provider = icu,
    locale = 'und-u-ks-level2',
    deterministic = false
);

-- Применяем к колонкам поиска axenta_account_snapshots.
-- ALTER COLUMN TYPE с COLLATE автоматически перестроит зависимые индексы.
ALTER TABLE axenta_account_snapshots
    ALTER COLUMN account_name TYPE TEXT COLLATE acrm_und_ci,
    ALTER COLUMN admin_fullname TYPE TEXT COLLATE acrm_und_ci,
    ALTER COLUMN parent_account_name TYPE TEXT COLLATE acrm_und_ci;
