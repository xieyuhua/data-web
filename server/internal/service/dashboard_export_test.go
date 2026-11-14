package service

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// TestBuildDashboardExcelMetaSheet 校验导出文件首页为「导出说明」，
// 且口径信息（看板名、数据范围、导出人）写入正确。
func TestBuildDashboardExcelMetaSheet(t *testing.T) {
	data := &DashboardData{
		StartDate: "2026-11-01",
		EndDate:   "2026-11-14",
		Widgets: []WidgetQueryResult{
			{
				WidgetID: "w1",
				Title:    "销售额",
				Data: &QueryResult{
					Columns: []string{"门店", "销售额"},
					Rows:    [][]any{{"静安旗舰店", 12345}},
					Count:   1,
				},
			},
		},
	}
	meta := ExportMeta{
		Operator:   "陈总（laozong）",
		Scope:      "角色：集团老总 · 可见组织 23 个",
		Filters:    "channel=线上",
		ExportedAt: time.Date(2026, 11, 14, 10, 30, 0, 0, time.Local),
	}
	content, err := BuildDashboardExcel("经营驾驶舱", data, "", meta)
	if err != nil {
		t.Fatalf("BuildDashboardExcel 失败: %v", err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("打开导出文件失败: %v", err)
	}
	defer file.Close()

	sheets := file.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("工作表数量 = %d，期望 2 (%v)", len(sheets), sheets)
	}
	if sheets[0] != "导出说明" {
		t.Fatalf("首个工作表 = %q，期望「导出说明」", sheets[0])
	}
	if sheets[1] != "销售额" {
		t.Fatalf("第二个工作表 = %q，期望「销售额」", sheets[1])
	}
	if value, _ := file.GetCellValue("导出说明", "B3"); value != "经营驾驶舱" {
		t.Fatalf("看板名称 = %q，期望「经营驾驶舱」", value)
	}
	if value, _ := file.GetCellValue("导出说明", "B6"); value != "channel=线上" {
		t.Fatalf("全局筛选 = %q，期望「channel=线上」", value)
	}
	if value, _ := file.GetCellValue("导出说明", "B7"); value != "陈总（laozong）" {
		t.Fatalf("导出人 = %q", value)
	}
}

// TestBuildDashboardExcelEmptyFilters 校验无筛选时导出说明给出可读默认文案。
func TestBuildDashboardExcelEmptyFilters(t *testing.T) {
	data := &DashboardData{StartDate: "2026-11-01", EndDate: "2026-11-14"}
	content, err := BuildDashboardExcel("空看板", data, "", ExportMeta{})
	if err != nil {
		t.Fatalf("BuildDashboardExcel 失败: %v", err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("打开导出文件失败: %v", err)
	}
	defer file.Close()
	if value, _ := file.GetCellValue("导出说明", "B6"); value != "无（全部数据）" {
		t.Fatalf("默认筛选文案 = %q，期望「无（全部数据）」", value)
	}
}
