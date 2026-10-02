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
		// Day-MonthName-Year
		"2-Jan-06",
		"02-Jan-06",
		"2-Jan-2006",
		"02-Jan-2006",
		"2 Jan 2006",
		"02 Jan 2006",
		"2 Jan 06",
		"02 Jan 06",

		// DD/MM/YYYY and DD-MM-YYYY (flexible digits)
		"2/1/2006",
		"02/01/2006",
		"2/1/06",
		"02/01/06",
		"2-1-2006",
		"02-01-2006",
		"2-1-06",
		"02-01-06",

		// MM/DD/YYYY fallback
		"1/2/2006",
		"01/02/2006",
		"1/2/06",
		"01/02/06",

		// YYYY-MM-DD and YYYY/MM/DD
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
	testCases := []string{
		"26/07/2026",
		"27/07/2026",
		"28/07/2026",
		"1-Aug-26",
		"2-Aug-26",
		"8-Aug-26",
		"12/08/2026",
		"15/08/2026",
		"17/08/2026",
		"1/8/2026",
		"26/7/2026",
		"26-07-2026",
		"26/07/26",
	}

	for _, tc := range testCases {
		t, ok := parseDateString(tc)
		fmt.Printf("Input: %-15s -> Parsed: %-12s (Success: %v)\n", tc, t.Format("2006-01-02"), ok)
	}
}
