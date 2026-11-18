package service

import (
	"context"
	"errors"

	"biaoge-bi/internal/export"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
)

type ExportService struct {
	repo      *repository.Repo
	charts    *ChartService
	dashboard *DashboardService
}

func NewExportService(repo *repository.Repo, charts *ChartService, dashboard *DashboardService) *ExportService {
	return &ExportService{repo: repo, charts: charts, dashboard: dashboard}
}

// ChartCSV renders a chart result as CSV.
func (s *ExportService) ChartCSV(ctx context.Context, chartID uint, params map[string]string) ([]byte, string, error) {
	c, err := s.charts.Get(chartID)
	if err != nil {
		return nil, "", errors.New("图表不存在")
	}
	rs, err := s.charts.Execute(ctx, c, params)
	if err != nil {
		return nil, "", err
	}
	data, err := export.WriteCSV(rs)
	if err != nil {
		return nil, "", err
	}
	return data, c.Name + ".csv", nil
}

func (s *ExportService) ChartXLSX(ctx context.Context, chartID uint, params map[string]string) ([]byte, string, error) {
	c, err := s.charts.Get(chartID)
	if err != nil {
		return nil, "", errors.New("图表不存在")
	}
	rs, err := s.charts.Execute(ctx, c, params)
	if err != nil {
		return nil, "", err
	}
	data, err := export.WriteWorkbook([]export.Sheet{{Name: c.Name, Result: rs}})
	if err != nil {
		return nil, "", err
	}
	return data, c.Name + ".xlsx", nil
}

// DashboardXLSX renders every chart widget as one sheet.
func (s *ExportService) DashboardXLSX(ctx context.Context, dashboardID uint, req DashboardDataRequest) ([]byte, string, error) {
	d, err := s.repo.DashboardByID(dashboardID)
	if err != nil {
		return nil, "", errors.New("仪表盘不存在")
	}
	data, err := s.dashboard.ExecuteData(ctx, dashboardID, req)
	if err != nil {
		return nil, "", err
	}
	inputs := make([]export.DashboardSheetInput, 0, len(d.Widgets))
	for _, w := range d.Widgets {
		if w.WidgetType != model.WidgetChart || w.ChartID == nil {
			continue
		}
		wr, ok := data.Widgets[uintToString(w.ID)]
		if !ok || wr.Result == nil {
			continue
		}
		title := w.Title
		if title == "" {
			if c, err := s.charts.Get(*w.ChartID); err == nil {
				title = c.Name
			}
		}
		inputs = append(inputs, export.DashboardSheetInput{Title: title, Result: wr.Result})
	}
	if len(inputs) == 0 {
		return nil, "", errors.New("仪表盘没有可导出的图表数据")
	}
	out, err := export.WriteWorkbook(export.SheetsFromDashboard(inputs))
	if err != nil {
		return nil, "", err
	}
	return out, d.Name + ".xlsx", nil
}
