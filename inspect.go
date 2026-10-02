//go:build ignore

package main

import (
	"fmt"
	"github.com/xuri/excelize/v2"
)

func main() {
	f, err := excelize.OpenFile(`C:\Users\User\Downloads\Overtime Agustus 2026.xlsx`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()

	sheets := f.GetSheetList()
	fmt.Println("=== SHEETS ===")
	for i, s := range sheets {
		fmt.Printf("  [%d] %s\n", i, s)
	}
	fmt.Println()

	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			fmt.Println("Error reading sheet:", sheet, err)
			continue
		}
		fmt.Printf("=== Sheet: '%s' | Total rows: %d ===\n", sheet, len(rows))
		limit := 25
		if len(rows) < limit {
			limit = len(rows)
		}
		for i := 0; i < limit; i++ {
			row := rows[i]
			fmt.Printf("  Row %2d (%2d cols): %v\n", i+1, len(row), row)
		}
		fmt.Println()
	}
}
