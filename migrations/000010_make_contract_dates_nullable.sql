-- Миграция: Сделать start_date и end_date необязательными для договоров
-- Теперь даты будут устанавливаться через подписку, а не при создании договора

-- Изменяем колонку start_date, делая её nullable
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;

-- Изменяем колонку end_date, делая её nullable (на всякий случай, если было NOT NULL)
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;

-- Добавляем комментарии к колонкам
COMMENT ON COLUMN contracts.start_date IS 'Дата начала действия договора. Устанавливается через подписку.';
COMMENT ON COLUMN contracts.end_date IS 'Дата окончания действия договора. Устанавливается через подписку.';

