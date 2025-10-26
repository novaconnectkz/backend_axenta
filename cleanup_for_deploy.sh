#!/bin/bash

echo "🧹 Очистка проекта перед деплоем..."

# Переходим в директорию backend
cd /Users/com/backend_axenta

echo "📁 Очистка backend..."

# Удаляем временные файлы и логи
rm -f *.log
rm -f server*.log
rm -f backend*.log
rm -f local_server.log

# Удаляем временные бинарные файлы
rm -f main
rm -f main_linux
rm -f main_production
rm -f main_secure
rm -f main_test
rm -f axenta_backend
rm -f axenta_backend_test
rm -f axenta-backend
rm -f backend_axenta
rm -f backend_clean
rm -f backend_debug.log
rm -f backend_final
rm -f backend_final.log
rm -f backend_fixed
rm -f backend_new.log
rm -f backend_test
rm -f backend_working.log
rm -f backend.log

# Удаляем тестовые билды
rm -f api_test_build
rm -f middleware_test_build
rm -f test_auth_build
rm -f test_build
rm -f test_contracts_build
rm -f test_server

# Удаляем временные скрипты и файлы
rm -f test_*.sh
rm -f test_*.html
rm -f test_*.json
rm -f test_*.go

# Удаляем резервные копии баз данных
rm -f backup_*.sql
rm -f backup_*.sh

# Удаляем временные архивы
rm -f deploy.tar.gz

# Удаляем README файлы (оставляем только основные)
rm -f *_README.md
rm -f *_GUIDE.md
rm -f *_INSTRUCTIONS.md
rm -f *_REPORT.md
rm -f *_STATUS.md
rm -f *_SPECS.md
rm -f *_EXPLANATION.md
rm -f *_ANALYSIS.md
rm -f *_UPDATE.md
rm -f *_FIX.md
rm -f *_SOLUTION.md
rm -f *_PROTECTION.md
rm -f *_SETUP.md
rm -f *_VERIFICATION.md
rm -f *_QUICK_START.md
rm -f *_COMPLETE_REPORT.md
rm -f *_FINAL_REPORT.md
rm -f *_WORKING_NOW.md
rm -f *_CURRENT_STATUS.md
rm -f *_QUICK_REFERENCE.md
rm -f *_USAGE_GUIDE.md
rm -f *_INTERFACE_README.md
rm -f *_FRONTEND_README.md
rm -f *_DEPLOYMENT_GUIDE.md
rm -f *_SECRETS_SETUP.md
rm -f *_ISSUE_REPORT.md
rm -f *_PROP_TYPE_FIX.md
rm -f *_SERVER_ISSUE_REPORT.md
rm -f *_FIX_REPORT.md
rm -f *_UNIFICATION_REPORT.md
rm -f *_PRODUCTION_FIX_REPORT.md
rm -f *_TECHNICAL_SPECS.md
rm -f *_DESIGN.md
rm -f *_REQUIRED.md
rm -f *_LIMITATIONS_EXPLANATION.md
rm -f *_RESTRICTION_EXPLANATION.md
rm -f *_HIGHLIGHTING_UPDATE.md
rm -f *_SYSTEM_ACCOUNTS_UPDATE.md
rm -f *_SYSTEM_UPDATE.md
rm -f *_FEATURE_README.md
rm -f *_FEATURE.md
rm -f *_PAGINATION_INTERFACE.md
rm -f *_REAL_OBJECTS_GUIDE.md
rm -f *_PAGINATION_SOLUTION.md
rm -f *_TRANSFER_FEATURE_REPORT.md
rm -f *_DIAGNOSIS_REPORT.md
rm -f *_AUTH_FIX.md
rm -f *_TABLE.md
rm -f *_INTERFACE.md
rm -f *_GUIDE.md
rm -f *_README.md
rm -f *_INSTRUCTIONS.md
rm -f *_REPORT.md
rm -f *_STATUS.md
rm -f *_SPECS.md
rm -f *_EXPLANATION.md
rm -f *_ANALYSIS.md
rm -f *_UPDATE.md
rm -f *_FIX.md
rm -f *_SOLUTION.md
rm -f *_PROTECTION.md
rm -f *_SETUP.md
rm -f *_VERIFICATION.md
rm -f *_QUICK_START.md
rm -f *_COMPLETE_REPORT.md
rm -f *_FINAL_REPORT.md
rm -f *_WORKING_NOW.md
rm -f *_CURRENT_STATUS.md
rm -f *_QUICK_REFERENCE.md
rm -f *_USAGE_GUIDE.md
rm -f *_INTERFACE_README.md
rm -f *_FRONTEND_README.md
rm -f *_DEPLOYMENT_GUIDE.md
rm -f *_SECRETS_SETUP.md
rm -f *_ISSUE_REPORT.md
rm -f *_PROP_TYPE_FIX.md
rm -f *_SERVER_ISSUE_REPORT.md
rm -f *_FIX_REPORT.md
rm -f *_UNIFICATION_REPORT.md
rm -f *_PRODUCTION_FIX_REPORT.md
rm -f *_TECHNICAL_SPECS.md
rm -f *_DESIGN.md
rm -f *_REQUIRED.md
rm -f *_LIMITATIONS_EXPLANATION.md
rm -f *_RESTRICTION_EXPLANATION.md
rm -f *_HIGHLIGHTING_UPDATE.md
rm -f *_SYSTEM_ACCOUNTS_UPDATE.md
rm -f *_SYSTEM_UPDATE.md
rm -f *_FEATURE_README.md
rm -f *_FEATURE.md
rm -f *_PAGINATION_INTERFACE.md
rm -f *_REAL_OBJECTS_GUIDE.md
rm -f *_PAGINATION_SOLUTION.md
rm -f *_TRANSFER_FEATURE_REPORT.md
rm -f *_DIAGNOSIS_REPORT.md
rm -f *_AUTH_FIX.md
rm -f *_TABLE.md
rm -f *_INTERFACE.md

