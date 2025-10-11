#!/usr/bin/env npx tsx

/**
 * Скрипт для исследования и просмотра всей информации, возвращаемой API Axenta
 * Показывает детальную структуру ответов, заголовки, данные
 */

import axios, { AxiosResponse } from 'axios';
import * as dotenv from 'dotenv';
import * as path from 'path';

// Загрузка переменных окружения
dotenv.config({ path: path.join(__dirname, '.env') });

const CONFIG = {
  baseUrl: process.env.AXENTA_BASE_URL || 'https://axenta.cloud/api',
  username: process.env.AXENTA_USERNAME || 'user@example.com',
  password: process.env.AXENTA_PASSWORD || 'password123',
  timeout: parseInt(process.env.AXENTA_TIMEOUT || '10000')
};

/**
 * Красивый вывод объекта с отступами
 */
function prettyPrint(obj: any, title: string, indent = 0): void {
  const spaces = '  '.repeat(indent);
  console.log(`${spaces}📋 ${title}:`);
  
  if (typeof obj === 'object' && obj !== null) {
    if (Array.isArray(obj)) {
      console.log(`${spaces}   Тип: Array (${obj.length} элементов)`);
      obj.forEach((item, index) => {
        console.log(`${spaces}   [${index}]:`);
        prettyPrint(item, `Элемент ${index}`, indent + 2);
      });
    } else {
      console.log(`${spaces}   Тип: Object`);
      Object.entries(obj).forEach(([key, value]) => {
        if (typeof value === 'object' && value !== null) {
          prettyPrint(value, key, indent + 2);
        } else {
          console.log(`${spaces}     ${key}: ${JSON.stringify(value)}`);
        }
      });
    }
  } else {
    console.log(`${spaces}   Значение: ${JSON.stringify(obj)}`);
  }
}

/**
 * Анализ HTTP ответа
 */
function analyzeResponse(response: AxiosResponse, operation: string): void {
  console.log(`\n🔍 Анализ ответа для операции: ${operation}`);
  console.log('═'.repeat(60));
  
  // Основная информация
  console.log(`📊 Статус: ${response.status} ${response.statusText}`);
  console.log(`🌐 URL: ${response.config.url}`);
  console.log(`📝 Метод: ${response.config.method?.toUpperCase()}`);
  
  // Заголовки ответа
  console.log(`\n📋 Заголовки ответа:`);
  Object.entries(response.headers).forEach(([key, value]) => {
    console.log(`   ${key}: ${value}`);
  });
  
  // Данные ответа
  console.log(`\n📦 Данные ответа:`);
  if (response.data) {
    prettyPrint(response.data, 'Response Data');
    
    // Дополнительный анализ структуры
    console.log(`\n🔬 Анализ структуры данных:`);
    console.log(`   • Тип данных: ${typeof response.data}`);
    
    if (typeof response.data === 'object') {
      const keys = Object.keys(response.data);
      console.log(`   • Количество полей: ${keys.length}`);
      console.log(`   • Поля: ${keys.join(', ')}`);
      
      // Специальный анализ для разных типов ответов
      if (response.data.access || response.data.refresh) {
        console.log(`   🔑 Обнаружены токены авторизации!`);
        if (response.data.access) {
          console.log(`   • Access token (длина): ${response.data.access.length} символов`);
        }
        if (response.data.refresh) {
          console.log(`   • Refresh token (длина): ${response.data.refresh.length} символов`);
        }
      }
      
      if (response.data.results || Array.isArray(response.data)) {
        const items = response.data.results || response.data;
        console.log(`   📋 Обнаружен список данных (${items.length} элементов)`);
        
        if (items.length > 0) {
          console.log(`   • Структура первого элемента:`);
          Object.keys(items[0]).forEach(key => {
            console.log(`     - ${key}: ${typeof items[0][key]}`);
          });
        }
      }
    }
  } else {
    console.log(`   Данные отсутствуют`);
  }
  
  console.log('\n' + '═'.repeat(60));
}

/**
 * Тестирование авторизации с детальным анализом
 */
async function testAuth(): Promise<string | null> {
  try {
    console.log('🔐 Тестирование авторизации...');
    
    const response = await axios.post(
      `${CONFIG.baseUrl}/auth/login/`,
      {
        username: CONFIG.username,
        password: CONFIG.password
      },
      {
        headers: {
          'Content-Type': 'application/json',
          'User-Agent': 'Axenta-API-Explorer/1.0'
        },
        timeout: CONFIG.timeout,
        validateStatus: () => true
      }
    );
    
    analyzeResponse(response, 'Авторизация');
    
    if (response.status === 200 || response.status === 201) {
      return response.data.access;
    }
    
    return null;
    
  } catch (error) {
    console.error('❌ Ошибка при авторизации:', error);
    return null;
  }
}

