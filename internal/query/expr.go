package query

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Calculated metrics are post-aggregation expressions such as
//
//	"sales_sum / NULLIF(target_sum, 0)"
//	"profit_sum / NULLIF(sales_sum, 0) * 100"
//
// They are parsed into a small AST and re-emitted as SQL. Only identifiers that
// match a previously declared metric alias, numeric literals, the four arithmetic
// operators, parentheses and a whitelist of functions are allowed. Nothing is
// ever interpolated from user input directly, so this cannot inject SQL.

type exprOp int

const (
	opNum exprOp = iota
	opIdent
	opAdd
	opSub
	opMul
	opDiv
	opNeg
	opCall
)

var allowedFuncs = map[string]bool{
	"NULLIF": true, "COALESCE": true, "ABS": true, "ROUND": true,
	"+": true, "-": true, "*": true, "/": true,
}

type exprNode struct {
	op    exprOp
	num   float64
	ident string
	fn    string
	args  []*exprNode
}

type exprParser struct {
	tokens []string
	pos    int
	names  map[string]bool
}

// Identifiers may contain CJK characters so Chinese metric aliases can be
// referenced directly, e.g. "实际销售 / NULLIF(基础目标, 0)".
var exprTokenRe = regexp.MustCompile(`[\p{L}_][\p{L}\p{N}_]*|\d+(?:\.\d+)?|[()+\-*/,]`)

func tokenizeExpr(s string) ([]string, error) {
	cleaned := strings.TrimSpace(s)
	if cleaned == "" {
		return nil, errors.New("表达式不能为空")
	}
	tokens := exprTokenRe.FindAllString(cleaned, -1)
	// Reject anything the tokenizer silently skipped (quotes, semicolons, ...).
	if strings.Join(tokens, "") != strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(cleaned) {
		return nil, errors.New("表达式包含非法字符")
	}
	return tokens, nil
}

// ValidateCalcExpr checks syntax and that every identifier is a known metric alias.
func ValidateCalcExpr(expr string, aliases []string) error {
	names := make(map[string]bool, len(aliases))
	for _, a := range aliases {
		names[a] = true
	}
	tokens, err := tokenizeExpr(expr)
	if err != nil {
		return err
	}
	p := &exprParser{tokens: tokens, names: names}
	if _, err := p.parseExpr(); err != nil {
		return err
	}
	if p.pos != len(p.tokens) {
		return fmt.Errorf("表达式在 %q 处存在多余内容", p.tokens[p.pos])
	}
	return nil
}

// BuildCalcSQL validates the expression and renders it for the given dialect.
// Aliases are quoted so they resolve against the aggregate SELECT list.
func BuildCalcSQL(expr string, aliases []string, dialect string) (string, error) {
	names := make(map[string]bool, len(aliases))
	for _, a := range aliases {
		names[a] = true
	}
	tokens, err := tokenizeExpr(expr)
	if err != nil {
		return "", err
	}
	p := &exprParser{tokens: tokens, names: names}
	node, err := p.parseExpr()
	if err != nil {
		return "", err
	}
	if p.pos != len(p.tokens) {
		return "", fmt.Errorf("表达式在 %q 处存在多余内容", p.tokens[p.pos])
	}
	var b strings.Builder
	if err := renderExpr(&b, node, dialect); err != nil {
		return "", err
	}
	return b.String(), nil
}

func (p *exprParser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *exprParser) next() string {
	t := p.peek()
	p.pos++
	return t
}

func (p *exprParser) parseExpr() (*exprNode, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for p.peek() == "+" || p.peek() == "-" {
		op := p.next()
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		kind := opAdd
		if op == "-" {
			kind = opSub
		}
		left = &exprNode{op: kind, args: []*exprNode{left, right}}
	}
	return left, nil
}

func (p *exprParser) parseTerm() (*exprNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek() == "*" || p.peek() == "/" {
		op := p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		kind := opMul
		if op == "/" {
			kind = opDiv
		}
		left = &exprNode{op: kind, args: []*exprNode{left, right}}
	}
	return left, nil
}

