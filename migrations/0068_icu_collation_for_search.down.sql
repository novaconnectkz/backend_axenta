-- Откат: возвращаем колонки на default collation базы.
ALTER TABLE axenta_account_snapshots
    ALTER COLUMN account_name TYPE TEXT COLLATE "default",
    ALTER COLUMN admin_fullname TYPE TEXT COLLATE "default",
    ALTER COLUMN parent_account_name TYPE TEXT COLLATE "default";

DROP COLLATION IF EXISTS acrm_und_ci;
