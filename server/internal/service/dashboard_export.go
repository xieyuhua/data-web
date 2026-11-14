package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// BuildDashboardExcel 生成整份看板的 Excel：每个数据组件一个工作表。
// widgetID 非空时只导出指定组件。
func BuildDashboardExcel(dashboardName string, data *DashboardData, widgetID string) ([]byte, error) {
	if data == nil {
		return nil, errors.New("没有可导出的看板数据")
	}
	file := excelize.NewFile()
	defer file.Close()

	used := make(map[string]bool, len(data.Widgets))
	created := 0
	first := true
	for index, widget := range data.Widgets {
		if widgetID != "" && widget.WidgetID != widgetID {
			continue
		}
		title := strings.TrimSpace(widget.Title)
		if title == "" {
			title = fmt.Sprintf("组件%d", index+1)
		}
		sheet := uniqueSheetName(title, used)
		if first {
			_ = file.SetSheetName("Sheet1", sheet)
			first = false
		} else if _, err := file.NewSheet(sheet); err != nil {
			return nil, err
		}
		if err := writeWidgetSheet(file, sheet, data, widget); err != nil {
			return nil, err
		}
		created++
	}
	if created == 0 {
		if widgetID != "" {
			return nil, errors.New("组件不存在或没有可导出的数据")
		}
		sheet := "看板概览"
		_ = file.SetSheetName("Sheet1", sheet)
		_ = file.SetCellValue(sheet, "A1", dashboardName)
		_ = file.SetCellValue(sheet, "A2", fmt.Sprintf("统计周期：%s 至 %s", data.StartDate, data.EndDate))
		_ = file.SetCellValue(sheet, "A3", "该看板暂无数据组件")
		_ = file.SetColWidth(sheet, "A", "A", 40)
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeWidgetSheet(file *excelize.File, sheet string, data *DashboardData, widget WidgetQueryResult) error {
	title := strings.TrimSpace(widget.Title)
	if title == "" {
		title = "数据组件"
	}
	headers := []string{}
	if widget.Data != nil {
		headers = widget.Data.Columns
	}
	colCount := len(headers)
	if colCount < 1 {
		colCount = 1
	}
	endColumn, _ := excelize.ColumnNumberToName(colCount)
	if colCount > 1 {
		_ = file.MergeCell(sheet, "A1", endColumn+"1")
	}
	_ = file.SetCellValue(sheet, "A1", title)
	titleStyle, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Color: "173A33"},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	_ = file.SetCellStyle(sheet, "A1", endColumn+"1", titleStyle)
	_ = file.SetRowHeight(sheet, 1, 26)

	summary := fmt.Sprintf("统计周期：%s 至 %s", data.StartDate, data.EndDate)
	if widget.Data != nil {
		summary += fmt.Sprintf(" · %d 行", widget.Data.Count)
	}
	_ = file.SetCellValue(sheet, "A2", summary)
	summaryStyle, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "6B7F76", Size: 10},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	_ = file.SetCellStyle(sheet, "A2", endColumn+"2", summaryStyle)

	if widget.Error != "" {
		_ = file.SetCellValue(sheet, "A3", "组件查询失败")
		_ = file.SetCellValue(sheet, "A4", excelSafeText(widget.Error))
		_ = file.SetColWidth(sheet, "A", "A", 60)
		return nil
	}

	if len(headers) == 0 {
		content := ""
		if widget.Config != nil {
			if value, ok := widget.Config["content"]; ok {
				content = fmt.Sprint(value)
			}
		}
		if strings.TrimSpace(content) == "" {
			content = "该组件没有可导出的明细数据"
		}
		_ = file.SetCellValue(sheet, "A3", "内容")
		_ = file.SetCellValue(sheet, "A4", excelSafeText(content))
		_ = file.SetColWidth(sheet, "A", "A", 60)
		return nil
	}

	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = file.SetCellValue(sheet, cell, excelSafeText(header))
	}
	headerStyle, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"173A33"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left"},
	})
	_ = file.SetCellStyle(sheet, "A3", endColumn+"3", headerStyle)
	_ = file.SetRowHeight(sheet, 3, 22)

	for rowIndex, row := range widget.Data.Rows {
		for colIndex, value := range row {
			if colIndex >= len(headers) {
				break
			}
			cell, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+4)
			_ = file.SetCellValue(sheet, cell, excelSafeValue(value))
		}
	}
	if len(widget.Data.Rows) == 0 {
		_ = file.SetCellValue(sheet, "A4", "暂无数据")
	}
	_ = file.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 3, TopLeftCell: "A4", ActivePane: "bottomLeft",
	})
	setAutoWidths(file, sheet, headers, widget.Data.Rows)
	return nil
}

