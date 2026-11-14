package seed

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"bi-platform/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Run 写入演示数据：角色、组织、用户、销售事实、员工、目标、数据集与看板。
// 各步骤内部均做了幂等判断，重复启动不会产生重复数据。
func Run(db *gorm.DB) error {
	if err := seedRoles(db); err != nil {
		return err
	}
	orgs, err := seedOrganizations(db)
	if err != nil {
		return err
	}
	if err := seedUsers(db, orgs); err != nil {
		return err
	}
	if err := seedSales(db, orgs); err != nil {
		return err
	}
	if err := seedEmployees(db, orgs); err != nil {
		return err
	}
	if err := seedTargets(db, orgs); err != nil {
		return err
	}
	if err := seedDatasets(db); err != nil {
		return err
	}
	if err := seedDashboards(db); err != nil {
		return err
	}
	if err := seedTemplates(db); err != nil {
		return err
	}
	if err := seedPush(db); err != nil {
		return err
	}
	if err := seedChannelConfigs(db); err != nil {
		return err
	}
	log.Println("demo data ready | admin/admin123 | laozong/123456 | huadong/123456 | shanghai/123456 | store01/123456")
	return nil
}

func seedRoles(db *gorm.DB) error {
	roles := []model.Role{
		{Code: "super_admin", Name: "平台管理员", DataScope: "all", Level: 0, Permissions: `["*"]`, Description: "管理系统配置、用户、数据集和全部组织数据"},
		{Code: "boss", Name: "集团老总", DataScope: "all", Level: 1, Permissions: `["dashboard:view","dashboard:create","share:create","push:view"]`, Description: "查看集团全部经营数据"},
		{Code: "big_region_manager", Name: "大区经理", DataScope: "own_org_tree", Level: 2, Permissions: `["dashboard:view","dashboard:create","share:create"]`, Description: "查看所辖大区及下级区域、门店数据"},
		{Code: "regional_manager", Name: "区域经理", DataScope: "own_org_tree", Level: 3, Permissions: `["dashboard:view","dashboard:create","share:create"]`, Description: "查看所辖区域及门店数据"},
		{Code: "store_manager", Name: "店长", DataScope: "own_org", Level: 4, Permissions: `["dashboard:view","share:create"]`, Description: "仅查看当前门店数据"},
		{Code: "store_staff", Name: "门店员工", DataScope: "own_org", Level: 5, Permissions: `["dashboard:view"]`, Description: "仅查看当前门店数据"},
	}
	for i := range roles {
		var count int64
		db.Model(&model.Role{}).Where("code = ?", roles[i].Code).Count(&count)
		if count == 0 {
			if err := db.Create(&roles[i]).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

type orgSet struct {
	Company   model.Organization
	BigEast   model.Organization
	BigSouth  model.Organization
	Regions   []model.Organization
	Stores    []model.Organization
	StoreMeta map[uint64]storeMeta
}

type storeMeta struct {
	RegionID    uint64
	BigRegionID uint64
}

func seedOrganizations(db *gorm.DB) (*orgSet, error) {
	set := &orgSet{StoreMeta: map[uint64]storeMeta{}}
	var count int64
	db.Model(&model.Organization{}).Count(&count)
	if count > 0 {
		all, _ := loadOrgs(db)
		for _, item := range all {
			switch item.Code {
			case "HQ":
				set.Company = item
			case "BR_EAST":
				set.BigEast = item
			case "BR_SOUTH":
				set.BigSouth = item
			}
		}
		set.Regions, set.Stores = []model.Organization{}, []model.Organization{}
		parentOf := make(map[uint64]uint64)
		for _, item := range all {
			if item.ParentID != nil {
				parentOf[item.ID] = *item.ParentID
			}
		}
		for _, item := range all {
			if item.Type == "region" {
				set.Regions = append(set.Regions, item)
			}
			if item.Type == "store" {
				set.Stores = append(set.Stores, item)
				regionID := parentOf[item.ID]
				bigID := parentOf[regionID]
				set.StoreMeta[item.ID] = storeMeta{RegionID: regionID, BigRegionID: bigID}
			}
		}
		return set, nil
	}
	company := model.Organization{Name: "众智零售集团", Code: "HQ", Type: "company", Level: 1, ManagerName: "陈总", Status: "active", Sort: 1}
	if err := db.Create(&company).Error; err != nil {
		return nil, err
	}
	set.Company = company
	east := model.Organization{ParentID: &company.ID, Name: "华东大区", Code: "BR_EAST", Type: "big_region", Level: 2, ManagerName: "周明", Status: "active", Sort: 1}
	south := model.Organization{ParentID: &company.ID, Name: "华南大区", Code: "BR_SOUTH", Type: "big_region", Level: 2, ManagerName: "梁欣", Status: "active", Sort: 2}
	if err := db.Create(&east).Error; err != nil {
		return nil, err
	}
	if err := db.Create(&south).Error; err != nil {
		return nil, err
	}
	set.BigEast, set.BigSouth = east, south
	regionSeeds := []struct {
		Code, Name string
		Parent     *model.Organization
		Manager    string
	}{
		{"RG_SH", "上海区域", &east, "李娜"},
		{"RG_JS", "江苏区域", &east, "王磊"},
		{"RG_ZJ", "浙江区域", &east, "赵敏"},
		{"RG_GZ", "广州区域", &south, "陈晨"},
		{"RG_SZ", "深圳区域", &south, "林峰"},
	}
	for i, item := range regionSeeds {
		org := model.Organization{
			ParentID: &item.Parent.ID, Name: item.Name, Code: item.Code, Type: "region",
			Level: 3, ManagerName: item.Manager, Status: "active", Sort: i + 1,
		}
		if err := db.Create(&org).Error; err != nil {
			return nil, err
		}
		set.Regions = append(set.Regions, org)
	}
	stores := []struct {
		Code, Name, Address string
		Region              *model.Organization
	}{
		{"ST_SH_JA", "静安旗舰店", "上海市静安区南京西路 1266 号", &set.Regions[0]},
		{"ST_SH_XH", "徐汇滨江店", "上海市徐汇区龙腾大道 2888 号", &set.Regions[0]},
		{"ST_SH_PD", "浦东世纪汇店", "上海市浦东新区世纪大道 1192 号", &set.Regions[0]},
		{"ST_JS_NJ", "南京新街口店", "南京市秦淮区中山南路 1 号", &set.Regions[1]},
		{"ST_JS_SZ", "苏州中心店", "苏州市工业园区苏绣路 58 号", &set.Regions[1]},
		{"ST_JS_WX", "无锡恒隆店", "无锡市梁溪区人民中路 139 号", &set.Regions[1]},
		{"ST_ZJ_HZ", "杭州湖滨店", "杭州市上城区延安路 258 号", &set.Regions[2]},
		{"ST_ZJ_NB", "宁波天一店", "宁波市海曙区中山东路 188 号", &set.Regions[2]},
		{"ST_ZJ_WZ", "温州万象城店", "温州市瓯海区温瑞大道 999 号", &set.Regions[2]},
		{"ST_GZ_TX", "广州天河城店", "广州市天河区天河路 208 号", &set.Regions[3]},
		{"ST_GZ_PY", "番禺万博店", "广州市番禺区汉溪大道东 366 号", &set.Regions[3]},
		{"ST_GZ_BY", "白云新城店", "广州市白云区云城东路 501 号", &set.Regions[3]},
		{"ST_SZ_FT", "福田中心店", "深圳市福田区福华三路 118 号", &set.Regions[4]},
		{"ST_SZ_NS", "南山海岸城店", "深圳市南山区文心五路 33 号", &set.Regions[4]},
		{"ST_SZ_LG", "龙岗大运店", "深圳市龙岗区龙翔大道 7188 号", &set.Regions[4]},
	}
	for i, item := range stores {
		org := model.Organization{
			ParentID: &item.Region.ID, Name: item.Name, Code: item.Code, Type: "store",
			Level: 4, ManagerName: fmt.Sprintf("店长%02d", i+1), Phone: fmt.Sprintf("1380000%04d", i),
			Address: item.Address, Status: "active", Sort: i + 1,
		}
		if err := db.Create(&org).Error; err != nil {
			return nil, err
		}
		set.Stores = append(set.Stores, org)
		bigID := east.ID
		if item.Region.ParentID != nil {
			bigID = *item.Region.ParentID
		}
		set.StoreMeta[org.ID] = storeMeta{RegionID: item.Region.ID, BigRegionID: bigID}
	}
	return set, nil
}

func loadOrgs(db *gorm.DB) ([]model.Organization, error) {
	var items []model.Organization
	return items, db.Order("id asc").Find(&items).Error
}

func seedUsers(db *gorm.DB, orgs *orgSet) error {
	type userSeed struct {
		Name, Username, Password, Role string
		Org                            model.Organization
		Wecom, Wechat                  string
	}
	users := []userSeed{
		{"平台管理员", "admin", "admin123", "super_admin", orgs.Company, "admin", "admin_openid"},
		{"陈总", "laozong", "123456", "boss", orgs.Company, "laozong", "laozong_openid"},
		{"周明", "huadong", "123456", "big_region_manager", orgs.BigEast, "huadong", "huadong_openid"},
		{"李娜", "shanghai", "123456", "regional_manager", orgs.Regions[0], "shanghai", "shanghai_openid"},
		{"店长01", "store01", "123456", "store_manager", orgs.Stores[0], "store01", "store01_openid"},
		{"王磊", "jiangsu", "123456", "regional_manager", orgs.Regions[1], "jiangsu", "jiangsu_openid"},
		{"陈晨", "guangzhou", "123456", "regional_manager", orgs.Regions[3], "guangzhou", "guangzhou_openid"},
	}
	for _, item := range users {
		var count int64
		db.Model(&model.User{}).Where("username = ?", item.Username).Count(&count)
		if count > 0 {
			continue
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(item.Password), bcrypt.DefaultCost)
		user := model.User{
			Name: item.Name, Username: item.Username, PasswordHash: string(hash),
			RoleCode: item.Role, OrganizationID: item.Org.ID, WecomUserID: item.Wecom,
			WechatOpenID: item.Wechat, Status: "active",
		}
		if err := db.Create(&user).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedSales(db *gorm.DB, orgs *orgSet) error {
	var count int64
	db.Model(&model.SalesFact{}).Count(&count)
	if count > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(20261114))
	channels := []string{"到店", "小程序", "外卖"}
	end := time.Now()
	start := end.AddDate(0, 0, -119)
	facts := make([]model.SalesFact, 0, len(orgs.Stores)*120)
	for storeIndex, store := range orgs.Stores {
		meta := orgs.StoreMeta[store.ID]
		base := 7800.0 + float64(storeIndex)*420
		for day := 0; day < 120; day++ {
			date := start.AddDate(0, 0, day)
			seasonal := 1.0
			if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
				seasonal = 1.28
			}
			trend := 1 + float64(day)*0.0017
			noise := 0.86 + rng.Float64()*0.28
			revenue := math.Round(base*seasonal*trend*noise*100) / 100
			ticket := 78 + rng.Float64()*42
			orders := int(math.Round(revenue / ticket))
			customers := int(math.Round(float64(orders) * (0.86 + rng.Float64()*0.1)))
			grossProfit := math.Round(revenue*(0.33+rng.Float64()*0.11)*100) / 100
			facts = append(facts, model.SalesFact{
				OrgID: store.ID, StoreID: store.ID, RegionID: meta.RegionID, BigRegionID: meta.BigRegionID,
				StatDate: date.Format("2006-01-02"), Revenue: revenue, Orders: orders,
				GrossProfit: grossProfit, Customers: customers, Channel: channels[rng.Intn(len(channels))],
			})
		}
	}
	return db.CreateInBatches(facts, 300).Error
}

// seedEmployees 为每个门店生成员工，并把门店销售按日拆分到员工，保证加减一致。
func seedEmployees(db *gorm.DB, orgs *orgSet) error {
	var employeeCount int64
	db.Model(&model.Employee{}).Count(&employeeCount)
	if employeeCount == 0 {
		names := []string{"张敏", "李强", "王芳", "陈杰"}
		positions := []string{"店长", "销售主管", "导购", "收银"}
		employees := make([]model.Employee, 0, len(orgs.Stores)*len(names))
		for _, store := range orgs.Stores {
			for i, name := range names {
				employees = append(employees, model.Employee{
					OrgID: store.ID, Code: fmt.Sprintf("%s_E%02d", store.Code, i+1),
					Name: name, Position: positions[i], Phone: fmt.Sprintf("1390000%04d", store.ID*10+uint64(i)),
					Status: "active",
				})
			}
		}
		if err := db.CreateInBatches(employees, 100).Error; err != nil {
			return err
		}
	}
	var saleCount int64
	db.Model(&model.EmployeeSale{}).Count(&saleCount)
	if saleCount > 0 {
		return nil
	}
	var employees []model.Employee
	if err := db.Order("org_id asc, id asc").Find(&employees).Error; err != nil {
		return err
	}
	byOrg := make(map[uint64][]model.Employee)
	for _, item := range employees {
		byOrg[item.OrgID] = append(byOrg[item.OrgID], item)
	}
	var facts []model.SalesFact
	if err := db.Order("org_id asc, stat_date asc").Find(&facts).Error; err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(20261115))
	rows := make([]model.EmployeeSale, 0, len(facts)*4)
	for _, fact := range facts {
		list := byOrg[fact.OrgID]
		if len(list) == 0 {
			continue
		}
		weights := make([]float64, len(list))
		sum := 0.0
		for i := range weights {
			weights[i] = 0.16 + rng.Float64()*0.18
			sum += weights[i]
		}
		revenueLeft, profitLeft := fact.Revenue, fact.GrossProfit
		ordersLeft, customersLeft := fact.Orders, fact.Customers
		for i, employee := range list {
			share := weights[i] / sum
			revenue := math.Round(fact.Revenue*share*100) / 100
			profit := math.Round(fact.GrossProfit*share*100) / 100
			orders := int(math.Round(float64(fact.Orders) * share))
			customers := int(math.Round(float64(fact.Customers) * share))
			if i == len(list)-1 {
				revenue, profit, orders, customers = revenueLeft, profitLeft, ordersLeft, customersLeft
			}
			revenueLeft = math.Round((revenueLeft-revenue)*100) / 100
			profitLeft = math.Round((profitLeft-profit)*100) / 100
			ordersLeft -= orders
			customersLeft -= customers
			rows = append(rows, model.EmployeeSale{
				EmployeeID: employee.ID, OrgID: fact.OrgID, StatDate: fact.StatDate,
				Revenue: revenue, Orders: orders, GrossProfit: profit, Customers: customers,
			})
		}
	}
	return db.CreateInBatches(rows, 300).Error
}

func seedTargets(db *gorm.DB, orgs *orgSet) error {
	var count int64
	db.Model(&model.Target{}).Count(&count)
	if count > 0 {
		return nil
	}
	period := time.Now().Format("2006-01")
	items := make([]model.Target, 0, len(orgs.Regions)*2+len(orgs.Stores)*2+6)
	add := func(org model.Organization, value float64, taskType, label string) {
		items = append(items, model.Target{
			Name: org.Name + label, OrgID: org.ID, PeriodType: "month", Period: period,
			Metric: "revenue", TaskType: taskType, TargetValue: value, Unit: "元",
		})
	}
	add(orgs.Company, 12000000, "base", "月度销售基础任务")
	add(orgs.Company, 15000000, "challenge", "月度销售挑战任务")
	add(orgs.BigEast, 7200000, "base", "月度销售基础任务")
	add(orgs.BigSouth, 4800000, "base", "月度销售基础任务")
	for _, region := range orgs.Regions {
		add(region, 1800000, "base", "月度销售基础任务")
		add(region, 2200000, "challenge", "月度销售挑战任务")
	}
	for _, store := range orgs.Stores {
		add(store, 380000, "base", "月度销售基础任务")
		add(store, 460000, "challenge", "月度销售挑战任务")
	}
	return db.CreateInBatches(items, 100).Error
}

func seedDatasets(db *gorm.DB) error {
	datasets := []model.Dataset{
		{
			Name: "销售核心指标", Code: "sales_summary", Description: "按权限范围汇总销售额、订单、毛利和客流",
			SQLText: `SELECT ROUND(COALESCE(SUM(revenue),0),2) AS revenue,
ROUND(COALESCE(SUM(gross_profit),0),2) AS gross_profit,
COALESCE(SUM(orders),0) AS orders,
COALESCE(SUM(customers),0) AS customers,
ROUND(COALESCE(SUM(gross_profit),0) / NULLIF(SUM(revenue),0) * 100,1) AS gross_margin
FROM sales_facts
WHERE stat_date BETWEEN {{start_date}} AND {{end_date}}
AND org_id IN ({{org_scope}})`,
			ScopeColumn: "org_id", RequireScope: true, Status: "active",
		},
		{
			Name: "销售趋势", Code: "sales_trend", Description: "日销售额与毛利趋势",
			SQLText: `SELECT stat_date AS date,
ROUND(SUM(revenue),2) AS revenue,
ROUND(SUM(gross_profit),2) AS profit,
SUM(orders) AS orders
FROM sales_facts
WHERE stat_date BETWEEN {{start_date}} AND {{end_date}}
AND org_id IN ({{org_scope}})
GROUP BY stat_date ORDER BY stat_date`,
			ScopeColumn: "org_id", RequireScope: true, Status: "active",
		},
		{
			Name: "区域销售排行", Code: "sales_region", Description: "按区域汇总销售额",
			SQLText: `SELECT p.name AS region, ROUND(SUM(f.revenue),2) AS revenue, SUM(f.orders) AS orders
FROM sales_facts f
JOIN organizations p ON p.id = f.region_id
WHERE f.stat_date BETWEEN {{start_date}} AND {{end_date}}
AND f.org_id IN ({{org_scope}})
GROUP BY p.name ORDER BY revenue DESC`,
			ScopeColumn: "org_id", RequireScope: true, Status: "active",
		},
		{
			Name: "门店销售排行", Code: "sales_store_ranking", Description: "按门店汇总销售表现",
			SQLText: `SELECT o.name AS store, o.manager_name AS manager,
ROUND(SUM(f.revenue),2) AS revenue, SUM(f.orders) AS orders,
ROUND(SUM(f.gross_profit),2) AS gross_profit
FROM sales_facts f
JOIN organizations o ON o.id = f.org_id
WHERE f.stat_date BETWEEN {{start_date}} AND {{end_date}}
AND f.org_id IN ({{org_scope}})
GROUP BY o.name, o.manager_name ORDER BY revenue DESC LIMIT 20`,
			ScopeColumn: "org_id", RequireScope: true, Status: "active",
		},
		{
			Name: "渠道销售构成", Code: "sales_channel", Description: "按销售渠道汇总销售额",
			SQLText: `SELECT channel AS channel, ROUND(SUM(revenue),2) AS revenue, SUM(orders) AS orders
FROM sales_facts
WHERE stat_date BETWEEN {{start_date}} AND {{end_date}}
AND org_id IN ({{org_scope}})
GROUP BY channel ORDER BY revenue DESC`,
			ScopeColumn: "org_id", RequireScope: true, Status: "active",
		},
	}
	for i := range datasets {
		var count int64
		db.Model(&model.Dataset{}).Where("code = ?", datasets[i].Code).Count(&count)
		if count == 0 {
			if err := db.Create(&datasets[i]).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultLayout() map[string]any {
	return map[string]any{
		"settings": map[string]any{
			"refresh_seconds": 0,
			"show_title":      true,
		},
		"widgets": []map[string]any{
			{
				"id": "kpi", "title": "核心经营指标", "subtitle": "当前权限范围 · 实时汇总",
				"chart_type": "kpi", "dataset_id": 1, "x": 0, "y": 0, "w": 12, "h": 2,
				"config": map[string]any{
					"columns": []map[string]any{
						{"field": "revenue", "label": "销售额", "unit": "元", "format": "currency"},
						{"field": "orders", "label": "订单量", "unit": "笔", "format": "integer"},
						{"field": "customers", "label": "客流", "unit": "人", "format": "integer"},
						{"field": "gross_margin", "label": "毛利率", "unit": "%", "format": "number"},
					},
				},
			},
			{
				"id": "trend", "title": "销售趋势", "subtitle": "销售额与毛利走势",
				"chart_type": "line", "dataset_id": 2, "x": 0, "y": 2, "w": 8, "h": 6,
				"config": map[string]any{
					"xField": "date",
					"yFields": []map[string]any{
						{"field": "revenue", "name": "销售额", "color": "#0F766E"},
						{"field": "profit", "name": "毛利", "color": "#F59E0B"},
					},
					"unit": "元",
				},
			},
			{
				"id": "region", "title": "区域销售排行", "subtitle": "按区域销售贡献",
				"chart_type": "bar", "dataset_id": 3, "x": 8, "y": 2, "w": 4, "h": 6,
				"config": map[string]any{
					"xField": "region", "yField": "revenue",
					"unit": "元", "color": "#14B8A6",
				},
			},
			{
				"id": "ranking", "title": "门店经营排行", "subtitle": "按销售额降序",
				"chart_type": "table", "dataset_id": 4, "x": 0, "y": 8, "w": 7, "h": 6,
				"config": map[string]any{
					"columns": []map[string]any{
						{"field": "store", "label": "门店"},
						{"field": "manager", "label": "店长"},
						{"field": "revenue", "label": "销售额", "format": "currency"},
						{"field": "orders", "label": "订单", "format": "integer"},
						{"field": "gross_profit", "label": "毛利", "format": "currency"},
					},
				},
			},
			{
				"id": "channel", "title": "渠道销售构成", "subtitle": "全渠道销售占比",
				"chart_type": "donut", "dataset_id": 5, "x": 7, "y": 8, "w": 5, "h": 6,
				"config": map[string]any{
					"xField": "channel", "yField": "revenue", "unit": "元",
				},
			},
		},
	}
}

func seedDashboards(db *gorm.DB) error {
	layout, _ := json.Marshal(defaultLayout())
	now := time.Now()
	dashboards := []model.Dashboard{
		{
			Name: "经营驾驶舱", Code: "sales_command_center", Description: "集团/区域/门店经营核心看板",
			Layout: string(layout), Theme: "light", Status: "published", Category: "经营分析", PublishedAt: &now,
		},
		{
			Name: "门店销售日报", Code: "store_sales_daily", Description: "面向店长的每日销售分析",
			Layout: string(layout), Theme: "light", Status: "published", Category: "销售分析", PublishedAt: &now,
		},
		{
			Name: "区域经营总览", Code: "region_overview", Description: "区域经理经营指标与门店排行",
			Layout: string(layout), Theme: "light", Status: "published", Category: "区域管理", PublishedAt: &now,
		},
	}
	for i := range dashboards {
		var count int64
		db.Model(&model.Dashboard{}).Where("code = ?", dashboards[i].Code).Count(&count)
		if count == 0 {
			if err := db.Create(&dashboards[i]).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedTemplates(db *gorm.DB) error {
	layout, _ := json.Marshal(defaultLayout())
	templates := []model.Template{
		{Name: "标准经营驾驶舱", Description: "指标 + 趋势 + 排行 + 渠道，适合总部和区域使用", Category: "经营分析", Cover: "emerald", Layout: string(layout), Theme: "light", BuiltIn: true},
		{Name: "门店日报模板", Description: "聚焦门店销售、毛利与渠道结构", Category: "销售分析", Cover: "amber", Layout: string(layout), Theme: "light", BuiltIn: true},
		{Name: "区域经营模板", Description: "区域视角下的门店排行和趋势分析", Category: "区域管理", Cover: "blue", Layout: string(layout), Theme: "light", BuiltIn: true},
	}
	for i := range templates {
		var count int64
		db.Model(&model.Template{}).Where("name = ? AND built_in = ?", templates[i].Name, true).Count(&count)
		if count == 0 {
			if err := db.Create(&templates[i]).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedPush(db *gorm.DB) error {
	templates := []model.PushTemplate{
		{
			Name: "企业微信销售数据卡片", Channel: "wecom_textcard",
			TitleTemplate:   "{{.Period}} 销售经营日报",
			ContentTemplate: "销售额 {{.Revenue}} 元\n订单量 {{.Orders}} 笔\n毛利额 {{.GrossProfit}} 元\n\n负责人：{{.RecipientName}}\n点击卡片查看所属范围完整数据。",
			StyleJSON:       `{"button_text":"查看经营看板","accent_color":"#0F766E","bg_color":"#F2FBF7"}`,
			Enabled:         true,
		},
		{
			Name: "企业微信纯文本日报", Channel: "wecom_text",
			TitleTemplate:   "【销售日报】{{.Period}}",
			ContentTemplate: "{{.DashboardName}}\n销售额：{{.Revenue}} 元\n订单量：{{.Orders}} 笔\n毛利额：{{.GrossProfit}} 元\n查看详情：{{.URL}}",
			StyleJSON:       `{"accent_color":"#0F766E"}`,
			Enabled:         true,
		},
		{
			Name: "微信公众号销售卡片", Channel: "wechat",
			TitleTemplate:   "{{.Period}} 销售经营日报",
			ContentTemplate: "销售额 {{.Revenue}} 元，订单 {{.Orders}} 笔，点击查看完整经营数据。",
			StyleJSON:       `{"button_text":"查看详情","accent_color":"#0F766E"}`,
			Enabled:         true,
		},
	}
	for i := range templates {
		var count int64
		db.Model(&model.PushTemplate{}).Where("name = ?", templates[i].Name).Count(&count)
		if count == 0 {
			if err := db.Create(&templates[i]).Error; err != nil {
				return err
			}
		}
	}
	var count int64
	db.Model(&model.Subscription{}).Count(&count)
	if count == 0 {
		var dashboard model.Dashboard
		db.Where("code = ?", "store_sales_daily").First(&dashboard)
		var tpl model.PushTemplate
		db.Where("channel = ?", "wecom_textcard").First(&tpl)
		recipients, _ := json.Marshal(RecipientSeed{UserIDs: []uint64{}, Roles: []string{"boss", "big_region_manager", "regional_manager", "store_manager"}, OrgIDs: []uint64{}})
		sub := model.Subscription{
			Name: "每日销售数据卡片", DashboardID: dashboard.ID, Cron: "0 9 * * *", Timezone: "Asia/Shanghai",
			Channels: `["wecom"]`, Recipients: string(recipients), TemplateID: &tpl.ID, Enabled: true,
		}
		if err := db.Create(&sub).Error; err != nil {
			return err
		}
	}
	return nil
}

type RecipientSeed struct {
	UserIDs []uint64 `json:"user_ids"`
	Roles   []string `json:"roles"`
	OrgIDs  []uint64 `json:"org_ids"`
}

func seedChannelConfigs(db *gorm.DB) error {
	configs := []model.ChannelConfig{
		{Channel: "wecom", Name: "企业微信", ConfigJSON: `{"corp_id":"","agent_id":0,"secret":"","webhook_url":"","dry_run":true}`, Enabled: false},
		{Channel: "wechat", Name: "微信公众号", ConfigJSON: `{"app_id":"","secret":"","template_id":"","dry_run":true}`, Enabled: false},
	}
	for i := range configs {
		var count int64
		db.Model(&model.ChannelConfig{}).Where("channel = ?", configs[i].Channel).Count(&count)
		if count == 0 {
			if err := db.Create(&configs[i]).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
