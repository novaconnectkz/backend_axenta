-- Миграция для добавления поля company_type в таблицу companies
-- Создано: 2024-11-28

-- Добавляем поле company_type в таблицу companies (в схеме public)
DO $$
BEGIN
    -- Проверяем, существует ли таблица companies
    IF EXISTS (
        SELECT 1 
        FROM information_schema.tables 
        WHERE table_schema = 'public' 
        AND table_name = 'companies'
    ) THEN
        -- Добавляем поле company_type
        IF NOT EXISTS (
            SELECT 1 
            FROM information_schema.columns 
            WHERE table_schema = 'public' 
            AND table_name = 'companies' 
            AND column_name = 'company_type'
        ) THEN
            ALTER TABLE public.companies ADD COLUMN company_type VARCHAR(20) DEFAULT 'client';
            RAISE NOTICE 'Добавлено поле company_type в таблицу companies';
        ELSE
            RAISE NOTICE 'Поле company_type уже существует в таблице companies';
        END IF;

        -- Создаем индекс для company_type
        IF NOT EXISTS (
            SELECT 1 
            FROM pg_indexes 
            WHERE schemaname = 'public' 
            AND tablename = 'companies' 
            AND indexname = 'idx_companies_company_type'
        ) THEN
            CREATE INDEX idx_companies_company_type ON public.companies (company_type);
            RAISE NOTICE 'Создан индекс idx_companies_company_type';
        ELSE
            RAISE NOTICE 'Индекс idx_companies_company_type уже существует';
        END IF;

        -- Обновляем существующие компании, устанавливая company_type = 'client' по умолчанию
        UPDATE public.companies SET company_type = 'client' WHERE company_type IS NULL;
        RAISE NOTICE 'Обновлены существующие компании (установлен company_type = client)';
    ELSE
        RAISE NOTICE 'Таблица companies не найдена';
    END IF;
END $$;

