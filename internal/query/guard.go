package query

import (
	"errors"
	"strings"
)

var ErrNotReadOnly = errors.New("仅允许只读查询（SELECT / WITH / EXPLAIN）")

// bannedKeywords are rejected anywhere outside string literals and comments.
var bannedKeywords = map[string]struct{}{
	"INSERT": {}, "UPDATE": {}, "DELETE": {}, "REPLACE": {}, "MERGE": {},
	"DROP": {}, "ALTER": {}, "TRUNCATE": {}, "CREATE": {}, "RENAME": {},
	"GRANT": {}, "REVOKE": {}, "ATTACH": {}, "DETACH": {}, "PRAGMA": {},
	"VACUUM": {}, "REINDEX": {}, "CALL": {}, "EXEC": {}, "EXECUTE": {},
	"INTO": {}, "OUTFILE": {}, "DUMPFILE": {}, "LOAD": {}, "COPY": {},
	"SET": {}, "USE": {}, "COMMIT": {}, "ROLLBACK": {}, "BEGIN": {},
	"SAVEPOINT": {}, "LOCK": {}, "UNLOCK": {}, "KILL": {}, "SHUTDOWN": {},
	"PREPARE": {}, "DEALLOCATE": {}, "ANALYZE": {},
}

var allowedFirst = map[string]struct{}{
	"SELECT": {}, "WITH": {}, "EXPLAIN": {}, "SHOW": {}, "DESCRIBE": {}, "DESC": {},
}

// ValidateReadOnly performs a conservative single-statement read-only check.
// It tokenizes the SQL while skipping quoted strings, quoted identifiers and
// comments, so words inside literals do not trigger false positives.
func ValidateReadOnly(sql string) error {
	tokens, err := tokenize(sql)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return errors.New("SQL 不能为空")
	}
	first := strings.ToUpper(tokens[0])
	if _, ok := allowedFirst[first]; !ok {
		return ErrNotReadOnly
	}
	for _, t := range tokens[1:] {
		up := strings.ToUpper(t)
		if _, bad := bannedKeywords[up]; bad {
			return ErrNotReadOnly
		}
	}
	return nil
}

// tokenize returns bare word tokens, skipping literals/comments and handling
// statement terminators (more than one statement is rejected).
func tokenize(sql string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(sql)
	statements := 1
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		switch {
		case ch == '\'' || ch == '"' || ch == '`':
			flush()
			quote := ch
			i++
			for ; i < len(runes); i++ {
				if runes[i] == quote {
					// doubled quote is an escaped quote
					if i+1 < len(runes) && runes[i+1] == quote {
						i++
						continue
					}
					break
				}
			}
			if i >= len(runes) {
				return nil, errors.New("SQL 中存在未闭合的引号")
			}
		case ch == '-' && i+1 < len(runes) && runes[i+1] == '-':
			flush()
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case ch == '/' && i+1 < len(runes) && runes[i+1] == '*':
			flush()
			i += 2
			closed := false
			for i+1 < len(runes) {
				if runes[i] == '*' && runes[i+1] == '/' {
					closed = true
					i++
					break
				}
				i++
			}
			if !closed {
				return nil, errors.New("SQL 中存在未闭合的注释")
			}
		case ch == ';':
			flush()
			// Allow trailing semicolons / whitespace only.
			rest := strings.TrimSpace(string(runes[i+1:]))
			if rest != "" && rest != ";" {
				statements++
				if statements > 1 {
					return nil, errors.New("只允许执行单条 SQL 语句")
				}
			}
			i = len(runes)
		case ch == '$' && i+1 < len(runes) && runes[i+1] == '$':
			// PostgreSQL dollar-quoted string
			flush()
			i += 2
			closed := false
			for i+1 < len(runes) {
				if runes[i] == '$' && runes[i+1] == '$' {
					closed = true
					i++
					break
				}
				i++
			}
			if !closed {
				return nil, errors.New("SQL 中存在未闭合的字符串")
			}
		case isWordRune(ch):
			cur.WriteRune(ch)
		default:
			flush()
		}
	}
	flush()
	return tokens, nil
}

func isWordRune(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}
