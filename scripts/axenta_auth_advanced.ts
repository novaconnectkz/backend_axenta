#!/usr/bin/env npx tsx

/**
 * Расширенный скрипт для тестирования авторизации в API Axenta
 * с поддержкой переменных окружения и детальной диагностики
 * 
 * Использование:
 * 1. Скопируйте env.example в .env
 * 2. Заполните реальными данными
 * 3. Запустите: npx tsx axenta_auth_advanced.ts
 */

import axios, { AxiosError } from 'axios';
import { Table } from 'console-table-printer';
import * as dotenv from 'dotenv';
import * as path from 'path';

// Загрузка переменных окружения
dotenv.config({ path: path.join(__dirname, '.env') });

// Конфигурация из переменных окружения
const CONFIG = {
  baseUrl: process.env.AXENTA_BASE_URL || 'https://axenta.cloud/api',
  username: process.env.AXENTA_USERNAME || 'user@example.com',
  password: process.env.AXENTA_PASSWORD || 'password123',
  timeout: parseInt(process.env.AXENTA_TIMEOUT || '10000'),
  perPage: parseInt(process.env.AXENTA_PER_PAGE || '50'),
  debugMode: process.env.DEBUG_MODE === 'true'
};

// Интерфейсы для типизации данных
interface LoginCredentials {
  username: string;
  password: string;
}

interface AuthResponse {
  access: string;
  refresh: string;
}

interface Account {
  id: number;
  name: string;
  manager?: string;
  service_company?: string;
  objects_count: number;
  status: string;
  created_at?: string;
  updated_at?: string;
}

interface AccountsResponse {
  results: Account[];
  count: number;
  next?: string;
  previous?: string;
}

/**
 * Функция диагностики подключения к API
 */
async function diagnoseConnection(): Promise<void> {
  console.log('🔍 Диагностика подключения к API...');
  
  try {
    // Проверяем доступность базового URL
    const response = await axios.get(CONFIG.baseUrl, {
      timeout: 5000,
      validateStatus: () => true // Принимаем любой статус код
    });
    
    console.log(`✅ Сервер доступен (статус: ${response.status})`);
    
    if (CONFIG.debugMode) {
      console.log('🐛 Заголовки ответа:', response.headers);
    }
    
  } catch (error) {
    if (axios.isAxiosError(error)) {
      if (error.code === 'ENOTFOUND') {
        console.log('❌ DNS ошибка: Домен axenta.cloud не найден');
      } else if (error.code === 'ECONNREFUSED') {
        console.log('❌ Соединение отклонено сервером');
      } else if (error.code === 'ETIMEDOUT') {
        console.log('❌ Таймаут подключения');
      } else {
        console.log(`❌ Ошибка подключения: ${error.message}`);
      }
    }
  }
}

/**
 * Функция авторизации в API Axenta с детальной диагностикой
 */
