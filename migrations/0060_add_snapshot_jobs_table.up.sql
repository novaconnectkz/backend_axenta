-- Таблица для хранения истории автоматических снимков
-- Хранит информацию о каждом запуске планировщика создания снимков

CREATE TABLE IF NOT EXISTS snapshot_jobs (
    id BIGSERIAL PRIMARY KEY,
    
    -- Информация о запуске
    job_type VARCHAR(50) NOT NULL DEFAULT 'daily_auto', -- daily_auto, manual, scheduled
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMP,
    duration_seconds INTEGER, -- Длительность выполнения в секундах
    
    -- Статус
    status VARCHAR(20) NOT NULL DEFAULT 'running', -- running, completed, failed, partial
    
    -- Диапазон дат
    date_from DATE NOT NULL,
    date_to DATE NOT NULL,
    
    -- Статистика
    total_companies INTEGER DEFAULT 0,
    total_contracts INTEGER DEFAULT 0,
    total_days_processed INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    
    -- Детали
    error_message TEXT,
    details JSONB, -- Детальная информация: какие компании, какие договоры, ошибки по каждому
    
    -- Метаданные
    triggered_by VARCHAR(100), -- system, user_id, cron, api
    server_info JSONB, -- hostname, version, etc.
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Индексы для быстрого поиска
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_status ON snapshot_jobs(status);
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_started_at ON snapshot_jobs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_job_type ON snapshot_jobs(job_type);
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_date_range ON snapshot_jobs(date_from, date_to);

-- Комментарии
COMMENT ON TABLE snapshot_jobs IS 'История запусков создания снимков партнерских объектов';
COMMENT ON COLUMN snapshot_jobs.job_type IS 'Тип задачи: daily_auto (авто ежедневно), manual (вручную), scheduled (по расписанию)';
COMMENT ON COLUMN snapshot_jobs.status IS 'Статус: running (выполняется), completed (успешно), failed (ошибка), partial (частично)';
COMMENT ON COLUMN snapshot_jobs.details IS 'JSON с детальной информацией: списки компаний, договоров, конкретные ошибки';
COMMENT ON COLUMN snapshot_jobs.triggered_by IS 'Кто/что запустило: system, user_id:123, cron, api';

