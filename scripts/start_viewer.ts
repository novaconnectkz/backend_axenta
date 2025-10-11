#!/usr/bin/env npx tsx

/**
 * Простой HTTP сервер для запуска Axenta API Viewer
 */

import * as http from 'http';
import * as fs from 'fs';
import * as path from 'path';

const PORT = 3001;
const HTML_FILE = path.join(__dirname, 'axenta_api_viewer.html');

const server = http.createServer((req, res) => {
  // Настройка CORS для работы с внешними API
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');

  if (req.method === 'OPTIONS') {
    res.writeHead(200);
    res.end();
    return;
  }

  if (req.url === '/' || req.url === '/index.html') {
    try {
      const html = fs.readFileSync(HTML_FILE, 'utf8');
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(html);
    } catch (error) {
      res.writeHead(500, { 'Content-Type': 'text/plain' });
      res.end('Ошибка загрузки страницы: ' + error);
    }
  } else {
    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('Страница не найдена');
  }
});

server.listen(PORT, () => {
  console.log('🚀 Axenta API Viewer запущен!');
  console.log(`📱 Откройте в браузере: http://localhost:${PORT}`);
  console.log('');
  console.log('💡 Инструкции:');
  console.log('1. Введите ваши учетные данные Axenta');
  console.log('2. Нажмите "Подключиться к API"');
  console.log('3. Просматривайте полученные данные в удобном интерфейсе');
  console.log('');
  console.log('⏹️  Для остановки нажмите Ctrl+C');
});

// Обработка завершения
process.on('SIGINT', () => {
  console.log('\n👋 Сервер остановлен');
  process.exit(0);
});
