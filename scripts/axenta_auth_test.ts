#!/usr/bin/env npx tsx

/**
 * Скрипт для тестирования авторизации в API Axenta и получения списка учётных записей
 * 
 * Использование:
 * npm run test-auth
 * 
 * или напрямую:
 * npx tsx axenta_auth_test.ts
 */

import axios, { AxiosError } from 'axios';
import { Table } from 'console-table-printer';

// Базовый URL API Axenta
const BASE_URL = 'https://axenta.cloud/api';

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
}

interface AccountsResponse {
  results: Account[];
  count: number;
  next?: string;
  previous?: string;
}

/**
 * Функция авторизации в API Axenta
 * @param credentials - учетные данные пользователя
 * @returns Promise с токенами доступа
 */
async function login(credentials: LoginCredentials): Promise<AuthResponse> {
  try {
    console.log('🔐 Выполняется авторизация в API Axenta...');
    
    const response = await axios.post<AuthResponse>(
      `${BASE_URL}/auth/login/`,
      credentials,
      {
        headers: {
          'Content-Type': 'application/json',
        },
        timeout: 10000, // 10 секунд таймаут
      }
    );

    console.log('✅ Авторизация успешна!');
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      const axiosError = error as AxiosError;
      
      if (axiosError.response) {
        // Сервер ответил с ошибкой
        switch (axiosError.response.status) {
          case 401:
            throw new Error('❌ Ошибка авторизации: Неверные учетные данные (401)');
          case 403:
            throw new Error('❌ Доступ запрещен: Недостаточно прав (403)');
          case 429:
            throw new Error('❌ Слишком много запросов: Превышен лимит (429)');
          default:
            throw new Error(`❌ Ошибка сервера: ${axiosError.response.status} - ${axiosError.response.statusText}`);
        }
      } else if (axiosError.request) {
        // Запрос был отправлен, но ответа не получено
        throw new Error('❌ Ошибка сети: Не удается подключиться к серверу Axenta');
      }
    }
    
    throw new Error(`❌ Неизвестная ошибка при авторизации: ${error}`);
  }
}

/**
 * Функция получения списка учётных записей
 * @param token - токен доступа
 * @returns Promise со списком учётных записей
 */
async function getAccounts(token: string): Promise<Account[]> {
  try {
    console.log('📋 Получение списка учётных записей...');
    
    const response = await axios.get<AccountsResponse>(
      `${BASE_URL}/cms/accounts/`,
      {
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        params: {
          page: 1,
          per_page: 50,
          ordering: 'name'
        },
        timeout: 15000, // 15 секунд таймаут
      }
    );

    console.log(`✅ Получено ${response.data.results.length} учётных записей из ${response.data.count} общих`);
    return response.data.results;
  } catch (error) {
    if (axios.isAxiosError(error)) {
      const axiosError = error as AxiosError;
      
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
 * @param accounts - массив учётных записей
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
  console.log(`\n📈 Статистика:`);
  console.log(`   • Всего учётных записей: ${accounts.length}`);
  console.log(`   • Общее количество объектов: ${totalObjects}`);
  console.log(`   • Среднее количество объектов на запись: ${Math.round(totalObjects / accounts.length)}`);
}

/**
 * Главная функция
 */
async function main(): Promise<void> {
  console.log('🚀 Запуск скрипта тестирования API Axenta\n');

  // Учетные данные для авторизации
  // В реальном проекте лучше использовать переменные окружения
  const credentials: LoginCredentials = {
    username: 'user@example.com',
    password: 'password123'
  };

  try {
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
    console.log('   • Проверьте правильность учетных данных');
    console.log('   • Убедитесь в доступности сервера axenta.cloud');
    console.log('   • Проверьте подключение к интернету');
    console.log('   • Убедитесь, что у пользователя есть права на просмотр учётных записей');
    
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
export { login, getAccounts, displayAccountsTable };
