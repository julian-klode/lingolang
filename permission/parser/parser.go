// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

// Package parser provides a parser for the permission specification
// language.
package parser

import (
	"fmt"
	"io"

	"github.com/julian-klode/lingolang/permission"
)

// Parser is a parser for the permission syntax.
//
// This parser is implemented as a simple recursive parser, it has horrible
// error reporting, but it works, and has a test suite.
//
// The parser requires one lookahead token in the scanner.
type Parser struct {
	sc *Scanner
}

// NewParser returns a new parser for the permission specification language.
func NewParser(rd io.Reader) *Parser {
	return &Parser{NewScanner(rd)}
}

// Parse parses the permission specification language.
//
// @syntax main <- inner EOF
func (p *Parser) Parse() (permission.Permission, error) {
	perm, err := p.parseInner()
	if err != nil {
		return nil, err
	}
	// Ensure that the inner run is complete
	if _, err := p.sc.Accept(EndOfFile); err != nil {
		return nil, err
	}
	return perm, nil
}

// parseInner parses a permission spec. The permission spec may contain
// arbitrary garbage at the end, use Parse() to make sure that does not
// happen
//
// @syntax inner <- basePermission [func | map | chan | pointer | sliceOrArray]
func (p *Parser) parseInner() (permission.Permission, error) {
	basePermTok, err := p.sc.Peek()
	if err != nil {
		return nil, err
	}
	if basePermTok.Type != Word {
		return nil, fmt.Errorf("Expected base permission at start of permission spec, received %v", basePermTok)
	}
	basePerm, err := p.parseBasePermission()
	if err != nil {
		return nil, err
	}

	tok, err := p.sc.Peek()
	if err != nil {
		return nil, err
	}

	switch tok.Type {
	case Func, Left:
		return p.parseFunc(basePerm)
	case Map:
		return p.parseMap(basePerm)
	case Chan:
		return p.parseChan(basePerm)
	case Star:
		return p.parsePointer(basePerm)
	case EndOfFile:
		return basePerm, nil
	case SliceOpen:
		return p.parseSliceOrArray(basePerm)
	default:
		return basePerm, nil
	}
}

// @syntax basePermission ('o'|'r'|'w'|'R'|'W'|'m'|'l'|'v'|'a'|'n')+
func (p *Parser) parseBasePermission() (permission.BasePermission, error) {
	var perm permission.BasePermission
	tok, err := p.sc.Accept(Word)
	if err != nil {
		return 0, err
	}
	for _, c := range tok.Value {
		switch c {
		case 'o':
			perm |= permission.Owned
		case 'r':
			perm |= permission.Read
		case 'w':
			perm |= permission.Write
		case 'R':
			perm |= permission.ExclRead
		case 'W':
			perm |= permission.ExclWrite
		case 'm':
			perm |= permission.Mutable
		case 'l':
			perm |= permission.LinearValue
		case 'v':
			perm |= permission.Value
		case 'a':
			perm |= permission.Any
		case 'n':
			perm |= permission.None
		default:
			return 0, fmt.Errorf("Unknown permission bit or type: %c", c)
		}
	}
	if perm.String() != tok.Value {
		fmt.Printf("Warning: Permission %v can be rewritten as %v\n", tok.Value, perm.String())
	}
	return perm, nil
}

// @syntax func <- ['(' paramList ')'] 'func' '(' paramList ')' ( inner |  '(' paramList ')')
func (p *Parser) parseFunc(bp permission.BasePermission) (permission.Permission, error) {
	var receiver []permission.Permission
	var params []permission.Permission
	var results []permission.Permission
	var err error

	// Try to parse the receiver
	if tok, _ := p.sc.Accept(Left, Func); tok.Type == Left {
		if receiver, err = p.parseParamList(); err != nil {
			return nil, err
		}
		if _, err = p.sc.Accept(Right); err != nil {
			return nil, err
		}
		if _, err = p.sc.Accept(Func); err != nil {
			return nil, err
		}
	}

	// Pararameters
	if _, err = p.sc.Accept(Left); err != nil {
		return nil, err
	}
	if params, err = p.parseParamList(); err != nil {
		return nil, err
	}
	if _, err = p.sc.Accept(Right); err != nil {
		return nil, err
	}

	// Results
	if tok, _ := p.sc.Accept(Left); tok.Type == Left {
		if results, err = p.parseParamList(); err != nil {
			return nil, err
		}
		if _, err = p.sc.Accept(Right); err != nil {
			return nil, err
		}
	} else if tok, _ := p.sc.Peek(); tok.Type == Word {
		result, err := p.parseInner()
		if err != nil {
			return nil, err
		}
		results = []permission.Permission{result}
	}

	return &permission.FuncPermission{BasePermission: bp, Receivers: receiver, Params: params, Results: results}, nil
}

// @syntax paramList <- inner (',' inner)*
func (p *Parser) parseParamList() ([]permission.Permission, error) {
	var tok Token
	var perms []permission.Permission
	perm, err := p.parseInner()
	if err != nil {
		return nil, err
	}
	perms = append(perms, perm)
	for tok, err = p.sc.Peek(); tok.Type == Comma; tok, err = p.sc.Peek() {
		p.sc.Scan()
		perm, err = p.parseInner()
		if err != nil {
			return nil, err
		}
		perms = append(perms, perm)
	}
	// Eww, underlying reader error
	if err != nil {
		return nil, err
	}
	return perms, nil
}

// @syntax sliceOrArray <- '[' [NUMBER] ']' inner
func (p *Parser) parseSliceOrArray(bp permission.BasePermission) (permission.Permission, error) {
	p.sc.Scan()
	p.sc.Accept(Number)
	if _, err := p.sc.Accept(SliceClose); err != nil {
		return nil, err
	}

	rhs, err := p.parseInner()
	if err != nil {
		return nil, err
	}
	return &permission.ArraySlicePermission{BasePermission: bp, ElementPermission: rhs}, nil
}

// @syntax chan <- 'chan' inner
func (p *Parser) parseChan(bp permission.BasePermission) (permission.Permission, error) {
	p.sc.Scan()
	rhs, err := p.parseInner()
	if err != nil {
		return nil, err
	}
	return &permission.ChanPermission{BasePermission: bp, ElementPermission: rhs}, nil
}

// @syntax map <- 'map' '[' inner ']' inner
func (p *Parser) parseMap(bp permission.BasePermission) (permission.Permission, error) {
	p.sc.Scan() // Map keyword

	if _, err := p.sc.Accept(SliceOpen); err != nil {
		return nil, err
	}
	key, err := p.parseInner()
	if err != nil {
		return nil, err
	}
	if _, err = p.sc.Accept(SliceClose); err != nil {
		return nil, err
	}

	val, err := p.parseInner()
	if err != nil {
		return nil, err
	}

	return &permission.MapPermission{BasePermission: bp, KeyPermission: key, ValuePermission: val}, nil
}

// @syntax pointer <- '*' inner
func (p *Parser) parsePointer(bp permission.BasePermission) (permission.Permission, error) {
	p.sc.Scan()
	rhs, err := p.parseInner()
	if err != nil {
		return nil, err
	}
	return &permission.PointerPermission{BasePermission: bp, Target: rhs}, nil
}
