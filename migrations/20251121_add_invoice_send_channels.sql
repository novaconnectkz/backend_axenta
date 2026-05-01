-- Добавляем поля для отправки счетов через разные каналы
-- +goose Up
-- +goose StatementBegin
ALTER TABLE invoices 
  ADD COLUMN IF NOT EXISTS send_channels VARCHAR(100),
  ADD COLUMN IF NOT EXISTS send_to_email VARCHAR(100),
  ADD COLUMN IF NOT EXISTS send_to_telegram VARCHAR(50),
  ADD COLUMN IF NOT EXISTS send_to_max VARCHAR(50),
  ADD COLUMN IF NOT EXISTS last_sent_at TIMESTAMP,
  ADD COLUMN IF NOT EXISTS last_sent_channels VARCHAR(100);

COMMENT ON COLUMN invoices.send_channels IS 'Каналы отправки через запятую (email,telegram,max)';
COMMENT ON COLUMN invoices.send_to_email IS 'Email для отправки счета';
COMMENT ON COLUMN invoices.send_to_telegram IS 'Telegram ID для отправки счета';
COMMENT ON COLUMN invoices.send_to_max IS 'MAX ID для отправки счета';
COMMENT ON COLUMN invoices.last_sent_at IS 'Дата последней отправки счета';
COMMENT ON COLUMN invoices.last_sent_channels IS 'Каналы успешной отправки';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE invoices 
  DROP COLUMN IF EXISTS send_channels,
  DROP COLUMN IF EXISTS send_to_email,
  DROP COLUMN IF EXISTS send_to_telegram,
  DROP COLUMN IF EXISTS send_to_max,
  DROP COLUMN IF EXISTS last_sent_at,
  DROP COLUMN IF EXISTS last_sent_channels;
-- +goose StatementEnd

