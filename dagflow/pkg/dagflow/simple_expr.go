package dagflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

type exprTokenType int

const (
	exprTokEOF exprTokenType = iota
	exprTokIdent
	exprTokString
	exprTokNumber
	exprTokBool
	exprTokNull
	exprTokAnd
	exprTokOr
	exprTokNot
	exprTokEQ
	exprTokNE
	exprTokGT
	exprTokGTE
	exprTokLT
	exprTokLTE
	exprTokLParen
	exprTokRParen
)

type exprToken struct {
	typ exprTokenType
	lit string
}

type exprLexer struct {
	s string
	i int
}

func lexSimpleBoolExpr(s string) ([]exprToken, error) {
	l := &exprLexer{s: s}
	toks := []exprToken{}
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		if tok.typ == exprTokEOF {
			return toks, nil
		}
	}
}

func (l *exprLexer) next() (exprToken, error) {
	for l.i < len(l.s) && unicode.IsSpace(rune(l.s[l.i])) {
		l.i++
	}
	if l.i >= len(l.s) {
		return exprToken{typ: exprTokEOF}, nil
	}
	if strings.HasPrefix(l.s[l.i:], "&&") {
		l.i += 2
		return exprToken{typ: exprTokAnd, lit: "&&"}, nil
	}
	if strings.HasPrefix(l.s[l.i:], "||") {
		l.i += 2
		return exprToken{typ: exprTokOr, lit: "||"}, nil
	}
	if strings.HasPrefix(l.s[l.i:], "==") {
		l.i += 2
		return exprToken{typ: exprTokEQ, lit: "=="}, nil
	}
	if strings.HasPrefix(l.s[l.i:], "!=") {
		l.i += 2
		return exprToken{typ: exprTokNE, lit: "!="}, nil
	}
	if strings.HasPrefix(l.s[l.i:], ">=") {
		l.i += 2
		return exprToken{typ: exprTokGTE, lit: ">="}, nil
	}
	if strings.HasPrefix(l.s[l.i:], "<=") {
		l.i += 2
		return exprToken{typ: exprTokLTE, lit: "<="}, nil
	}
	switch l.s[l.i] {
	case '!':
		l.i++
		return exprToken{typ: exprTokNot, lit: "!"}, nil
	case '>':
		l.i++
		return exprToken{typ: exprTokGT, lit: ">"}, nil
	case '<':
		l.i++
		return exprToken{typ: exprTokLT, lit: "<"}, nil
	case '(':
		l.i++
		return exprToken{typ: exprTokLParen, lit: "("}, nil
	case ')':
		l.i++
		return exprToken{typ: exprTokRParen, lit: ")"}, nil
	case '`', '\'', '"':
		return l.scanString(l.s[l.i])
	}
	c := rune(l.s[l.i])
	if c == '-' || unicode.IsDigit(c) {
		return l.scanNumber()
	}
	if isExprIdentStart(c) {
		start := l.i
		l.i++
		for l.i < len(l.s) && isExprIdentPart(rune(l.s[l.i])) {
			l.i++
		}
		lit := l.s[start:l.i]
		switch lit {
		case "true", "false":
			return exprToken{typ: exprTokBool, lit: lit}, nil
		case "nil", "null":
			return exprToken{typ: exprTokNull, lit: lit}, nil
		default:
			return exprToken{typ: exprTokIdent, lit: lit}, nil
		}
	}
	return exprToken{}, fmt.Errorf("unsupported expression character %q at offset %d", l.s[l.i], l.i)
}

func (l *exprLexer) scanString(quote byte) (exprToken, error) {
	start := l.i
	l.i++
	var b strings.Builder
	for l.i < len(l.s) {
		c := l.s[l.i]
		l.i++
		if c == quote {
			if quote == '`' {
				return exprToken{typ: exprTokString, lit: l.s[start+1 : l.i-1]}, nil
			}
			return exprToken{typ: exprTokString, lit: b.String()}, nil
		}
		if quote != '`' && c == '\\' {
			if l.i >= len(l.s) {
				return exprToken{}, fmt.Errorf("unterminated escape in string")
			}
			esc := l.s[l.i]
			l.i++
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\', '"', '\'':
				b.WriteByte(esc)
			default:
				b.WriteByte(esc)
			}
			continue
		}
		b.WriteByte(c)
	}
	return exprToken{}, fmt.Errorf("unterminated string literal")
}

func (l *exprLexer) scanNumber() (exprToken, error) {
	start := l.i
	if l.s[l.i] == '-' {
		l.i++
	}
	digits := 0
	for l.i < len(l.s) && unicode.IsDigit(rune(l.s[l.i])) {
		digits++
		l.i++
	}
	if l.i < len(l.s) && l.s[l.i] == '.' {
		l.i++
		for l.i < len(l.s) && unicode.IsDigit(rune(l.s[l.i])) {
			digits++
			l.i++
		}
	}
	if digits == 0 {
		return exprToken{}, fmt.Errorf("invalid number at offset %d", start)
	}
	return exprToken{typ: exprTokNumber, lit: l.s[start:l.i]}, nil
}

func isExprIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isExprIdentPart(r rune) bool {
	return r == '_' || r == '-' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

type simpleExprParser struct {
	toks  []exprToken
	pos   int
	facts map[string]any
}

func evalSimpleBoolExpression(expr string, facts map[string]any) (bool, bool, error) {
	toks, err := lexSimpleBoolExpr(expr)
	if err != nil {
		return false, false, err
	}
	p := &simpleExprParser{toks: toks, facts: facts}
	v, err := p.parseOr()
	if err != nil {
		return false, false, err
	}
	if p.peek().typ != exprTokEOF {
		return false, false, fmt.Errorf("unexpected token %q", p.peek().lit)
	}
	b, ok := valueAsBool(v)
	if !ok {
		return false, false, fmt.Errorf("expression did not return a boolean")
	}
	return b, true, nil
}

func (p *simpleExprParser) parseOr() (any, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.match(exprTokOr) {
		lb, ok := valueAsBool(left)
		if !ok {
			return nil, fmt.Errorf("left side of || is not boolean")
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		rb, ok := valueAsBool(right)
		if !ok {
			return nil, fmt.Errorf("right side of || is not boolean")
		}
		left = lb || rb
	}
	return left, nil
}

func (p *simpleExprParser) parseAnd() (any, error) {
	left, err := p.parseCompare()
	if err != nil {
		return nil, err
	}
	for p.match(exprTokAnd) {
		lb, ok := valueAsBool(left)
		if !ok {
			return nil, fmt.Errorf("left side of && is not boolean")
		}
		right, err := p.parseCompare()
		if err != nil {
			return nil, err
		}
		rb, ok := valueAsBool(right)
		if !ok {
			return nil, fmt.Errorf("right side of && is not boolean")
		}
		left = lb && rb
	}
	return left, nil
}

func (p *simpleExprParser) parseCompare() (any, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	tok := p.peek()
	switch tok.typ {
	case exprTokEQ, exprTokNE, exprTokGT, exprTokGTE, exprTokLT, exprTokLTE:
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return compareExprValues(left, right, tok.typ)
	default:
		return left, nil
	}
}

func (p *simpleExprParser) parseUnary() (any, error) {
	if p.match(exprTokNot) {
		v, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		b, ok := valueAsBool(v)
		if !ok {
			return nil, fmt.Errorf("! operand is not boolean")
		}
		return !b, nil
	}
	return p.parsePrimary()
}

func (p *simpleExprParser) parsePrimary() (any, error) {
	tok := p.peek()
	p.pos++
	switch tok.typ {
	case exprTokIdent:
		return resolveFactPath(p.facts, tok.lit), nil
	case exprTokString:
		return tok.lit, nil
	case exprTokNumber:
		n, err := strconv.ParseFloat(tok.lit, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case exprTokBool:
		return tok.lit == "true", nil
	case exprTokNull:
		return nil, nil
	case exprTokLParen:
		v, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(exprTokRParen) {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return v, nil
	default:
		return nil, fmt.Errorf("unexpected token %q", tok.lit)
	}
}

func (p *simpleExprParser) peek() exprToken {
	if p.pos >= len(p.toks) {
		return exprToken{typ: exprTokEOF}
	}
	return p.toks[p.pos]
}

func (p *simpleExprParser) match(tt exprTokenType) bool {
	if p.peek().typ == tt {
		p.pos++
		return true
	}
	return false
}

func compareExprValues(left, right any, op exprTokenType) (bool, error) {
	switch op {
	case exprTokEQ:
		return exprEqual(left, right), nil
	case exprTokNE:
		return !exprEqual(left, right), nil
	}
	lf, lok := valueAsFloat(left)
	rf, rok := valueAsFloat(right)
	if lok && rok {
		switch op {
		case exprTokGT:
			return lf > rf, nil
		case exprTokGTE:
			return lf >= rf, nil
		case exprTokLT:
			return lf < rf, nil
		case exprTokLTE:
			return lf <= rf, nil
		}
	}
	ls, lok := valueAsString(left)
	rs, rok := valueAsString(right)
	if lok && rok {
		switch op {
		case exprTokGT:
			return ls > rs, nil
		case exprTokGTE:
			return ls >= rs, nil
		case exprTokLT:
			return ls < rs, nil
		case exprTokLTE:
			return ls <= rs, nil
		}
	}
	return false, fmt.Errorf("operator %v cannot compare %T and %T", op, left, right)
}

func exprEqual(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if lb, ok := valueAsBool(left); ok {
		if rb, ok := valueAsBool(right); ok {
			return lb == rb
		}
	}
	if lf, ok := valueAsFloat(left); ok {
		if rf, ok := valueAsFloat(right); ok {
			return lf == rf
		}
	}
	if ls, ok := valueAsString(left); ok {
		if rs, ok := valueAsString(right); ok {
			return ls == rs
		}
	}
	return reflect.DeepEqual(left, right)
}

func valueAsBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		if x == "true" {
			return true, true
		}
		if x == "false" {
			return false, true
		}
	}
	return false, false
}

func valueAsFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func valueAsString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	}
	return "", false
}

func resolveFactPath(facts map[string]any, path string) any {
	if facts == nil || path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	var cur any = facts
	for _, part := range parts {
		cur = derefValue(cur)
		switch x := cur.(type) {
		case map[string]any:
			cur = x[part]
		case map[string]string:
			cur = x[part]
		default:
			rv := reflect.ValueOf(cur)
			if !rv.IsValid() {
				return nil
			}
			if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
				mv := rv.MapIndex(reflect.ValueOf(part))
				if !mv.IsValid() {
					return nil
				}
				cur = mv.Interface()
				continue
			}
			if rv.Kind() == reflect.Struct {
				fv := fieldByJSONOrName(rv, part)
				if !fv.IsValid() {
					return nil
				}
				cur = fv.Interface()
				continue
			}
			return nil
		}
	}
	return cur
}

func derefValue(v any) any {
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil
	}
	return rv.Interface()
}

func fieldByJSONOrName(rv reflect.Value, name string) reflect.Value {
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		sf := rt.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		jsonName := strings.Split(sf.Tag.Get("json"), ",")[0]
		if jsonName == name || strings.EqualFold(sf.Name, name) {
			return rv.Field(i)
		}
	}
	return reflect.Value{}
}
