package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"whatsrook/pkg/addons/sdk"
)

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		sdk.Respond("Usage: calc [expression] (e.g. calc 2 + 2 * 4, calc sqrt(144) + 2^3)")
		return
	}

	result, err := evaluate(query)
	if err != nil {
		sdk.Respond(fmt.Sprintf("Math error: %s", err))
		return
	}

	var formatted string
	if math.IsNaN(result) {
		formatted = "NaN"
	} else if math.IsInf(result, 1) {
		formatted = "Infinity"
	} else if math.IsInf(result, -1) {
		formatted = "-Infinity"
	} else if result == math.Trunc(result) && math.Abs(result) < 1e15 {
		formatted = strconv.FormatInt(int64(result), 10)
	} else {
		formatted = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.8f", result), "0"), ".")
	}

	sdk.Respond(fmt.Sprintf("Result: %s", formatted))
}

func evaluate(expr string) (float64, error) {
	var cleaned strings.Builder
	for _, r := range expr {
		if !unicode.IsSpace(r) {
			cleaned.WriteRune(r)
		}
	}
	s := cleaned.String()
	if s == "" {
		return 0, fmt.Errorf("empty expression")
	}

	p := &parser{input: []rune(s)}
	val, err := p.parseExpression()
	if err != nil {
		return 0, err
	}
	if p.pos < len(p.input) {
		return 0, fmt.Errorf("unexpected token %q at position %d", string(p.input[p.pos:]), p.pos)
	}
	return val, nil
}

type parser struct {
	input []rune
	pos   int
}

func (p *parser) peek() (rune, bool) {
	if p.pos < len(p.input) {
		return p.input[p.pos], true
	}
	return 0, false
}

func (p *parser) next() (rune, bool) {
	if p.pos < len(p.input) {
		r := p.input[p.pos]
		p.pos++
		return r, true
	}
	return 0, false
}

func (p *parser) parseExpression() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}

	for {
		c, ok := p.peek()
		if !ok || (c != '+' && c != '-') {
			break
		}
		p.next()
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if c == '+' {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *parser) parseTerm() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}

	for {
		c, ok := p.peek()
		if !ok || (c != '*' && c != '/' && c != '%') {
			break
		}
		p.next()
		right, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		switch c {
		case '*':
			left *= right
		case '/':
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		case '%':
			if right == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			left = math.Mod(left, right)
		}
	}
	return left, nil
}

func (p *parser) parsePower() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}

	c, ok := p.peek()
	if ok && c == '^' {
		p.next()
		right, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}
	return left, nil
}

func (p *parser) parseFactor() (float64, error) {
	c, ok := p.peek()
	if !ok {
		return 0, fmt.Errorf("unexpected end of input")
	}

	if c == '+' {
		p.next()
		return p.parseFactor()
	}
	if c == '-' {
		p.next()
		val, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}

	if c == '(' {
		p.next()
		val, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		closing, ok := p.next()
		if !ok || closing != ')' {
			return 0, fmt.Errorf("missing closing parenthesis ')'")
		}
		return val, nil
	}

	if unicode.IsLetter(c) {
		var ident strings.Builder
		for {
			ch, ok := p.peek()
			if !ok || (!unicode.IsLetter(ch) && !unicode.IsDigit(ch)) {
				break
			}
			p.next()
			ident.WriteRune(ch)
		}

		name := strings.ToLower(ident.String())
		switch name {
		case "pi":
			return math.Pi, nil
		case "e":
			return math.E, nil
		}

		nextChar, ok := p.peek()
		if !ok || nextChar != '(' {
			return 0, fmt.Errorf("unknown identifier %q", name)
		}
		p.next()

		arg, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		closing, ok := p.next()
		if !ok || closing != ')' {
			return 0, fmt.Errorf("missing closing parenthesis for function %s", name)
		}

		switch name {
		case "sqrt":
			if arg < 0 {
				return 0, fmt.Errorf("square root of negative number")
			}
			return math.Sqrt(arg), nil
		case "abs":
			return math.Abs(arg), nil
		case "sin":
			return math.Sin(arg), nil
		case "cos":
			return math.Cos(arg), nil
		case "tan":
			return math.Tan(arg), nil
		case "log":
			if arg <= 0 {
				return 0, fmt.Errorf("log of non-positive number")
			}
			return math.Log10(arg), nil
		case "ln":
			if arg <= 0 {
				return 0, fmt.Errorf("ln of non-positive number")
			}
			return math.Log(arg), nil
		case "floor":
			return math.Floor(arg), nil
		case "ceil":
			return math.Ceil(arg), nil
		case "round":
			return math.Round(arg), nil
		default:
			return 0, fmt.Errorf("unknown function %q", name)
		}
	}

	if unicode.IsDigit(c) || c == '.' {
		var num strings.Builder
		for {
			ch, ok := p.peek()
			if !ok || (!unicode.IsDigit(ch) && ch != '.') {
				break
			}
			p.next()
			num.WriteRune(ch)
		}
		val, err := strconv.ParseFloat(num.String(), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number %q", num.String())
		}
		return val, nil
	}

	return 0, fmt.Errorf("unexpected character %q at position %d", c, p.pos)
}
