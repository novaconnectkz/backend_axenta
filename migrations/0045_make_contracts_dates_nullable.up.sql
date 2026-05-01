-- Миграция для изменения колонок start_date и end_date в таблице contracts на nullable
-- Дата создания: 2025-11-18
-- Причина: Модель GORM определяет эти поля как опциональные (*time.Time с default:NULL),
--          но в старой миграции они были созданы как NOT NULL, что вызывает ошибки при создании договоров

-- Применить для всех tenant-схем:
-- SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%';
-- FOR EACH схеме выполнить:

-- Изменяем start_date на nullable
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;

-- Изменяем end_date на nullable  
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;

-- Проверяем результат
-- SELECT column_name, is_nullable, data_type 
-- FROM information_schema.columns 
-- WHERE table_name = 'contracts' AND column_name IN ('start_date', 'end_date');

