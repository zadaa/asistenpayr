package main

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
)

//go:embed logo.png
var logoBytes []byte

// Static credentials
const (
	authUsername = "lia"
	authPassword = "422079"
)

// Default Claude API key provided by user
const defaultClaudeAPIKey = "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"

func getClaudeAPIKey() string {
	if key := os.Getenv("CLAUDE_API_KEY"); key != "" {
		return key
	}
	return defaultClaudeAPIKey
}

// Session store
var (
	sessions     = make(map[string]time.Time)
	sessionsLock sync.RWMutex
)

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}
	sessionsLock.RLock()
	defer sessionsLock.RUnlock()
	expiry, ok := sessions[cookie.Value]
	if !ok || time.Now().After(expiry) {
		return false
	}
	return true
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func authAPIMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
			return
		}
		next(w, r)
	}
}

// PreviewResponse is the JSON structure returned by /api/preview
type PreviewResponse struct {
	Success    bool       `json:"success"`
	Message    string     `json:"message,omitempty"`
	Error      string     `json:"error,omitempty"`
	TotalFiles int        `json:"totalFiles,omitempty"`
	Sheets     []string   `json:"sheets,omitempty"`
	Headers    []string   `json:"headers,omitempty"`
	Rows       [][]string `json:"rows,omitempty"`
	TotalRows  int        `json:"totalRows,omitempty"`
	SheetStats []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"sheetStats,omitempty"`
}

// ExtractedRow is the structure returned by Claude API for PDF OCR
type ExtractedRow struct {
	EmpName         string `json:"emp_name"`
	NIK             string `json:"nik"`
	Position        string `json:"position"`
	Charge          string `json:"charge"`
	Date            string `json:"date"`
	Day             string `json:"day"`
	DayCategory     string `json:"day_category"`
	OTMorningStart  string `json:"ot_morning_start"`
	OTMorningFinish string `json:"ot_morning_finish"`
	OTNightStart    string `json:"ot_night_start"`
	OTNightFinish   string `json:"ot_night_finish"`
	TotalOT         string `json:"total_ot"`
}

type ClaudeDocSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ClaudeContentItem struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *ClaudeDocSource `json:"source,omitempty"`
}

type ClaudeMessage struct {
	Role    string              `json:"role"`
	Content []ClaudeContentItem `json:"content"`
}

type ClaudeReq struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []ClaudeMessage `json:"messages"`
}

type ClaudeResp struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/logo.png", handleLogo)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/api/login", handleLoginAPI)
	mux.HandleFunc("/api/logout", handleLogout)
	mux.HandleFunc("/", authMiddleware(handleIndex))
	mux.HandleFunc("/api/preview", authAPIMiddleware(handlePreview))
	mux.HandleFunc("/api/merge", authAPIMiddleware(handleMerge))

	// Find available listener
	listener, url, err := createListener()
	if err != nil {
		fmt.Printf("❌ Gagal membuka port: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║         ASISTEN PAYROL - Penggabung Data Lembur        ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  🌐 Buka browser: %-38s ║\n", url)
	fmt.Println("║  ⏹  Tekan Ctrl+C untuk berhenti                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	// Open browser automatically
	go func() {
		time.Sleep(300 * time.Millisecond)
		openBrowser(url)
	}()

	server := &http.Server{Handler: mux}
	if err := server.Serve(listener); err != nil {
		fmt.Printf("❌ Server error: %v\n", err)
	}
}

func createListener() (net.Listener, string, error) {
	if portStr := os.Getenv("PORT"); portStr != "" {
		l, err := net.Listen("tcp", "0.0.0.0:"+portStr)
		if err == nil {
			return l, fmt.Sprintf("http://0.0.0.0:%s", portStr), nil
		}
	}

	// Try 0.0.0.0 binding on preferred ports for Cloud/Docker compatibility
	for _, p := range []int{8080, 8081, 8082, 5000, 3000} {
		l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", p))
		if err == nil {
			return l, fmt.Sprintf("http://127.0.0.1:%d", p), nil
		}
	}
	// Fallback to dynamic port on all interfaces
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, "", err
	}
	port := l.Addr().(*net.TCPAddr).Port
	return l, fmt.Sprintf("http://127.0.0.1:%d", port), nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if isAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		user := strings.TrimSpace(r.FormValue("username"))
		pass := r.FormValue("password")

		if user == authUsername && pass == authPassword {
			token := generateToken()
			expiry := time.Now().Add(24 * time.Hour)

			sessionsLock.Lock()
			sessions[token] = expiry
			sessionsLock.Unlock()

			http.SetCookie(w, &http.Cookie{
				Name:     "session",
				Value:    token,
				Path:     "/",
				Expires:  expiry,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, loginHTML)
}

func handleLoginAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		w.WriteHeader(405)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request"})
		return
	}

	if req.Username != authUsername || req.Password != authPassword {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username atau password salah"})
		return
	}

	token := generateToken()
	expiry := time.Now().Add(24 * time.Hour)

	sessionsLock.Lock()
	sessions[token] = expiry
	sessionsLock.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "user": req.Username})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		sessionsLock.Lock()
		delete(sessions, cookie.Value)
		sessionsLock.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func handleLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(logoBytes)
}

func parsePDFWithClaude(pdfBytes []byte) ([]ExtractedRow, error) {
	apiKey := getClaudeAPIKey()
	base64PDF := base64.StdEncoding.EncodeToString(pdfBytes)

	prompt := `Anda adalah AI extractor data lembur / SPL (Surat Perintah Lembur). 
Silakan baca dokumen PDF ini (yang mungkin berupa tulisan tangan atau formulir cetakan) dan ekstrak SEMUA entri data lembur ke dalam JSON array murni.

Setiap objek dalam array JSON harus berisi kunci-kunci berikut:
- "emp_name": Nama Karyawan
- "nik": NIK / Nomor Induk Karyawan (jika ada, jika tidak ada isi "")
- "position": Jabatan / Posisi (jika ada, jika tidak ada isi "")
- "charge": Tag Charge / Customer / Divisi / Cost Center (jika ada, contoh: Chg Persol, Chg Coke, dll)
- "date": Tanggal lembur (format YYYY-MM-DD atau sesuai tertera)
- "day": Nama Hari (contoh: Senin, Selasa, Sab, Jum, dll)
- "day_category": Kategori Hari (contoh: WD untuk Workday, HO untuk Holiday/Off, AL untuk Annual Leave)
- "ot_morning_start": Jam Mulai Lembur Pagi (contoh: 06:00, jika tidak ada isi "")
- "ot_morning_finish": Jam Selesai Lembur Pagi (contoh: 08:00, jika tidak ada isi "")
- "ot_night_start": Jam Mulai Lembur Malam (contoh: 17:00, jika tidak ada isi "")
- "ot_night_finish": Jam Selesai Lembur Malam (contoh: 21:00, jika tidak ada isi "")
- "total_ot": Total Jam Lembur (contoh: 4.0 atau 4, jika tidak ada isi "")

PENTING: Kembalikan HANYA JSON array murni saja (diawali [ dan diakhiri ]). DILARANG menyertakan teks pembuka, penutup, atau formatting markdown.`

	models := []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-7-sonnet-20250219",
		"claude-3-5-haiku-20241022",
		"claude-3-haiku-20240307",
	}

	var lastErr error
	for _, model := range models {
		reqBody := ClaudeReq{
			Model:     model,
			MaxTokens: 4096,
			Messages: []ClaudeMessage{
				{
					Role: "user",
					Content: []ClaudeContentItem{
						{
							Type: "document",
							Source: &ClaudeDocSource{
								Type:      "base64",
								MediaType: "application/pdf",
								Data:      base64PDF,
							},
						},
						{
							Type: "text",
							Text: prompt,
						},
					},
				},
			},
		}

		jsonBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonBytes))
		if err != nil {
			return nil, err
		}

		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("content-type", "application/json")

		client := &http.Client{Timeout: 90 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var cErr ClaudeResp
			_ = json.Unmarshal(respBytes, &cErr)
			if cErr.Error != nil {
				lastErr = fmt.Errorf("Claude API error (%s): %s", model, cErr.Error.Message)
			} else {
				lastErr = fmt.Errorf("Claude API HTTP %d: %s", resp.StatusCode, string(respBytes))
			}
			continue
		}

		var cResp ClaudeResp
		if err := json.Unmarshal(respBytes, &cResp); err != nil {
			lastErr = err
			continue
		}

		if len(cResp.Content) == 0 {
			lastErr = fmt.Errorf("Claude API returning empty content")
			continue
		}

		rawText := ""
		for _, cnt := range cResp.Content {
			if cnt.Type == "text" {
				rawText += cnt.Text
			}
		}

		// Clean JSON response
		rawText = strings.TrimSpace(rawText)
		if idx := strings.Index(rawText, "["); idx != -1 {
			if lastIdx := strings.LastIndex(rawText, "]"); lastIdx != -1 && lastIdx > idx {
				rawText = rawText[idx : lastIdx+1]
			}
		}

		var rows []ExtractedRow
		if err := json.Unmarshal([]byte(rawText), &rows); err != nil {
			lastErr = fmt.Errorf("Gagal parsing JSON output dari AI (%s): %v", model, err)
			continue
		}

		return rows, nil
	}

	return nil, fmt.Errorf("Gagal memproses PDF dengan AI: %v", lastErr)
}

type ProcessedResult struct {
	TotalFiles int
	Headers    []string
	Rows       [][]string
	SheetStats []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	SheetNames []string
}

func processUploadedFiles(r *http.Request) (*ProcessedResult, error) {
	err := r.ParseMultipartForm(100 << 20) // 100MB max
	if err != nil {
		return nil, fmt.Errorf("gagal membaca form: %v", err)
	}

	var fileHeaders []*multipart.FileHeader
	if fhs, ok := r.MultipartForm.File["file"]; ok && len(fhs) > 0 {
		fileHeaders = append(fileHeaders, fhs...)
	}
	if fhs, ok := r.MultipartForm.File["files"]; ok && len(fhs) > 0 {
		fileHeaders = append(fileHeaders, fhs...)
	}

	if len(fileHeaders) == 0 {
		return nil, fmt.Errorf("tidak ada file yang diunggah")
	}

	headers := []string{"Nama Karyawan", "NIK", "Position", "Charge", "Date", "Day", "Day Category",
		"OT Morning Start", "OT Morning Finish", "OT Night Start", "OT Night Finish", "Total Overtime"}

	result := &ProcessedResult{
		TotalFiles: len(fileHeaders),
		Headers:    headers,
	}

	for _, handler := range fileHeaders {
		filename := handler.Filename
		lowerName := strings.ToLower(filename)

		file, err := handler.Open()
		if err != nil {
			continue
		}

		fileBytes, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			continue
		}

		if strings.HasSuffix(lowerName, ".pdf") {
			// Process PDF with Claude Vision AI
			pdfRows, err := parsePDFWithClaude(fileBytes)
			if err != nil {
				return nil, fmt.Errorf("gagal membaca PDF '%s': %v", filename, err)
			}

			dataCount := 0
			empNameMap := make(map[string]int)

			for _, pr := range pdfRows {
				emp := strings.TrimSpace(pr.EmpName)
				if emp == "" {
					emp = strings.TrimSuffix(filename, ".pdf")
				}

				dateVal := pr.Date
				if t, ok := parseDateString(dateVal); ok {
					dateVal = t.Format("2006-01-02")
				}

				outRow := []string{
					emp,
					pr.NIK,
					pr.Position,
					pr.Charge,
					dateVal,
					pr.Day,
					pr.DayCategory,
					pr.OTMorningStart,
					pr.OTMorningFinish,
					pr.OTNightStart,
					pr.OTNightFinish,
					pr.TotalOT,
				}
				result.Rows = append(result.Rows, outRow)
				dataCount++
				empNameMap[emp]++
			}

			for emp, cnt := range empNameMap {
				sheetLabel := fmt.Sprintf("%s (PDF)", emp)
				result.SheetNames = append(result.SheetNames, sheetLabel)
				result.SheetStats = append(result.SheetStats, struct {
					Name  string `json:"name"`
					Count int    `json:"count"`
				}{Name: sheetLabel, Count: cnt})
			}
			if len(empNameMap) == 0 {
				sheetLabel := fmt.Sprintf("%s (PDF)", filename)
				result.SheetNames = append(result.SheetNames, sheetLabel)
				result.SheetStats = append(result.SheetStats, struct {
					Name  string `json:"name"`
					Count int    `json:"count"`
				}{Name: sheetLabel, Count: 0})
			}

		} else if strings.HasSuffix(lowerName, ".xlsx") || strings.HasSuffix(lowerName, ".xls") {
			// Save to temp file for excelize
			tmpFile, err := os.CreateTemp("", "merge-lembur-*.xlsx")
			if err != nil {
				continue
			}
			tmpPath := tmpFile.Name()
			tmpFile.Write(fileBytes)
			tmpFile.Close()

			f, err := excelize.OpenFile(tmpPath)
			if err != nil {
				os.Remove(tmpPath)
				continue
			}

			sheets := f.GetSheetList()
			for _, sheetName := range sheets {
				result.SheetNames = append(result.SheetNames, sheetName)
				rows, err := f.GetRows(sheetName)
				if err != nil || len(rows) < 19 {
					result.SheetStats = append(result.SheetStats, struct {
						Name  string `json:"name"`
						Count int    `json:"count"`
					}{Name: sheetName, Count: 0})
					continue
				}

				empName := extractEmployeeName(rows)
				if empName == "" {
					empName = sheetName
				}
				nik := getCell(rows[2], 1)
				position := ""
				for i := 4; i < 8 && i < len(rows); i++ {
					if len(rows[i]) >= 3 && strings.EqualFold(strings.TrimSpace(rows[i][1]), "Position") {
						position = strings.TrimSpace(rows[i][2])
						break
					}
				}
				chargeTag := extractChargeTag(sheetName)

				headerRowIdx := findHeaderRow(rows)
				if headerRowIdx < 0 {
					result.SheetStats = append(result.SheetStats, struct {
						Name  string `json:"name"`
						Count int    `json:"count"`
					}{Name: sheetName, Count: 0})
					continue
				}

				dataStartIdx := findDataStartRow(rows, headerRowIdx)
				if dataStartIdx < 0 {
					result.SheetStats = append(result.SheetStats, struct {
						Name  string `json:"name"`
						Count int    `json:"count"`
					}{Name: sheetName, Count: 0})
					continue
				}

				dataCount := 0
				for i := dataStartIdx; i < len(rows); i++ {
					row := rows[i]
					if isEmptyDataRow(row) {
						continue
					}

					dateVal := getCell(row, 0)
					if t, ok := parseDateString(dateVal); ok {
						dateVal = t.Format("2006-01-02")
					}

					outRow := []string{
						empName,
						nik,
						position,
						chargeTag,
						dateVal,
						getCell(row, 1),
						getCell(row, 2),
						getCell(row, 3),
						getCell(row, 4),
						getCell(row, 5),
						getCell(row, 6),
						getCell(row, 7),
					}
					result.Rows = append(result.Rows, outRow)
					dataCount++
				}
				result.SheetStats = append(result.SheetStats, struct {
					Name  string `json:"name"`
					Count int    `json:"count"`
				}{Name: empName, Count: dataCount})
			}

			f.Close()
			os.Remove(tmpPath)
		}
	}

	if len(result.Rows) == 0 && len(result.SheetStats) == 0 {
		return nil, fmt.Errorf("tidak dapat menemukan data lembur dalam file yang diunggah")
	}

	return result, nil
}

// getCell safely gets a cell value from a row slice
func getCell(row []string, idx int) string {
	if idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

// extractEmployeeName extracts the employee name from the sheet metadata.
func extractEmployeeName(rows [][]string) string {
	for i := 2; i < 8 && i < len(rows); i++ {
		if len(rows[i]) >= 3 {
			label := strings.TrimSpace(rows[i][1])
			if strings.EqualFold(label, "Name") {
				return strings.TrimSpace(rows[i][2])
			}
		}
	}
	return ""
}

// extractChargeTag extracts the charge category/tag (e.g. "Chg Coke", "Chg Persol") from sheet name.
func extractChargeTag(sheetName string) string {
	idx := strings.LastIndex(sheetName, "_")
	if idx != -1 {
		res := strings.TrimSpace(sheetName[idx+1:])
		if res != "" {
			return res
		}
	}
	return ""
}

// parseDateString converts date strings into time.Time.
func parseDateString(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
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

		"1/2/2006",
		"01/02/2006",
		"1/2/06",
		"01/02/06",

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

func findHeaderRow(rows [][]string) int {
	for i := 10; i < len(rows) && i < 25; i++ {
		for _, cell := range rows[i] {
			c := strings.TrimSpace(cell)
			if strings.EqualFold(c, "Date") {
				return i
			}
		}
	}
	return -1
}

func findDataStartRow(rows [][]string, headerRowIdx int) int {
	for i := headerRowIdx + 1; i < len(rows); i++ {
		if len(rows[i]) > 0 {
			first := strings.TrimSpace(rows[i][0])
			if first != "" && !strings.EqualFold(first, "Start") && !strings.EqualFold(first, "Finish") {
				return i
			}
		}
	}
	return -1
}

func isEmptyDataRow(row []string) bool {
	if len(row) == 0 {
		return true
	}
	if strings.TrimSpace(row[0]) == "" {
		return true
	}
	return false
}

func handlePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(PreviewResponse{Error: "Method not allowed"})
		return
	}

	res, err := processUploadedFiles(r)
	if err != nil {
		json.NewEncoder(w).Encode(PreviewResponse{Error: err.Error()})
		return
	}

	resp := PreviewResponse{
		Success:    true,
		TotalFiles: res.TotalFiles,
		Sheets:     res.SheetNames,
		Headers:    res.Headers,
		TotalRows:  len(res.Rows),
		SheetStats: res.SheetStats,
	}

	previewLimit := 200
	if len(res.Rows) <= previewLimit {
		resp.Rows = res.Rows
	} else {
		resp.Rows = res.Rows[:previewLimit]
	}

	json.NewEncoder(w).Encode(resp)
}

func handleMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	res, err := processUploadedFiles(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	out := excelize.NewFile()
	outSheet := "Data Lembur Gabungan"
	idx, _ := out.NewSheet(outSheet)
	out.SetActiveSheet(idx)
	out.DeleteSheet("Sheet1")

	headerStyle, _ := out.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#DB2777"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#831843", Style: 1},
			{Type: "top", Color: "#831843", Style: 1},
			{Type: "right", Color: "#831843", Style: 1},
			{Type: "bottom", Color: "#831843", Style: 1},
		},
	})

	evenRowStyle, _ := out.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#FCE7F3"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "#FBCFE8", Style: 1},
			{Type: "top", Color: "#FBCFE8", Style: 1},
			{Type: "right", Color: "#FBCFE8", Style: 1},
			{Type: "bottom", Color: "#FBCFE8", Style: 1},
		},
	})

	oddRowStyle, _ := out.NewStyle(&excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Color: "#FBCFE8", Style: 1},
			{Type: "top", Color: "#FBCFE8", Style: 1},
			{Type: "right", Color: "#FBCFE8", Style: 1},
			{Type: "bottom", Color: "#FBCFE8", Style: 1},
		},
	})

	dateFmt := "dd/mm/yyyy"

	evenDateStyle, _ := out.NewStyle(&excelize.Style{
		CustomNumFmt: &dateFmt,
		Fill:         excelize.Fill{Type: "pattern", Color: []string{"#FCE7F3"}, Pattern: 1},
		Alignment:    &excelize.Alignment{Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#FBCFE8", Style: 1},
			{Type: "top", Color: "#FBCFE8", Style: 1},
			{Type: "right", Color: "#FBCFE8", Style: 1},
			{Type: "bottom", Color: "#FBCFE8", Style: 1},
		},
	})

	oddDateStyle, _ := out.NewStyle(&excelize.Style{
		CustomNumFmt: &dateFmt,
		Alignment:    &excelize.Alignment{Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#FBCFE8", Style: 1},
			{Type: "top", Color: "#FBCFE8", Style: 1},
			{Type: "right", Color: "#FBCFE8", Style: 1},
			{Type: "bottom", Color: "#FBCFE8", Style: 1},
		},
	})

	// Write headers
	outRow := 1
	for colIdx, h := range res.Headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, outRow)
		out.SetCellValue(outSheet, cell, h)
		out.SetCellStyle(outSheet, cell, cell, headerStyle)
	}
	outRow++

	for _, dataRow := range res.Rows {
		for colIdx, val := range dataRow {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, outRow)
			if colIdx == 4 { // Date column
				if t, ok := parseDateString(val); ok {
					out.SetCellValue(outSheet, cell, t)
				} else {
					out.SetCellValue(outSheet, cell, val)
				}
			} else {
				out.SetCellValue(outSheet, cell, val)
			}
		}

		for colIdx := range res.Headers {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, outRow)
			var style int
			if colIdx == 4 { // Date column
				style = oddDateStyle
				if outRow%2 == 0 {
					style = evenDateStyle
				}
			} else {
				style = oddRowStyle
				if outRow%2 == 0 {
					style = evenRowStyle
				}
			}
			out.SetCellStyle(outSheet, cell, cell, style)
		}
		outRow++
	}

	// Auto-fit columns
	for colIdx, h := range res.Headers {
		maxLen := len(h)
		for r := 2; r <= outRow; r++ {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, r)
			val, _ := out.GetCellValue(outSheet, cell)
			if len(val) > maxLen {
				maxLen = len(val)
			}
		}
		colName, _ := excelize.ColumnNumberToName(colIdx + 1)
		width := float64(maxLen) + 4
		if width < 12 {
			width = 12
		}
		if width > 50 {
			width = 50
		}
		out.SetColWidth(outSheet, colName, colName, width)
	}

	// Freeze header
	out.SetPanes(outSheet, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 1,
		TopLeftCell: "A2", ActivePane: "bottomLeft",
	})

	// Auto filter
	if len(res.Headers) > 0 {
		lastColName, _ := excelize.ColumnNumberToName(len(res.Headers))
		lastCell := fmt.Sprintf("%s%d", lastColName, outRow-1)
		out.AutoFilter(outSheet, fmt.Sprintf("A1:%s", lastCell), nil)
	}

	tmpOut, err := os.CreateTemp("", "merged-*.xlsx")
	if err != nil {
		http.Error(w, "Gagal membuat file output", 500)
		return
	}
	tmpOutPath := tmpOut.Name()
	tmpOut.Close()
	defer os.Remove(tmpOutPath)

	if err := out.SaveAs(tmpOutPath); err != nil {
		http.Error(w, "Gagal menyimpan file output", 500)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=hasil_gabungan_lembur.xlsx")
	http.ServeFile(w, r, tmpOutPath)
}

const loginHTML = `<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Login — Asisten Payrol</title>
<link rel="icon" type="image/png" href="/logo.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
body{
  font-family:'Inter',system-ui,-apple-system,sans-serif;
  background:#0a0e1a;
  color:#f1f5f9;
  min-height:100vh;
  display:flex;
  align-items:center;
  justify-content:center;
  overflow:hidden;
}

/* Animated bg */
.bg-pattern{
  position:fixed;inset:0;z-index:0;pointer-events:none;
  background:
    radial-gradient(ellipse 600px 400px at 30% 20%,rgba(236,72,153,.15),transparent),
    radial-gradient(ellipse 500px 350px at 70% 80%,rgba(244,63,94,.12),transparent);
}
.bg-grid{
  position:fixed;inset:0;z-index:0;pointer-events:none;
  background-image:
    linear-gradient(rgba(236,72,153,.03) 1px,transparent 1px),
    linear-gradient(90deg,rgba(236,72,153,.03) 1px,transparent 1px);
  background-size:60px 60px;
}
.orb{
  position:fixed;border-radius:50%;filter:blur(100px);pointer-events:none;z-index:0;
  animation:orbFloat 18s ease-in-out infinite;
}
.orb-1{width:350px;height:350px;background:rgba(236,72,153,.18);top:5%;left:10%;animation-delay:0s}
.orb-2{width:280px;height:280px;background:rgba(244,63,94,.14);bottom:10%;right:5%;animation-delay:-6s}
@keyframes orbFloat{
  0%,100%{transform:translate(0,0) scale(1)}
  33%{transform:translate(40px,-50px) scale(1.1)}
  66%{transform:translate(-30px,40px) scale(.9)}
}

.login-wrapper{
  position:relative;z-index:1;
  width:100%;max-width:420px;
  padding:20px;
}

.login-card{
  background:rgba(26,32,53,.7);
  backdrop-filter:blur(24px);
  border:1px solid rgba(236,72,153,.25);
  border-radius:24px;
  padding:48px 40px 40px;
  box-shadow:0 20px 60px rgba(0,0,0,.4),0 0 80px rgba(236,72,153,.12);
  animation:cardIn .6s cubic-bezier(.16,1,.3,1) both;
}
@keyframes cardIn{
  from{opacity:0;transform:translateY(30px) scale(.96)}
}

.login-header{text-align:center;margin-bottom:36px}
.login-logo{
  width:84px;height:84px;border-radius:24px;
  display:inline-flex;align-items:center;justify-content:center;
  box-shadow:0 8px 32px rgba(236,72,153,.4);
  margin-bottom:20px;overflow:hidden;
  animation:logoPulse 3s ease-in-out infinite;
}
.login-logo img{width:100%;height:100%;object-fit:cover}
@keyframes logoPulse{
  0%,100%{box-shadow:0 8px 32px rgba(236,72,153,.35)}
  50%{box-shadow:0 8px 48px rgba(236,72,153,.55)}
}
.login-title{
  font-size:24px;font-weight:800;
  background:linear-gradient(135deg,#ec4899,#f43f5e,#fb7185);
  -webkit-background-clip:text;-webkit-text-fill-color:transparent;
  background-clip:text;
  margin-bottom:8px;
}
.login-subtitle{font-size:14px;color:#94a3b8;font-weight:400}

.form-group{margin-bottom:20px;position:relative}
.form-label{
  display:block;font-size:12px;font-weight:600;
  color:#94a3b8;margin-bottom:8px;
  text-transform:uppercase;letter-spacing:.8px;
}
.input-wrapper{
  position:relative;
  display:flex;align-items:center;
}
.input-icon{
  position:absolute;left:16px;font-size:18px;
  color:#64748b;pointer-events:none;
  transition:color .3s;
}
.form-input{
  width:100%;
  padding:14px 16px 14px 48px;
  background:rgba(17,24,39,.6);
  border:1.5px solid #2a3450;
  border-radius:12px;
  color:#f1f5f9;
  font-size:15px;font-family:inherit;font-weight:500;
  outline:none;
  transition:all .3s;
}
.form-input::placeholder{color:#4a5568}
.form-input:focus{
  border-color:#ec4899;
  box-shadow:0 0 0 3px rgba(236,72,153,.2),0 0 20px rgba(236,72,153,.15);
  background:rgba(17,24,39,.8);
}
.form-input:focus ~ .input-icon,.form-input:focus + .input-icon{color:#f472b6}
.input-wrapper:focus-within .input-icon{color:#f472b6}

.toggle-password{
  position:absolute;right:14px;
  background:none;border:none;color:#64748b;
  font-size:18px;cursor:pointer;
  padding:4px;transition:color .2s;
}
.toggle-password:hover{color:#94a3b8}

.login-btn{
  width:100%;
  padding:15px;
  background:linear-gradient(135deg,#ec4899,#db2777);
  border:none;border-radius:12px;
  color:#fff;
  font-size:15px;font-weight:700;font-family:inherit;
  cursor:pointer;
  transition:all .25s;
  box-shadow:0 4px 24px rgba(236,72,153,.35);
  position:relative;overflow:hidden;
  margin-top:8px;
}
.login-btn::before{
  content:'';position:absolute;inset:0;
  background:linear-gradient(135deg,rgba(255,255,255,.15),transparent);
  opacity:0;transition:opacity .25s;
}
.login-btn:hover{transform:translateY(-2px);box-shadow:0 8px 32px rgba(236,72,153,.5)}
.login-btn:hover::before{opacity:1}
.login-btn:active{transform:translateY(0)}
.login-btn:disabled{
  opacity:.6;cursor:not-allowed;
  transform:none!important;
}
.login-btn .btn-spinner{
  display:inline-block;width:18px;height:18px;
  border:2px solid rgba(255,255,255,.3);
  border-top-color:#fff;
  border-radius:50%;
  animation:spin .7s linear infinite;
  vertical-align:middle;
  margin-right:8px;
}
@keyframes spin{to{transform:rotate(360deg)}}

.error-msg{
  display:none;
  align-items:center;gap:8px;
  padding:12px 16px;
  background:rgba(239,68,68,.08);
  border:1px solid rgba(239,68,68,.2);
  border-radius:10px;
  font-size:13px;color:#f87171;
  margin-bottom:20px;
  animation:shakeIn .4s ease;
}
.error-msg.visible{display:flex}
@keyframes shakeIn{
  0%{transform:translateX(0)}
  20%{transform:translateX(-8px)}
  40%{transform:translateX(8px)}
  60%{transform:translateX(-4px)}
  80%{transform:translateX(4px)}
  100%{transform:translateX(0)}
}

.login-footer{
  text-align:center;margin-top:28px;
  font-size:12px;color:#4a5568;
}

@media(max-width:480px){
  .login-card{padding:36px 24px 32px;border-radius:20px}
  .login-title{font-size:20px}
}
</style>
</head>
<body>

<div class="bg-pattern"></div>
<div class="bg-grid"></div>
<div class="orb orb-1"></div>
<div class="orb orb-2"></div>

<div class="login-wrapper">
  <div class="login-card">
    <div class="login-header">
      <div class="login-logo"><img src="/logo.png" alt="Asisten Payroll Logo"></div>
      <div class="login-title">Asisten Payrol</div>
      <div class="login-subtitle">Silakan login untuk melanjutkan</div>
    </div>

    <div class="error-msg" id="errorMsg">
      <span>⚠️</span>
      <span id="errorText"></span>
    </div>

    <form id="loginForm" action="/login" method="POST" autocomplete="off">
      <div class="form-group">
        <label class="form-label" for="username">Username</label>
        <div class="input-wrapper">
          <span class="input-icon">👤</span>
          <input class="form-input" type="text" id="username" name="username" placeholder="Masukkan username" autocomplete="username" required autofocus>
        </div>
      </div>

      <div class="form-group">
        <label class="form-label" for="password">Password</label>
        <div class="input-wrapper">
          <span class="input-icon">🔒</span>
          <input class="form-input" type="password" id="password" name="password" placeholder="Masukkan password" autocomplete="current-password" required>
          <button type="button" class="toggle-password" id="togglePw" tabindex="-1">👁️</button>
        </div>
      </div>

      <button type="submit" class="login-btn" id="loginBtn">Masuk</button>
    </form>

    <div class="login-footer">
      🔐 Akses terbatas — hanya untuk pengguna terdaftar
    </div>
  </div>
</div>

<script>
const form = document.getElementById('loginForm');
const loginBtn = document.getElementById('loginBtn');
const errorMsg = document.getElementById('errorMsg');
const errorText = document.getElementById('errorText');
const togglePw = document.getElementById('togglePw');
const pwInput = document.getElementById('password');

let isSubmitting = false;

togglePw.addEventListener('click', () => {
  const isPassword = pwInput.type === 'password';
  pwInput.type = isPassword ? 'text' : 'password';
  togglePw.textContent = isPassword ? '🙈' : '👁️';
});

form.addEventListener('submit', async (e) => {
  if (isSubmitting) {
    e.preventDefault();
    return;
  }

  const username = document.getElementById('username').value.trim();
  const password = document.getElementById('password').value;

  if (!username || !password) {
    e.preventDefault();
    showError('Username dan password harus diisi');
    return;
  }

  e.preventDefault();
  isSubmitting = true;
  errorMsg.classList.remove('visible');
  loginBtn.disabled = true;
  loginBtn.innerHTML = '<span class="btn-spinner"></span> Memproses...';

  try {
    const resp = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password })
    });
    const data = await resp.json();

    if (data.success) {
      loginBtn.innerHTML = '✅ Berhasil!';
      loginBtn.style.background = 'linear-gradient(135deg,#10b981,#059669)';
      setTimeout(() => { window.location.href = '/'; }, 500);
    } else {
      showError(data.error || 'Username atau password salah');
      isSubmitting = false;
      loginBtn.disabled = false;
      loginBtn.innerHTML = 'Masuk';
    }
  } catch (err) {
    console.warn('Fetch failed, falling back to standard form submission', err);
    form.submit();
  }
});

function showError(msg) {
  errorText.textContent = msg;
  errorMsg.classList.add('visible');
}
</script>
</body>
</html>
` + "\n"

const indexHTML = `<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Asisten Payrol — Penggabung Data Lembur Excel & PDF</title>
<link rel="icon" type="image/png" href="/logo.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg-primary:#0a0e1a;
  --bg-secondary:#111827;
  --bg-card:#1a2035;
  --bg-card-hover:#1f2847;
  --border:#2a3450;
  --border-glow:rgba(236,72,153,.35);
  --text-primary:#f1f5f9;
  --text-secondary:#94a3b8;
  --text-muted:#64748b;
  --accent:#ec4899;
  --accent-hover:#f472b6;
  --accent-glow:rgba(236,72,153,.25);
  --success:#10b981;
  --success-glow:rgba(16,185,129,.2);
  --warning:#f59e0b;
  --danger:#ef4444;
  --gradient-1:linear-gradient(135deg,#ec4899,#db2777,#f43f5e);
  --gradient-2:linear-gradient(135deg,#f43f5e,#ec4899);
  --glass:rgba(26,32,53,.65);
  --radius:16px;
  --radius-sm:10px;
  --radius-xs:6px;
  --shadow:0 8px 32px rgba(0,0,0,.35);
  --shadow-lg:0 20px 60px rgba(0,0,0,.5);
}
html{scroll-behavior:smooth}
body{
  font-family:'Inter',system-ui,-apple-system,sans-serif;
  background:var(--bg-primary);
  color:var(--text-primary);
  min-height:100vh;
  overflow-x:hidden;
}

/* Animated background */
.bg-pattern{
  position:fixed;inset:0;z-index:0;pointer-events:none;
  background:
    radial-gradient(ellipse 600px 400px at 20% 20%,rgba(236,72,153,.1),transparent),
    radial-gradient(ellipse 500px 350px at 80% 80%,rgba(244,63,94,.08),transparent),
    radial-gradient(ellipse 400px 300px at 50% 50%,rgba(251,113,133,.05),transparent);
}
.bg-grid{
  position:fixed;inset:0;z-index:0;pointer-events:none;
  background-image:
    linear-gradient(rgba(236,72,153,.03) 1px,transparent 1px),
    linear-gradient(90deg,rgba(236,72,153,.03) 1px,transparent 1px);
  background-size:60px 60px;
}

/* Floating orbs */
.orb{
  position:fixed;border-radius:50%;filter:blur(80px);pointer-events:none;z-index:0;
  animation:orbFloat 20s ease-in-out infinite;
}
.orb-1{width:300px;height:300px;background:rgba(236,72,153,.15);top:10%;left:5%;animation-delay:0s}
.orb-2{width:250px;height:250px;background:rgba(244,63,94,.12);bottom:15%;right:10%;animation-delay:-7s}
.orb-3{width:200px;height:200px;background:rgba(251,113,133,.1);top:50%;left:60%;animation-delay:-14s}
@keyframes orbFloat{
  0%,100%{transform:translate(0,0) scale(1)}
  33%{transform:translate(30px,-40px) scale(1.1)}
  66%{transform:translate(-20px,30px) scale(.95)}
}

.container{
  position:relative;z-index:1;
  max-width:960px;margin:0 auto;padding:40px 24px 60px;
}

/* Header */
.header{text-align:center;margin-bottom:48px}
.logo{
  display:inline-flex;align-items:center;gap:14px;
  margin-bottom:16px;
}
.logo-icon{
  width:64px;height:64px;border-radius:18px;
  display:flex;align-items:center;justify-content:center;
  overflow:hidden;
  box-shadow:0 6px 24px var(--accent-glow);
}
.logo-icon img{width:100%;height:100%;object-fit:cover}
.logo-text{
  font-size:28px;font-weight:800;
  background:var(--gradient-1);
  -webkit-background-clip:text;-webkit-text-fill-color:transparent;
  background-clip:text;
  letter-spacing:-.5px;
}
.header p{color:var(--text-secondary);font-size:15px;font-weight:400;line-height:1.5}

/* Card */
.card{
  background:var(--glass);
  backdrop-filter:blur(20px);
  border:1px solid var(--border);
  border-radius:var(--radius);
  padding:32px;
  box-shadow:var(--shadow);
  transition:border-color .3s,box-shadow .3s;
  margin-bottom:24px;
}
.card:hover{border-color:var(--border-glow)}

/* Drop zone */
.drop-zone{
  position:relative;
  border:2px dashed var(--border);
  border-radius:var(--radius);
  padding:56px 32px;
  text-align:center;
  cursor:pointer;
  transition:all .3s ease;
  background:rgba(236,72,153,.02);
  overflow:hidden;
}
.drop-zone::before{
  content:'';position:absolute;inset:0;
  background:radial-gradient(circle at center,rgba(236,72,153,.06),transparent 70%);
  opacity:0;transition:opacity .3s;
}
.drop-zone:hover,.drop-zone.drag-over{
  border-color:var(--accent);
  background:rgba(236,72,153,.06);
  box-shadow:0 0 40px var(--accent-glow);
}
.drop-zone:hover::before,.drop-zone.drag-over::before{opacity:1}
.drop-zone.has-file{
  border-color:var(--success);
  background:rgba(16,185,129,.05);
  border-style:solid;
}
.drop-icon{
  font-size:48px;margin-bottom:16px;
  filter:drop-shadow(0 4px 12px var(--accent-glow));
  transition:transform .3s;
}
.drop-zone:hover .drop-icon{transform:translateY(-4px) scale(1.05)}
.drop-title{font-size:18px;font-weight:600;margin-bottom:8px;color:var(--text-primary)}
.drop-subtitle{font-size:13px;color:var(--text-muted);margin-bottom:20px}
.drop-btn{
  display:inline-flex;align-items:center;gap:8px;
  padding:10px 24px;
  background:var(--accent);color:#fff;
  border:none;border-radius:var(--radius-sm);
  font-size:14px;font-weight:600;font-family:inherit;
  cursor:pointer;transition:all .2s;
  box-shadow:0 4px 16px var(--accent-glow);
}
.drop-btn:hover{background:var(--accent-hover);transform:translateY(-1px)}
input[type=file]{display:none}

/* File list */
.file-list{
  display:none;flex-direction:column;gap:10px;
  margin-top:20px;
}
.file-list.visible{display:flex}
.file-item{
  display:flex;align-items:center;gap:14px;
  padding:12px 18px;
  background:rgba(236,72,153,.08);
  border:1px solid rgba(236,72,153,.25);
  border-radius:var(--radius-sm);
  transition:all .2s;
}
.file-item:hover{border-color:var(--accent);background:rgba(236,72,153,.12)}
.file-item-icon{font-size:24px;flex-shrink:0}
.file-item-details{flex:1;text-align:left;overflow:hidden}
.file-item-name{font-size:13.5px;font-weight:600;color:var(--text-primary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.file-item-size{font-size:11.5px;color:var(--text-secondary)}
.file-item-remove{
  background:none;border:none;color:var(--text-muted);font-size:18px;cursor:pointer;
  width:28px;height:28px;border-radius:50%;
  display:flex;align-items:center;justify-content:center;
  transition:all .2s;flex-shrink:0;
}
.file-item-remove:hover{background:rgba(239,68,68,.15);color:var(--danger)}

/* Stats bar */
.stats-bar{
  display:none;gap:12px;margin-bottom:24px;
  flex-wrap:wrap;
}
.stats-bar.visible{display:flex}
.stat-chip{
  display:flex;align-items:center;gap:8px;
  padding:10px 16px;
  background:var(--bg-card);
  border:1px solid var(--border);
  border-radius:var(--radius-sm);
  font-size:13px;
  flex:1;min-width:140px;
}
.stat-chip-icon{font-size:18px}
.stat-chip-label{color:var(--text-secondary);font-weight:400}
.stat-chip-value{color:var(--text-primary);font-weight:700;margin-left:auto}

/* Sheet pills */
.sheet-list{
  display:none;gap:8px;flex-wrap:wrap;margin-bottom:24px;
}
.sheet-list.visible{display:flex}
.sheet-pill{
  display:inline-flex;align-items:center;gap:6px;
  padding:6px 14px;
  background:rgba(236,72,153,.1);
  border:1px solid rgba(236,72,153,.2);
  border-radius:20px;
  font-size:12px;font-weight:500;
  color:var(--accent-hover);
  animation:pillIn .3s ease both;
}
.sheet-pill .pill-count{
  background:var(--accent);color:#fff;
  font-size:10px;font-weight:700;
  padding:2px 7px;border-radius:10px;
}
@keyframes pillIn{from{opacity:0;transform:scale(.9)}}

/* Preview table */
.table-wrapper{
  display:none;overflow-x:auto;
  border:1px solid var(--border);
  border-radius:var(--radius-sm);
  margin-bottom:24px;
  max-height:420px;overflow-y:auto;
}
.table-wrapper.visible{display:block}
.table-wrapper::-webkit-scrollbar{width:6px;height:6px}
.table-wrapper::-webkit-scrollbar-track{background:var(--bg-secondary)}
.table-wrapper::-webkit-scrollbar-thumb{background:var(--border);border-radius:3px}
.table-wrapper::-webkit-scrollbar-thumb:hover{background:var(--text-muted)}
.preview-table{
  width:100%;border-collapse:collapse;font-size:13px;
}
.preview-table thead{position:sticky;top:0;z-index:2}
.preview-table th{
  background:var(--bg-secondary);
  color:var(--accent-hover);
  font-weight:600;font-size:12px;text-transform:uppercase;letter-spacing:.5px;
  padding:12px 16px;text-align:left;white-space:nowrap;
  border-bottom:2px solid var(--accent);
}
.preview-table td{
  padding:10px 16px;
  border-bottom:1px solid var(--border);
  white-space:nowrap;
  color:var(--text-secondary);
}
.preview-table tbody tr{transition:background .15s}
.preview-table tbody tr:hover{background:rgba(236,72,153,.06)}
.preview-table tbody tr:nth-child(even){background:rgba(236,72,153,.02)}
.preview-table tbody tr:nth-child(even):hover{background:rgba(236,72,153,.08)}
.preview-table td:first-child{
  font-weight:600;color:var(--text-primary);
  position:relative;
}
.preview-table td:first-child::before{
  content:'';position:absolute;left:0;top:25%;height:50%;
  width:3px;border-radius:2px;background:var(--accent);
}
.preview-label{
  font-size:12px;color:var(--text-muted);margin-bottom:10px;
  display:none;align-items:center;gap:6px;
}
.preview-label.visible{display:flex}

/* Buttons */
.actions{display:none;gap:12px;justify-content:center}
.actions.visible{display:flex}
.btn{
  display:inline-flex;align-items:center;gap:10px;
  padding:14px 32px;
  border:none;border-radius:var(--radius-sm);
  font-size:15px;font-weight:600;font-family:inherit;
  cursor:pointer;transition:all .25s;
  position:relative;overflow:hidden;
}
.btn::after{
  content:'';position:absolute;inset:0;
  background:linear-gradient(135deg,rgba(255,255,255,.1),transparent);
  opacity:0;transition:opacity .25s;
}
.btn:hover::after{opacity:1}
.btn-primary{
  background:var(--gradient-1);color:#fff;
  box-shadow:0 4px 24px var(--accent-glow);
}
.btn-primary:hover{transform:translateY(-2px);box-shadow:0 8px 32px rgba(236,72,153,.45)}
.btn-primary:active{transform:translateY(0)}
.btn-secondary{
  background:var(--bg-card);color:var(--text-primary);
  border:1px solid var(--border);
}
.btn-secondary:hover{border-color:var(--accent);background:var(--bg-card-hover)}

/* Loading spinner */
.spinner{
  display:none;flex-direction:column;align-items:center;gap:16px;
  padding:32px;
}
.spinner.visible{display:flex}
.spinner-ring{
  width:44px;height:44px;
  border:3px solid var(--border);
  border-top-color:var(--accent);
  border-radius:50%;
  animation:spin .8s linear infinite;
}
@keyframes spin{to{transform:rotate(360deg)}}
.spinner-text{font-size:14px;color:var(--text-secondary)}

/* Toast */
.toast{
  position:fixed;bottom:32px;right:32px;z-index:100;
  display:flex;align-items:center;gap:12px;
  padding:14px 24px;
  background:var(--bg-card);
  border:1px solid var(--border);
  border-radius:var(--radius-sm);
  box-shadow:var(--shadow-lg);
  font-size:14px;
  transform:translateY(120%);opacity:0;
  transition:all .4s cubic-bezier(.16,1,.3,1);
}
.toast.show{transform:translateY(0);opacity:1}
.toast.toast-success{border-color:rgba(16,185,129,.3)}
.toast.toast-error{border-color:rgba(239,68,68,.3)}
.toast-icon{font-size:20px}

/* Footer */
.footer{
  text-align:center;margin-top:48px;
  font-size:12px;color:var(--text-muted);
}
.footer a{color:var(--accent);text-decoration:none}
.footer a:hover{text-decoration:underline}

/* Responsive */
@media(max-width:640px){
  .container{padding:24px 16px 40px}
  .card{padding:24px 20px}
  .drop-zone{padding:40px 20px}
  .logo-text{font-size:22px}
  .btn{padding:12px 24px;font-size:14px}
  .stat-chip{min-width:120px}
}
</style>
</head>
<body>

<div class="bg-pattern"></div>
<div class="bg-grid"></div>
<div class="orb orb-1"></div>
<div class="orb orb-2"></div>
<div class="orb orb-3"></div>

<div class="container">
  <header class="header">
    <div class="logo">
      <div class="logo-icon"><img src="/logo.png" alt="Asisten Payroll Logo"></div>
      <span class="logo-text">Asisten Payrol</span>
    </div>
    <p>Upload file Excel &amp; PDF lembur karyawan (termasuk formulir tulisan tangan),<br>gabungkan semua data jadi satu tabel rapi secara otomatis dengan Claude AI.</p>
  </header>

  <div style="position:fixed;top:20px;right:24px;z-index:50">
    <a href="/api/logout" id="logoutBtn" style="
      display:inline-flex;align-items:center;gap:8px;
      padding:8px 18px;
      background:rgba(26,32,53,.7);
      backdrop-filter:blur(12px);
      border:1px solid var(--border);
      border-radius:10px;
      color:var(--text-secondary);
      font-size:13px;font-weight:500;
      text-decoration:none;
      transition:all .2s;
      cursor:pointer;
    " onmouseover="this.style.borderColor='var(--danger)';this.style.color='#f87171'" onmouseout="this.style.borderColor='var(--border)';this.style.color='var(--text-secondary)'">
      <span>🚪</span> Logout
    </a>
  </div>

  <div class="card">
    <div class="drop-zone" id="dropZone">
      <div class="drop-icon">📁</div>
      <div class="drop-title">Drag &amp; drop file Excel (.xlsx) atau PDF di sini</div>
      <div class="drop-subtitle">atau klik tombol di bawah untuk memilih file (.xlsx, .pdf) &bull; dukung baca formulir PDF tulisan tangan dengan AI</div>
      <button class="drop-btn" id="browseBtn">
        <span>📎</span> Pilih File Excel / PDF (Multiple)
      </button>
      <input type="file" id="fileInput" accept=".xlsx,.xls,.pdf" multiple>

      <div class="file-list" id="fileList"></div>
    </div>
  </div>

  <div class="stats-bar" id="statsBar">
    <div class="stat-chip">
      <span class="stat-chip-icon">📁</span>
      <span class="stat-chip-label">Total File</span>
      <span class="stat-chip-value" id="statFiles">0</span>
    </div>
    <div class="stat-chip">
      <span class="stat-chip-icon">👥</span>
      <span class="stat-chip-label">Karyawan</span>
      <span class="stat-chip-value" id="statSheets">0</span>
    </div>
    <div class="stat-chip">
      <span class="stat-chip-icon">📋</span>
      <span class="stat-chip-label">Total Baris</span>
      <span class="stat-chip-value" id="statRows">0</span>
    </div>
    <div class="stat-chip">
      <span class="stat-chip-icon">📊</span>
      <span class="stat-chip-label">Kolom</span>
      <span class="stat-chip-value" id="statCols">0</span>
    </div>
  </div>

  <div class="sheet-list" id="sheetList"></div>

  <div class="preview-label" id="previewLabel">
    <span>👁️</span> Preview data gabungan (Excel + PDF)
  </div>
  <div class="table-wrapper" id="tableWrapper">
    <table class="preview-table" id="previewTable">
      <thead id="previewHead"></thead>
      <tbody id="previewBody"></tbody>
    </table>
  </div>

  <div class="spinner" id="spinner">
    <div class="spinner-ring"></div>
    <div class="spinner-text" id="spinnerText">Memproses file Excel &amp; PDF dengan AI...</div>
  </div>

  <div class="actions" id="actions">
    <button class="btn btn-primary" id="downloadBtn">
      <span>⬇️</span> Download Hasil Gabungan
    </button>
    <button class="btn btn-secondary" id="resetBtn">
      <span>🔄</span> Upload Ulang
    </button>
  </div>
</div>

<div class="footer">
  <p>Asisten Payrol &mdash; dibuat dengan ❤️ menggunakan Go + Excelize + Claude AI</p>
</div>

<div class="toast" id="toast">
  <span class="toast-icon" id="toastIcon"></span>
  <span id="toastMsg"></span>
</div>

<script>
const $ = id => document.getElementById(id);
const dropZone = $('dropZone');
const fileInput = $('fileInput');
const fileList = $('fileList');
const browseBtn = $('browseBtn');
const statsBar = $('statsBar');
const sheetList = $('sheetList');
const previewLabel = $('previewLabel');
const tableWrapper = $('tableWrapper');
const previewHead = $('previewHead');
const previewBody = $('previewBody');
const spinner = $('spinner');
const actions = $('actions');
const downloadBtn = $('downloadBtn');
const resetBtn = $('resetBtn');

let selectedFiles = [];

// Drag & drop
['dragenter','dragover'].forEach(ev => {
  dropZone.addEventListener(ev, e => { e.preventDefault(); dropZone.classList.add('drag-over'); });
});
['dragleave','drop'].forEach(ev => {
  dropZone.addEventListener(ev, e => { e.preventDefault(); dropZone.classList.remove('drag-over'); });
});
dropZone.addEventListener('drop', e => {
  const files = Array.from(e.dataTransfer.files).filter(f => f.name.match(/\.(xlsx?|pdf)$/i));
  if (files.length > 0) addFiles(files);
  else showToast('❌', 'Hanya file .xlsx dan .pdf yang didukung', 'error');
});

browseBtn.addEventListener('click', e => { e.stopPropagation(); fileInput.click(); });
dropZone.addEventListener('click', e => {
  if (e.target === browseBtn || browseBtn.contains(e.target) || e.target.closest('.file-item-remove')) return;
  fileInput.click();
});
fileInput.addEventListener('change', () => {
  if (fileInput.files.length > 0) {
    addFiles(Array.from(fileInput.files));
  }
});

resetBtn.addEventListener('click', resetAll);

function formatBytes(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1048576).toFixed(1) + ' MB';
}

function escapeHtml(str) {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function addFiles(files) {
  files.forEach(f => {
    if (!f.name.match(/\.(xlsx?|pdf)$/i)) return;
    if (!selectedFiles.some(sf => sf.name === f.name && sf.size === f.size)) {
      selectedFiles.push(f);
    }
  });
  renderFileList();
  if (selectedFiles.length > 0) {
    previewFiles();
  }
}

function removeFile(index) {
  selectedFiles.splice(index, 1);
  renderFileList();
  if (selectedFiles.length > 0) {
    previewFiles();
  } else {
    resetAll();
  }
}

function renderFileList() {
  fileList.innerHTML = '';
  if (selectedFiles.length === 0) {
    fileList.classList.remove('visible');
    dropZone.classList.remove('has-file');
    return;
  }
  dropZone.classList.add('has-file');
  selectedFiles.forEach((file, idx) => {
    const item = document.createElement('div');
    item.className = 'file-item';
    const isPDF = file.name.match(/\.pdf$/i);
    const icon = isPDF ? '📄' : '📊';
    item.innerHTML = '<span class="file-item-icon">' + icon + '</span>' +
      '<div class="file-item-details">' +
        '<div class="file-item-name">' + escapeHtml(file.name) + (isPDF ? ' <span style="font-size:10px;background:#ec4899;color:#fff;padding:1px 6px;border-radius:6px;margin-left:4px">PDF AI</span>' : '') + '</div>' +
        '<div class="file-item-size">' + formatBytes(file.size) + '</div>' +
      '</div>' +
      '<button class="file-item-remove" title="Hapus file" onclick="event.stopPropagation(); removeFile(' + idx + ')">✕</button>';
    fileList.appendChild(item);
  });
  fileList.classList.add('visible');
}

async function previewFiles() {
  if (selectedFiles.length === 0) return;
  
  const hasPDF = selectedFiles.some(f => f.name.match(/\.pdf$/i));
  $('spinnerText').textContent = hasPDF ? 'Membaca formulir PDF dengan Claude AI & Excel...' : 'Memproses file Excel...';
  
  spinner.classList.add('visible');
  actions.classList.remove('visible');
  statsBar.classList.remove('visible');
  sheetList.classList.remove('visible');
  previewLabel.classList.remove('visible');
  tableWrapper.classList.remove('visible');

  const form = new FormData();
  selectedFiles.forEach(f => {
    form.append('files', f);
  });

  try {
    const resp = await fetch('/api/preview', { method: 'POST', body: form });
    const data = await resp.json();

    spinner.classList.remove('visible');

    if (data.error) {
      showToast('❌', data.error, 'error');
      return;
    }

    $('statFiles').textContent = data.totalFiles || selectedFiles.length;
    $('statSheets').textContent = data.sheets ? data.sheets.length : 0;
    $('statRows').textContent = data.totalRows || 0;
    $('statCols').textContent = data.headers ? data.headers.length : 0;
    statsBar.classList.add('visible');

    sheetList.innerHTML = '';
    if (data.sheetStats) {
      data.sheetStats.forEach((s, i) => {
        const pill = document.createElement('span');
        pill.className = 'sheet-pill';
        pill.style.animationDelay = (i * 40) + 'ms';
        const isPDFSheet = s.name.includes('(PDF)');
        const icon = isPDFSheet ? '📄' : '👤';
        pill.innerHTML = icon + ' ' + escapeHtml(s.name) + ' <span class="pill-count">' + s.count + '</span>';
        sheetList.appendChild(pill);
      });
      sheetList.classList.add('visible');
    }

    previewHead.innerHTML = '';
    previewBody.innerHTML = '';
    if (data.headers && data.headers.length > 0) {
      const tr = document.createElement('tr');
      data.headers.forEach(h => {
        const th = document.createElement('th');
        th.textContent = h;
        tr.appendChild(th);
      });
      previewHead.appendChild(tr);

      if (data.rows) {
        data.rows.forEach(row => {
          const tr = document.createElement('tr');
          data.headers.forEach((_, ci) => {
            const td = document.createElement('td');
            td.textContent = row[ci] || '';
            tr.appendChild(td);
          });
          previewBody.appendChild(tr);
        });
      }

      previewLabel.classList.add('visible');
      tableWrapper.classList.add('visible');
    }

    actions.classList.add('visible');
    showToast('✅', selectedFiles.length + ' file berhasil diproses! Siap untuk download.', 'success');
  } catch (err) {
    spinner.classList.remove('visible');
    showToast('❌', 'Terjadi kesalahan: ' + err.message, 'error');
  }
}

downloadBtn.addEventListener('click', async () => {
  if (selectedFiles.length === 0) return;

  downloadBtn.innerHTML = '<span class="spinner-ring" style="width:18px;height:18px;border-width:2px;display:inline-block"></span> Mengunduh...';
  downloadBtn.disabled = true;

  const form = new FormData();
  selectedFiles.forEach(f => {
    form.append('files', f);
  });

  try {
    const resp = await fetch('/api/merge', { method: 'POST', body: form });
    if (!resp.ok) throw new Error('Gagal generate file');

    const blob = await resp.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'hasil_gabungan_lembur.xlsx';
    a.click();
    URL.revokeObjectURL(url);

    showToast('🎉', 'File berhasil diunduh!', 'success');
  } catch (err) {
    showToast('❌', err.message, 'error');
  } finally {
    downloadBtn.innerHTML = '<span>⬇️</span> Download Hasil Gabungan';
    downloadBtn.disabled = false;
  }
});

function resetAll() {
  selectedFiles = [];
  fileInput.value = '';
  renderFileList();
  statsBar.classList.remove('visible');
  sheetList.classList.remove('visible');
  previewLabel.classList.remove('visible');
  tableWrapper.classList.remove('visible');
  actions.classList.remove('visible');
  spinner.classList.remove('visible');
  previewHead.innerHTML = '';
  previewBody.innerHTML = '';
  sheetList.innerHTML = '';
}

let toastTimer;
function showToast(icon, msg, type) {
  const toast = $('toast');
  $('toastIcon').textContent = icon;
  $('toastMsg').textContent = msg;
  toast.className = 'toast toast-' + type + ' show';
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { toast.classList.remove('show'); }, 4000);
}
</script>
</body>
</html>
`
