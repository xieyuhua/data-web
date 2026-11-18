package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"biaoge-bi/internal/datasource"
	"biaoge-bi/internal/model"
)

const (
	PreviewMaxRows = 1000
	ChartMaxRows   = 5000
	ExportMaxRows  = 100000
)

var ErrTruncatedValue = errors.New("结果集超过上限")

// Executor runs guarded read-only SQL against external data sources.
type Executor struct {
	Manager *datasource.Manager
}

type ExecuteOptions struct {
	Dataset    *model.Dataset
	Params     map[string]string
	MaxRows    int // hard cap, wraps the SQL
	Timeout    time.Duration
	LimitValue int // optional LIMIT applied inside wrapper (0 = none)
}

// ExecuteDataset binds parameters and runs the dataset SQL inside a read-only
// transaction with a wrapped LIMIT and a hard timeout.
func (e *Executor) ExecuteDataset(ctx context.Context, opts ExecuteOptions) (*ResultSet, error) {
	if opts.Dataset == nil {
		return nil, errors.New("dataset is required")
	}
	if err := ValidateReadOnly(opts.Dataset.SQLText); err != nil {
		return nil, err
	}
	db, dsType, err := e.Manager.Get(ctx, opts.Dataset.DataSourceID)
	if err != nil {
		return nil, err
	}

	sqlText, args, err := BindParams(opts.Dataset.SQLText, SpecsFromModel(opts.Dataset.Params), opts.Params, dsType)
	if err != nil {
		return nil, err
	}

	maxRows := opts.MaxRows
	if maxRows <= 0 {
		maxRows = ChartMaxRows
	}
	if maxRows > ExportMaxRows {
		maxRows = ExportMaxRows
	}

	wrapped := fmt.Sprintf("SELECT * FROM (%s) AS bi_q LIMIT %d", sqlText, maxRows+1)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	qctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	tx, err := db.BeginTx(qctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("开始只读事务失败: %w", err)
	}
	defer tx.Rollback() // always rollback; read-only anyway

	rows, err := tx.QueryContext(qctx, wrapped, args...)
	if err != nil {
		return nil, normalizeExecError(err, qctx)
	}
	defer rows.Close()

	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	data, truncatedExtra, err := ScanRows(rows, maxRows+1)
	if err != nil {
		return nil, normalizeExecError(err, qctx)
	}
	truncated := truncatedExtra
	if len(data) > maxRows {
		data = data[:maxRows]
		truncated = true
	}
	colMeta := BuildColumns(cols)
	RefineColumns(colMeta, data)
	return &ResultSet{
		Columns:   colMeta,
		Rows:      data,
		RowCount:  len(data),
		Truncated: truncated,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

// ExecuteRaw runs a fully-built, trusted SQL string (used by the chart builder).
// The caller must have produced identifiers from a validated allowlist.
func (e *Executor) ExecuteRaw(ctx context.Context, dataSourceID uint, sqlText string, args []any, maxRows int, timeout time.Duration) (*ResultSet, error) {
	if err := ValidateReadOnly(sqlText); err != nil {
		return nil, err
	}
	db, _, err := e.Manager.Get(ctx, dataSourceID)
	if err != nil {
		return nil, err
	}
	if maxRows <= 0 {
		maxRows = ChartMaxRows
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	qctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()

	wrapped := fmt.Sprintf("SELECT * FROM (%s) AS bi_q LIMIT %d", sqlText, maxRows+1)

	tx, err := db.BeginTx(qctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("开始只读事务失败: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(qctx, wrapped, args...)
	if err != nil {
		return nil, normalizeExecError(err, qctx)
	}
	defer rows.Close()

	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	data, _, err := ScanRows(rows, maxRows+1)
	if err != nil {
		return nil, normalizeExecError(err, qctx)
	}
	truncated := false
	if len(data) > maxRows {
		data = data[:maxRows]
		truncated = true
	}
	colMeta := BuildColumns(cols)
	RefineColumns(colMeta, data)
	return &ResultSet{
		Columns:   colMeta,
		Rows:      data,
		RowCount:  len(data),
		Truncated: truncated,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

func normalizeExecError(err error, ctx context.Context) error {
	if ctx.Err() == context.DeadlineExceeded {
		return errors.New("查询超时，请优化 SQL 或缩小数据范围")
	}
	msg := err.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return errors.New("查询失败: " + msg)
}

var identifierRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// ValidIdentifier reports whether s is a safe, unquoted column identifier.
// Dataset column names come from database metadata and must match this pattern.
func ValidIdentifier(s string) bool { return identifierRe.MatchString(s) }

// ValidAlias accepts any short printable label (including CJK) for SELECT aliases.
func ValidAlias(s string) bool {
	if s == "" || len([]rune(s)) > 64 {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '"' || r == '`' || r == '\\' || r == '\'' {
			return false
		}
	}
	return true
}

// QuoteIdentifier quotes a validated column identifier per dialect.
func QuoteIdentifier(name, dialect string) (string, error) {
	if !ValidIdentifier(name) {
		return "", fmt.Errorf("非法字段名 %q", name)
	}
	if dialect == model.DSMySQL {
		return "`" + name + "`", nil
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`, nil
}
