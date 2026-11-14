package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// SalesAggregate 按组织聚合的销售指标（下钻看板使用）。
type SalesAggregate struct {
	OrgID       uint64
	ActualValue float64
	Orders      int
	GrossProfit float64
	Customers   int
}

// EmployeeAggregate 按员工聚合的销售指标。
type EmployeeAggregate struct {
	EmployeeID  uint64
	ActualValue float64
	Orders      int
	GrossProfit float64
	Customers   int
}

// DailyAggregate 按日期聚合的销售指标（趋势图使用）。
type DailyAggregate struct {
	StatDate    string
	ActualValue float64
	Orders      int
	GrossProfit float64
	Customers   int
}

// DrillRepository 组织下钻查询仓储。
// 负责读取组织下的销售事实数据、员工数据以及按日汇总数据。
type DrillRepository struct{ db *gorm.DB }

// NewDrillRepository 创建下钻查询仓储。
func NewDrillRepository(db *gorm.DB) *DrillRepository { return &DrillRepository{db: db} }

// EmployeesByOrgIDs 查询指定组织下的在职员工。
func (r *DrillRepository) EmployeesByOrgIDs(orgIDs []uint64) ([]model.Employee, error) {
	if len(orgIDs) == 0 {
		return []model.Employee{}, nil
	}
	var items []model.Employee
	err := r.db.Where("org_id IN ? AND status = ?", orgIDs, "active").Order("org_id asc, id asc").Find(&items).Error
	return items, err
}

// FindEmployee 按主键查询员工。
func (r *DrillRepository) FindEmployee(id uint64) (*model.Employee, error) {
	var item model.Employee
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// SalesByOrg 按组织聚合销售数据；metricExpr 由上层按指标白名单拼装，避免 SQL 注入。
func (r *DrillRepository) SalesByOrg(orgIDs []uint64, startDate, endDate, metricExpr string) ([]SalesAggregate, error) {
	if len(orgIDs) == 0 {
		return []SalesAggregate{}, nil
	}
	var rows []SalesAggregate
	err := r.db.Model(&model.SalesFact{}).
		Select("org_id, "+metricExpr+" AS actual_value, COALESCE(SUM(orders),0) AS orders, COALESCE(SUM(gross_profit),0) AS gross_profit, COALESCE(SUM(customers),0) AS customers").
		Where("org_id IN ? AND stat_date BETWEEN ? AND ?", orgIDs, startDate, endDate).
		Group("org_id").Scan(&rows).Error
	return rows, err
}

// SalesByEmployee 按员工聚合销售数据。
func (r *DrillRepository) SalesByEmployee(employeeIDs []uint64, startDate, endDate, metricExpr string) ([]EmployeeAggregate, error) {
	if len(employeeIDs) == 0 {
		return []EmployeeAggregate{}, nil
	}
	var rows []EmployeeAggregate
	err := r.db.Model(&model.EmployeeSale{}).
		Select("employee_id, "+metricExpr+" AS actual_value, COALESCE(SUM(orders),0) AS orders, COALESCE(SUM(gross_profit),0) AS gross_profit, COALESCE(SUM(customers),0) AS customers").
		Where("employee_id IN ? AND stat_date BETWEEN ? AND ?", employeeIDs, startDate, endDate).
		Group("employee_id").Scan(&rows).Error
	return rows, err
}

// SalesDailyByOrg 按天汇总组织销售，用于趋势图。
// SalesDailyByOrg 按天汇总组织销售，用于组织趋势图。
func (r *DrillRepository) SalesDailyByOrg(orgIDs []uint64, startDate, endDate, metricExpr string) ([]DailyAggregate, error) {
	if len(orgIDs) == 0 {
		return []DailyAggregate{}, nil
	}
	var rows []DailyAggregate
	err := r.db.Model(&model.SalesFact{}).
		Select("stat_date, "+metricExpr+" AS actual_value, COALESCE(SUM(orders),0) AS orders, COALESCE(SUM(gross_profit),0) AS gross_profit, COALESCE(SUM(customers),0) AS customers").
		Where("org_id IN ? AND stat_date BETWEEN ? AND ?", orgIDs, startDate, endDate).
		Group("stat_date").Order("stat_date asc").Scan(&rows).Error
	return rows, err
}

// SalesDailyByEmployee 按天汇总员工销售，用于员工趋势图。
// SalesDailyByEmployee 按天汇总单个员工销售，用于员工趋势图。
func (r *DrillRepository) SalesDailyByEmployee(employeeID uint64, startDate, endDate, metricExpr string) ([]DailyAggregate, error) {
	if employeeID == 0 {
		return []DailyAggregate{}, nil
	}
	var rows []DailyAggregate
	err := r.db.Model(&model.EmployeeSale{}).
		Select("stat_date, "+metricExpr+" AS actual_value, COALESCE(SUM(orders),0) AS orders, COALESCE(SUM(gross_profit),0) AS gross_profit, COALESCE(SUM(customers),0) AS customers").
		Where("employee_id = ? AND stat_date BETWEEN ? AND ?", employeeID, startDate, endDate).
		Group("stat_date").Order("stat_date asc").Scan(&rows).Error
	return rows, err
}

// DB 暴露底层连接。
func (r *DrillRepository) DB() *gorm.DB { return r.db }