/**
 * Тестирование получения аккаунтов с детальным анализом
 */
async function testAccounts(token: string): Promise<void> {
  try {
    console.log('📋 Тестирование получения аккаунтов...');
    
    const response = await axios.get(
      `${CONFIG.baseUrl}/cms/accounts/`,
      {
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
          'User-Agent': 'Axenta-API-Explorer/1.0'
        },
        params: {
          page: 1,
          per_page: 10,
          ordering: 'name'
        },
        timeout: CONFIG.timeout,
        validateStatus: () => true
      }
    );
    
    analyzeResponse(response, 'Получение аккаунтов');
    
  } catch (error) {
    console.error('❌ Ошибка при получении аккаунтов:', error);
  }
}

/**
 * Исследование различных endpoints
 */
async function exploreEndpoints(token?: string): Promise<void> {
  const endpoints = [
    { path: '/cms/accounts/', method: 'GET', needsAuth: true, description: 'Список аккаунтов' },
    { path: '/accounts/', method: 'GET', needsAuth: false, description: 'Альтернативный список аккаунтов' },
    { path: '/companies/', method: 'GET', needsAuth: false, description: 'Список компаний' },
    { path: '/api/user/', method: 'GET', needsAuth: true, description: 'Информация о пользователе' },
    { path: '/api/profile/', method: 'GET', needsAuth: true, description: 'Профиль пользователя' }
  ];
  
  console.log('\n🔍 Исследование дополнительных endpoints...');
  
  for (const endpoint of endpoints) {
    if (endpoint.needsAuth && !token) {
      console.log(`⏭️ Пропускаем ${endpoint.path} (требует авторизации)`);
      continue;
    }
    
    try {
      console.log(`\n🌐 Тестируем: ${endpoint.method} ${endpoint.path} - ${endpoint.description}`);
      
      const headers: any = {
        'Content-Type': 'application/json',
        'User-Agent': 'Axenta-API-Explorer/1.0'
      };
      
      if (endpoint.needsAuth && token) {
        headers['Authorization'] = `Bearer ${token}`;
      }
      
      const response = await axios({
        method: endpoint.method.toLowerCase() as any,
        url: `${CONFIG.baseUrl}${endpoint.path}`,
        headers,
        timeout: 5000,
        validateStatus: () => true
      });
      
      console.log(`   Статус: ${response.status} ${response.statusText}`);
      
      if (response.status >= 200 && response.status < 300) {
        console.log(`   ✅ Успешный ответ!`);
        if (response.data) {
          console.log(`   📦 Размер данных: ${JSON.stringify(response.data).length} символов`);
          
          // Краткий анализ структуры
          if (typeof response.data === 'object') {
            const keys = Object.keys(response.data);
            console.log(`   🔑 Поля: ${keys.slice(0, 5).join(', ')}${keys.length > 5 ? '...' : ''}`);
          }
        }
      } else if (response.status === 401 || response.status === 403) {
        console.log(`   🔒 Требует авторизации`);
      } else if (response.status === 404) {
        console.log(`   ❌ Endpoint не найден`);
      } else {
        console.log(`   ⚠️ Другая ошибка`);
      }
      
    } catch (error) {
      console.log(`   💥 Ошибка подключения`);
    }
  }
}

/**
 * Главная функция
 */
async function main(): Promise<void> {
  console.log('🚀 Запуск исследователя API Axenta');
  console.log(`🌐 Базовый URL: ${CONFIG.baseUrl}`);
  console.log(`👤 Пользователь: ${CONFIG.username}`);
  console.log('');
  
  // Тестируем авторизацию
  const token = await testAuth();
  
  if (token) {
    console.log('\n✅ Авторизация успешна! Тестируем получение данных...');
    await testAccounts(token);
  } else {
    console.log('\n❌ Авторизация не удалась, но продолжаем исследование...');
  }
  
  // Исследуем другие endpoints
  await exploreEndpoints(token);
  
  console.log('\n🎉 Исследование завершено!');
  
  if (!token) {
    console.log('\n💡 Для получения полной информации:');
    console.log('1. Создайте .env файл с реальными учетными данными');
    console.log('2. Запустите: npm run explore-api');
  }
}

// Запуск скрипта
if (require.main === module) {
  main().catch(error => {
    console.error('💥 Критическая ошибка:', error);
    process.exit(1);
  });
}

export { testAuth, testAccounts, exploreEndpoints, analyzeResponse };
