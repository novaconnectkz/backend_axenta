-- Migration: Add VAT rate preset fields to billing_settings
-- Created: 2025-11-19

-- Add vat_rate_preset and vat_rate_custom columns
ALTER TABLE public.billing_settings 
ADD COLUMN IF NOT EXISTS vat_rate_preset VARCHAR(20) DEFAULT 'russia',
ADD COLUMN IF NOT EXISTS vat_rate_custom DECIMAL(5,2) DEFAULT 20;

-- Update existing records to set default preset
UPDATE public.billing_settings 
SET vat_rate_preset = 'russia', 
    vat_rate_custom = 20
WHERE vat_rate_preset IS NULL;

-- Add comment
COMMENT ON COLUMN public.billing_settings.vat_rate_preset IS 'VAT rate preset: russia (20%), kazakhstan (12%), none (0%), custom';
COMMENT ON COLUMN public.billing_settings.vat_rate_custom IS 'Custom VAT rate percentage (used when vat_rate_preset = custom)';