async function login(credentials: LoginCredentials): Promise<AuthResponse> {
  try {
    console.log('🔐 Выполняется авторизация в API Axenta...');
    
    if (CONFIG.debugMode) {
      console.log(`🐛 URL: ${CONFIG.baseUrl}/auth/login/`);
      console.log(`🐛 Username: ${credentials.username}`);
      console.log(`🐛 Timeout: ${CONFIG.timeout}ms`);
    }
    
    const response = await axios.post<AuthResponse>(
      `${CONFIG.baseUrl}/auth/login/`,
      credentials,
      {
        headers: {
          'Content-Type': 'application/json',
          'User-Agent': 'Axenta-CRM-Script/1.0'
        },
        timeout: CONFIG.timeout,
        validateStatus: () => true // Принимаем любой статус код
      }
    );

    if (CONFIG.debugMode) {
      console.log('🐛 Статус ответа:', response.status);
      console.log('🐛 Данные ответа:', response.data);
    }

    if (response.status === 200 || response.status === 201) {
      console.log('✅ Авторизация успешна!');
      
      if (CONFIG.debugMode && response.data.access) {
        console.log('🐛 Получен токен доступа (первые 20 символов):', 
                   response.data.access.substring(0, 20) + '...');
      }
      
      return response.data;
    } else {
      // Обрабатываем различные статусы ошибок
      switch (response.status) {
        case 400:
          const errorData = response.data as any;
          let errorMsg = 'Проверьте структуру данных';
          
          if (Array.isArray(errorData?.detail)) {
            errorMsg = errorData.detail.join(', ');
          } else if (errorData?.detail) {
            errorMsg = errorData.detail;
          } else if (errorData?.message) {
            errorMsg = errorData.message;
          }
          
          throw new Error(`❌ Неверные данные запроса (400): ${errorMsg}`);
        case 401:
          throw new Error('❌ Ошибка авторизации: Неверные учетные данные (401)');
        case 403:
          throw new Error('❌ Доступ запрещен: Недостаточно прав (403)');
        case 404:
          throw new Error('❌ Endpoint не найден (404): Возможно, неверный URL API');
        case 429:
          throw new Error('❌ Слишком много запросов: Превышен лимит (429)');
        case 500:
          throw new Error('❌ Внутренняя ошибка сервера (500)');
        default:
          throw new Error(`❌ Ошибка сервера: ${response.status} - ${response.statusText}`);
      }
    }
    
  } catch (error) {
    if (axios.isAxiosError(error)) {
      const axiosError = error as AxiosError;
      
      if (axiosError.request && !axiosError.response) {
        throw new Error('❌ Ошибка сети: Не удается подключиться к серверу Axenta');
      }
    }
    
    // Если ошибка уже обработана выше, просто пробрасываем её
    if (error instanceof Error && error.message.startsWith('❌')) {
      throw error;
    }
    
    throw new Error(`❌ Неизвестная ошибка при авторизации: ${error}`);
  }
}

/**
 * Функция получения списка учётных записей с детальной диагностикой
 */
async function getAccounts(token: string): Promise<Account[]> {
  try {
    console.log('📋 Получение списка учётных записей...');
    
    const url = `${CONFIG.baseUrl}/cms/accounts/`;
    
    if (CONFIG.debugMode) {
      console.log(`🐛 URL: ${url}`);
      console.log(`🐛 Параметры: page=1, per_page=${CONFIG.perPage}, ordering=name`);
    }
    
    const response = await axios.get<AccountsResponse>(url, {
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'User-Agent': 'Axenta-CRM-Script/1.0'
      },
      params: {
        page: 1,
        per_page: CONFIG.perPage,
        ordering: 'name'
      },
      timeout: CONFIG.timeout,
      validateStatus: (status) => status < 500
    });

    if (response.status === 200) {
      console.log(`✅ Получено ${response.data.results.length} учётных записей из ${response.data.count} общих`);
      
      if (CONFIG.debugMode) {
        console.log('🐛 Структура первой записи:', 
                   response.data.results[0] ? JSON.stringify(response.data.results[0], null, 2) : 'Нет записей');
      }
      
      return response.data.results;
    } else {
      throw new Error(`Неожиданный статус ответа: ${response.status}`);
    }
    
  } catch (error) {
    if (axios.isAxiosError(error)) {
      const axiosError = error as AxiosError;
      
      if (CONFIG.debugMode && axiosError.response) {
        console.log('🐛 Детали ошибки:', axiosError.response.data);
      }
      
      if (axiosError.response) {
        switch (axiosError.response.status) {
          case 401:
            throw new Error('❌ Токен недействителен или истек (401)');
          case 403:
            throw new Error('❌ Недостаточно прав для просмотра учётных записей (403)');
          case 404:
            throw new Error('❌ Endpoint не найден: Проверьте URL API (404)');
          case 429:
            throw new Error('❌ Слишком много запросов: Превышен лимит (429)');
          default:
            throw new Error(`❌ Ошибка сервера: ${axiosError.response.status} - ${axiosError.response.statusText}`);
        }
      } else if (axiosError.request) {
        throw new Error('❌ Ошибка сети: Не удается подключиться к серверу Axenta');
      }
    }
    
    throw new Error(`❌ Неизвестная ошибка при получении учётных записей: ${error}`);
  }
}

