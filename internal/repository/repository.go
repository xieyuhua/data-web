package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"biaoge-bi/internal/model"
)

var ErrNotFound = errors.New("记录不存在")

type Repo struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Repo { return &Repo{DB: db} }

func isNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// ---------- Users ----------

func (r *Repo) UserByID(id uint) (*model.User, error) {
	var u model.User
	if err := r.DB.First(&u, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repo) UserByUsername(username string) (*model.User, error) {
	var u model.User
	if err := r.DB.Where("username = ?", username).First(&u).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repo) UserByWeComID(wecomUserID string) (*model.User, error) {
	if strings.TrimSpace(wecomUserID) == "" {
		return nil, ErrNotFound
	}
	var u model.User
	if err := r.DB.Where("we_com_user_id = ?", wecomUserID).First(&u).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repo) CountUsers() (int64, error) {
	var n int64
	err := r.DB.Model(&model.User{}).Count(&n).Error
	return n, err
}

func (r *Repo) ListUsers(keyword string, page, pageSize int) ([]model.User, int64, error) {
	q := r.DB.Model(&model.User{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("username LIKE ? OR display_name LIKE ? OR email LIKE ?", kw, kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []model.User
	err := q.Order("id").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error
	return users, total, err
}

// ---------- Data sources ----------

func (r *Repo) ListDataSources() ([]model.DataSource, error) {
	var list []model.DataSource
	err := r.DB.Order("id").Find(&list).Error
	return list, err
}

func (r *Repo) DataSourceByID(id uint) (*model.DataSource, error) {
	var ds model.DataSource
	if err := r.DB.First(&ds, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ds, nil
}

// ---------- Datasets ----------

func (r *Repo) ListDatasets(keyword string) ([]model.Dataset, error) {
	q := r.DB.Preload("Params").Order("id")
	if keyword != "" {
		q = q.Where("name LIKE ?", "%"+keyword+"%")
	}
	var list []model.Dataset
	err := q.Find(&list).Error
	return list, err
}

func (r *Repo) DatasetByID(id uint) (*model.Dataset, error) {
	var d model.Dataset
	if err := r.DB.Preload("Params").First(&d, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

// ---------- Charts ----------

func (r *Repo) ListCharts(keyword string) ([]model.Chart, error) {
	q := r.DB.Order("id")
	if keyword != "" {
		q = q.Where("name LIKE ?", "%"+keyword+"%")
	}
	var list []model.Chart
	err := q.Find(&list).Error
	return list, err
}

func (r *Repo) ChartByID(id uint) (*model.Chart, error) {
	var c model.Chart
	if err := r.DB.First(&c, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ---------- Dashboards ----------

// AccessibleDashboardIDs returns dashboard IDs visible to a user (own, granted, or all for admin).
func (r *Repo) AccessibleDashboardIDs(userID uint, isAdmin bool) (map[uint]string, error) {
	access := make(map[uint]string)
	if isAdmin {
		var ids []uint
		if err := r.DB.Model(&model.Dashboard{}).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		for _, id := range ids {
			access[id] = "edit"
		}
		return access, nil
	}

	var owned []uint
	if err := r.DB.Model(&model.Dashboard{}).Where("owner_id = ?", userID).Pluck("id", &owned).Error; err != nil {
		return nil, err
	}
	for _, id := range owned {
		access[id] = "edit"
	}
	var grants []model.DashboardGrant
	if err := r.DB.Where("user_id = ?", userID).Find(&grants).Error; err != nil {
		return nil, err
	}
	for _, g := range grants {
		if cur, ok := access[g.DashboardID]; !ok || (cur == "view" && g.Permission == "edit") {
			access[g.DashboardID] = g.Permission
		}
	}
	return access, nil
}

func (r *Repo) DashboardByID(id uint) (*model.Dashboard, error) {
	var d model.Dashboard
	if err := r.DB.Preload("Widgets").Preload("Filters").First(&d, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *Repo) BindingsByDashboard(dashboardID uint) ([]model.FilterBinding, error) {
	var bindings []model.FilterBinding
	err := r.DB.Table("filter_bindings").
		Joins("JOIN dashboard_filters ON dashboard_filters.id = filter_bindings.filter_id").
		Where("dashboard_filters.dashboard_id = ?", dashboardID).
		Find(&bindings).Error
	return bindings, err
}

// ---------- Schedules ----------

func (r *Repo) ScheduleByID(id uint) (*model.ScheduleTask, error) {
	var t model.ScheduleTask
	if err := r.DB.First(&t, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repo) ListScheduleRuns(taskID uint, limit int) ([]model.ScheduleRun, error) {
	var runs []model.ScheduleRun
	err := r.DB.Where("task_id = ?", taskID).Order("id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

func (r *Repo) ScheduleRunByID(id uint) (*model.ScheduleRun, error) {
	var run model.ScheduleRun
	if err := r.DB.First(&run, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &run, nil
}
