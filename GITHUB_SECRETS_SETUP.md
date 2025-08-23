# Настройка GitHub Secrets для автоматического деплоя

## Проблема
GitHub Actions не может подключиться к серверу из-за отсутствующих или неправильно настроенных секретов.

## Ошибки в логах:
- `Error: missing server host` - отсутствует секрет SERVER_HOST
- `ssh.ParsePrivateKey: ssh: no key found` - отсутствует SSH ключ
- `dial tcp: lookup your-server-host on 168.63.129.16:53: no such host` - неправильный хост сервера

## Быстрая настройка

### Шаг 1: Перейдите в настройки репозитория
1. Откройте ваш репозиторий на GitHub
2. Перейдите в **Settings** (Настройки)
3. В левом меню выберите **Secrets and variables** → **Actions**

## Необходимые GitHub Secrets

Перейдите в настройки репозитория: **Settings → Secrets and variables → Actions**

Добавьте следующие секреты:

### 1. SERVER_HOST
- **Название**: `SERVER_HOST`
- **Значение**: IP адрес или доменное имя вашего сервера
- **Пример**: `192.168.1.100` или `your-server.com`

### 2. SERVER_USERNAME  
- **Название**: `SERVER_USERNAME`
- **Значение**: имя пользователя для SSH подключения
- **Пример**: `root` или `deploy`

### 3. SERVER_SSH_KEY
- **Название**: `SERVER_SSH_KEY`
- **Значение**: приватный SSH ключ для подключения к серверу
- **Как получить**:
  ```bash
  # На вашем локальном компьютере или сервере
  cat ~/.ssh/id_rsa
  ```
- **Формат**: Полный приватный ключ включая заголовки:
  ```
  -----BEGIN OPENSSH PRIVATE KEY-----
  [содержимое ключа]
  -----END OPENSSH PRIVATE KEY-----
  ```

### 4. SERVER_PORT (опционально)
- **Название**: `SERVER_PORT`
- **Значение**: порт SSH (по умолчанию 22)
- **Пример**: `22` или `2222`

## Настройка SSH ключа на сервере

1. **Скопируйте публичный ключ на сервер**:
   ```bash
   # На локальном компьютере
   ssh-copy-id username@your-server-host
   
   # Или вручную добавьте в ~/.ssh/authorized_keys на сервере
   cat ~/.ssh/id_rsa.pub >> ~/.ssh/authorized_keys
   ```

2. **Проверьте права доступа на сервере**:
   ```bash
   chmod 700 ~/.ssh
   chmod 600 ~/.ssh/authorized_keys
   ```

3. **Проверьте подключение**:
   ```bash
   ssh -i ~/.ssh/id_rsa username@your-server-host
   ```

## Настройка сервера для деплоя

1. **Создайте директорию проекта**:
   ```bash
   sudo mkdir -p /var/www/backend_axenta
   sudo chown $USER:$USER /var/www/backend_axenta
   ```

2. **Клонируйте репозиторий**:
   ```bash
   cd /var/www
   git clone https://github.com/novaconnectkz/backend_axenta.git
   ```

3. **Установите Go** (если не установлен):
   ```bash
   wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
   sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
   ```

4. **Создайте systemd сервис** `/etc/systemd/system/axenta-backend.service`:
   ```ini
   [Unit]
   Description=Axenta Backend Service
   After=network.target

   [Service]
   Type=simple
   User=www-data
   WorkingDirectory=/var/www/backend_axenta
   ExecStart=/var/www/backend_axenta/axenta_backend
   Restart=always
   RestartSec=5

   [Install]
   WantedBy=multi-user.target
   ```

5. **Активируйте сервис**:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable axenta-backend
   sudo systemctl start axenta-backend
   ```

## Проверка деплоя

После настройки секретов деплой будет выполняться автоматически при каждом push в ветку `main`.

Проверить статус можно в разделе **Actions** вашего GitHub репозитория.

## Безопасность

- ✅ Никогда не коммитьте приватные ключи в репозиторий
- ✅ Используйте отдельного пользователя для деплоя с минимальными правами
- ✅ Регулярно обновляйте SSH ключи
- ✅ Используйте файрвол для ограничения доступа к серверу
