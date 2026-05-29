services/axenta_sync_service.go:665:	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
services/billing_service_test.go:240:		DefaultTaxRate:          decimal.NewFromFloat(20),
services/billing_service_test.go:349:		DefaultTaxRate:         decimal.NewFromFloat(20),
services/billing_service_test.go:696:		DefaultTaxRate:          decimal.NewFromFloat(20),
services/billing_service_test.go:825:		DefaultTaxRate:          decimal.NewFromFloat(20),
services/billing_service_test.go:904:		DefaultTaxRate:          decimal.NewFromFloat(20),
services/snapshot_invalidator.go:47:	tickRate time.Duration
services/snapshot_invalidator.go:64:		tickRate:  2 * time.Second,
services/snapshot_invalidator.go:116:	ticker := time.NewTicker(s.tickRate)
services/snapshot_invalidator_test.go:15:		tickRate: 10 * time.Millisecond,
services/snapshot_invalidator_test.go:51:		tickRate: 10 * time.Millisecond,
services/skif_service.go:39:// HTTPClient создаёт http.Client с cookie jar и опционально пред-загруженной cookie из БД.
services/skif_service.go:41:func (s *SkifService) httpClient(conn *models.SkifConnection) (*http.Client, *cookiejar.Jar) {
services/skif_service.go:49:	return &http.Client{
services/skif_service.go:51:		Timeout: 30 * time.Second,
services/skif_service.go:225:// Используется один http.Client с persistent cookie jar — все шаги в одной сессии.
services/skif_service.go:269:	// Rate limit SKIF: ~60 req/min. Между companies минимум 1s + retry на 429.
services/skif_service.go:293:	// Mark deleted: юниты которых нет в выгрузке (только если errorRate приемлем).
services/skif_service.go:328:func (s *SkifService) loginWithClient(conn *models.SkifConnection, client *http.Client) error {
services/skif_service.go:360:func (s *SkifService) fetchMe(conn *models.SkifConnection, client *http.Client) (*skifMeResponse, int, error) {
services/skif_service.go:385:func (s *SkifService) switchWithRetry(conn *models.SkifConnection, client *http.Client, companyID string) error {
services/skif_service.go:413:func (s *SkifService) switchCompany(conn *models.SkifConnection, client *http.Client, companyID string) error {
services/skif_service.go:435:func (s *SkifService) fetchUnitsForCompany(conn *models.SkifConnection, client *http.Client, comp skifCompanyBrief, now time.Time) (int, []string, error) {
services/skif_service.go:562:// Rate limit SKIF ~60/min: throttle 1.1s между companies.
services/skif_service.go:672:func (s *SkifService) fetchUsersForCompany(conn *models.SkifConnection, client *http.Client, comp skifCompanyBrief, now time.Time) (int, []string, error) {
services/skif_service.go:921:func (s *SkifService) fetchRoleIDByKey(conn *models.SkifConnection, client *http.Client, roleKey string) (string, string, error) {
services/skif_service.go:1356:func (s *SkifService) postJSON(conn *models.SkifConnection, client *http.Client, path string, body []byte) ([]byte, int, error) {
services/skif_service.go:1475:	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
services/skif_service.go:1514:	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
services/skif_service.go:1732:	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
services/gelios_service.go:38:	cli *http.Client
services/gelios_service.go:42:	return &GeliosService{db: db, cli: &http.Client{Timeout: 30 * time.Second}}
services/gelios_service.go:279:var geliosConnLocks sync.Map // map[uint]*sync.Mutex
services/simple_messengers.go:36:	client := &http.Client{}
services/simple_messengers.go:90:	client := &http.Client{}
services/load_test_service_test.go:33:		Timeout:         5 * time.Second,
services/load_test_service_test.go:56:		Timeout:         5 * time.Second,
services/telegram_integration_service.go:177:	client := &http.Client{Timeout: 30 * time.Second}
services/telegram_integration_service.go:225:	client := &http.Client{Timeout: 30 * time.Second}
services/1c_integration_service_test.go:98:		TaxRate:            decimal.NewFromFloat(20),
services/cache.go:412:	HitRate     float64 `json:"hit_rate"`
services/cache.go:437:	hitRate := 0.0
services/cache.go:439:		hitRate = float64(pcs.hitCount) / float64(total) * 100
services/cache.go:445:		HitRate:   hitRate,
services/max_integration_service.go:155:	client := &http.Client{Timeout: 10 * time.Second}
services/max_integration_service.go:202:	client := &http.Client{Timeout: 10 * time.Second}
services/account_hierarchy_service.go:43:	client := &http.Client{Timeout: 60 * time.Second}
services/account_hierarchy_service.go:255:	client := &http.Client{Timeout: 300 * time.Second}

[35m[3mcodex[0m[0m
Рекомендация: Да, основной фикс: в [services/wialon_service.go](/Users/com/ACRM/backend_axenta/services/wialon_service.go:2272) сделать `force=1` только для первой страницы `searchItemsPaginated`, далее `force=0`; параллельно ввести per-connection throttling/single-flight для стартовых Wialon-задач.

Блокирующие риски:
- `force=1` на каждой странице действительно запускает новый поиск по документации Wialon: `force=0` возвращает уже выполненный поиск, `force=1` стартует новый. Это подтверждает гипотезу про повторный full-search на каждой странице.
- Нельзя делать `force=1` повторно “посередине” пагинации как recovery: offsets могут сдвинуться, появятся дубли/пропуски.
- `sortType=sys_name` не уникален; при fresh-search на каждой странице это особенно опасно. Для полной выгрузки безопаснее `sys_id`, а UI-сортировку по имени делать локально.
- В коде уже есть `http.Client{Timeout: 300s}` в `NewWialonService`, поэтому “нет HTTP timeout” сейчас неверно. Проблема не в полном отсутствии timeout, а в слишком долгих запросах и отсутствии per-call context/retry policy.
- 5 schedulers после рестарта создают конкуренцию за один токен/host; даже если `searchItemsPaginated` ускорить, без общего limiter возможны 429/лимиты сессий/пилообразное замедление.

Допущения:
- Используем актуальную документацию Wialon `core/search_items`: `force=0` = вернуть результат уже выполненного такого поиска, `force=1` = выполнить новый поиск; `from/to` режут результат по индексам. Источник: https://help.wialon.com/en/api/user-guide/api-reference/core/search_items
- Wialon держит cached search result в рамках текущей `sid`; при обычной быстрой пагинации внутри одного `LoginWithHost` set не должен протухать.
- Snapshot-id у `core/search_items` в документации нет; консистентность достигается не отдельным id, а повторным чтением cached result через `force=0` в той же сессии.
- Если cached result потерян/сессия умерла, правильный recovery: прервать текущую пагинацию и начать весь scan заново с `force=1`, а не re-force конкретную страницу.

Уверенность: high

Тесты для верификации:
- На staging/prod для `glomoskz` залогировать по страницам: `itemsType`, `from/to`, `force`, `duration`, `len(items)`, `totalItemsCount`, `indexFrom/indexTo`.
- Сравнить до/после: users pages должны быть `1,0,0,0,0`; resources `1,0,0,0`; wall time должен упасть кратно, ориентир с 2m30s до десятков секунд при тех же payload.
- Проверить итоговые counts: `len(usersItems)==usersTotal`, `len(allResItems)==allResTotal`, итог аккаунтов около текущих `3283`.
- Добавить dedup-метрику по `id`: `duplicates=0`; если есть дубли, не падать, но логировать.
- Смоделировать ошибку на странице `from>0`: убедиться, что retry перезапускает весь scan, а не продолжает со старым offset после нового `force=1`.
- После рестарта systemd проверить, что одновременных `LoginWithHost` по `glomoskz` не больше заданного лимита, а scheduler-и не стартуют все в одну миллисекунду.

План правок:
- `services/wialon_service.go` / `searchItemsPaginated` / заменить `"force": 1` на `forceVal := 1 если from==0 иначе 0`; добавить проверку `indexFrom/indexTo`, dedup по `id`, лог per-page duration / severity: critical.
- `services/wialon_service.go` / `GetAccountsBatchFromHost` / для полных scans `user` и `avl_resource` рассмотреть `sortType:"sys_id"` вместо `sys_name`; если нужен порядок по имени, сортировать итог локально / severity: high.
- `services/wialon_service.go` / `searchItemsPaginated` / при ошибке cached/page read не делать midstream `force=1`; вернуть ошибку наверх или выполнить bounded full-retry всей функции с новым login/scan / severity: high.
- `services/wialon_service.go` или новый `services/wialon_limiter.go` / общий per-host+token semaphore/rate-limiter для всех Wialon HTTP calls или хотя бы login/search heavy paths; лимит 1-2 concurrent jobs на connection, burst маленький / severity: high.
- `api/wialon_connections.go` / `buildAndCacheAllAccountsForCompany` / single-flight по `companyID/connectionID` для `GetAccountsBatchFromHost`, чтобы handler и scheduler не дублировали один all-accounts fetch / severity: high.
- `services/wialon_all_accounts_scheduler.go`, `services/wialon_stats_scheduler.go`, `services/wialon_billing_plans_scheduler.go`, `services/wialon_history_scheduler.go` / добавить startup jitter/stagger и общий guard per connection, чтобы после рестарта не было 5 тяжелых Wialon задач сразу / severity: medium-high.
- `services/wialon_service.go` / `usersFlags=407` / аудит реально используемых полей; если `custom_fields/admin_fields/connection_settings/prp` нужны не всем consumers, разделить lightweight и full режимы / severity: medium.
- `services/wialon_service.go` / `NewWialonService` и HTTP helpers / оставить `Timeout:300s`, но добавить per-request context/logging и явную классификацию timeout/rate-limit ошибок / severity: medium.
[2mtokens used[0m
62 469
Рекомендация: Да, основной фикс: в [services/wialon_service.go](/Users/com/ACRM/backend_axenta/services/wialon_service.go:2272) сделать `force=1` только для первой страницы `searchItemsPaginated`, далее `force=0`; параллельно ввести per-connection throttling/single-flight для стартовых Wialon-задач.

Блокирующие риски:
- `force=1` на каждой странице действительно запускает новый поиск по документации Wialon: `force=0` возвращает уже выполненный поиск, `force=1` стартует новый. Это подтверждает гипотезу про повторный full-search на каждой странице.
- Нельзя делать `force=1` повторно “посередине” пагинации как recovery: offsets могут сдвинуться, появятся дубли/пропуски.
- `sortType=sys_name` не уникален; при fresh-search на каждой странице это особенно опасно. Для полной выгрузки безопаснее `sys_id`, а UI-сортировку по имени делать локально.
- В коде уже есть `http.Client{Timeout: 300s}` в `NewWialonService`, поэтому “нет HTTP timeout” сейчас неверно. Проблема не в полном отсутствии timeout, а в слишком долгих запросах и отсутствии per-call context/retry policy.
- 5 schedulers после рестарта создают конкуренцию за один токен/host; даже если `searchItemsPaginated` ускорить, без общего limiter возможны 429/лимиты сессий/пилообразное замедление.

Допущения:
- Используем актуальную документацию Wialon `core/search_items`: `force=0` = вернуть результат уже выполненного такого поиска, `force=1` = выполнить новый поиск; `from/to` режут результат по индексам. Источник: https://help.wialon.com/en/api/user-guide/api-reference/core/search_items
- Wialon держит cached search result в рамках текущей `sid`; при обычной быстрой пагинации внутри одного `LoginWithHost` set не должен протухать.
- Snapshot-id у `core/search_items` в документации нет; консистентность достигается не отдельным id, а повторным чтением cached result через `force=0` в той же сессии.
- Если cached result потерян/сессия умерла, правильный recovery: прервать текущую пагинацию и начать весь scan заново с `force=1`, а не re-force конкретную страницу.

Уверенность: high

Тесты для верификации:
- На staging/prod для `glomoskz` залогировать по страницам: `itemsType`, `from/to`, `force`, `duration`, `len(items)`, `totalItemsCount`, `indexFrom/indexTo`.
- Сравнить до/после: users pages должны быть `1,0,0,0,0`; resources `1,0,0,0`; wall time должен упасть кратно, ориентир с 2m30s до десятков секунд при тех же payload.
- Проверить итоговые counts: `len(usersItems)==usersTotal`, `len(allResItems)==allResTotal`, итог аккаунтов около текущих `3283`.
- Добавить dedup-метрику по `id`: `duplicates=0`; если есть дубли, не падать, но логировать.
- Смоделировать ошибку на странице `from>0`: убедиться, что retry перезапускает весь scan, а не продолжает со старым offset после нового `force=1`.
- После рестарта systemd проверить, что одновременных `LoginWithHost` по `glomoskz` не больше заданного лимита, а scheduler-и не стартуют все в одну миллисекунду.

План правок:
- `services/wialon_service.go` / `searchItemsPaginated` / заменить `"force": 1` на `forceVal := 1 если from==0 иначе 0`; добавить проверку `indexFrom/indexTo`, dedup по `id`, лог per-page duration / severity: critical.
- `services/wialon_service.go` / `GetAccountsBatchFromHost` / для полных scans `user` и `avl_resource` рассмотреть `sortType:"sys_id"` вместо `sys_name`; если нужен порядок по имени, сортировать итог локально / severity: high.
- `services/wialon_service.go` / `searchItemsPaginated` / при ошибке cached/page read не делать midstream `force=1`; вернуть ошибку наверх или выполнить bounded full-retry всей функции с новым login/scan / severity: high.
- `services/wialon_service.go` или новый `services/wialon_limiter.go` / общий per-host+token semaphore/rate-limiter для всех Wialon HTTP calls или хотя бы login/search heavy paths; лимит 1-2 concurrent jobs на connection, burst маленький / severity: high.
- `api/wialon_connections.go` / `buildAndCacheAllAccountsForCompany` / single-flight по `companyID/connectionID` для `GetAccountsBatchFromHost`, чтобы handler и scheduler не дублировали один all-accounts fetch / severity: high.
- `services/wialon_all_accounts_scheduler.go`, `services/wialon_stats_scheduler.go`, `services/wialon_billing_plans_scheduler.go`, `services/wialon_history_scheduler.go` / добавить startup jitter/stagger и общий guard per connection, чтобы после рестарта не было 5 тяжелых Wialon задач сразу / severity: medium-high.
- `services/wialon_service.go` / `usersFlags=407` / аудит реально используемых полей; если `custom_fields/admin_fields/connection_settings/prp` нужны не всем consumers, разделить lightweight и full режимы / severity: medium.
- `services/wialon_service.go` / `NewWialonService` и HTTP helpers / оставить `Timeout:300s`, но добавить per-request context/logging и явную классификацию timeout/rate-limit ошибок / severity: medium.
