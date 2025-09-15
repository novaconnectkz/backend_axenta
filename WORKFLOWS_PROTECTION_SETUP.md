# 🚀 Быстрая настройка защиты GitHub Workflows

## ⚡ Что уже создано

✅ **Защитный workflow**: `.github/workflows/protect-workflows.yml`  
✅ **CODEOWNERS файл**: `.github/CODEOWNERS`  
✅ **Документация**: `WORKFLOWS_PROTECTION_GUIDE.md`  

## 🔧 Что нужно настроить вручную

### 1. Обновите CODEOWNERS файл

Отредактируйте `.github/CODEOWNERS` и замените placeholder'ы:

```bash
# ЗАМЕНИТЬ ЭТО:
/.github/workflows/ @admin-username

# НА РЕАЛЬНЫЙ USERNAME:
/.github/workflows/ @your-real-username
```

### 2. Настройте Branch Protection Rules

1. Перейдите на GitHub: **Settings → Branches**
2. Найдите правило для `main` или создайте новое
3. Включите следующие опции:

```
✅ Require a pull request before merging
✅ Require review from CODEOWNERS  
✅ Include administrators
✅ Require status checks to pass before merging
✅ Require branches to be up to date before merging
❌ Allow force pushes (отключить)
❌ Allow deletions (отключить)
```

### 3. Протестируйте защиту

```bash
# Создайте тестовую ветку
git checkout -b test-workflow-protection

# Внесите изменение в любой workflow файл
echo "# test comment" >> .github/workflows/main.yml

# Закоммитьте и запушьте
git add .
git commit -m "Test: попытка изменить workflow"
git push origin test-workflow-protection

# Создайте pull request и убедитесь, что сработала защита
```

## ⚠️ Важные моменты

1. **Замените все `@admin-username`** на реальные GitHub username'ы
2. **Включите "Include administrators"** для максимальной защиты
3. **Протестируйте** работу защиты перед внедрением
4. **Проинформируйте команду** о новых правилах

## 🆘 Если что-то не работает

1. Проверьте статус в **Actions → Workflows**
2. Убедитесь, что в CODEOWNERS указаны **существующие пользователи**
3. Проверьте **Branch Protection Rules** в Settings
4. Посмотрите **подробную документацию** в `WORKFLOWS_PROTECTION_GUIDE.md`

---

**⏱️ Время настройки:** ~10 минут  
**🔒 Уровень защиты:** Максимальный  
**✅ Готово к использованию:** После настройки username'ов
