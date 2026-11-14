package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type SalesAggregate struct {
	OrgID       uint64
	ActualValue float64
	Orders      int
	GrossProfit float64
	Customers   int
}

type EmployeeAggregate struct {
	EmployeeID  uint64
	ActualValue float64
	Orders      int
	GrossProfit float64
	Customers   int
}

type DailyAggregate struct {
	StatDate    string
	ActualValue float64
	Orders      int
	GrossProfit float64
	Customers   int
}

type DrillRepository struct{ db *gorm.DB }

func NewDrillRepository(db *gorm.DB) *DrillRepository { return &DrillRepository{db: db} }

func (r *DrillRepository) EmployeesByOrgIDs(orgIDs []uint64) ([]model.Employee, error) {
	if len(orgIDs) == 0 {
		return []model.Employee{}, nil
	}
	var items []model.Employee
	err := r.db.Where("org_id IN ? AND status = ?", orgIDs, "active").Order("org_id asc, id asc").Find(&items).Error
	return items, err
}

func (r *DrillRepository) FindEmployee(id uint64) (*model.Employee, error) {
	var item model.Employee
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

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

func (r *DrillRepository) DB() *gorm.DB { return r.db }
