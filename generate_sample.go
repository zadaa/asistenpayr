//go:build ignore

package main

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

func main() {
	f := excelize.NewFile()

	// Sheet 1: Budi
	f.SetSheetName("Sheet1", "Budi")
	f.SetCellValue("Budi", "A1", "Tanggal")
	f.SetCellValue("Budi", "B1", "Jam Mulai")
	f.SetCellValue("Budi", "C1", "Jam Selesai")
	f.SetCellValue("Budi", "D1", "Total Jam")
	f.SetCellValue("Budi", "E1", "Keterangan")

	f.SetCellValue("Budi", "A2", "2024-01-05")
	f.SetCellValue("Budi", "B2", "18:00")
	f.SetCellValue("Budi", "C2", "21:00")
	f.SetCellValue("Budi", "D2", 3)
	f.SetCellValue("Budi", "E2", "Project deadline")

	f.SetCellValue("Budi", "A3", "2024-01-12")
	f.SetCellValue("Budi", "B3", "17:30")
	f.SetCellValue("Budi", "C3", "20:30")
	f.SetCellValue("Budi", "D3", 3)
	f.SetCellValue("Budi", "E3", "Server maintenance")

	f.SetCellValue("Budi", "A4", "2024-01-20")
	f.SetCellValue("Budi", "B4", "18:00")
	f.SetCellValue("Budi", "C4", "22:00")
	f.SetCellValue("Budi", "D4", 4)
	f.SetCellValue("Budi", "E4", "Release deployment")

	// Sheet 2: Sari
	f.NewSheet("Sari")
	f.SetCellValue("Sari", "A1", "Tanggal")
	f.SetCellValue("Sari", "B1", "Jam Mulai")
	f.SetCellValue("Sari", "C1", "Jam Selesai")
	f.SetCellValue("Sari", "D1", "Total Jam")
	f.SetCellValue("Sari", "E1", "Keterangan")

	f.SetCellValue("Sari", "A2", "2024-01-08")
	f.SetCellValue("Sari", "B2", "17:00")
	f.SetCellValue("Sari", "C2", "20:00")
	f.SetCellValue("Sari", "D2", 3)
	f.SetCellValue("Sari", "E2", "Laporan bulanan")

	f.SetCellValue("Sari", "A3", "2024-01-15")
	f.SetCellValue("Sari", "B3", "18:00")
	f.SetCellValue("Sari", "C3", "21:30")
	f.SetCellValue("Sari", "D3", 3.5)
	f.SetCellValue("Sari", "E3", "Audit data")

	// Sheet 3: Andi
	f.NewSheet("Andi")
	f.SetCellValue("Andi", "A1", "Tanggal")
	f.SetCellValue("Andi", "B1", "Jam Mulai")
	f.SetCellValue("Andi", "C1", "Jam Selesai")
	f.SetCellValue("Andi", "D1", "Total Jam")
	f.SetCellValue("Andi", "E1", "Keterangan")

	f.SetCellValue("Andi", "A2", "2024-01-03")
	f.SetCellValue("Andi", "B2", "18:00")
	f.SetCellValue("Andi", "C2", "23:00")
	f.SetCellValue("Andi", "D2", 5)
	f.SetCellValue("Andi", "E2", "Migrasi database")

	f.SetCellValue("Andi", "A3", "2024-01-10")
	f.SetCellValue("Andi", "B3", "17:00")
	f.SetCellValue("Andi", "C3", "19:00")
	f.SetCellValue("Andi", "D3", 2)
	f.SetCellValue("Andi", "E3", "Bug fixing")

	f.SetCellValue("Andi", "A4", "2024-01-17")
	f.SetCellValue("Andi", "B4", "18:00")
	f.SetCellValue("Andi", "C4", "20:00")
	f.SetCellValue("Andi", "D4", 2)
	f.SetCellValue("Andi", "E4", "Testing")

	f.SetCellValue("Andi", "A5", "2024-01-25")
	f.SetCellValue("Andi", "B5", "17:30")
	f.SetCellValue("Andi", "C5", "21:30")
	f.SetCellValue("Andi", "D5", 4)
	f.SetCellValue("Andi", "E5", "Sprint review")

	if err := f.SaveAs("contoh_lembur.xlsx"); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Println("✅ File contoh_lembur.xlsx berhasil dibuat!")
}
