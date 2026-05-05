package api

import "time"

// SnapshotTTL — единое окно свежести snapshot-таблиц для read-path.
//
// Зачем: scheduled sync идёт раз в 10 мин. Если sync упал и не восстановился —
// старые данные в snapshot не должны висеть «мёртвыми» сутками. Через 60 мин
// без обновления read-path делает fallback на live Axenta Cloud API.
//
// Все snapshot-таблицы используют один TTL чтобы поведение было предсказуемым:
//   - axenta_account_snapshots
//   - axenta_user_snapshots
//   - axenta_object_snapshots
//
// Wialon snapshot'ы (wialon_object_stats, wialon_users) живут в public-схеме и
// инвалидируются через wialon_stats_scheduler — отдельный механизм.
const SnapshotTTL = 60 * time.Minute
