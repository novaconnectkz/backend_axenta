-- ВНИМАНИЕ: Этот скрипт удаляет ВСЕ договоры из базы данных!
-- Используйте с осторожностью!

-- Шаг 1: Отвязываем все объекты от договоров (устанавливаем contract_id = 0)
UPDATE objects 
SET contract_id = 0 
WHERE contract_id IS NOT NULL AND contract_id != 0;

-- Шаг 2: Удаляем все приложения к договорам
DELETE FROM contract_appendices;

-- Шаг 3: Удаляем все договоры
DELETE FROM contracts;

-- Проверка: убедимся, что все удалено
SELECT COUNT(*) as remaining_contracts FROM contracts;
SELECT COUNT(*) as objects_with_contracts FROM objects WHERE contract_id IS NOT NULL AND contract_id != 0;

