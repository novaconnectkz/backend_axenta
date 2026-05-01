-- Migration: Add sync metadata columns
-- Created: 2025-11-21
-- Description: Adds synchronization metadata columns for contracts, objects, and users

-- Contracts
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT 'idle';
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS sync_checksum TEXT;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS sync_error TEXT;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT 'local';
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS sequential_number INTEGER DEFAULT 0;
ALTER TABLE IF EXISTS public.contracts ADD COLUMN IF NOT EXISTS client_short_name VARCHAR(200) DEFAULT NULL;

-- Objects
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT 'idle';
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0;
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS sync_checksum TEXT;
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE;
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS sync_error TEXT;
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT 'local';
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS public.objects ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ;

-- Users
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT 'idle';
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0;
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS sync_checksum TEXT;
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE;
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS sync_error TEXT;
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT 'local';
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ;
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_contracts_sequential_number ON public.contracts(sequential_number);

-- Comments
COMMENT ON COLUMN public.contracts.client_short_name IS 'Сокращенное название с ОПФ (для организаций)';
COMMENT ON COLUMN public.contracts.sync_status IS 'Status of synchronization: idle, pending, syncing, error';
COMMENT ON COLUMN public.objects.sync_status IS 'Status of synchronization: idle, pending, syncing, error';
COMMENT ON COLUMN public.users.sync_status IS 'Status of synchronization: idle, pending, syncing, error';

