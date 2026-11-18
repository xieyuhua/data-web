package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"biaoge-bi/internal/model"
)

var paramToken = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

var validParamName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// ParamNames returns the distinct placeholder names referenced in the SQL text.
func ParamNames(sqlText string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, m := range paramToken.FindAllStringSubmatch(sqlText, -1) {
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		out = append(out, m[1])
	}
	return out
}

// ParamSpec is a declared dataset parameter.
type ParamSpec struct {
	Name         string
	Type         string
	Required     bool
	DefaultValue string
}

func SpecsFromModel(params []model.DatasetParam) []ParamSpec {
	out := make([]ParamSpec, 0, len(params))
	for _, p := range params {
		out = append(out, ParamSpec{
			Name:         p.Name,
			Type:         p.Type,
			Required:     p.Required,
			DefaultValue: p.DefaultValue,
		})
	}
	return out
}

// BindParams replaces {{name}} placeholders with dialect-specific placeholders and
// returns the ordered argument list. Values are never interpolated into SQL text.
// Unknown placeholders are rejected; missing required values produce an error.
func BindParams(sqlText string, specs []ParamSpec, values map[string]string, dialect string) (string, []any, error) {
	byName := make(map[string]ParamSpec, len(specs))
	for _, s := range specs {
		if !validParamName.MatchString(s.Name) {
			return "", nil, fmt.Errorf("非法的参数名 %q", s.Name)
		}
		byName[s.Name] = s
	}

	// Detect placeholders and validate they are declared.
	matches := paramToken.FindAllStringSubmatch(sqlText, -1)
	for _, m := range matches {
		if _, ok := byName[m[1]]; !ok {
			return "", nil, fmt.Errorf("SQL 中使用了未声明的参数 {{%s}}", m[1])
		}
	}

	args := make([]any, 0, len(matches))
	idx := 0
	var missing []string

	replaced := paramToken.ReplaceAllStringFunc(sqlText, func(raw string) string {
		name := paramToken.FindStringSubmatch(raw)[1]
		spec := byName[name]
		val, provided := values[name]
		if !provided {
			val = spec.DefaultValue
		}
		if val == "" && spec.Required {
			missing = append(missing, name)
		}
		// List params expand to one placeholder per element so callers can write
		// `field IN ({{regions}})`. Values are still bound, never interpolated.
		if spec.Type == "list" {
			items := splitList(val)
			if len(items) == 0 {
				// Empty selection matches nothing (IN (NULL)). Callers that want
				// "no filter" should supply the full value list; dashboard filters
				// carry their default selection, and dataset preview falls back to
				// the declared defaults.
				return "NULL"
			}
			placeholders := make([]string, 0, len(items))
			for _, item := range items {
				args = append(args, convertParamValue(item, "string"))
				idx++
				switch dialect {
				case model.DSPostgres:
					placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
				case model.DSOracle:
					placeholders = append(placeholders, fmt.Sprintf(":%d", idx))
				default:
					placeholders = append(placeholders, "?")
				}
			}
			return strings.Join(placeholders, ", ")
		}

		converted := convertParamValue(val, spec.Type)
		args = append(args, converted)
		idx++
		if dialect == model.DSPostgres {
			return fmt.Sprintf("$%d", idx)
		}
		if dialect == model.DSOracle {
			return fmt.Sprintf(":%d", idx)
		}
		return "?"
	})

	if len(missing) > 0 {
		return "", nil, fmt.Errorf("缺少必填参数: %s", strings.Join(missing, ", "))
	}
	return replaced, args, nil
}

// splitList parses a JSON array or comma-separated string into trimmed values.
func splitList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if strings.HasPrefix(v, "[") {
		var arr []any
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			out := make([]string, 0, len(arr))
			for _, item := range arr {
				s := fmt.Sprintf("%v", item)
				if s != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func convertParamValue(v string, typ string) any {
	switch typ {
	case "number":
		if v == "" {
			return nil
		}
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		return v
	case "bool":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return v
		}
		return b
	case "date":
		if t, err := parseDate(v); err == nil {
			return t
		}
		return v
	default:
		return v
	}
}

func parseDate(v string) (time.Time, error) {
	layouts := []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"}
	for _, l := range layouts {
		if t, err := time.Parse(l, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid date")
}
