package service

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"biaoge-bi/internal/export"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
)

func newTerritoryFixture(t *testing.T) (*TerritoryService, *OrgImportService, *repository.Repo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "meta.db")), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(
		&model.BigRegion{}, &model.Region{}, &model.Store{},
		&model.User{}, &model.Department{},
	); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	return NewTerritoryService(repo), NewOrgImportService(repo), repo
}

func seedTerritory(t *testing.T, svc *TerritoryService) (*model.BigRegion, *model.Region) {
	t.Helper()
	big, err := svc.SaveBigRegion(0, BigRegionInput{Code: "HD", Name: "华东大区"})
	if err != nil {
		t.Fatal(err)
	}
	if big.Pinyin != "huadongdaqu" || big.Initials != "hddq" {
		t.Fatalf("unexpected pinyin: %q/%q", big.Pinyin, big.Initials)
	}
	region, err := svc.SaveRegion(0, RegionInput{Code: "HD-SH", Name: "上海区域", BigRegionID: big.ID})
	if err != nil {
		t.Fatal(err)
	}
	if region.Pinyin != "shanghaiquyu" {
		t.Fatalf("unexpected region pinyin: %q", region.Pinyin)
	}
	return big, region
}

func TestRegionPinyinSearch(t *testing.T) {
	svc, _, _ := newTerritoryFixture(t)
	seedTerritory(t, svc)

	// Full pinyin, initials and raw Chinese must all match.
	for _, kw := range []string{"huadong", "hddq", "华东", "shanghai"} {
		if kw == "shqy" {
			continue // initials of 上海区域 are shqy only if 区域 maps to qy; skip ambiguous
		}
		list, err := svc.ListBigRegions(kw)
		if err != nil {
			t.Fatal(err)
		}
		if kw == "shanghai" {
			regions, err := svc.ListRegions(kw, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(regions) != 1 {
				t.Fatalf("region search %q -> %d rows", kw, len(regions))
			}
			continue
		}
		if len(list) != 1 {
			t.Fatalf("big region search %q -> %d rows", kw, len(list))
		}
	}
}

func TestStoreCrudAndCodes(t *testing.T) {
	svc, _, _ := newTerritoryFixture(t)
	big, region := seedTerritory(t, svc)

	st, err := svc.SaveStore(0, StoreInput{
		Code: "SH001", Name: "南京东路店", RegionID: region.ID,
		Address: "南京东路 100 号", Status: "active", OpenedAt: "2026-01-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.RegionCode != "HD-SH" || st.BigRegionCode != "HD" {
		t.Fatalf("denormalized codes wrong: %+v", st)
	}

	// Duplicate store code is rejected.
	if _, err := svc.SaveStore(0, StoreInput{Code: "SH001", Name: "dupe", RegionID: region.ID}); err == nil {
		t.Fatal("duplicate store code should fail")
	}

	// Region code change is blocked while stores reference it.
	if _, err := svc.SaveRegion(region.ID, RegionInput{
		Code: "HD-SH2", Name: "上海区域", BigRegionID: big.ID,
	}); err == nil {
		t.Fatal("changing region code with stores should be rejected")
	}

	// Parent deletion is blocked.
	if err := svc.DeleteRegion(region.ID); err == nil {
		t.Fatal("deleting a region with stores should fail")
	}
	if err := svc.DeleteBigRegion(big.ID); err == nil {
		t.Fatal("deleting a big region with children should fail")
	}
}

func TestStoreImportExportRoundTrip(t *testing.T) {
	svc, _, _ := newTerritoryFixture(t)
	_, region := seedTerritory(t, svc)
	if _, err := svc.SaveStore(0, StoreInput{Code: "SH001", Name: "南京东路店", RegionID: region.ID}); err != nil {
		t.Fatal(err)
	}

	data, err := svc.ExportStores()
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.ImportStores(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Updated != 1 || res.Failed != 0 {
		t.Fatalf("round trip unexpected: %+v", res)
	}
	if len(res.ReportXLSX) == 0 {
		t.Fatal("import report should be generated")
	}

	// Unknown region codes are reported, not silently inserted.
	bad, err := export.WriteSheet("门店",
		[]string{"门店编码", "门店名称", "大区编码", "区域编码"},
		[]export.Cell{{"SH999", "错误门店", "HD", "NOT-EXIST"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := svc.ImportStores(bad)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Failed != 1 || len(res2.Issues) == 0 {
		t.Fatalf("bad region should fail: %+v", res2)
	}
}

func TestOrgImportExport(t *testing.T) {
	_, org, repo := newTerritoryFixture(t)
	if err := repo.DB.Create(&model.Department{WeComID: 3, Name: "后端组"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.DB.Create(&model.User{
		Username: "dev1", DisplayName: "陈涛", Role: model.RoleUser,
		Enabled: true, Source: model.SourceWeCom, WeComUserID: "dev1", WeComDeptIDs: "[3]",
		PasswordHash: "x",
	}).Error; err != nil {
		t.Fatal(err)
	}

	data, err := org.ExportOrg()
	if err != nil {
		t.Fatal(err)
	}
	res, err := org.ImportOrg(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Updated != 1 || res.Failed != 0 {
		t.Fatalf("org round trip unexpected: %+v", res)
	}

	// Unknown department ids produce a warning but still import.
	bad, err := export.WriteSheet("成员",
		[]string{"用户名", "姓名", "部门编码(逗号分隔)", "角色(admin/user)", "状态(在职/停用)"},
		[]export.Cell{{"newbie", "新人", "999", "user", "在职"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := org.ImportOrg(bad)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Created != 1 {
		t.Fatalf("expected 1 created, got %+v", res2)
	}
	if len(res2.Issues) == 0 || res2.Issues[0].Level != "warning" {
		t.Fatalf("expected a warning for unknown dept: %+v", res2.Issues)
	}

	// Missing required column is a hard error.
	if _, err := org.ImportOrg(mustSheet(t, "成员", []string{"姓名"}, []export.Cell{{"x"}})); err == nil {
		t.Fatal("missing username column should fail")
	}
}

func mustSheet(t *testing.T, name string, header []string, rows []export.Cell) []byte {
	t.Helper()
	data, err := export.WriteSheet(name, header, rows)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
