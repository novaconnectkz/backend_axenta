<template>
  <div class="test-page">
    <h1>Тестовая страница объектов</h1>
    <p>Если вы видите это сообщение, значит роутинг работает правильно.</p>
    
    <v-data-table
      :headers="headers"
      :items="items"
      class="elevation-1"
    >
      <template #item.is_active="{ item }">
        <v-checkbox
          :model-value="item.is_active"
          hide-details
          density="compact"
        />
      </template>
    </v-data-table>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const headers = [
  { title: 'Активность', value: 'is_active', sortable: false },
  { title: 'ID', value: 'id', sortable: true },
  { title: 'Название', value: 'name', sortable: true },
  { title: 'Название учетной записи', value: 'accountName', sortable: true },
  { title: 'Создатель (ФИО)', value: 'creatorName', sortable: true },
  { title: 'Модель устройства', value: 'deviceTypeName', sortable: true },
  { title: 'Номера телефонов', value: 'phoneNumbers', sortable: false },
  { title: 'Дата создания', value: 'createdAt', sortable: true },
  { title: 'Дата последнего сообщения', value: 'lastMessageDatetime', sortable: true },
  { title: 'Уникальный ID', value: 'uniqueId', sortable: true },
];

const items = ref([
  {
    id: 1,
    name: 'Тестовый объект 1',
    accountName: 'Тестовая учетная запись',
    creatorName: 'Иван Иванов',
    deviceTypeName: 'GPS Tracker v1.0',
    phoneNumbers: ['+7 900 123-45-67'],
    createdAt: '2024-01-15T10:30:00Z',
    lastMessageDatetime: '2024-01-20T15:45:00Z',
    uniqueId: 'UNIQUE-001',
    is_active: true
  },
  {
    id: 2,
    name: 'Тестовый объект 2',
    accountName: 'Другая учетная запись',
    creatorName: 'Петр Петров',
    deviceTypeName: 'GPS Tracker v2.0',
    phoneNumbers: ['+7 900 987-65-43', '+7 901 111-22-33'],
    createdAt: '2024-01-16T09:15:00Z',
    lastMessageDatetime: '2024-01-21T12:30:00Z',
    uniqueId: 'UNIQUE-002',
    is_active: false
  }
]);
</script>

<style scoped>
.test-page {
  padding: 20px;
}
</style>
