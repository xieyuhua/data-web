package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"biaoge-bi/internal/query"
)

// Sheet is one exportable result set.
type Sheet struct {
	Name   string
	Result *query.ResultSet
}

// WriteWorkbook renders sheets into an xlsx byte buffer.
func WriteWorkbook(sheets []Sheet) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	defaultSheet := "Sheet1"
	first := true
	for _, sheet := range sheets {
		name := sanitizeSheetName(sheet.Name)
		if name == "" {
			name = "数据"
		}
		name = uniqueName(f, name)
		if first && defaultSheet != name {
			if err := f.SetSheetName(defaultSheet, name); err != nil {
				return nil, err
			}
			first = false
		} else {
			if _, err := f.NewSheet(name); err != nil {
				return nil, err
			}
		}
		if err := writeSheet(f, name, sheet.Result); err != nil {
			return nil, err
		}
	}
	if first {
		// No sheets provided; leave the empty default sheet.
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeSheet(f *excelize.File, name string, rs *query.ResultSet) error {
	if rs == nil || len(rs.Columns) == 0 {
		return nil
	}
	// Header
	for i, c := range rs.Columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(name, cell, c.Name); err != nil {
			return err
		}
	}
	// Bold header
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err == nil {
		end, _ := excelize.CoordinatesToCellName(len(rs.Columns), 1)
		_ = f.SetCellStyle(name, "A1", end, style)
	}
	for r, row := range rs.Rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if v == nil {
				continue
			}
			if err := f.SetCellValue(name, cell, v); err != nil {
				return err
			}
		}
	}
	// Reasonable widths
	for i, c := range rs.Columns {
		width := float64(len(c.Name)) + 4
		if width < 12 {
			width = 12
		}
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(name, col, col, width)
	}
	return nil
}

func sanitizeSheetName(name string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", "?", "-", "*", "-", "[", "(", "]", ")", ":", "-")
	name = replacer.Replace(strings.TrimSpace(name))
	runes := []rune(name)
	if len(runes) > 28 {
		name = string(runes[:28])
	}
	return name
}

func uniqueName(f *excelize.File, base string) string {
	existing := map[string]bool{}
	for _, s := range f.GetSheetList() {
		existing[s] = true
	}
	if !existing[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

// WriteCSV renders a single result set as UTF-8 CSV with a BOM so Excel opens
// Chinese text correctly.
func WriteCSV(rs *query.ResultSet) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	header := make([]string, len(rs.Columns))
	for i, c := range rs.Columns {
		header[i] = c.Name
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	record := make([]string, len(rs.Columns))
	for _, row := range rs.Rows {
		for i := range rs.Columns {
			if i < len(row) {
				record[i] = csvValue(row[i])
			} else {
				record[i] = ""
			}
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func csvValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return strings.Trim(string(b), `"`)
	}
}

// SheetsFromDashboard builds sheets from chart results in widget order.
type DashboardSheetInput struct {
	Title  string
	Result *query.ResultSet
}

func SheetsFromDashboard(inputs []DashboardSheetInput) []Sheet {
	sheets := make([]Sheet, 0, len(inputs))
	for i, in := range inputs {
		name := in.Title
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("图表%d", i+1)
		}
		sheets = append(sheets, Sheet{Name: name, Result: in.Result})
	}
	return sheets
}
