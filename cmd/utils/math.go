package cliutils

import (
	"fmt"
	"strconv"
	"strings"
)

func EvalMathExpr(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}
	p := &exprParser{s: expr}
	res, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.pos < len(p.s) {
		return 0, fmt.Errorf("unexpected token %q at position %d", p.s[p.pos:], p.pos)
	}
	return res, nil
}

type exprParser struct {
	s   string
	pos int
}

func (p *exprParser) parseExpr() (float64, error) {
	val, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.s) {
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		nextVal, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			val += nextVal
		} else {
			val -= nextVal
		}
	}
	return val, nil
}

func (p *exprParser) parseTerm() (float64, error) {
	val, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.s) {
		op := p.s[p.pos]
		if op != '*' && op != '/' && op != '%' {
			break
		}
		p.pos++
		nextVal, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			val *= nextVal
		case '/':
			if nextVal == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			val /= nextVal
		default:
			if nextVal == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			val = float64(int64(val) % int64(nextVal))
		}
	}
	return val, nil
}

func (p *exprParser) parseFactor() (float64, error) {
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	if p.s[p.pos] == '-' {
		p.pos++
		val, err := p.parseFactor()
		return -val, err
	}
	if p.s[p.pos] == '+' {
		p.pos++
		return p.parseFactor()
	}

	if p.s[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}

	start := p.pos
	for p.pos < len(p.s) && ((p.s[p.pos] >= '0' && p.s[p.pos] <= '9') || p.s[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("invalid character %q", p.s[p.pos:p.pos+1])
	}

	val, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", p.s[start:p.pos])
	}
	return val, nil
}
