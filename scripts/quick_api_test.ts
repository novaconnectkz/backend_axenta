#!/usr/bin/env npx tsx

/**
 * Быстрый тест API с выводом всей информации
 * Использование: npx tsx quick_api_test.ts username password
 */

import axios from 'axios';

const BASE_URL = 'https://axenta.cloud/api';

async function quickTest(username: string, password: string): Promise<void> {
  console.log('🚀 Быстрый тест API Axenta');
  console.log(`👤 Пользователь: ${username}`);
  console.log('');

  try {
    // Шаг 1: Авторизация
    console.log('🔐 Авторизация...');
    const authResponse = await axios.post(
      `${BASE_URL}/auth/login/`,
      { username, password },
      {
        headers: { 'Content-Type': 'application/json' },
        timeout: 10000,
        validateStatus: () => true
      }
    );

    console.log(`📊 Статус авторизации: ${authResponse.status}`);
    console.log('📦 Ответ авторизации:');
    console.log(JSON.stringify(authResponse.data, null, 2));
    console.log('');

    if (authResponse.status === 200 || authResponse.status === 201) {
      const token = authResponse.data.access;
      console.log('✅ Авторизация успешна!');
      console.log('');

      // Шаг 2: Получение аккаунтов
      console.log('📋 Получение аккаунтов...');
      const accountsResponse = await axios.get(
        `${BASE_URL}/cms/accounts/`,
        {
          headers: {
            'Authorization': `Bearer ${token}`,
            'Content-Type': 'application/json'
          },
          params: { page: 1, per_page: 10 },
          timeout: 10000,
          validateStatus: () => true
        }
      );

      console.log(`📊 Статус получения аккаунтов: ${accountsResponse.status}`);
      console.log('📦 Ответ с аккаунтами:');
      console.log(JSON.stringify(accountsResponse.data, null, 2));

      if (accountsResponse.data?.results) {
        console.log('');
        console.log('📋 Структура первого аккаунта:');
        const firstAccount = accountsResponse.data.results[0];
        if (firstAccount) {
          Object.entries(firstAccount).forEach(([key, value]) => {
            console.log(`   ${key}: ${typeof value} = ${JSON.stringify(value)}`);
          });
        }
      }

    } else {
      console.log('❌ Авторизация не удалась');
    }

  } catch (error) {
    console.error('💥 Ошибка:', error);
  }
}

// Получение аргументов командной строки
const args = process.argv.slice(2);
if (args.length < 2) {
  console.log('Использование: npx tsx quick_api_test.ts <username> <password>');
  console.log('Пример: npx tsx quick_api_test.ts user@example.com password123');
  process.exit(1);
}

const [username, password] = args;
quickTest(username, password);
