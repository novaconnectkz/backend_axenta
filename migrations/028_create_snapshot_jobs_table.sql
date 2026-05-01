-- Создание таблицы snapshot_jobs для логирования задач создания снимков
-- Дата: 2025-12-02

\echo 'Создание таблицы snapshot_jobs в схеме public...'

CREATE TABLE IF NOT EXISTS public.snapshot_jobs (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Информация о запуске
    job_type VARCHAR(50) NOT NULL DEFAULT 'daily_auto',
    started_at TIMESTAMP NOT NULL,
    finished_at TIMESTAMP,
    duration_seconds INTEGER,
    
    -- Статус
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    
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
    details JSONB,
    
    -- Метаданные
    triggered_by VARCHAR(100),
    server_info JSONB
);

-- Индексы
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_status ON public.snapshot_jobs(status);
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_started_at ON public.snapshot_jobs(started_at);
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_date_from ON public.snapshot_jobs(date_from);
CREATE INDEX IF NOT EXISTS idx_snapshot_jobs_job_type ON public.snapshot_jobs(job_type);

\echo 'Проверка:'
SELECT COUNT(*) as table_exists FROM information_schema.tables 
WHERE table_schema = 'public' AND table_name = 'snapshot_jobs';

\echo ''
\echo 'Готово!'

