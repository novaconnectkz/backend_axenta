# Подготовка production к golang-migrate

**Этот файл — одноразовая инструкция перед деплоем коммита внедряющего golang-migrate.**

## Что нужно сделать ОДИН РАЗ перед деплоем

На production-БД нужно создать таблицу `schema_migrations` и записать в неё текущую "версию" — 62 (количество уже применённых миграций). Без этого `migrate.Up()` при старте сервера попытается применить ВСЕ 62 SQL-файла заново.

Большинство миграций имеют `IF NOT EXISTS` (идемпотентны) и пройдут no-op, но 5-7 файлов содержат `UPDATE` и неидемпотентный `ALTER COLUMN` — они либо упадут с ошибкой, либо изменят данные нежелательным образом.

## SQL для выполнения на production

Подключиться к prod БД (`psql ...`) и выполнить:

```sql
-- Создаём таблицу учёта миграций
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT NOT NULL PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);

-- Записываем baseline = 62 (все текущие миграции уже применены)
INSERT INTO schema_migrations (version, dirty) VALUES (62, false)
ON CONFLICT (version) DO NOTHING;

-- Проверка
SELECT * FROM schema_migrations;
-- Должно вывести: version=62, dirty=f
```

## После выполнения этого SQL

Можно делать `git push` — auto-deploy подхватит, сервер стартанёт, migrate увидит `version=62, dirty=false` → `migrate.Up()` вернёт `ErrNoChange` → лог `✅ migrate: миграции применены, текущая версия = 62`.

## Если что-то пошло не так

### `dirty=true` после неудачной миграции

Если migrate уронил миграцию посередине, БД помечается dirty. Чтобы разблокировать:

```sql
UPDATE schema_migrations SET dirty = false WHERE version = ?;
```

Потом разобраться что конкретно упало (по логам), пофиксить SQL-файл, перекатить.

### Откатить отдельную миграцию

```bash
# На сервере
go run cmd/migrate/main.go down 1   # откатить одну
```

(не реализовано в текущем коде, для отката пока вручную через psql)

### Полное удаление миграций (NUKE)

```sql
DROP TABLE schema_migrations;
```

Затем при следующем старте migrate подумает что БД пустая и попробует применить всё с нуля — **не делать на проде с данными**.

## Бэкап перед всеми манипуляциями

```bash
# На production-сервере перед SQL команды выше
pg_dump -h localhost -U axenta -d axenta_production -F c -f /tmp/before_migrate_$(date +%Y%m%d_%H%M%S).dump
```

## После успешного деплоя

Будущие миграции добавляются как `migrations/NNNN_<name>.up.sql` + `.down.sql`. Номер NNNN — следующий после максимального текущего (63, 64, ...).

Сервер при перезапуске автоматически применит новые миграции. Никаких ручных `apply_*.sh` больше не нужно.