# Оставляем только основные README файлы
echo "✅ Оставляем основные README файлы..."

# Удаляем временные директории
rm -rf axetna-crm-system/
rm -rf backend_clean/
rm -rf backend_debug.log/
rm -rf backend_final/
rm -rf backend_fixed/
rm -rf backend_test/
rm -rf src/

echo "✅ Backend очищен!"

# Переходим в директорию frontend
cd /Users/com/frontend_axenta

echo "📁 Очистка frontend..."

# Удаляем временные файлы и логи
rm -f *.log
rm -f debug*.log
rm -f deploy*.log

# Удаляем тестовые HTML файлы
rm -f test_*.html

# Удаляем временные скрипты
rm -f *.exp
rm -f *.sh

# Удаляем README файлы (оставляем только основные)
rm -f *_README.md
rm -f *_GUIDE.md
rm -f *_INSTRUCTIONS.md
rm -f *_REPORT.md
rm -f *_STATUS.md
rm -f *_SPECS.md
rm -f *_EXPLANATION.md
rm -f *_ANALYSIS.md
rm -f *_UPDATE.md
rm -f *_FIX.md
rm -f *_SOLUTION.md
rm -f *_PROTECTION.md
rm -f *_SETUP.md
rm -f *_VERIFICATION.md
rm -f *_QUICK_START.md
rm -f *_COMPLETE_REPORT.md
rm -f *_FINAL_REPORT.md
rm -f *_WORKING_NOW.md
rm -f *_CURRENT_STATUS.md
rm -f *_QUICK_REFERENCE.md
rm -f *_USAGE_GUIDE.md
rm -f *_INTERFACE_README.md
rm -f *_FRONTEND_README.md
rm -f *_DEPLOYMENT_GUIDE.md
rm -f *_SECRETS_SETUP.md
rm -f *_ISSUE_REPORT.md
rm -f *_PROP_TYPE_FIX.md
rm -f *_SERVER_ISSUE_REPORT.md
rm -f *_FIX_REPORT.md
rm -f *_UNIFICATION_REPORT.md
rm -f *_PRODUCTION_FIX_REPORT.md
rm -f *_TECHNICAL_SPECS.md
rm -f *_DESIGN.md
rm -f *_REQUIRED.md
rm -f *_LIMITATIONS_EXPLANATION.md
rm -f *_RESTRICTION_EXPLANATION.md
rm -f *_HIGHLIGHTING_UPDATE.md
rm -f *_SYSTEM_ACCOUNTS_UPDATE.md
rm -f *_SYSTEM_UPDATE.md
rm -f *_FEATURE_README.md
rm -f *_FEATURE.md
rm -f *_PAGINATION_INTERFACE.md
rm -f *_REAL_OBJECTS_GUIDE.md
rm -f *_PAGINATION_SOLUTION.md
rm -f *_TRANSFER_FEATURE_REPORT.md
rm -f *_DIAGNOSIS_REPORT.md
rm -f *_AUTH_FIX.md
rm -f *_TABLE.md
rm -f *_INTERFACE.md

# Удаляем временные директории
rm -rf scripts/

echo "✅ Frontend очищен!"

echo "🎉 Очистка завершена! Проект готов к деплою."
echo ""
echo "📊 Статистика очистки:"
echo "   - Удалены временные логи и файлы"
echo "   - Удалены тестовые билды"
echo "   - Удалены README файлы (оставлены только основные)"
echo "   - Удалены временные скрипты и HTML файлы"
echo "   - Удалены резервные копии БД"
echo ""
echo "🚀 Теперь можно делать деплой!"
