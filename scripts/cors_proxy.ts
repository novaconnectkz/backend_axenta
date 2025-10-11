#!/usr/bin/env npx tsx

/**
 * Простой CORS прокси для обхода ограничений браузера
 * при подключении к API Axenta
 */

import * as http from 'http';
import axios from 'axios';

const PORT = 3003;
const AXENTA_BASE_URL = 'https://axenta.cloud';

const server = http.createServer(async (req, res) => {
  // Настройка CORS заголовков
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  res.setHeader('Access-Control-Max-Age', '86400');

  // Обработка preflight запросов
  if (req.method === 'OPTIONS') {
    res.writeHead(200);
    res.end();
    return;
  }

  try {
    // Получаем данные запроса
    let body = '';
    req.on('data', chunk => {
      body += chunk.toString();
    });

    req.on('end', async () => {
      try {
        console.log(`📡 Прокси запрос: ${req.method} ${req.url}`);
        console.log(`📦 Тело запроса: ${body}`);

        // Формируем URL для Axenta API
        const axentaUrl = `${AXENTA_BASE_URL}${req.url}`;
        console.log(`🎯 Перенаправляем на: ${axentaUrl}`);

        // Подготавливаем заголовки
        const headers: any = {
          'Content-Type': 'application/json'
        };

        // Добавляем Authorization если есть
        const authHeader = req.headers.authorization;
        if (authHeader) {
          headers['Authorization'] = authHeader;
        }

        // Выполняем запрос к Axenta API
        const axiosConfig: any = {
          method: req.method?.toLowerCase(),
          url: axentaUrl,
          headers,
          timeout: 15000,
          validateStatus: () => true // Принимаем любой статус
        };

        if (body && (req.method === 'POST' || req.method === 'PUT')) {
          axiosConfig.data = JSON.parse(body);
        }

        const response = await axios(axiosConfig);

        console.log(`✅ Ответ от Axenta: ${response.status} ${response.statusText}`);
        console.log(`📦 Данные ответа:`, response.data);

        // Отправляем ответ клиенту
        res.writeHead(response.status, {
          'Content-Type': 'application/json',
          'Access-Control-Allow-Origin': '*'
        });
        res.end(JSON.stringify(response.data));

      } catch (error: any) {
        console.error('❌ Ошибка прокси:', error.message);

        let status = 500;
        let errorData = { error: 'Proxy error', message: error.message };

        if (error.response) {
          status = error.response.status;
          errorData = error.response.data || errorData;
        } else if (error.code === 'ECONNABORTED') {
          status = 408;
          errorData = { error: 'Timeout', message: 'Request timeout' };
        }

        res.writeHead(status, {
          'Content-Type': 'application/json',
          'Access-Control-Allow-Origin': '*'
        });
        res.end(JSON.stringify(errorData));
      }
    });

  } catch (error: any) {
    console.error('❌ Критическая ошибка прокси:', error);
    res.writeHead(500, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': '*'
    });
    res.end(JSON.stringify({ error: 'Critical proxy error', message: error.message }));
  }
});

server.listen(PORT, () => {
  console.log('🚀 CORS прокси для Axenta API запущен!');
  console.log(`📡 Прокси сервер: http://localhost:${PORT}`);
  console.log(`🎯 Перенаправляет на: ${AXENTA_BASE_URL}`);
  console.log('');
  console.log('💡 Использование:');
  console.log(`   Вместо: https://axenta.cloud/api/auth/login/`);
  console.log(`   Используйте: http://localhost:${PORT}/api/auth/login/`);
  console.log('');
  console.log('⏹️  Для остановки нажмите Ctrl+C');
});

// Обработка завершения
process.on('SIGINT', () => {
  console.log('\n👋 CORS прокси остановлен');
  process.exit(0);
});
