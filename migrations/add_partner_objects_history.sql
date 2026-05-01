-- Таблица для хранения истории всех объектов партнерских учетных записей
-- Записывает детальную информацию: когда создан, когда деактивирован, когда удален

CREATE TABLE IF NOT EXISTS partner_objects_history (
    id BIGSERIAL PRIMARY KEY,
    
    -- Привязка к компании (мультитенантность)
    company_id INTEGER NOT NULL,
    
    -- Привязка к партнерскому договору (для тарификации)
    contract_id INTEGER, -- ID партнерского договора (для расчета стоимости)
    
    -- Информация об учетной записи партнера из Axenta Cloud
    partner_account_id INTEGER NOT NULL, -- ID учетной записи в Axenta
    partner_account_name VARCHAR(255), -- Название учетной записи
    parent_account_id INTEGER, -- Родительская учетная запись (компания)
    
    -- Информация об объекте из Axenta
    object_id INTEGER NOT NULL, -- ID объекта в Axenta
    object_name VARCHAR(255) NOT NULL, -- Название объекта
    
    -- Временные метки состояний объекта
    created_datetime TIMESTAMP, -- Когда объект был создан
    deactivated_datetime TIMESTAMP, -- Когда объект был деактивирован  
    deleted_datetime TIMESTAMP, -- Когда объект был удален
    
    -- Текущее состояние объекта
    is_active BOOLEAN NOT NULL DEFAULT true, -- Активен ли объект
    
    -- Дата снимка (когда была собрана эта информация)
    snapshot_date DATE NOT NULL,
    
    -- Метаданные
    recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, -- Когда запись была создана в нашей БД
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- Дополнительная информация из Axenta (JSON)
    extra_data JSONB,
    
    -- Уникальность: один объект - одна запись на дату
    CONSTRAINT unique_partner_object_snapshot 
        UNIQUE (company_id, partner_account_id, object_id, snapshot_date)
);

-- Индексы для быстрого поиска
CREATE INDEX IF NOT EXISTS idx_partner_objects_company ON partner_objects_history(company_id);
CREATE INDEX IF NOT EXISTS idx_partner_objects_contract ON partner_objects_history(contract_id);
CREATE INDEX IF NOT EXISTS idx_partner_objects_partner_account ON partner_objects_history(partner_account_id);
CREATE INDEX IF NOT EXISTS idx_partner_objects_object_id ON partner_objects_history(object_id);
CREATE INDEX IF NOT EXISTS idx_partner_objects_snapshot_date ON partner_objects_history(snapshot_date DESC);
CREATE INDEX IF NOT EXISTS idx_partner_objects_active ON partner_objects_history(is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_partner_objects_deleted ON partner_objects_history(deleted_datetime) WHERE deleted_datetime IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_partner_objects_contract_date ON partner_objects_history(contract_id, snapshot_date);

-- Комментарии
COMMENT ON TABLE partner_objects_history IS 'История всех объектов партнерских учетных записей с детальной информацией о состояниях';
COMMENT ON COLUMN partner_objects_history.partner_account_id IS 'ID учетной записи партнера в Axenta Cloud';
COMMENT ON COLUMN partner_objects_history.object_id IS 'ID объекта в Axenta Cloud';
COMMENT ON COLUMN partner_objects_history.created_datetime IS 'Когда объект был создан (из Axenta)';
COMMENT ON COLUMN partner_objects_history.deactivated_datetime IS 'Когда объект был деактивирован';
COMMENT ON COLUMN partner_objects_history.deleted_datetime IS 'Когда объект был удален';
COMMENT ON COLUMN partner_objects_history.snapshot_date IS 'Дата снимка (за какой день собрана информация)';
COMMENT ON COLUMN partner_objects_history.recorded_at IS 'Когда запись была создана в нашей БД';

