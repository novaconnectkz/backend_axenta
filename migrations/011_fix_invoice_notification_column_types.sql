-- Migration: Fix invoice notification column types
-- Created: 2025-11-22
-- Description: Changes send_to_email, send_to_telegram, send_to_max from BOOLEAN to VARCHAR to store addresses/IDs

-- Drop and recreate columns with correct types
ALTER TABLE public.invoices DROP COLUMN IF EXISTS send_to_email;
ALTER TABLE public.invoices DROP COLUMN IF EXISTS send_to_telegram;
ALTER TABLE public.invoices DROP COLUMN IF EXISTS send_to_max;

ALTER TABLE public.invoices ADD COLUMN send_to_email VARCHAR(100);
ALTER TABLE public.invoices ADD COLUMN send_to_telegram VARCHAR(50);
ALTER TABLE public.invoices ADD COLUMN send_to_max VARCHAR(50);

-- Update comments
COMMENT ON COLUMN public.invoices.send_to_email IS 'Email address for sending invoice';
COMMENT ON COLUMN public.invoices.send_to_telegram IS 'Telegram ID for sending invoice';
COMMENT ON COLUMN public.invoices.send_to_max IS 'MAX messenger ID for sending invoice';

