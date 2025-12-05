# Настройка Cursor Remote - SSH для подключения к продакшен серверу

## Вариант 1: Через SSH ключ (Рекомендуется)

### Шаг 1: Создайте SSH ключ (если еще нет)

```bash
ssh-keygen -t rsa -b 4096 -f ~/.ssh/axenta_production -C "axenta-production"
```

### Шаг 2: Скопируйте публичный ключ на сервер

```bash
ssh-copy-id -i ~/.ssh/axenta_production.pub root@194.87.143.169
# Введите пароль: g-t+XM#3an2YJM
```

Или вручную:

```bash
cat ~/.ssh/axenta_production.pub | ssh root@194.87.143.169 "mkdir -p ~/.ssh && chmod 700 ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"
```

### Шаг 3: Обновите SSH config

Файл `~/.ssh/config` уже создан с записью `axenta-production`.

Обновите его для использования ключа:

```bash
cat >> ~/.ssh/config << 'EOF'
Host axenta-production
    HostName 194.87.143.169
    User root
    Port 22
    IdentityFile ~/.ssh/axenta_production
    IdentitiesOnly yes
EOF
```

### Шаг 4: Подключитесь через Cursor

1. Откройте Command Palette: `Cmd+Shift+P` (Mac) или `Ctrl+Shift+P` (Windows/Linux)
2. Введите: `Remote-SSH: Connect to Host`
3. Выберите: `axenta-production`
4. Cursor подключится к серверу и откроет удаленную сессию

---

## Вариант 2: С использованием пароля (через sshpass)

Если вы хотите использовать пароль без ключа:

### Шаг 1: Установите sshpass

```bash
# macOS
brew install hudochenkov/sshpass/sshpass

# Linux
sudo apt-get install sshpass
```

### Шаг 2: Создайте wrapper скрипт

Создайте файл `~/.ssh/ssh-axenta-production.sh`:

```bash
#!/bin/bash
sshpass -p 'g-t+XM#3an2YJM' ssh -o StrictHostKeyChecking=no root@194.87.143.169 "$@"
```

Сделайте его исполняемым:

```bash
chmod +x ~/.ssh/ssh-axenta-production.sh
```

### Шаг 3: Обновите SSH config

```bash
cat >> ~/.ssh/config << 'EOF'
Host axenta-production
    HostName 194.87.143.169
    User root
    Port 22
    ProxyCommand ~/.ssh/ssh-axenta-production.sh -W %h:%p
EOF
```

---

## Вариант 3: Интерактивный ввод пароля

Если не хотите настраивать ключ или sshpass:

1. Откройте Command Palette: `Cmd+Shift+P`
2. Введите: `Remote-SSH: Connect to Host`
3. Введите: `root@194.87.143.169`
4. При запросе пароля введите: `g-t+XM#3an2YJM`

---

## После подключения

После успешного подключения Cursor откроет удаленную сессию, и вы сможете:

- Открывать файлы на сервере
- Редактировать код напрямую
- Использовать все функции Cursor (автодополнение, AI и т.д.)

### Рабочая директория на сервере

Приложение находится в: `/var/www/app/backend_axenta`

Откройте эту папку в Cursor после подключения:
- `File` -> `Open Folder...`
- Введите: `/var/www/app/backend_axenta`

---

## Полезные команды

После подключения через Remote SSH вы можете использовать терминал Cursor для выполнения команд на сервере:

```bash
# Проверить статус сервиса
sudo systemctl status axenta-backend

# Перезапустить сервис
sudo systemctl restart axenta-backend

# Посмотреть логи
sudo journalctl -u axenta-backend -f

# Редактировать .env
nano /var/www/app/backend_axenta/.env
```

---

## Устранение проблем

### Проблема: "Permission denied"

**Решение:** Убедитесь, что SSH ключ скопирован на сервер или используйте правильный пароль.

### Проблема: "Host key verification failed"

**Решение:** Добавьте в `~/.ssh/config`:
```
StrictHostKeyChecking no
UserKnownHostsFile /dev/null
```

### Проблема: Cursor не может подключиться

**Решение:** 
1. Проверьте, что сервер доступен: `ping 194.87.143.169`
2. Проверьте SSH подключение: `ssh root@194.87.143.169`
3. Убедитесь, что установлено расширение "Remote - SSH" в Cursor

---

## Безопасность

⚠️ **Важно:** 
- Не коммитьте SSH ключи в git
- Используйте SSH ключи вместо паролей в production
- Ограничьте доступ к SSH ключам: `chmod 600 ~/.ssh/axenta_production`
