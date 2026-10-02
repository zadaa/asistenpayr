package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func parseDateString(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}

	if num, err := strconv.ParseFloat(s, 64); err == nil && num > 30000 && num < 100000 {
		if t, err := excelize.ExcelDateToTime(num, false); err == nil {
			return t, true
		}
	}

	r := strings.NewReplacer(
		"Januari", "Jan", "Februari", "Feb", "Maret", "Mar",
		"April", "Apr", "Mei", "May", "Juni", "Jun", "Juli", "Jul",
		"Agustus", "Aug", "Agt", "Aug", "Agst", "Aug",
		"September", "Sep", "Sept", "Sep",
		"Oktober", "Oct", "Okt", "Oct",
		"November", "Nov", "Nop", "Nov",
		"Desember", "Dec", "Des", "Dec",
	)
	normalized := r.Replace(s)

	formats := []string{
		"2-Jan-06",
		"02-Jan-06",
		"2-Jan-2006",
		"02-Jan-2006",
		"2 Jan 2006",
		"02 Jan 2006",
		"2 Jan 06",
		"02 Jan 06",
		"2/1/2006",
		"02/01/2006",
		"2/1/06",
		"02/01/06",
		"2-1-2006",
		"02-01-2006",
		"2-1-06",
		"02-01-06",
		"2006-1-2",
		"2006-01-02",
		"2006/1/2",
		"2006/01/02",
	}

	for _, fmtStr := range formats {
		if t, err := time.Parse(fmtStr, normalized); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func main() {
	f, err := excelize.OpenFile(`C:\Users\User\Downloads\Overtime Agustus 2026.xlsx`)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()

	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		fmt.Printf("--- Sheet: %s ---\n", sheet)
		for i, row := range rows {
			if i >= 18 && len(row) > 0 {
				dateStr := strings.TrimSpace(row[0])
				if dateStr != "" && dateStr != "Start" && dateStr != "Finish" {
					t, ok := parseDateString(dateStr)
					fmt.Printf("Row %d: Raw='%s' -> Parsed=%v (ok=%v)\n", i+1, dateStr, t.Format("2006-01-02"), ok)
				}
			}
		}
	}
}
