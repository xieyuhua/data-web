package query

import (
	"fmt"
	"strings"

	"biaoge-bi/internal/model"
)

var allowedGrains = map[string]bool{
	"day": true, "week": true, "month": true, "quarter": true, "year": true,
}

// truncateDate renders a dialect-specific date-truncation expression for a
// quoted column. Only fixed SQL fragments are emitted; the column name has
// already been validated and quoted by the caller.
func truncateDate(quotedCol, grain, dialect string) (string, error) {
	g := strings.ToLower(strings.TrimSpace(grain))
	if !allowedGrains[g] {
		return "", fmt.Errorf("不支持的时间粒度 %q", grain)
	}
	switch dialect {
	case model.DSMySQL:
		switch g {
		case "day":
			return fmt.Sprintf("DATE(%s)", quotedCol), nil
		case "week":
			// Monday-based week start.
			return fmt.Sprintf("DATE_SUB(DATE(%s), INTERVAL WEEKDAY(%s) DAY)", quotedCol, quotedCol), nil
		case "month":
			return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m')", quotedCol), nil
		case "quarter":
			return fmt.Sprintf("CONCAT(YEAR(%s), '-Q', QUARTER(%s))", quotedCol, quotedCol), nil
		case "year":
			return fmt.Sprintf("DATE_FORMAT(%s, '%%Y')", quotedCol), nil
		}
	case model.DSPostgres:
		switch g {
		case "day":
			return fmt.Sprintf("DATE_TRUNC('day', %s)", quotedCol), nil
		case "week":
			return fmt.Sprintf("DATE_TRUNC('week', %s)", quotedCol), nil
		case "month":
			return fmt.Sprintf("TO_CHAR(DATE_TRUNC('month', %s), 'YYYY-MM')", quotedCol), nil
		case "quarter":
			return fmt.Sprintf("TO_CHAR(DATE_TRUNC('quarter', %s), 'YYYY-\"Q\"Q')", quotedCol), nil
		case "year":
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY')", quotedCol), nil
		}
	case model.DSOracle:
		switch g {
		case "day":
			return fmt.Sprintf("TRUNC(%s)", quotedCol), nil
		case "week":
			return fmt.Sprintf("TRUNC(%s, 'IW')", quotedCol), nil
		case "month":
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM')", quotedCol), nil
		case "quarter":
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY-\"Q\"Q')", quotedCol), nil
		case "year":
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY')", quotedCol), nil
		}
	default: // sqlite
		switch g {
		case "day":
			return fmt.Sprintf("DATE(%s)", quotedCol), nil
		case "week":
			return fmt.Sprintf("DATE(%s, 'weekday 1', '-7 days')", quotedCol), nil
		case "month":
			return fmt.Sprintf("STRFTIME('%%Y-%%m', %s)", quotedCol), nil
		case "quarter":
			return fmt.Sprintf("(STRFTIME('%%Y', %s) || '-Q' || ((CAST(STRFTIME('%%m', %s) AS INTEGER) + 2) / 3))", quotedCol, quotedCol), nil
		case "year":
			return fmt.Sprintf("STRFTIME('%%Y', %s)", quotedCol), nil
		}
	}
	return "", fmt.Errorf("不支持的时间粒度 %q", grain)
}
