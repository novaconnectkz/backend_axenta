-- Таблица для маппинга Wialon user_id → name + short_name (prp.label/prp.short_name).
-- Заполняется WialonStatsScheduler через core/search_items itemsType=avl_user flag=0x00000002.
-- Используется в /api/auth/unified/objects для подмены creator/accountName реальными именами
-- 1:1 с Wialon CMS Manager (где CMS показывает короткое имя, не полное name).

CREATE TABLE IF NOT EXISTS wialon_users (
    id BIGSERIAL PRIMARY KEY,

    connection_id INTEGER NOT NULL REFERENCES wialon_connections(id) ON DELETE CASCADE,
    user_id       BIGINT  NOT NULL,                 -- avl_user.id в Wialon

    name          VARCHAR(255) NOT NULL DEFAULT '', -- avl_user.nm (полное имя)
    short_name    VARCHAR(255) NOT NULL DEFAULT '', -- prp.label или prp.short_name (то что CMS показывает)
    is_active     BOOLEAN      NOT NULL DEFAULT true,

    last_collected_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT uq_wialon_users_conn_user UNIQUE (connection_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_wialon_users_connection ON wialon_users (connection_id);
CREATE INDEX IF NOT EXISTS idx_wialon_users_user_id    ON wialon_users (user_id);
CREATE INDEX IF NOT EXISTS idx_wialon_users_last_coll  ON wialon_users (last_collected_at);
