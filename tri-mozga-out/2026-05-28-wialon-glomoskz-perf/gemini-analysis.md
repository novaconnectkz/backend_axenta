Ripgrep is not available. Falling back to GrepTool.
Attempt 1 failed: You have exhausted your capacity on this model. Your quota will reset after 4s.. Retrying after 5982ms...
(node:12584) [DEP0190] DeprecationWarning: Passing args to a child process with shell option true can lead to security vulnerabilities, as the arguments are not escaped, only concatenated.
(Use `node --trace-deprecation ...` to show where the warning was created)
Attempt 1 failed: You have exhausted your capacity on this model. Your quota will reset after 1s.. Retrying after 5754ms...
Attempt 1 failed: You have exhausted your capacity on this model. Your quota will reset after 2s.. Retrying after 5093ms...
Attempt 1 failed: You have exhausted your capacity on this model. Your quota will reset after 0s.. Retrying after 5234ms...
Проблема производительности при загрузке Wialon-подключений (особенно крупных, как `glomoskz`) вызвана неоптимальным алгоритмом пагинации в SDK и избыточным параллелизмом планировщиков при старте приложения.

### Рекомендация
**Оптимизировать пагинацию (перейти на `force: 0` для страниц > 0) и внедрить `singleflight` для предотвращения лавинообразных запросов от планировщиков.**

### Блокирующие риски
*   **Консистентность выборки:** При `force: 0` Wialon использует закэшированный результат поиска. Если между запросами страниц (миллисекунды) состав объектов изменится, возможны дубли или пропуски. Это нивелируется использованием стабильной сортировки (`sys_name` или `sys_id`).
*   **Лимиты сессий:** Без `singleflight` одновременный старт 5 планировщиков тратит 5 сессий на один токен, что впритык к лимиту Wialon (~5-10) и может приводить к ошибкам `error: 11` (limit exceeded).

### Допущения
*   В `services/wialon_service.go` функция `searchItemsPaginated` является основным узким местом для `all-accounts`.
*   Сессия, созданная в `GetAccountsBatchFromHost`, используется монопольно одним запросом, поэтому `force: 0` для последующих страниц безопасен.

### Уверенность: High
Диагностика подтверждается кодом: `searchItemsPaginated` жестко забит на `force: 1`, что заставляет Wialon пересчитывать 12к+ объектов на каждой из 5-8 страниц. Это дает O(N·Pages) нагрузку вместо O(N).

### Тесты для верификации
*   **Логирование:** Проверить, что в логах после фикса `LoginWithHost` для `glomoskz` встречается 1 раз (благодаря `singleflight`), а не 5.
*   **Замер времени:** Ожидаемое время загрузки `glomoskz` должно упасть с **150с** до **~30-40с**.
*   **Проверка данных:** Сравнить количество и ID аккаунтов до и после правок на `glomoskz`.

### План правок

| Файл / Функция | Изменение | Severity | Почему это работает |
| :--- | :--- | :--- | :--- |
| **`services/wialon_service.go`** / `searchItemsPaginated` | Заменить `force: 1` на `forceVal := 0; if from == 0 { forceVal = 1 }` | **Critical** | Устраняет пересоздание результата поиска на каждой странице. Экономит ~20с на каждой странице после первой. |
| **`services/wialon_service.go`** / `WialonService` struct | Добавить `sf singleflight.Group` | **High** | Позволяет объединять идентичные запросы. |
| **`services/wialon_service.go`** / `GetAccountsBatchFromHost` | Обернуть тело функции в `s.sf.Do(key, ...)` | **High** | 5 планировщиков подождут один результат, вместо 5 параллельных логинов и 40+ HTTP-запросов. |
| **`services/wialon_service.go`** / `searchItemsPaginated` | Убедиться в использовании `sortType: "sys_id"` или `"sys_name"` | **Medium** | Стабильная сортировка необходима для корректности `force: 0` при пагинации. |
| **`api/wialon_connections.go`** / `buildAndCacheAllAccountsForCompany` | Добавить кэширование на уровне функции (singleflight) | **Low** | Защита от одновременных кликов пользователей в UI при пустом кэше Redis. |

**Результат:** Суммарная нагрузка на Wialon при старте снизится в ~10-15 раз, время готовности данных сократится с 2.5 минут до приемлемых 40 секунд.