// BuildDashboardWidgetCSV 导出单个组件为 CSV（带 BOM，便于 Excel 直接打开）。
// 返回 CSV 内容、组件标题和错误。
func BuildDashboardWidgetCSV(data *DashboardData, widgetID string) ([]byte, string, error) {
	if data == nil {
		return nil, "", errors.New("没有可导出的看板数据")
	}
	var target *WidgetQueryResult
	for i := range data.Widgets {
		widget := &data.Widgets[i]
		if widgetID != "" {
			if widget.WidgetID == widgetID {
				target = widget
				break
			}
			continue
		}
		if widget.Data != nil && len(widget.Data.Columns) > 0 {
			target = widget
			break
		}
	}
	if target == nil {
		if widgetID != "" {
			return nil, "", errors.New("组件不存在")
		}
		return nil, "", errors.New("请指定要导出的组件")
	}
	if target.Data == nil {
		if target.Error != "" {
			return nil, "", errors.New(target.Error)
		}
		return nil, "", errors.New("该组件没有可导出的明细数据")
	}

	var buffer bytes.Buffer
	buffer.WriteString("\ufeff")
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(target.Data.Columns); err != nil {
		return nil, "", err
	}
	for _, row := range target.Data.Rows {
		record := make([]string, len(target.Data.Columns))
		for index := range record {
			if index < len(row) {
				record[index] = csvSafeText(row[index])
			}
		}
		if err := writer.Write(record); err != nil {
			return nil, "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), strings.TrimSpace(target.Title), nil
}

// excelSafeValue 保留数字类型，文本则转义公式前缀，避免 Excel 公式注入。
func excelSafeValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return excelSafeText(typed)
	default:
		return value
	}
}

func excelSafeText(value string) string {
	if value == "" {
		return value
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}
	switch value[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

func csvSafeText(value any) string {
	if value == nil {
		return ""
	}
	text := numericText(value)
	if text == "" {
		return text
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return text
	}
	switch text[0] {
	case '=', '+', '@', '\t', '\r', '-':
		return "'" + text
	}
	return text
}

// numericText 避免大数在 CSV 中被格式化为科学计数法。
func numericText(value any) string {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.FormatInt(int64(typed), 10)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	default:
		return fmt.Sprint(value)
	}
}

func uniqueSheetName(title string, used map[string]bool) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '[', ']', ':', '*', '?', '/', '\\':
			return '_'
		}
		if r < 32 {
			return '_'
		}
		return r
	}, title)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		cleaned = "组件"
	}
	cleaned = truncateRunes(cleaned, 31)
	base := cleaned
	for suffix := 2; used[cleaned]; suffix++ {
		tail := fmt.Sprintf("(%d)", suffix)
		cleaned = truncateRunes(base, 31-len(tail)) + tail
	}
	used[cleaned] = true
	return cleaned
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func setAutoWidths(file *excelize.File, sheet string, headers []string, rows [][]any) {
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = displayWidth(header)
	}
	limit := len(rows)
	if limit > 60 {
		limit = 60
	}
	for _, row := range rows[:limit] {
		for index := range headers {
			if index >= len(row) {
				continue
			}
			width := displayWidth(numericText(row[index]))
			if width > widths[index] {
				widths[index] = width
			}
		}
	}
	for index, width := range widths {
		if width < 10 {
			width = 10
		}
		if width > 42 {
			width = 42
		}
		column, _ := excelize.ColumnNumberToName(index + 1)
		_ = file.SetColWidth(sheet, column, column, float64(width)+3)
	}
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		if r > 127 {
			width += 2
		} else {
			width++
		}
	}
	return width
}