/**
 * Отображение результатов в виде таблицы
 */
function displayAccountsTable(accounts: Account[]): void {
  if (accounts.length === 0) {
    console.log('📭 Учётные записи не найдены');
    return;
  }

  console.log('\n📊 Список учётных записей:');
  
  const table = new Table({
    title: 'Учётные записи Axenta',
    columns: [
      { name: 'id', title: 'ID', alignment: 'right' },
      { name: 'name', title: 'Название', alignment: 'left' },
      { name: 'manager', title: 'Менеджер', alignment: 'left' },
      { name: 'service_company', title: 'Сервисная компания', alignment: 'left' },
      { name: 'objects_count', title: 'Кол-во объектов', alignment: 'right' },
      { name: 'status', title: 'Статус', alignment: 'center' },
    ],
  });

  // Добавляем строки в таблицу
  accounts.forEach(account => {
    table.addRow({
      id: account.id,
      name: account.name || 'Не указано',
      manager: account.manager || 'Не назначен',
      service_company: account.service_company || 'Не указана',
      objects_count: account.objects_count,
      status: account.status || 'Неизвестно',
    });
  });

  table.printTable();
  
  // Дополнительная статистика
  const totalObjects = accounts.reduce((sum, account) => sum + account.objects_count, 0);
  const activeAccounts = accounts.filter(account => account.status === 'active' || account.status === 'Активен').length;
  
  console.log(`\n📈 Статистика:`);
  console.log(`   • Всего учётных записей: ${accounts.length}`);
  console.log(`   • Активных записей: ${activeAccounts}`);
  console.log(`   • Общее количество объектов: ${totalObjects}`);
  console.log(`   • Среднее количество объектов на запись: ${Math.round(totalObjects / accounts.length)}`);
}

/**
 * Отображение конфигурации
 */
function displayConfiguration(): void {
  console.log('⚙️ Текущая конфигурация:');
  console.log(`   • Базовый URL: ${CONFIG.baseUrl}`);
  console.log(`   • Пользователь: ${CONFIG.username}`);
  console.log(`   • Таймаут: ${CONFIG.timeout}ms`);
  console.log(`   • Записей на страницу: ${CONFIG.perPage}`);
  console.log(`   • Режим отладки: ${CONFIG.debugMode ? 'включен' : 'выключен'}`);
  console.log('');
}

/**
 * Главная функция
 */
async function main(): Promise<void> {
  console.log('🚀 Запуск расширенного скрипта тестирования API Axenta\n');

  displayConfiguration();

  // Учетные данные для авторизации
  const credentials: LoginCredentials = {
    username: CONFIG.username,
    password: CONFIG.password
  };

  try {
    // Шаг 0: Диагностика подключения
    await diagnoseConnection();
    console.log('');
    
    // Шаг 1: Авторизация
    const authData = await login(credentials);
    
    // Шаг 2: Получение учётных записей
    const accounts = await getAccounts(authData.access);
    
    // Шаг 3: Отображение результатов
    displayAccountsTable(accounts);
    
    console.log('\n✅ Скрипт выполнен успешно!');
    
  } catch (error) {
    console.error('\n💥 Ошибка выполнения скрипта:');
    console.error(error instanceof Error ? error.message : String(error));
    
    console.log('\n💡 Рекомендации:');
    console.log('   • Проверьте правильность учетных данных в .env файле');
    console.log('   • Убедитесь в доступности сервера axenta.cloud');
    console.log('   • Проверьте подключение к интернету');
    console.log('   • Убедитесь, что у пользователя есть права на просмотр учётных записей');
    console.log('   • Включите режим отладки (DEBUG_MODE=true) для детальной информации');
    
    process.exit(1);
  }
}

// Запуск скрипта только если он выполняется напрямую
if (require.main === module) {
  main().catch(error => {
    console.error('💥 Критическая ошибка:', error);
    process.exit(1);
  });
}

// Экспорт функций для возможного переиспользования
export { login, getAccounts, displayAccountsTable, diagnoseConnection };
