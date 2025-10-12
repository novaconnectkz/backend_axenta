-- Миграция для добавления полей Axenta пользователей
-- Дата: 2025-01-27
-- Описание: Добавляет поля для поддержки ролей пользователей из Axenta (partner, client) и локальных пользователей

-- Добавляем поля для Axenta интеграции в таблицу users
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS axenta_user_type VARCHAR(50) DEFAULT NULL,
ADD COLUMN IF NOT EXISTS axenta_user_id VARCHAR(100) DEFAULT NULL,
ADD COLUMN IF NOT EXISTS is_axenta_user BOOLEAN DEFAULT FALSE;

-- Создаем индексы для оптимизации поиска
CREATE INDEX IF NOT EXISTS idx_users_axenta_user_type ON users(axenta_user_type);
CREATE INDEX IF NOT EXISTS idx_users_axenta_user_id ON users(axenta_user_id);
CREATE INDEX IF NOT EXISTS idx_users_is_axenta_user ON users(is_axenta_user);
CREATE INDEX IF NOT EXISTS idx_users_axenta_composite ON users(is_axenta_user, axenta_user_type);

-- Обновляем существующие записи - помечаем их как локальных пользователей
UPDATE users 
SET 
    axenta_user_type = 'local',
    is_axenta_user = FALSE
WHERE 
    axenta_user_type IS NULL 
    AND is_axenta_user IS NULL;

-- Создаем роли по умолчанию для Axenta пользователей, если они не существуют
INSERT INTO roles (name, display_name, description, color, priority, is_active, is_system, created_at, updated_at)
VALUES 
    ('partner', 'Партнер', 'Роль партнера из Axenta', '#2196F3', 100, TRUE, TRUE, NOW(), NOW()),
    ('client', 'Клиент', 'Роль клиента из Axenta', '#4CAF50', 50, TRUE, TRUE, NOW(), NOW()),
    ('user', 'Пользователь', 'Локальный пользователь системы', '#FF9800', 25, TRUE, TRUE, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;

-- Комментарии к полям
COMMENT ON COLUMN users.axenta_user_type IS 'Тип пользователя в Axenta: partner, client, local';
COMMENT ON COLUMN users.axenta_user_id IS 'ID пользователя в системе Axenta';
COMMENT ON COLUMN users.is_axenta_user IS 'Флаг: пользователь из Axenta или локальный';
