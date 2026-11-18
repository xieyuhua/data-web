package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Cell is one row of a template/generic sheet.
type Cell = []any

// WriteSheet renders a single-sheet workbook with a bold header row.
func WriteSheet(sheetName string, header []string, rows []Cell) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	name := sanitizeSheetName(sheetName)
	if name == "" {
		name = "Sheet1"
	}
	if err := f.SetSheetName("Sheet1", name); err != nil {
		return nil, err
	}
	for i, h := range header {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(name, cell, h); err != nil {
			return nil, err
		}
	}
	if style, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"F2F3F5"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	}); err == nil {
		end, _ := excelize.CoordinatesToCellName(len(header), 1)
		_ = f.SetCellStyle(name, "A1", end, style)
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if v == nil {
				continue
			}
			if err := f.SetCellValue(name, cell, v); err != nil {
				return nil, err
			}
		}
	}
	// Readable widths based on header length (content may be long, cap it).
	for i, h := range header {
		width := float64(len([]rune(h))*2 + 6)
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(name, col, col, width)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteMultiSheet renders several sheets (used for import error reports).
type SheetSpec struct {
	Name   string
	Header []string
	Rows   []Cell
}

func WriteMultiSheet(sheets []SheetSpec) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if len(sheets) == 0 {
		sheets = []SheetSpec{{Name: "Sheet1", Header: []string{"说明"}, Rows: []Cell{{"无数据"}}}}
	}
	for idx, spec := range sheets {
		name := sanitizeSheetName(spec.Name)
		if name == "" {
			name = fmt.Sprintf("Sheet%d", idx+1)
		}
		name = uniqueName(f, name)
		if idx == 0 {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				return nil, err
			}
		} else if _, err := f.NewSheet(name); err != nil {
			return nil, err
		}
		for i, h := range spec.Header {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			_ = f.SetCellValue(name, cell, h)
		}
		if style, err := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"F2F3F5"}, Pattern: 1},
		}); err == nil {
			end, _ := excelize.CoordinatesToCellName(len(spec.Header), 1)
			_ = f.SetCellStyle(name, "A1", end, style)
		}
		for r, row := range spec.Rows {
			for c, v := range row {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
				if v != nil {
					_ = f.SetCellValue(name, cell, v)
				}
			}
		}
		for i, h := range spec.Header {
			col, _ := excelize.ColumnNumberToName(i + 1)
			_ = f.SetColWidth(name, col, col, float64(len([]rune(h))*2+6))
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ParseWorkbook reads the first sheet into rows of strings (header included).
func ParseWorkbook(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("无法解析 Excel 文件: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel 文件没有工作表")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	// Trim trailing empty columns so headers map cleanly.
	for i, row := range rows {
		last := -1
		for j, v := range row {
			if strings.TrimSpace(v) != "" {
				last = j
			}
		}
		if last >= 0 && last+1 < len(row) {
			rows[i] = row[:last+1]
		}
	}
	return rows, nil
}
