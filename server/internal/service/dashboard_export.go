package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// ExportMeta 导出说明信息。
// 导出文件会自带一页「导出说明」，记录统计周期、数据范围、筛选条件与导出人，
// 方便财务对账、审计追溯，避免同名表不同口径。
type ExportMeta struct {
	Operator   string    // 导出人（姓名 / 账号）
	Scope      string    // 数据权限范围描述（角色 + 可见组织数量）
	Filters    string    // 生效的全局筛选条件描述
	ExportedAt time.Time // 导出时间
}

// BuildDashboardExcel 生成整份看板的 Excel：首页为「导出说明」，其后每个数据组件一个工作表。
// widgetID 非空时只导出指定组件（此时仍保留导出说明页）。
func BuildDashboardExcel(dashboardName string, data *DashboardData, widgetID string, meta ExportMeta) ([]byte, error) {
	if data == nil {
		return nil, errors.New("没有可导出的看板数据")
	}
	file := excelize.NewFile()
	defer file.Close()

	used := make(map[string]bool, len(data.Widgets))
	created := 0
	first := true
	// firstSheet 记录第一个数据工作表名称，用于把「导出说明」移动到最前面
	firstSheet := ""
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
			firstSheet = sheet
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
		firstSheet = sheet
		_ = file.SetCellValue(sheet, "A1", dashboardName)
		_ = file.SetCellValue(sheet, "A2", fmt.Sprintf("统计周期：%s 至 %s", data.StartDate, data.EndDate))
		_ = file.SetCellValue(sheet, "A3", "该看板暂无数据组件")
		_ = file.SetColWidth(sheet, "A", "A", 40)
	}
	// 生成「导出说明」页并移动到首位；移动失败不影响数据导出
	if metaSheet, err := writeExportMetaSheet(file, dashboardName, data, meta, created); err == nil && firstSheet != "" {
		_ = file.MoveSheet(metaSheet, firstSheet)
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// writeExportMetaSheet 写入「导出说明」工作表：口径、范围、筛选、导出人与时间。
func writeExportMetaSheet(file *excelize.File, dashboardName string, data *DashboardData, meta ExportMeta, widgetCount int) (string, error) {
	const sheet = "导出说明"
	if _, err := file.NewSheet(sheet); err != nil {
		return "", err
	}
	_ = file.SetColWidth(sheet, "A", "A", 14)
	_ = file.SetColWidth(sheet, "B", "B", 64)
	_ = file.MergeCell(sheet, "A1", "B1")
	_ = file.SetCellValue(sheet, "A1", excelSafeText(dashboardName)+" · 导出说明")
	titleStyle, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Color: "173A33"},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	_ = file.SetCellStyle(sheet, "A1", "B1", titleStyle)
	_ = file.SetRowHeight(sheet, 1, 28)

	// 口径信息：空值统一给出可读的默认说明，避免导出后出现空白单元格
	rows := [][2]string{
		{"看板名称", dashboardName},
		{"统计周期", fmt.Sprintf("%s 至 %s", data.StartDate, data.EndDate)},
		{"数据范围", fallbackText(meta.Scope, "按导出账号的角色数据范围")},
		{"全局筛选", fallbackText(meta.Filters, "无（全部数据）")},
		{"导出人", fallbackText(meta.Operator, "—")},
		{"导出时间", meta.ExportedAt.Format("2006-01-02 15:04:05")},
		{"数据组件", fmt.Sprintf("%d 个", widgetCount)},
	}
	labelStyle, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "3F5B53"},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	valueStyle, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "1F2E2A"},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	for index, row := range rows {
		line := index + 3
		labelCell, _ := excelize.CoordinatesToCellName(1, line)
		valueCell, _ := excelize.CoordinatesToCellName(2, line)
		_ = file.SetCellValue(sheet, labelCell, row[0])
		_ = file.SetCellValue(sheet, valueCell, excelSafeText(row[1]))
		_ = file.SetCellStyle(sheet, labelCell, labelCell, labelStyle)
		_ = file.SetCellStyle(sheet, valueCell, valueCell, valueStyle)
		_ = file.SetRowHeight(sheet, line, 20)
	}

	// 口径提示：说明数据来源与快照性质，便于业务方正确理解数字
	noteLine := len(rows) + 5
	_ = file.MergeCell(sheet, fmt.Sprintf("A%d", noteLine), fmt.Sprintf("B%d", noteLine))
	note := "数据说明：以上数据由受控 SQL 数据集按当前筛选条件实时查询生成，单次最多返回 2000 行；" +
		"本文件为导出时刻的数据快照，与看板实时数字可能存在时间差。"
	_ = file.SetCellValue(sheet, fmt.Sprintf("A%d", noteLine), excelSafeText(note))
	noteStyle, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "6B7F76"},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	_ = file.SetCellStyle(sheet, fmt.Sprintf("A%d", noteLine), fmt.Sprintf("B%d", noteLine), noteStyle)
	_ = file.SetRowHeight(sheet, noteLine, 34)
	return sheet, nil
}

// fallbackText 返回去空格后的值，空值时使用默认文案。
func fallbackText(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
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