func (p *exprParser) parseUnary() (*exprNode, error) {
	if p.peek() == "-" {
		p.next()
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &exprNode{op: opNeg, args: []*exprNode{inner}}, nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (*exprNode, error) {
	tok := p.peek()
	switch {
	case tok == "":
		return nil, errors.New("表达式不完整")
	case tok == "(":
		p.next()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, errors.New("缺少右括号")
		}
		return inner, nil
	case isNumber(tok):
		p.next()
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return nil, fmt.Errorf("非法数字 %q", tok)
		}
		return &exprNode{op: opNum, num: v}, nil
	case isIdentifier(tok):
		p.next()
		if p.peek() == "(" {
			fn := strings.ToUpper(tok)
			if !allowedFuncs[fn] {
				return nil, fmt.Errorf("不支持的函数 %q", tok)
			}
			p.next() // consume "("
			var args []*exprNode
			if p.peek() != ")" {
				for {
					a, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.peek() == "," {
						p.next()
						continue
					}
					break
				}
			}
			if p.next() != ")" {
				return nil, errors.New("缺少右括号")
			}
			return &exprNode{op: opCall, fn: fn, args: args}, nil
		}
		if !p.names[tok] {
			return nil, fmt.Errorf("未知指标 %q（请先添加同别名的基础指标）", tok)
		}
		return &exprNode{op: opIdent, ident: tok}, nil
	default:
		return nil, fmt.Errorf("表达式存在非法符号 %q", tok)
	}
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= '0' && c <= '9'
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func renderExpr(b *strings.Builder, n *exprNode, dialect string) error {
	switch n.op {
	case opNum:
		b.WriteString(strconv.FormatFloat(n.num, 'f', -1, 64))
	case opIdent:
		// Identifiers here are metric aliases (user-authored, may be Chinese),
		// not database column names, so validate with ValidAlias and quote.
		if !ValidAlias(n.ident) {
			return fmt.Errorf("非法指标别名 %q", n.ident)
		}
		b.WriteString(quoteAlias(n.ident, dialect))
	case opAdd:
		b.WriteString("(")
		if err := renderExpr(b, n.args[0], dialect); err != nil {
			return err
		}
		b.WriteString(" + ")
		if err := renderExpr(b, n.args[1], dialect); err != nil {
			return err
		}
		b.WriteString(")")
	case opSub:
		b.WriteString("(")
		if err := renderExpr(b, n.args[0], dialect); err != nil {
			return err
		}
		b.WriteString(" - ")
		if err := renderExpr(b, n.args[1], dialect); err != nil {
			return err
		}
		b.WriteString(")")
	case opMul:
		b.WriteString("(")
		if err := renderExpr(b, n.args[0], dialect); err != nil {
			return err
		}
		b.WriteString(" * ")
		if err := renderExpr(b, n.args[1], dialect); err != nil {
			return err
		}
		b.WriteString(")")
	case opDiv:
		// Division always guards against zero so a report never fails on empty
		// rows. When the divisor is already an explicit NULLIF/COALESCE call we
		// keep it as-is instead of nesting another guard.
		b.WriteString("(")
		if err := renderExpr(b, n.args[0], dialect); err != nil {
			return err
		}
		b.WriteString(" / ")
		if n.args[1].op == opCall && (n.args[1].fn == "NULLIF" || n.args[1].fn == "COALESCE") {
			if err := renderExpr(b, n.args[1], dialect); err != nil {
				return err
			}
			b.WriteString(")")
			return nil
		}
		b.WriteString("NULLIF(")
		if err := renderExpr(b, n.args[1], dialect); err != nil {
			return err
		}
		b.WriteString(", 0))")
	case opNeg:
		b.WriteString("(-")
		if err := renderExpr(b, n.args[0], dialect); err != nil {
			return err
		}
		b.WriteString(")")
	case opCall:
		b.WriteString(n.fn)
		b.WriteString("(")
		for i, a := range n.args {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := renderExpr(b, a, dialect); err != nil {
				return err
			}
		}
		b.WriteString(")")
	default:
		return errors.New("表达式节点无效")
	}
	return nil
}
