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
func (p *Parser) Parse() (perm permission.Permission, err error) {
	defer func() {
		if r := recover(); r != nil {
			if r.(error) == nil {
				panic(r)
			}

			perm = nil
			err = r.(error)
		}
	}()
	perm = p.parseInner()

	// Ensure that the inner run is complete
	p.sc.Accept(EndOfFile)
	return perm, nil
}

// parseInner parses a permission spec. The permission spec may contain
// arbitrary garbage at the end, use Parse() to make sure that does not
// happen
//
// @syntax inner <- basePermission [func | map | chan | pointer | sliceOrArray]
func (p *Parser) parseInner() permission.Permission {
	basePerm := p.parseBasePermission()

	switch tok := p.sc.Peek(); tok.Type {
	case Func, Left:
		return p.parseFunc(basePerm)
	case Map:
		return p.parseMap(basePerm)
	case Chan:
		return p.parseChan(basePerm)
	case Star:
		return p.parsePointer(basePerm)
	case SliceOpen:
		return p.parseSliceOrArray(basePerm)
	default:
		return basePerm
	}
}

// @syntax basePermission ('o'|'r'|'w'|'R'|'W'|'m'|'l'|'v'|'a'|'n')+
func (p *Parser) parseBasePermission() permission.BasePermission {
	var perm permission.BasePermission
	tok := p.sc.Accept(Word)

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
			panic(fmt.Errorf("Unknown permission bit or type: %c", c))
		}
	}
	if perm.String() != tok.Value {
		fmt.Printf("Warning: Permission %v can be rewritten as %v\n", tok.Value, perm.String())
	}
	return perm
}

// @syntax func <- ['(' paramList ')'] 'func' '(' paramList ')' ( inner |  '(' paramList ')')
func (p *Parser) parseFunc(bp permission.BasePermission) permission.Permission {
	var receiver []permission.Permission
	var params []permission.Permission
	var results []permission.Permission

	// Try to parse the receiver
	if tok, _ := p.sc.TryAccept(Left, Func); tok.Type == Left {
		receiver = p.parseParamList()
		p.sc.Accept(Right)
		p.sc.Accept(Func)
	}

	// Pararameters
	p.sc.Accept(Left)
	params = p.parseParamList()
	p.sc.Accept(Right)

	// Results
	if tok, _ := p.sc.TryAccept(Left); tok.Type == Left {
		results = p.parseParamList()
		p.sc.Accept(Right)
	} else if tok := p.sc.Peek(); tok.Type == Word {
		results = []permission.Permission{p.parseInner()}
	}

	return &permission.FuncPermission{BasePermission: bp, Receivers: receiver, Params: params, Results: results}
}

// @syntax paramList <- inner (',' inner)*
func (p *Parser) parseParamList() []permission.Permission {
	var tok Token
	var perms []permission.Permission
	perm := p.parseInner()

	perms = append(perms, perm)
	for tok = p.sc.Peek(); tok.Type == Comma; tok = p.sc.Peek() {
		p.sc.Scan()
		perms = append(perms, p.parseInner())
	}

	return perms
}

// @syntax sliceOrArray <- '[' [NUMBER] ']' inner
func (p *Parser) parseSliceOrArray(bp permission.BasePermission) permission.Permission {
	p.sc.Accept(SliceOpen)
	p.sc.TryAccept(Number)
	p.sc.Accept(SliceClose)

	rhs := p.parseInner()

	return &permission.ArraySlicePermission{BasePermission: bp, ElementPermission: rhs}
}

// @syntax chan <- 'chan' inner
func (p *Parser) parseChan(bp permission.BasePermission) permission.Permission {
	p.sc.Accept(Chan)
	rhs := p.parseInner()

	return &permission.ChanPermission{BasePermission: bp, ElementPermission: rhs}
}

// @syntax map <- 'map' '[' inner ']' inner
func (p *Parser) parseMap(bp permission.BasePermission) permission.Permission {
	p.sc.Accept(Map) // Map keyword
	p.sc.Accept(SliceOpen)

	key := p.parseInner()
	p.sc.Accept(SliceClose)

	val := p.parseInner()

	return &permission.MapPermission{BasePermission: bp, KeyPermission: key, ValuePermission: val}
}

// @syntax pointer <- '*' inner
func (p *Parser) parsePointer(bp permission.BasePermission) permission.Permission {
	p.sc.Scan()
	rhs := p.parseInner()
	return &permission.PointerPermission{BasePermission: bp, Target: rhs}
}
