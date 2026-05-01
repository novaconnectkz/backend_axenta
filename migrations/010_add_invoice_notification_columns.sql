-- Migration: Add invoice notification columns
-- Created: 2025-11-21
-- Description: Adds columns for invoice notification tracking and admin account relationship

-- Add admin account relationship
ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS admin_account_id BIGINT;

-- Add billing period
ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS billing_period_end TIMESTAMPTZ;

-- Add notification tracking columns
ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS last_sent_at TIMESTAMPTZ;

ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS last_sent_channels TEXT;

ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS last_sent_error TEXT;

ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS send_channels TEXT DEFAULT 'email';

ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS send_to_email VARCHAR(100);

ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS send_to_max VARCHAR(50);

ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS send_to_telegram VARCHAR(50);

ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS sent_count INTEGER DEFAULT 0;

-- Create index for admin account
CREATE INDEX IF NOT EXISTS idx_invoices_admin_account ON public.invoices(admin_account_id);

-- Comments
COMMENT ON COLUMN public.invoices.admin_account_id IS 'Admin account that owns this invoice';
COMMENT ON COLUMN public.invoices.billing_period_end IS 'End date of billing period for this invoice';
COMMENT ON COLUMN public.invoices.last_sent_at IS 'Timestamp of last notification sent';
COMMENT ON COLUMN public.invoices.last_sent_channels IS 'Channels used for last notification (email, telegram, max)';
COMMENT ON COLUMN public.invoices.last_sent_error IS 'Error message from last send attempt if failed';
COMMENT ON COLUMN public.invoices.send_channels IS 'Preferred channels for sending notifications';
COMMENT ON COLUMN public.invoices.send_to_email IS 'Email address for sending invoice';
COMMENT ON COLUMN public.invoices.send_to_max IS 'MAX messenger ID for sending invoice';
COMMENT ON COLUMN public.invoices.send_to_telegram IS 'Telegram ID for sending invoice';
COMMENT ON COLUMN public.invoices.sent_count IS 'Number of times this invoice notification was sent';

