package database

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSearchPathInvariant — регресс-guard по долгу #15.
//
// ИНВАРИАНТ архитектуры (доказан анализом 2026-05-19):
//   - main-пул (database.DB) подключается БЕЗ search_path в DSN
//     (config.GetDatabaseDSN) → connections всегда в `public`
//     (Postgres-дефолт). Никто не загрязняет его tenant-схемой
//     в request-time.
//   - tenant-доступ ТОЛЬКО через database.ConnectToTenant → отдельный
//     per-schema пул (search_path вшит в DSN, кэширован). Главный пул
//     не трогает.
//   - Поэтому распространённый паттерн `db.Session()+Exec("SET
//     search_path TO public")` (getPublicDB во многих пакетах) —
//     избыточно-защитный no-op, а не дыра: confused-schema на
//     pooled-conn невозможен, пока main-пул не переключают на tenant
//     в request-time.
//
// Единственный способ РЕАЛЬНО создать confused-schema баг —
// request-time `SET search_path TO <не-public>` на общем пуле.
// Этот тест падает, если такой паттерн появится в коде, кроме
// разрешённых стартап-мест (migrations.go тенантный цикл на старте,
// однопоточно + reset в public; database.go — конфиг пула).
func TestSearchPathInvariant(t *testing.T) {
	// RE2 (Go regexp) без lookahead: ловим имя схемы группой,
	// «не-public» проверяем в Go.
	re := regexp.MustCompile(`SET search_path TO\s+([A-Za-z_][A-Za-z0-9_]*)`)

	allowed := map[string]bool{
		"database/migrations.go": true, // стартап-миграции (не request-time)
		"database/database.go":   true, // конфигурация пулов
	}

	root := ".."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := strings.TrimPrefix(filepath.ToSlash(path), "../")
		// cmd/* — одноразовые CLI-утилиты (single-process, не
		// request-serving): мутация search_path там безопасна, как и
		// в стартап-миграциях.
		if allowed[rel] || strings.Contains(rel, "/vendor/") ||
			strings.HasPrefix(rel, "cmd/") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for _, m := range re.FindAllSubmatchIndex(b, -1) {
			schema := strings.ToLower(string(b[m[2]:m[3]]))
			if schema == "public" {
				continue
			}
			line := 1 + strings.Count(string(b[:m[0]]), "\n")
			t.Errorf("долг #15 НАРУШЕН: %s:%d — request-time `SET search_path TO %s` "+
				"на общем пуле создаёт confused-schema. Используй database.ConnectToTenant "+
				"(отдельный per-schema пул), а не DB.Exec(SET search_path TO <schema>).", rel, line, schema)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
