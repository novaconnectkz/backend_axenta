#!/usr/bin/env python3
import sys
try:
    import pandas as pd
    df = pd.read_excel('akk.xlsx')
    print("Колонки:", list(df.columns))
    print("\nКоличество строк:", len(df))
    print("\nПервые 100 строк:")
    print(df.head(100).to_string())
    print("\n\nВсе данные в CSV формате:")
    print(df.to_csv(index=False))
except ImportError:
    print("Pandas не установлен. Пробую openpyxl...")
    try:
        import openpyxl
        wb = openpyxl.load_workbook('akk.xlsx')
        ws = wb.active
        rows = list(ws.iter_rows(values_only=True))
        print(f"Количество строк: {len(rows)}")
        print("\nПервые 100 строк:")
        for i, row in enumerate(rows[:100], 1):
            print(f"{i}: {row}")
    except ImportError:
        print("Ошибка: не установлены ни pandas, ни openpyxl")
        sys.exit(1)
