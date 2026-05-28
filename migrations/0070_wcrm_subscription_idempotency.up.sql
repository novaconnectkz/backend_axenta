-- WCRM-миграция: идемпотентность подписок.
-- subscriptions — глобальная (public) таблица. Добавляем external_id +
-- partial unique index для ключа 'wcrm:sub:<attachment_id>'.
-- Уникальность по (admin_account_id, external_id) — на случай импорта под разные admin.

ALTER TABLE public.subscriptions ADD COLUMN IF NOT EXISTS external_id varchar(100);

CREATE UNIQUE INDEX IF NOT EXISTS uq_subscriptions_wcrm_extid
  ON public.subscriptions (admin_account_id, external_id)
  WHERE external_id LIKE 'wcrm:sub:%' AND deleted_at IS NULL;
