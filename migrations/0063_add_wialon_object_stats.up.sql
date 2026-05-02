-- Таблица для кэша статистики объектов Wialon
-- Заполняется фоновым планировщиком (WialonStatsScheduler) — раз в N минут обходит все
-- активные wialon_connections, для каждого ресурса получает usage из account/get_account_data
-- и upsert-ит в эту таблицу. Endpoint /api/wialon/connections/:id/objects-stats читает
-- данные из этой таблицы вместо живого запроса к Wialon (raw запрос для WH с 3412 ресурсов
-- занимает 6.5 минут — неприемлемо для UI).

CREATE TABLE IF NOT EXISTS wialon_object_stats (
    id BIGSERIAL PRIMARY KEY,

    connection_id   INTEGER NOT NULL REFERENCES wialon_connections(id) ON DELETE CASCADE,
    resource_id     BIGINT  NOT NULL, -- ID ресурса (avl_resource) в Wialon
    user_id         BIGINT, -- ID пользователя-владельца (для маппинга в /accounts UI)

    objects_total       INTEGER NOT NULL DEFAULT 0, -- всего объектов
    objects_active      INTEGER NOT NULL DEFAULT 0, -- активных (activated_units)
    objects_deactivated INTEGER NOT NULL DEFAULT 0, -- сезонных/деактивированных

    last_collected_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (connection_id, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_wialon_object_stats_connection ON wialon_object_stats(connection_id);
CREATE INDEX IF NOT EXISTS idx_wialon_object_stats_user_id    ON wialon_object_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_wialon_object_stats_collected  ON wialon_object_stats(last_collected_at);

COMMENT ON TABLE  wialon_object_stats IS 'Кэш статистики объектов Wialon (заполняется фоновым cron)';
COMMENT ON COLUMN wialon_object_stats.resource_id IS 'avl_resource.id в Wialon';
COMMENT ON COLUMN wialon_object_stats.user_id    IS 'creator user.id, по которому фронт идентифицирует аккаунт';
COMMENT ON COLUMN wialon_object_stats.last_collected_at IS 'Момент последнего успешного сбора. Если устарел >> TTL — данные считаются stale';
