package ibdf

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// TokenType represents the lexical token type.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenNumber
	TokenString
	TokenAnd
	TokenOr
	TokenNot
	TokenLike
	TokenEqual
	TokenNotEqual
	TokenGreater
	TokenGreaterEqual
	TokenLess
	TokenLessEqual
	TokenLParen
	TokenRParen
)

// Token represents a single lexical token.
type Token struct {
	Type  TokenType
	Value string
}

// lex performs lexical analysis on the input query.
func lex(input string) ([]Token, error) {
	var tokens []Token
	runes := []rune(input)
	i := 0
	n := len(runes)

	for i < n {
		r := runes[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}

		switch r {
		case '(':
			tokens = append(tokens, Token{Type: TokenLParen, Value: "("})
			i++
		case ')':
			tokens = append(tokens, Token{Type: TokenRParen, Value: ")"})
			i++
		case '=':
			tokens = append(tokens, Token{Type: TokenEqual, Value: "="})
			i++
		case '!':
			if i+1 < n && runes[i+1] == '=' {
				tokens = append(tokens, Token{Type: TokenNotEqual, Value: "!="})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected character '!' at position %d. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", i)
			}
		case '>':
			if i+1 < n && runes[i+1] == '=' {
				tokens = append(tokens, Token{Type: TokenGreaterEqual, Value: ">="})
				i += 2
			} else {
				tokens = append(tokens, Token{Type: TokenGreater, Value: ">"})
				i++
			}
		case '<':
			if i+1 < n && runes[i+1] == '=' {
				tokens = append(tokens, Token{Type: TokenLessEqual, Value: "<="})
				i += 2
			} else if i+1 < n && runes[i+1] == '>' {
				tokens = append(tokens, Token{Type: TokenNotEqual, Value: "<>"})
				i += 2
			} else {
				tokens = append(tokens, Token{Type: TokenLess, Value: "<"})
				i++
			}
		case '\'', '"', '`':
			quote := r
			start := i + 1
			i++
			found := false
			for i < n {
				if runes[i] == quote {
					found = true
					break
				}
				i++
			}
			if !found {
				return nil, fmt.Errorf("unterminated string starting at position %d. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", start-1)
			}
			val := string(runes[start:i])
			i++ // consume closing quote
			if quote == '\'' {
				tokens = append(tokens, Token{Type: TokenString, Value: val})
			} else {
				tokens = append(tokens, Token{Type: TokenIdent, Value: val})
			}
		default:
			if unicode.IsDigit(r) || r == '.' {
				start := i
				hasDot := r == '.'
				i++
				for i < n {
					nextR := runes[i]
					if unicode.IsDigit(nextR) {
						i++
					} else if nextR == '.' {
						if hasDot {
							return nil, fmt.Errorf("invalid numeric literal at position %d. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", start)
						}
						hasDot = true
						i++
					} else {
						break
					}
				}
				tokens = append(tokens, Token{Type: TokenNumber, Value: string(runes[start:i])})
			} else if unicode.IsLetter(r) || r == '_' {
				start := i
				i++
				for i < n {
					nextR := runes[i]
					if unicode.IsLetter(nextR) || unicode.IsDigit(nextR) || nextR == '_' {
						i++
					} else {
						break
					}
				}
				val := string(runes[start:i])
				valUpper := strings.ToUpper(val)
				switch valUpper {
				case "AND":
					tokens = append(tokens, Token{Type: TokenAnd, Value: val})
				case "OR":
					tokens = append(tokens, Token{Type: TokenOr, Value: val})
				case "NOT":
					tokens = append(tokens, Token{Type: TokenNot, Value: val})
				case "LIKE":
					tokens = append(tokens, Token{Type: TokenLike, Value: val})
				default:
					tokens = append(tokens, Token{Type: TokenIdent, Value: val})
				}
			} else {
				return nil, fmt.Errorf("unexpected character %q at position %d. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", r, i)
			}
		}
	}
	tokens = append(tokens, Token{Type: TokenEOF, Value: ""})
	return tokens, nil
}

// Filter represents a compiled and validated query filter.
type Filter interface {
	Match(pair IBDPair, rowIndex int, samples []string) bool
}

// Expression represents an AST node that can be evaluated.
type Expression interface {
	Eval(pair IBDPair, rowIndex int, samples []string) (interface{}, error)
	Validate() error
}

// LiteralExpr represents literal numbers or strings.
type LiteralExpr struct {
	Value interface{}
}

func (e *LiteralExpr) Eval(pair IBDPair, rowIndex int, samples []string) (interface{}, error) {
	return e.Value, nil
}

func (e *LiteralExpr) Validate() error {
	return nil
}

// IdentExpr represents column names or their aliases.
type IdentExpr struct {
	Name string
}

