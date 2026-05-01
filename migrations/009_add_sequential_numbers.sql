-- Migration: Add sequential_number columns
-- Created: 2025-11-21
-- Description: Adds sequential_number columns for automatic numbering of invoices and subscriptions

-- Add sequential_number to invoices
ALTER TABLE public.invoices 
ADD COLUMN IF NOT EXISTS sequential_number INTEGER DEFAULT 0;

-- Add sequential_number to subscriptions
ALTER TABLE public.subscriptions 
ADD COLUMN IF NOT EXISTS sequential_number INTEGER DEFAULT 0;

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_invoices_sequential_number ON public.invoices(sequential_number);
CREATE INDEX IF NOT EXISTS idx_subscriptions_sequential_number ON public.subscriptions(sequential_number);

-- Comments
COMMENT ON COLUMN public.invoices.sequential_number IS 'Sequential number for invoices within company';
COMMENT ON COLUMN public.subscriptions.sequential_number IS 'Sequential number for subscriptions within company';

