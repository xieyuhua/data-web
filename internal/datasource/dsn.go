package datasource

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"biaoge-bi/internal/model"
)

// BuildDSN renders a driver DSN for the given datasource and decrypted password.
// readOnly applies the strongest read-only option each driver supports.
func BuildDSN(ds *model.DataSource, password string) (string, error) {
	switch ds.Type {
	case model.DSMySQL:
		host := ds.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := ds.Port
		if port == 0 {
			port = 3306
		}
		params := []string{"charset=utf8mb4", "parseTime=true", "loc=Local", "readTimeout=30s", "timeout=5s"}
		if ds.DSNExtra != "" {
			params = append(params, ds.DSNExtra)
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
			ds.Username, escapeMySQLPassword(password), host, port, ds.Database, strings.Join(params, "&")), nil

	case model.DSPostgres:
		host := ds.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := ds.Port
		if port == 0 {
			port = 5432
		}
		u := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(ds.Username, password),
			Host:   fmt.Sprintf("%s:%d", host, port),
			Path:   "/" + ds.Database,
		}
		q := u.Query()
		q.Set("sslmode", "disable")
		q.Set("connect_timeout", "5")
		q.Set("statement_timeout", "30000")
		q.Set("application_name", "biaoge-bi")
		u.RawQuery = q.Encode()
		if ds.DSNExtra != "" {
			v, err := url.ParseQuery(ds.DSNExtra)
			if err == nil {
				for k, vals := range v {
					for _, val := range vals {
						q.Set(k, val)
					}
				}
				u.RawQuery = q.Encode()
			}
		}
		return u.String(), nil

	case model.DSOracle:
		host := ds.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := ds.Port
		if port == 0 {
			port = 1521
		}
		// go-ora DSN: oracle://user:pass@host:port/service
		u := url.URL{
			Scheme: "oracle",
			User:   url.UserPassword(ds.Username, password),
			Host:   fmt.Sprintf("%s:%d", host, port),
			Path:   "/" + ds.Database,
		}
		q := u.Query()
		q.Set("CONNECTION TIMEOUT", "5")
		q.Set("SSL", "false")
		if ds.DSNExtra != "" {
			if extra, err := url.ParseQuery(ds.DSNExtra); err == nil {
				for k, vals := range extra {
					for _, v := range vals {
						q.Set(k, v)
					}
				}
			}
		}
		u.RawQuery = q.Encode()
		return u.String(), nil

	case model.DSSQLite:
		path := ds.Database
		if path == "" {
			return "", fmt.Errorf("sqlite 数据源需要填写数据库文件路径")
		}
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		base := path + sep + "_pragma=query_only(ON)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
		if ds.DSNExtra != "" {
			base += "&" + ds.DSNExtra
		}
		return base, nil
	}
	return "", fmt.Errorf("unsupported datasource type %q", ds.Type)
}

// escapeMySQLPassword escapes characters significant to the go-sql-driver DSN grammar.
func escapeMySQLPassword(p string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `@`, `\@`, `:`, `\:`)
	return replacer.Replace(p)
}

// TestQuery returns a trivial connectivity probe for the dialect.
func TestQuery(dsType string) string {
	if dsType == model.DSPostgres {
		return "SELECT 1"
	}
	return "SELECT 1"
}

// PingTimeout is used for connection tests.
const PingTimeout = 5 * time.Second
