-- Увеличиваем размер поля client_type для поддержки значений типа "individual_entrepreneur"
-- Эта миграция должна быть применена ко всем tenant-схемам

-- Увеличиваем размер client_type с VARCHAR(20) до VARCHAR(50)
-- Это необходимо для поддержки значений:
-- - "organization" (12 символов)
-- - "individual_entrepreneur" (23 символа)
-- - "physical_person" (15 символов)
-- и других возможных значений

ALTER TABLE contracts
ALTER COLUMN client_type TYPE VARCHAR(50);
