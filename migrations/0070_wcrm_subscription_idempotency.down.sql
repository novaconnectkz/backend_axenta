DROP INDEX IF EXISTS public.uq_subscriptions_wcrm_extid;
ALTER TABLE public.subscriptions DROP COLUMN IF EXISTS external_id;