func (e *IdentExpr) Eval(pair IBDPair, rowIndex int, samples []string) (interface{}, error) {
	name := strings.ToLower(e.Name)
	switch name {
	case "row":
		return float64(rowIndex), nil
	case "length", "cm", "length(cm)":
		return float64(pair.CM), nil
	case "sample1", "s1", "p1", "sample 1":
		return getSampleName(pair.P1, samples), nil
	case "sample2", "s2", "p2", "sample 2":
		return getSampleName(pair.P2, samples), nil
	case "sample", "s":
		return "SPECIAL_SAMPLE_ALIAS", nil
	default:
		return nil, fmt.Errorf("invalid column name %q. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", e.Name)
	}
}

func (e *IdentExpr) Validate() error {
	name := strings.ToLower(e.Name)
	switch name {
	case "row", "length", "cm", "length(cm)", "sample1", "s1", "p1", "sample 1", "sample2", "s2", "p2", "sample 2", "sample", "s":
		return nil
	default:
		return fmt.Errorf("invalid column name %q. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", e.Name)
	}
}

func getSampleName(id uint32, samples []string) string {
	if int(id) < len(samples) {
		return samples[id]
	}
	return fmt.Sprintf("Sample_%d", id)
}

// NotExpr represents a logical NOT operator.
type NotExpr struct {
	Expr Expression
}

func (e *NotExpr) Eval(pair IBDPair, rowIndex int, samples []string) (interface{}, error) {
	val, err := e.Expr.Eval(pair, rowIndex, samples)
	if err != nil {
		return nil, err
	}
	b, ok := val.(bool)
	if !ok {
		return nil, fmt.Errorf("operand of NOT must be a boolean expression")
	}
	return !b, nil
}

func (e *NotExpr) Validate() error {
	return e.Expr.Validate()
}

// LogicalExpr represents logical AND / OR operators.
type LogicalExpr struct {
	Left  Expression
	Op    TokenType
	Right Expression
}

func (e *LogicalExpr) Eval(pair IBDPair, rowIndex int, samples []string) (interface{}, error) {
	leftVal, err := e.Left.Eval(pair, rowIndex, samples)
	if err != nil {
		return nil, err
	}
	l, ok := leftVal.(bool)
	if !ok {
		return nil, fmt.Errorf("left operand of AND/OR must be a boolean expression")
	}

	if e.Op == TokenOr && l {
		return true, nil
	}
	if e.Op == TokenAnd && !l {
		return false, nil
	}

	rightVal, err := e.Right.Eval(pair, rowIndex, samples)
	if err != nil {
		return nil, err
	}
	r, ok := rightVal.(bool)
	if !ok {
		return nil, fmt.Errorf("right operand of AND/OR must be a boolean expression")
	}

	return r, nil
}

func (e *LogicalExpr) Validate() error {
	if err := e.Left.Validate(); err != nil {
		return err
	}
	return e.Right.Validate()
}

// ComparisonExpr represents comparison operators (=, !=, >, >=, <, <=, LIKE).
type ComparisonExpr struct {
	Left  Expression
	Op    TokenType
	Right Expression
}

func (e *ComparisonExpr) Eval(pair IBDPair, rowIndex int, samples []string) (interface{}, error) {
	// Handle special "sample" / "s" alias matching against both Sample 1 and Sample 2
	if ident, ok := e.Left.(*IdentExpr); ok && (strings.ToLower(ident.Name) == "sample" || strings.ToLower(ident.Name) == "s") {
		s1Expr := &ComparisonExpr{Left: &IdentExpr{Name: "sample1"}, Op: e.Op, Right: e.Right}
		s2Expr := &ComparisonExpr{Left: &IdentExpr{Name: "sample2"}, Op: e.Op, Right: e.Right}

		v1, err := s1Expr.Eval(pair, rowIndex, samples)
		if err != nil {
			return false, err
		}
		v2, err := s2Expr.Eval(pair, rowIndex, samples)
		if err != nil {
			return false, err
		}
		return v1.(bool) || v2.(bool), nil
	}

	leftVal, err := e.Left.Eval(pair, rowIndex, samples)
	if err != nil {
		return nil, err
	}
	rightVal, err := e.Right.Eval(pair, rowIndex, samples)
	if err != nil {
		return nil, err
	}

	switch l := leftVal.(type) {
	case float64:
		r, ok := rightVal.(float64)
		if !ok {
			return nil, fmt.Errorf("type mismatch: comparing number with non-number")
		}
		switch e.Op {
		case TokenEqual:
			return l == r, nil
		case TokenNotEqual:
			return l != r, nil
		case TokenGreater:
			return l > r, nil
		case TokenGreaterEqual:
			return l >= r, nil
		case TokenLess:
			return l < r, nil
		case TokenLessEqual:
			return l <= r, nil
		default:
			return nil, fmt.Errorf("unsupported operator for numbers: %v", e.Op)
		}
	case string:
		r, ok := rightVal.(string)
		if !ok {
			return nil, fmt.Errorf("type mismatch: comparing string with non-string")
		}
		switch e.Op {
		case TokenEqual:
			return strings.ToLower(l) == strings.ToLower(r), nil
		case TokenNotEqual:
			return strings.ToLower(l) != strings.ToLower(r), nil
		case TokenLike:
			return matchLike(l, r), nil
		default:
			return nil, fmt.Errorf("unsupported operator for strings: %v", e.Op)
		}
	}
	return nil, fmt.Errorf("invalid operand types for comparison")
}

