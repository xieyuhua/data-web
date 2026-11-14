package service

import (
	"database/sql"
	"strings"
	"testing"
)

func TestOracleDriverRegistered(t *testing.T) {
	for _, driver := range sql.Drivers() {
		if driver == "oracle" {
			return
		}
	}
	t.Fatal("未注册 database/sql 驱动 oracle")
}

func TestValidateDriverSupportsOracle(t *testing.T) {
	if err := validateDriver("oracle"); err != nil {
		t.Fatalf("oracle 驱动应受支持: %v", err)
	}
	if err := validateDriver("db2"); err == nil {
		t.Fatal("不支持的驱动应返回错误")
	}
}

func TestMaskOracleDSN(t *testing.T) {
	masked := maskDSN("oracle://bi_user:secret123@10.0.0.8:1521/ORCLPDB1")
	if strings.Contains(masked, "secret123") {
		t.Fatalf("DSN 密码未掩码: %s", masked)
	}
	if !strings.Contains(masked, "bi_user") || !strings.Contains(masked, "ORCLPDB1") {
		t.Fatalf("掩码不应破坏用户名与服务名: %s", masked)
	}
}
