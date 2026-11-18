package query

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"biaoge-bi/internal/model"
)

// ResultSet is the transport shape returned to the frontend and reused for exports.
type ResultSet struct {
	Columns   []model.ColumnMeta `json:"columns"`
	Rows      [][]any            `json:"rows"`
	RowCount  int                `json:"rowCount"`
	Truncated bool               `json:"truncated"`
	ElapsedMs int64              `json:"elapsedMs"`
}

func BuildColumns(types []*sql.ColumnType) []model.ColumnMeta {
	out := make([]model.ColumnMeta, 0, len(types))
	for _, ct := range types {
		nullable, _ := ct.Nullable()
		out = append(out, model.ColumnMeta{
			Name:        ct.Name(),
			DBType:      ct.DatabaseTypeName(),
			LogicalType: logicalType(ct.DatabaseTypeName(), ct.ScanType()),
			Nullable:    nullable,
		})
	}
	return out
}

func logicalType(dbType string, scanType any) string {
	t := strings.ToLower(dbType)
	switch t {
	case "bool", "boolean", "tinyint(1)":
		return "bool"
	case "date":
		return "date"
	case "datetime", "timestamp", "timestamptz", "datetime64":
		return "datetime"
	}
	if scanType != nil {
		st := strings.ToLower(fmt.Sprintf("%T", scanType))
		switch {
		case strings.Contains(st, "time.Time"):
			return "datetime"
		case strings.Contains(st, "int"), strings.Contains(st, "uint"),
			strings.Contains(st, "float"), strings.Contains(st, "decimal"):
			return "number"
		case strings.Contains(st, "bool"):
			return "bool"
		}
	}
	switch {
	case strings.Contains(t, "int"), strings.Contains(t, "float"), strings.Contains(t, "double"),
		strings.Contains(t, "decimal"), strings.Contains(t, "numeric"), strings.Contains(t, "real"),
		strings.Contains(t, "number"):
		return "number"
	case strings.Contains(t, "byte"), strings.Contains(t, "blob"):
		return "string"
	}
	return "string"
}

// RefineColumns upgrades weak logical types using sampled values. Wrapped
// subqueries (the chart builder) often lose driver type information, so numeric
// aggregate columns would otherwise be reported as strings.
func RefineColumns(cols []model.ColumnMeta, rows [][]any) {
	for i := range cols {
		if cols[i].LogicalType != "string" || len(rows) == 0 {
			continue
		}
		numbers, booleans, nonNull := 0, 0, 0
		for _, row := range rows {
			if i >= len(row) || row[i] == nil {
				continue
			}
			nonNull++
			switch row[i].(type) {
			case int64, float64, int32, int, int16, int8, uint64:
				numbers++
			case bool:
				booleans++
			}
		}
		if nonNull == 0 || nonNull != len(rows) {
			// Only promote when every value is numeric/boolean; mixed columns stay strings.
			continue
		}
		switch {
		case numbers == nonNull:
			cols[i].LogicalType = "number"
		case booleans == nonNull:
			cols[i].LogicalType = "bool"
		}
	}
}

// ScanRows drains a result set into normalized values. It stops after maxRows,
// marking truncated when at least one extra row exists.
func ScanRows(rows *sql.Rows, maxRows int) ([][]any, bool, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, false, err
	}
	n := len(cols)
	result := make([][]any, 0, 64)
	truncated := false
	for rows.Next() {
		if maxRows > 0 && len(result) >= maxRows {
			truncated = true
			break
		}
		raw := make([]any, n)
		ptrs := make([]any, n)
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, false, err
		}
		for i, v := range raw {
			raw[i] = NormalizeValue(v)
		}
		result = append(result, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return result, truncated, nil
}

// NormalizeValue converts driver values into JSON-friendly types.
func NormalizeValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339)
	case int64, int32, int, float64, float32, bool, string:
		return x
	case json.Number:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}
