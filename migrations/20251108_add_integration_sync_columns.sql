ALTER TABLE integrations
    ADD COLUMN IF NOT EXISTS last_sync_summary TEXT,
    ADD COLUMN IF NOT EXISTS last_sync_remote_total INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_sync_objects_total INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_sync_objects_created INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_sync_objects_updated INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_sync_objects_archived INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_sync_objects_unchanged INTEGER DEFAULT 0;

