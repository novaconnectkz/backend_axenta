#!/usr/bin/env npx tsx

/**
 * Скрипт для обнаружения правильных endpoints API Axenta
 * Тестирует различные варианты путей для авторизации
 */

import axios, { AxiosError } from 'axios';

const BASE_URL = 'https://axenta.cloud';

// Возможные варианты endpoints для авторизации
const AUTH_ENDPOINTS = [
  '/api/token/',
  '/api/auth/token/',
  '/api/v1/token/',
  '/api/v1/auth/token/',
  '/token/',
  '/auth/token/',
  '/api/login/',
  '/api/auth/login/',
  '/login/',
  '/auth/login/',
  '/api/signin/',
  '/signin/',
  '/api/authenticate/',
  '/authenticate/'
];

// Возможные варианты endpoints для получения аккаунтов
const ACCOUNTS_ENDPOINTS = [
  '/api/cms/accounts/',
  '/api/accounts/',
  '/api/v1/accounts/',
  '/api/v1/cms/accounts/',
  '/cms/accounts/',
  '/accounts/',
  '/api/companies/',
  '/api/v1/companies/',
  '/companies/'
];

interface TestResult {
  endpoint: string;
  status: number;
  statusText: string;
  contentType?: string;
  hasAuthFields?: boolean;
  errorDetails?: string;
}

/**
 * Тестирование endpoint'а
 */
async function testEndpoint(endpoint: string, method: 'GET' | 'POST' = 'POST', withAuth = false): Promise<TestResult> {
  try {
    const url = `${BASE_URL}${endpoint}`;
    const headers: any = {
      'Content-Type': 'application/json',
      'User-Agent': 'Axenta-Discovery-Script/1.0'
    };
    
    if (withAuth) {
      headers['Authorization'] = 'Bearer test-token';
    }
    
    const config: any = {
      url,
      method,
      headers,
      timeout: 5000,
      validateStatus: () => true // Принимаем любой статус
    };
    
    if (method === 'POST') {
      config.data = {
        username: 'test@example.com',
        password: 'test123'
      };
    }
    
    const response = await axios(config);
    
    // Проверяем, есть ли в ответе поля, характерные для авторизации
    let hasAuthFields = false;
    if (response.data && typeof response.data === 'object') {
      const responseStr = JSON.stringify(response.data).toLowerCase();
      hasAuthFields = responseStr.includes('access') || 
                     responseStr.includes('token') || 
                     responseStr.includes('refresh') ||
                     responseStr.includes('jwt') ||
                     responseStr.includes('auth');
    }
    
    return {
      endpoint,
      status: response.status,
      statusText: response.statusText,
      contentType: response.headers['content-type'],
      hasAuthFields,
      errorDetails: response.status >= 400 ? JSON.stringify(response.data) : undefined
    };
    
  } catch (error) {
    if (axios.isAxiosError(error)) {
      const axiosError = error as AxiosError;
      return {
        endpoint,
        status: axiosError.response?.status || 0,
        statusText: axiosError.response?.statusText || 'Network Error',
        errorDetails: axiosError.message
      };
    }
    
    return {
      endpoint,
      status: 0,
      statusText: 'Unknown Error',
      errorDetails: String(error)
    };
  }
}

/**
 * Отображение результатов тестирования
 */
function displayResults(results: TestResult[], title: string): void {
  console.log(`\n📊 ${title}:`);
  console.log('─'.repeat(80));
  
  results.forEach(result => {
    const statusColor = result.status >= 200 && result.status < 300 ? '✅' : 
                       result.status >= 400 && result.status < 500 ? '🟡' : 
                       result.status >= 500 ? '🔴' : '❌';
    
    console.log(`${statusColor} ${result.endpoint.padEnd(25)} | ${result.status} ${result.statusText}`);
    
    if (result.contentType) {
      console.log(`   Content-Type: ${result.contentType}`);
    }
    
    if (result.hasAuthFields) {
      console.log(`   🔑 Обнаружены поля авторизации!`);
    }
    
    if (result.errorDetails && result.status !== 404) {
      console.log(`   Error: ${result.errorDetails.substring(0, 100)}...`);
    }
    
    console.log('');
  });
}