func (e *ComparisonExpr) Validate() error {
	if err := e.Left.Validate(); err != nil {
		return err
	}
	return e.Right.Validate()
}

func matchLike(val, pattern string) bool {
	val = strings.ToLower(val)
	pattern = strings.ToLower(pattern)

	hasPrefixWildcard := strings.HasPrefix(pattern, "%")
	hasSuffixWildcard := strings.HasSuffix(pattern, "%")

	cleanPattern := strings.Trim(pattern, "%")
	if hasPrefixWildcard && hasSuffixWildcard {
		return strings.Contains(val, cleanPattern)
	}
	if hasPrefixWildcard {
		return strings.HasSuffix(val, cleanPattern)
	}
	if hasSuffixWildcard {
		return strings.HasPrefix(val, cleanPattern)
	}
	return val == cleanPattern
}

// parser parses the token stream.
type parser struct {
	tokens []Token
	pos    int
}

func newParser(tokens []Token) *parser {
	return &parser{tokens: tokens, pos: 0}
}

func (p *parser) Parse() (Expression, error) {
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().Type != TokenEOF {
		return nil, fmt.Errorf("unexpected token %q at the end of expression. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", p.peek().Value)
	}
	return expr, nil
}

func (p *parser) parseOr() (Expression, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.match(TokenOr) {
		op := p.previous()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &LogicalExpr{Left: left, Op: op.Type, Right: right}
	}

	return left, nil
}

func (p *parser) parseAnd() (Expression, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}

	for p.match(TokenAnd) {
		op := p.previous()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &LogicalExpr{Left: left, Op: op.Type, Right: right}
	}

	return left, nil
}

func (p *parser) parseNot() (Expression, error) {
	if p.match(TokenNot) {
		expr, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &NotExpr{Expr: expr}, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (Expression, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	if p.match(TokenEqual, TokenNotEqual, TokenGreater, TokenGreaterEqual, TokenLess, TokenLessEqual, TokenLike) {
		op := p.previous()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ComparisonExpr{Left: left, Op: op.Type, Right: right}, nil
	}

	return left, nil
}

func (p *parser) parsePrimary() (Expression, error) {
	if p.match(TokenNumber) {
		val, err := strconv.ParseFloat(p.previous().Value, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number literal: %s", p.previous().Value)
		}
		return &LiteralExpr{Value: val}, nil
	}

	if p.match(TokenString) {
		return &LiteralExpr{Value: p.previous().Value}, nil
	}

	if p.match(TokenIdent) {
		return &IdentExpr{Name: p.previous().Value}, nil
	}

	if p.match(TokenLParen) {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(TokenRParen) {
			return nil, fmt.Errorf("expected closing parenthesis, got %q. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", p.peek().Value)
		}
		return expr, nil
	}

	return nil, fmt.Errorf("expected identifier, literal, or expression, got %q. Valid columns are: Row, Sample 1, Sample 2, Length(cM)", p.peek().Value)
}

func (p *parser) match(types ...TokenType) bool {
	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *parser) check(t TokenType) bool {
	if p.isAtEnd() {
		return false
	}
	return p.peek().Type == t
}

func (p *parser) advance() Token {
	if !p.isAtEnd() {
		p.pos++
	}
	return p.previous()
}

func (p *parser) isAtEnd() bool {
	return p.peek().Type == TokenEOF
}

func (p *parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF, Value: ""}
	}
	return p.tokens[p.pos]
}

func (p *parser) previous() Token {
	return p.tokens[p.pos-1]
}

func isBooleanExpression(expr Expression) bool {
	switch expr.(type) {
	case *ComparisonExpr, *LogicalExpr, *NotExpr:
		return true
	}
	return false
}

type compiledFilter struct {
	expr Expression
}

func (f *compiledFilter) Match(pair IBDPair, rowIndex int, samples []string) bool {
	res, err := f.expr.Eval(pair, rowIndex, samples)
	if err != nil {
		return false
	}
	val, ok := res.(bool)
	return ok && val
}

// ParseFilter compiles the filter query into a Filter object.
func ParseFilter(query string) (Filter, error) {
	tokens, err := lex(query)
	if err != nil {
		return nil, err
	}

	p := newParser(tokens)
	expr, err := p.Parse()
	if err != nil {
		return nil, err
	}

	if err := expr.Validate(); err != nil {
		return nil, err
	}

	if !isBooleanExpression(expr) {
		return nil, fmt.Errorf("filter expression must be a comparison (e.g. length >= 5) or logical condition")
	}

	return &compiledFilter{expr: expr}, nil
}
