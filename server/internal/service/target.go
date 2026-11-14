package service

import (
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type TargetMetric struct {
	Key   string
	Label string
	Unit  string
	Expr  string
}

var TargetMetrics = []TargetMetric{
	{Key: "revenue", Label: "销售额", Unit: "元", Expr: "COALESCE(SUM(revenue),0)"},
	{Key: "orders", Label: "订单量", Unit: "笔", Expr: "COALESCE(SUM(orders),0)"},
	{Key: "gross_profit", Label: "毛利额", Unit: "元", Expr: "COALESCE(SUM(gross_profit),0)"},
	{Key: "customers", Label: "客流", Unit: "人", Expr: "COALESCE(SUM(customers),0)"},
	{Key: "gross_margin", Label: "毛利率", Unit: "%", Expr: "CASE WHEN SUM(revenue) > 0 THEN SUM(gross_profit) / SUM(revenue) * 100 ELSE 0 END"},
}

type TargetInput struct {
	Name        string  `json:"name"`
	OrgID       uint64  `json:"org_id"`
	PeriodType  string  `json:"period_type"`
	Period      string  `json:"period"`
	Metric      string  `json:"metric"`
	TaskType    string  `json:"task_type"`
	TargetValue float64 `json:"target_value"`
	Unit        string  `json:"unit"`
	Remark      string  `json:"remark"`
}

type TargetView struct {
	model.Target
	OrgName     string `json:"org_name"`
	OrgCode     string `json:"org_code"`
	MetricLabel string `json:"metric_label"`
	TaskLabel   string `json:"task_type_label"`
}

type TargetAchievement struct {
	ID          uint64  `json:"id"`
	Name        string  `json:"name"`
	OrgID       uint64  `json:"org_id"`
	OrgName     string  `json:"org_name"`
	PeriodType  string  `json:"period_type"`
	Period      string  `json:"period"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	Metric      string  `json:"metric"`
	MetricLabel string  `json:"metric_label"`
	TaskType    string  `json:"task_type"`
	TaskLabel   string  `json:"task_type_label"`
	Unit        string  `json:"unit"`
	TargetValue float64 `json:"target_value"`
	ActualValue float64 `json:"actual_value"`
	Rate        float64 `json:"rate"`
	Status      string  `json:"status"`
}

type TargetSummary struct {
	TargetValue float64 `json:"target_value"`
	ActualValue float64 `json:"actual_value"`
	Rate        float64 `json:"rate"`
	Count       int     `json:"count"`
	RootCount   int     `json:"root_count"`
}

type TargetAchievementResult struct {
	PeriodType       string              `json:"period_type"`
	Period           string              `json:"period"`
	StartDate        string              `json:"start_date"`
	EndDate          string              `json:"end_date"`
	Metric           string              `json:"metric"`
	Summary          TargetSummary       `json:"summary"`
	BaseSummary      TargetSummary       `json:"base_summary"`
	ChallengeSummary TargetSummary       `json:"challenge_summary"`
	Targets          []TargetAchievement `json:"targets"`
}

type TargetService struct {
	repo    *repository.TargetRepository
	orgRepo *repository.OrganizationRepository
}

func NewTargetService(repo *repository.TargetRepository, orgRepo *repository.OrganizationRepository) *TargetService {
	return &TargetService{repo: repo, orgRepo: orgRepo}
}

// TaskType 任务类型：base 基础任务、challenge 挑战任务。
const (
	TaskTypeBase      = "base"
	TaskTypeChallenge = "challenge"
)

func NormalizeTaskType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TaskTypeChallenge, "挑战", "挑战任务":
		return TaskTypeChallenge
	default:
		return TaskTypeBase
	}
}

func TaskTypeLabel(value string) string {
	if NormalizeTaskType(value) == TaskTypeChallenge {
		return "挑战任务"
	}
	return "基础任务"
}

func (s *TargetService) List(periodType, period, metric, taskType string, orgIDs []uint64) ([]TargetView, error) {
	items, err := s.repo.List(periodType, period, metric, taskType, orgIDs)
	if err != nil {
		return nil, err
	}
	orgs, _ := s.orgRepo.All()
	orgMap := make(map[uint64]model.Organization, len(orgs))
	for _, item := range orgs {
		orgMap[item.ID] = item
	}
	result := make([]TargetView, 0, len(items))
	for _, item := range items {
		view := TargetView{Target: item, MetricLabel: metricLabel(item.Metric), TaskLabel: TaskTypeLabel(item.TaskType)}
		if org, ok := orgMap[item.OrgID]; ok {
			view.OrgName = org.Name
			view.OrgCode = org.Code
		}
		result = append(result, view)
	}
	return result, nil
}

func (s *TargetService) Create(input TargetInput, userID uint64) (*TargetView, error) {
	item, err := s.fromInput(input, 0)
	if err != nil {
		return nil, err
	}
	item.CreatedBy = userID
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	return s.view(item)
}

func (s *TargetService) Update(id uint64, input TargetInput) (*TargetView, error) {
	existing, err := s.repo.FindByID(id)
	if err != nil {
		return nil, errors.New("目标不存在")
	}
	item, err := s.fromInput(input, id)
	if err != nil {
		return nil, err
	}
	item.CreatedBy = existing.CreatedBy
	item.CreatedAt = existing.CreatedAt
	if err := s.repo.Update(item); err != nil {
		return nil, err
	}
	return s.view(item)
}

func (s *TargetService) Delete(id uint64) error { return s.repo.Delete(id) }

func (s *TargetService) Periods(periodType string) ([]string, error) {
	return s.repo.DistinctPeriods(periodType)
}

func (s *TargetService) view(item *model.Target) (*TargetView, error) {
	view := &TargetView{Target: *item, MetricLabel: metricLabel(item.Metric), TaskLabel: TaskTypeLabel(item.TaskType)}
	if org, err := s.orgRepo.FindByID(item.OrgID); err == nil {
		view.OrgName = org.Name
		view.OrgCode = org.Code
	}
	return view, nil
}

func (s *TargetService) fromInput(input TargetInput, id uint64) (*model.Target, error) {
	org, err := s.orgRepo.FindByID(input.OrgID)
	if err != nil {
		return nil, errors.New("目标所属组织不存在")
	}
	periodType := strings.ToLower(strings.TrimSpace(input.PeriodType))
	if periodType == "" {
		periodType = "month"
	}
	switch periodType {
	case "month", "quarter", "year", "custom":
	default:
		return nil, errors.New("周期类型仅支持 month / quarter / year / custom")
	}
	period := strings.TrimSpace(input.Period)
	if period == "" {
		period = CurrentPeriod(periodType)
	}
	if _, _, err := PeriodRange(periodType, period, time.Now()); err != nil {
		return nil, err
	}
	metricDef := findMetric(input.Metric)
	if metricDef == nil {
		return nil, errors.New("指标仅支持 revenue / orders / gross_profit / customers / gross_margin")
	}
	if input.TargetValue <= 0 {
		return nil, errors.New("目标值必须大于 0")
	}
	taskType := NormalizeTaskType(input.TaskType)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = fmt.Sprintf("%s%s%s", org.Name, metricDef.Label, TaskTypeLabel(taskType))
	}
	unit := input.Unit
	if unit == "" {
		unit = metricDef.Unit
	}
	return &model.Target{
		ID: id, Name: name, OrgID: input.OrgID, PeriodType: periodType, Period: period,
		Metric: metricDef.Key, TaskType: taskType, TargetValue: input.TargetValue, Unit: unit, Remark: input.Remark,
	}, nil
}

// Achievement 计算当前权限范围内的目标达成率。
func (s *TargetService) Achievement(orgIDs []uint64, periodType, period, metric, taskType string) (*TargetAchievementResult, error) {
	if periodType == "" {
		periodType = "month"
	}
	if period == "" {
		period = CurrentPeriod(periodType)
	}
	start, end, err := PeriodRange(periodType, period, time.Now())
	if err != nil {
		return nil, err
	}
	taskType = strings.ToLower(strings.TrimSpace(taskType))
	if taskType != TaskTypeBase && taskType != TaskTypeChallenge {
		taskType = ""
	}
	items, err := s.repo.List(periodType, period, metric, taskType, orgIDs)
	if err != nil {
		return nil, err
	}
	orgs, _ := s.orgRepo.All()
	orgMap := make(map[uint64]model.Organization, len(orgs))
	for _, org := range orgs {
		orgMap[org.ID] = org
	}
	allowed := make(map[uint64]bool, len(orgs))
	if orgIDs == nil {
		// nil 表示不受组织限制（管理员 / 全部数据范围）
		for _, org := range orgs {
			allowed[org.ID] = true
		}
	} else {
		for _, id := range orgIDs {
			allowed[id] = true
		}
	}
	result := &TargetAchievementResult{
		PeriodType: periodType, Period: period, Metric: metric,
		StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
		Targets: make([]TargetAchievement, 0, len(items)),
	}
	for _, item := range items {
		ids, err := s.orgRepo.DescendantIDs(item.OrgID)
		if err != nil {
			continue
		}
		filtered := make([]uint64, 0, len(ids))
		for _, id := range ids {
			if allowed[id] {
				filtered = append(filtered, id)
			}
		}
		actual, err := s.actualValue(filtered, item.Metric, start, end)
		if err != nil {
			return nil, err
		}
		rate := 0.0
		if item.TargetValue > 0 {
			rate = actual / item.TargetValue * 100
		}
		row := TargetAchievement{
			ID: item.ID, Name: item.Name, OrgID: item.OrgID,
			OrgName: orgMap[item.OrgID].Name, PeriodType: item.PeriodType, Period: item.Period,
			StartDate: result.StartDate, EndDate: result.EndDate,
			Metric: item.Metric, MetricLabel: metricLabel(item.Metric), Unit: item.Unit,
			TaskType: NormalizeTaskType(item.TaskType), TaskLabel: TaskTypeLabel(item.TaskType),
			TargetValue: round2(item.TargetValue), ActualValue: round2(actual),
			Rate: round1(rate), Status: achievementStatus(rate),
		}
		result.Targets = append(result.Targets, row)
	}
	// 汇总按任务类型分别统计，且只累加“顶层任务”，避免多层组织同时设目标时重复计算。
	result.Summary = s.summarizeTargets(result.Targets, orgMap, "")
	result.BaseSummary = s.summarizeTargets(result.Targets, orgMap, TaskTypeBase)
	result.ChallengeSummary = s.summarizeTargets(result.Targets, orgMap, TaskTypeChallenge)
	return result, nil
}

// summarizeTargets 按任务类型汇总达成情况；taskType 为空表示全部类型。
func (s *TargetService) summarizeTargets(rows []TargetAchievement, orgMap map[uint64]model.Organization, taskType string) TargetSummary {
	matched := make([]TargetAchievement, 0, len(rows))
	targetOrgs := make(map[uint64]bool, len(rows))
	for _, row := range rows {
		if taskType != "" && NormalizeTaskType(row.TaskType) != taskType {
			continue
		}
		matched = append(matched, row)
		targetOrgs[row.OrgID] = true
	}
	var summary TargetSummary
	for _, row := range matched {
		if hasTargetAncestor(row.OrgID, orgMap, targetOrgs) {
			continue
		}
		summary.TargetValue += row.TargetValue
		summary.ActualValue += row.ActualValue
		summary.RootCount++
	}
	summary.TargetValue = round2(summary.TargetValue)
	summary.ActualValue = round2(summary.ActualValue)
	if summary.TargetValue > 0 {
		summary.Rate = round1(summary.ActualValue / summary.TargetValue * 100)
	}
	summary.Count = len(matched)
	return summary
}

func hasTargetAncestor(orgID uint64, orgMap map[uint64]model.Organization, targetOrgs map[uint64]bool) bool {
	seen := make(map[uint64]bool)
	current := orgID
	for {
		org, ok := orgMap[current]
		if !ok || org.ParentID == nil || *org.ParentID == 0 || seen[org.ID] {
			return false
		}
		seen[org.ID] = true
		parentID := *org.ParentID
		if targetOrgs[parentID] {
			return true
		}
		current = parentID
	}
}

func (s *TargetService) actualValue(orgIDs []uint64, metric string, start, end time.Time) (float64, error) {
	if len(orgIDs) == 0 {
		return 0, nil
	}
	def := findMetric(metric)
	if def == nil {
		return 0, errors.New("指标类型不正确")
	}
	var row struct {
		Actual float64
	}
	err := s.orgRepo.DB().Model(&model.SalesFact{}).
		Select(def.Expr+" AS actual").
		Where("org_id IN ? AND stat_date BETWEEN ? AND ?", orgIDs, start.Format("2006-01-02"), end.Format("2006-01-02")).
		Scan(&row).Error
	return row.Actual, err
}

func (s *TargetService) Export(orgIDs []uint64) ([]byte, error) {
	items, err := s.repo.List("", "", "", "", orgIDs)
	if err != nil {
		return nil, err
	}
	orgs, _ := s.orgRepo.All()
	orgMap := make(map[uint64]model.Organization, len(orgs))
	for _, org := range orgs {
		orgMap[org.ID] = org
	}
	file := excelize.NewFile()
	defer file.Close()
	sheet := "经营目标"
	index, _ := file.NewSheet(sheet)
	file.SetActiveSheet(index)
	_ = file.DeleteSheet("Sheet1")
	headers := []string{"组织编码", "组织名称", "任务类型", "目标名称", "周期类型", "周期", "指标", "指标名称", "目标值", "单位", "备注"}
	writeHeader(file, sheet, headers, "173A33")
	for rowIndex, item := range items {
		row := rowIndex + 2
		org := orgMap[item.OrgID]
		values := []any{
			org.Code, org.Name, TaskTypeLabel(item.TaskType), item.Name, item.PeriodType, item.Period, item.Metric,
			metricLabel(item.Metric), item.TargetValue, item.Unit, item.Remark,
		}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			_ = file.SetCellValue(sheet, cell, value)
		}
	}
	_ = file.SetColWidth(sheet, "A", "K", 16)
	_ = file.SetColWidth(sheet, "D", "D", 24)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *TargetService) Template() ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	sheet := "经营目标"
	index, _ := file.NewSheet(sheet)
	file.SetActiveSheet(index)
	_ = file.DeleteSheet("Sheet1")
	headers := []string{"组织编码", "组织名称", "任务类型", "目标名称", "周期类型", "周期", "指标", "指标名称", "目标值", "单位", "备注"}
	writeHeader(file, sheet, headers, "173A33")
	sample := []any{
		"ST_SH_JA", "静安旗舰店", "基础任务", "静安旗舰店月度销售基础任务", "month", CurrentPeriod("month"),
		"revenue", "销售额", 320000, "元", "示例行，导入前请删除",
	}
	for col, value := range sample {
		cell, _ := excelize.CoordinatesToCellName(col+1, 2)
		_ = file.SetCellValue(sheet, cell, value)
	}
	_ = file.SetColWidth(sheet, "A", "K", 16)
	_ = file.SetColWidth(sheet, "D", "D", 26)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Import 通过 Excel 导入目标，按 组织 + 周期类型 + 周期 + 指标 幂等更新。
func (s *TargetService) Import(filename string, reader io.Reader) (int, int, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".xlsx" && ext != ".xls" {
		return 0, 0, errors.New("请上传 Excel 文件（.xlsx）")
	}
	file, err := excelize.OpenReader(reader)
	if err != nil {
		return 0, 0, fmt.Errorf("无法读取 Excel: %w", err)
	}
	defer file.Close()
	rows := pickSheetRows(file, "组织编码", "目标值")
	if len(rows) < 2 {
		return 0, 0, errors.New("Excel 中没有可导入的数据，请确认表头包含“组织编码”和“目标值”")
	}
	headers := map[string]int{}
	for i, title := range rows[0] {
		headers[strings.TrimSpace(title)] = i
	}
	get := func(row []string, name string) string {
		index, ok := headers[name]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	orgs, _ := s.orgRepo.All()
	orgByCode := make(map[string]model.Organization, len(orgs))
	orgByName := make(map[string]model.Organization, len(orgs))
	for _, org := range orgs {
		orgByCode[org.Code] = org
		orgByName[org.Name] = org
	}
	created, updated := 0, 0
	for _, row := range rows[1:] {
		orgCode := get(row, "组织编码")
		orgName := get(row, "组织名称")
		if orgCode == "" && orgName == "" {
			continue
		}
		org, ok := orgByCode[orgCode]
		if !ok {
			org, ok = orgByName[orgName]
		}
		if !ok {
			return created, updated, fmt.Errorf("组织不存在: %s", firstNonEmpty(orgCode, orgName))
		}
		metricRaw := get(row, "指标")
		metricDef := findMetric(metricRaw)
		if metricDef == nil {
			metricDef = findMetricByLabel(get(row, "指标名称"))
		}
		if metricDef == nil {
			return created, updated, fmt.Errorf("组织 %s 的指标不正确: %s", org.Code, metricRaw)
		}
		value, err := strconv.ParseFloat(strings.ReplaceAll(get(row, "目标值"), ",", ""), 64)
		if err != nil || value <= 0 {
			return created, updated, fmt.Errorf("组织 %s 的目标值不正确", org.Code)
		}
		periodType := strings.ToLower(get(row, "周期类型"))
		if periodType == "" {
			periodType = "month"
		}
		taskType := NormalizeTaskType(get(row, "任务类型"))
		period := get(row, "周期")
		if period == "" {
			period = CurrentPeriod(periodType)
		}
		if _, _, err := PeriodRange(periodType, period, time.Now()); err != nil {
			return created, updated, fmt.Errorf("组织 %s 的周期不正确: %s", org.Code, period)
		}
		name := get(row, "目标名称")
		if name == "" {
			name = fmt.Sprintf("%s%s%s", org.Name, metricDef.Label, TaskTypeLabel(taskType))
		}
		existing, err := s.repo.FindUnique(org.ID, periodType, period, metricDef.Key, taskType)
		item := &model.Target{
			Name: name, OrgID: org.ID, PeriodType: periodType, Period: period, Metric: metricDef.Key,
			TaskType: taskType, TargetValue: value, Unit: firstNonEmpty(get(row, "单位"), metricDef.Unit), Remark: get(row, "备注"),
		}
		if err == nil {
			item.ID = existing.ID
			item.CreatedBy = existing.CreatedBy
			item.CreatedAt = existing.CreatedAt
			if err := s.repo.Update(item); err != nil {
				return created, updated, err
			}
			updated++
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, updated, err
		}
		if err := s.repo.Create(item); err != nil {
			return created, updated, err
		}
		created++
	}
	return created, updated, nil
}

func findMetric(key string) *TargetMetric {
	key = strings.ToLower(strings.TrimSpace(key))
	for i := range TargetMetrics {
		if TargetMetrics[i].Key == key {
			return &TargetMetrics[i]
		}
	}
	return nil
}

func findMetricByLabel(label string) *TargetMetric {
	label = strings.TrimSpace(label)
	for i := range TargetMetrics {
		if TargetMetrics[i].Label == label {
			return &TargetMetrics[i]
		}
	}
	return nil
}

func metricLabel(key string) string {
	if def := findMetric(key); def != nil {
		return def.Label
	}
	return key
}

func CurrentPeriod(periodType string) string {
	now := time.Now()
	switch periodType {
	case "quarter":
		return fmt.Sprintf("%d-Q%d", now.Year(), (int(now.Month())-1)/3+1)
	case "year":
		return strconv.Itoa(now.Year())
	case "custom":
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		last := first.AddDate(0, 1, -1)
		return first.Format("2006-01-02") + "~" + last.Format("2006-01-02")
	default:
		return now.Format("2006-01")
	}
}

// PeriodRange 将周期转换为日期区间。
func PeriodRange(periodType, period string, now time.Time) (time.Time, time.Time, error) {
	period = strings.TrimSpace(period)
	loc := now.Location()
	switch periodType {
	case "month":
		start, err := time.ParseInLocation("2006-01", period, loc)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("月份格式应为 2026-11")
		}
		return start, start.AddDate(0, 1, -1), nil
	case "quarter":
		normalized := strings.ToUpper(strings.ReplaceAll(period, "Q", "-Q"))
		parts := strings.Split(normalized, "-Q")
		if len(parts) != 2 {
			return time.Time{}, time.Time{}, errors.New("季度格式应为 2026-Q4")
		}
		year, errYear := strconv.Atoi(parts[0])
		quarter, errQuarter := strconv.Atoi(parts[1])
		if errYear != nil || errQuarter != nil || quarter < 1 || quarter > 4 {
			return time.Time{}, time.Time{}, errors.New("季度格式应为 2026-Q4")
		}
		start := time.Date(year, time.Month((quarter-1)*3+1), 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 3, -1), nil
	case "year":
		year, err := strconv.Atoi(period)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("年份格式应为 2026")
		}
		start := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(1, 0, -1), nil
	case "custom":
		parts := strings.Split(period, "~")
		if len(parts) != 2 {
			parts = strings.Split(period, "至")
		}
		if len(parts) != 2 {
			return time.Time{}, time.Time{}, errors.New("自定义周期格式应为 2026-11-01~2026-11-30")
		}
		start, errStart := time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[0]), loc)
		end, errEnd := time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[1]), loc)
		if errStart != nil || errEnd != nil || start.After(end) {
			return time.Time{}, time.Time{}, errors.New("自定义周期格式应为 2026-11-01~2026-11-30")
		}
		return start, end, nil
	default:
		return time.Time{}, time.Time{}, errors.New("周期类型不正确")
	}
}

func achievementStatus(rate float64) string {
	switch {
	case rate >= 100:
		return "ahead"
	case rate >= 80:
		return "normal"
	default:
		return "behind"
	}
}

func round1(value float64) float64 { return math.Round(value*10) / 10 }
func round2(value float64) float64 { return math.Round(value*100) / 100 }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeHeader(file *excelize.File, sheet string, headers []string, color string) {
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = file.SetCellValue(sheet, cell, header)
	}
	style, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{color}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	_ = file.SetCellStyle(sheet, "A1", fmt.Sprintf("%c1", 'A'+len(headers)-1), style)
}

func pickSheetRows(file *excelize.File, required ...string) [][]string {
	for _, sheet := range file.GetSheetList() {
		rows, err := file.GetRows(sheet)
		if err != nil || len(rows) < 2 {
			continue
		}
		found := make(map[string]bool, len(required))
		for _, title := range rows[0] {
			found[strings.TrimSpace(title)] = true
		}
		ok := true
		for _, title := range required {
			if !found[title] {
				ok = false
				break
			}
		}
		if ok {
			return rows
		}
	}
	return nil
}
