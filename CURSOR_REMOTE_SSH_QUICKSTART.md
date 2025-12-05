# Быстрый старт: Подключение к продакшену через Cursor Remote - SSH

## ✅ Что уже сделано:

1. ✅ Установлен `sshpass` (версия 1.06)
2. ✅ Создан SSH config файл `~/.ssh/config` с записью `axenta-production`

## 🚀 Подключение через Cursor:

### Способ 1: Через SSH config (Рекомендуется)

1. Откройте Command Palette в Cursor:
   - **Mac**: `Cmd+Shift+P`
   - **Windows/Linux**: `Ctrl+Shift+P`

2. Введите и выберите:
   ```
   Remote-SSH: Connect to Host
   ```

3. Выберите из списка:
   ```
   axenta-production
   ```

4. При запросе пароля введите:
   ```
   g-t+XM#3an2YJM
   ```

5. После подключения откройте папку:
   - `File` → `Open Folder...`
   - Введите: `/var/www/app/backend_axenta`

---

### Способ 2: Прямое подключение

1. Command Palette: `Cmd+Shift+P` (или `Ctrl+Shift+P`)

2. Введите:
   ```
   Remote-SSH: Connect to Host
   ```

3. Введите напрямую:
   ```
   root@194.87.143.169
   ```

4. При запросе пароля: `g-t+XM#3an2YJM`

---

## 🔧 Настройка SSH ключа (для автоматического подключения)

Чтобы не вводить пароль каждый раз, настройте SSH ключ:

### Шаг 1: Создайте SSH ключ (если еще нет)

```bash
ssh-keygen -t rsa -b 4096 -f ~/.ssh/axenta_production -C "axenta-production"
# Нажмите Enter для всех вопросов (или введите passphrase)
```

### Шаг 2: Скопируйте ключ на сервер

```bash
sshpass -p 'g-t+XM#3an2YJM' ssh-copy-id -i ~/.ssh/axenta_production.pub root@194.87.143.169
```

Или вручную:

```bash
cat ~/.ssh/axenta_production.pub | sshpass -p 'g-t+XM#3an2YJM' ssh root@194.87.143.169 "mkdir -p ~/.ssh && chmod 700 ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"
```

### Шаг 3: Обновите SSH config

```bash
cat >> ~/.ssh/config << 'EOF'

Host axenta-production
    IdentityFile ~/.ssh/axenta_production
    IdentitiesOnly yes
EOF
```

Теперь подключение будет автоматическим без пароля!

---

## 📁 Рабочая директория на сервере

После подключения откройте:
```
/var/www/app/backend_axenta
```

Здесь находятся:
- `main.go` - основной файл приложения
- `.env` - переменные окружения
- `axenta_backend` - скомпилированный бинарник

---

## 🛠️ Полезные команды в терминале Cursor

После подключения используйте встроенный терминал Cursor:

```bash
# Статус сервиса
sudo systemctl status axenta-backend

# Перезапуск сервиса
sudo systemctl restart axenta-backend

# Логи в реальном времени
sudo journalctl -u axenta-backend -f

# Редактирование .env
nano /var/www/app/backend_axenta/.env

# Проверка планировщика снимков
sudo journalctl -u axenta-backend | grep -i "snapshot\|снимк"
```

---

## ⚠️ Важные замечания

1. **Безопасность**: Не коммитьте SSH ключи или пароли в git
2. **Права доступа**: Файлы на сервере принадлежат пользователю `app`, для редактирования может потребоваться `sudo`
3. **Перезапуск**: После изменения `.env` или кода нужно перезапустить сервис

---

## 🔍 Проверка подключения

Проверьте, что все работает:

```bash
# В терминале Cursor (после подключения)
cd /var/www/app/backend_axenta
ls -la
cat .env | grep ENABLE_SNAPSHOT_SCHEDULER
```

---

## 📝 Текущие настройки на сервере

- **ENABLE_SNAPSHOT_SCHEDULER**: `true` ✅
- **AXENTA_ADMIN_TOKEN**: `your_axenta_admin_token_here` ⚠️ (нужно установить реальный токен)
- **Планировщик**: Запущен и работает ✅

---

Готово! Теперь вы можете работать с кодом на продакшене напрямую из Cursor! 🎉
