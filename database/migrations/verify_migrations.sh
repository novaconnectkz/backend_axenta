#!/bin/bash
# Migration Verification Script
# Checks if all required columns exist in production database

echo "🔍 Проверка миграций базы данных на продакшене..."
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
TOTAL=0
PASSED=0
FAILED=0

check_column() {
    local table=$1
    local column=$2
    local schema=${3:-public}
    
    TOTAL=$((TOTAL + 1))
    
    result=$(ssh root@194.87.143.169 "sudo -u postgres psql -d axenta_db -t -c \"SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = '$schema' AND table_name = '$table' AND column_name = '$column');\"" 2>/dev/null | xargs)
    
    if [ "$result" = "t" ]; then
        echo -e "${GREEN}✅${NC} $schema.$table.$column"
        PASSED=$((PASSED + 1))
    else
        echo -e "${RED}❌${NC} $schema.$table.$column - MISSING"
        FAILED=$((FAILED + 1))
    fi
}

echo "📋 Проверка таблицы contracts (public):"
check_column "contracts" "client_short_name" "public"
check_column "contracts" "sequential_number" "public"
check_column "contracts" "sync_status" "public"
check_column "contracts" "is_dirty" "public"

echo ""
echo "📋 Проверка таблицы contracts (tenant_186):"
check_column "contracts" "client_short_name" "tenant_186"
check_column "contracts" "sequential_number" "tenant_186"
check_column "contracts" "sync_status" "tenant_186"
check_column "contracts" "is_dirty" "tenant_186"

echo ""
echo "📋 Проверка таблицы subscriptions:"
check_column "subscriptions" "contract_id" "public"
check_column "subscriptions" "sequential_number" "public"

echo ""
echo "📋 Проверка таблицы invoices:"
check_column "invoices" "sequential_number" "public"

echo ""
echo "📋 Проверка таблицы notification_settings:"
check_column "notification_settings" "max_bot_token" "public"
check_column "notification_settings" "max_webhook_url" "public"
check_column "notification_settings" "max_enabled" "public"
check_column "notification_settings" "max_use_polling" "public"
check_column "notification_settings" "max_parse_mode" "public"

echo ""
echo "📋 Проверка таблицы billing_settings:"
check_column "billing_settings" "vat_rate_preset" "public"
check_column "billing_settings" "vat_rate_custom" "public"

echo ""
echo "📋 Проверка таблицы system_settings:"
result=$(ssh root@194.87.143.169 "sudo -u postgres psql -d axenta_db -t -c \"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'system_settings');\"" 2>/dev/null | xargs)
TOTAL=$((TOTAL + 1))
if [ "$result" = "t" ]; then
    echo -e "${GREEN}✅${NC} public.system_settings - EXISTS"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}❌${NC} public.system_settings - MISSING"
    FAILED=$((FAILED + 1))
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "📊 Результаты проверки:"
echo -e "   Всего проверок: ${YELLOW}$TOTAL${NC}"
echo -e "   Успешно: ${GREEN}$PASSED${NC}"
echo -e "   Ошибки: ${RED}$FAILED${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}🎉 Все миграции применены успешно!${NC}"
    exit 0
else
    echo -e "${RED}⚠️  Обнаружены отсутствующие колонки!${NC}"
    exit 1
fi