/**
 * Главная функция
 */
async function main(): Promise<void> {
  console.log('🔍 Обнаружение endpoints API Axenta');
  console.log(`🌐 Базовый URL: ${BASE_URL}`);
  console.log('');
  
  // Тестируем endpoints авторизации
  console.log('🔐 Тестирование endpoints авторизации...');
  const authResults: TestResult[] = [];
  
  for (const endpoint of AUTH_ENDPOINTS) {
    process.stdout.write(`Тестирую ${endpoint}... `);
    const result = await testEndpoint(endpoint, 'POST');
    authResults.push(result);
    
    const statusIcon = result.status >= 200 && result.status < 300 ? '✅' : 
                      result.status === 400 || result.status === 401 ? '🟡' : 
                      result.status === 404 ? '❌' : '🔴';
    console.log(`${statusIcon} ${result.status}`);
  }
  
  displayResults(authResults, 'Результаты тестирования endpoints авторизации');
  
  // Тестируем endpoints аккаунтов (без авторизации)
  console.log('📋 Тестирование endpoints аккаунтов (GET без авторизации)...');
  const accountsResults: TestResult[] = [];
  
  for (const endpoint of ACCOUNTS_ENDPOINTS) {
    process.stdout.write(`Тестирую ${endpoint}... `);
    const result = await testEndpoint(endpoint, 'GET', false);
    accountsResults.push(result);
    
    const statusIcon = result.status >= 200 && result.status < 300 ? '✅' : 
                      result.status === 401 || result.status === 403 ? '🔐' : 
                      result.status === 404 ? '❌' : '🔴';
    console.log(`${statusIcon} ${result.status}`);
  }
  
  displayResults(accountsResults, 'Результаты тестирования endpoints аккаунтов');
  
  // Анализ результатов
  console.log('📈 Анализ результатов:');
  console.log('─'.repeat(50));
  
  const successfulAuth = authResults.filter(r => r.status >= 200 && r.status < 300);
  const authRequired = authResults.filter(r => r.status === 400 || r.status === 401);
  const authWithFields = authResults.filter(r => r.hasAuthFields);
  
  const successfulAccounts = accountsResults.filter(r => r.status >= 200 && r.status < 300);
  const accountsAuthRequired = accountsResults.filter(r => r.status === 401 || r.status === 403);
  
  if (successfulAuth.length > 0) {
    console.log('✅ Найдены работающие endpoints авторизации:');
    successfulAuth.forEach(r => console.log(`   • ${r.endpoint}`));
  }
  
  if (authRequired.length > 0) {
    console.log('🟡 Endpoints требующие правильных данных авторизации:');
    authRequired.forEach(r => console.log(`   • ${r.endpoint} (${r.status})`));
  }
  
  if (authWithFields.length > 0) {
    console.log('🔑 Endpoints с полями авторизации в ответе:');
    authWithFields.forEach(r => console.log(`   • ${r.endpoint}`));
  }
  
  if (successfulAccounts.length > 0) {
    console.log('✅ Найдены работающие endpoints аккаунтов:');
    successfulAccounts.forEach(r => console.log(`   • ${r.endpoint}`));
  }
  
  if (accountsAuthRequired.length > 0) {
    console.log('🔐 Endpoints аккаунтов требующие авторизации:');
    accountsAuthRequired.forEach(r => console.log(`   • ${r.endpoint} (${r.status})`));
  }
  
  // Рекомендации
  console.log('\n💡 Рекомендации:');
  if (authRequired.length > 0) {
    console.log('• Попробуйте использовать endpoints с кодами 400/401 - они могут быть правильными');
  }
  if (accountsAuthRequired.length > 0) {
    console.log('• Endpoints аккаунтов с кодами 401/403 требуют токен авторизации');
  }
  if (successfulAuth.length === 0 && authRequired.length === 0) {
    console.log('• Возможно, API использует другую схему авторизации (OAuth, API ключи)');
  }
  
  console.log('\n✅ Обнаружение завершено!');
}

// Запуск скрипта
if (require.main === module) {
  main().catch(error => {
    console.error('💥 Ошибка:', error);
    process.exit(1);
  });
}
