-- Migration: Add MAX messenger integration columns
-- Created: 2025-11-21
-- Description: Adds columns for MAX messenger (Russian messenger) integration to notification_settings

ALTER TABLE public.notification_settings 
ADD COLUMN IF NOT EXISTS max_bot_token VARCHAR(500);

ALTER TABLE public.notification_settings 
ADD COLUMN IF NOT EXISTS max_webhook_url TEXT;

ALTER TABLE public.notification_settings 
ADD COLUMN IF NOT EXISTS max_enabled BOOLEAN DEFAULT FALSE;

ALTER TABLE public.notification_settings 
ADD COLUMN IF NOT EXISTS max_use_polling BOOLEAN DEFAULT FALSE;

ALTER TABLE public.notification_settings 
ADD COLUMN IF NOT EXISTS max_parse_mode VARCHAR(20) DEFAULT 'HTML';

-- Comments
COMMENT ON COLUMN public.notification_settings.max_bot_token IS 'MAX bot token for API access';
COMMENT ON COLUMN public.notification_settings.max_webhook_url IS 'MAX webhook URL for receiving updates';
COMMENT ON COLUMN public.notification_settings.max_enabled IS 'Whether MAX integration is enabled';
COMMENT ON COLUMN public.notification_settings.max_use_polling IS 'Use Long Polling instead of Webhook';
COMMENT ON COLUMN public.notification_settings.max_parse_mode IS 'Message parse mode (HTML, Markdown, MarkdownV2)';

