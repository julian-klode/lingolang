// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"fmt"
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
func NewParser(input string) *Parser {
	return &Parser{NewScanner(input)}
}

// Parse parses the permission specification language.
//
// @syntax main <- inner EOF
func (p *Parser) Parse() (perm Permission, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch rr := r.(type) {
			case scannerError:
				perm = nil
				err = rr
			default:
				panic(rr)
			}
		}
	}()
	perm = p.parseInner()

	// Ensure that the inner run is complete
	p.sc.Expect(TokenEndOfFile)
	return perm, nil
}

// parseInner parses a permission spec. The permission spec may contain
// arbitrary garbage at the end, use Parse() to make sure that does not
// happen
//
// @syntax inner <- '_' | basePermission [func | map | chan | pointer | sliceOrArray]
func (p *Parser) parseInner() Permission {
	if _, ok := p.sc.Accept(TokenWildcard); ok {
		return &WildcardPermission{}
	}
	basePerm := p.parseBasePermission()

	switch tok := p.sc.Peek(); tok.Type {
	case TokenFunc, TokenParenLeft:
		return p.parseFunc(basePerm)
	case TokenInterface:
		return p.parseInterface(basePerm)
	case TokenMap:
		return p.parseMap(basePerm)
	case TokenChan:
		return p.parseChan(basePerm)
	case TokenStruct:
		return p.parseStruct(basePerm)
	case TokenStar:
		return p.parsePointer(basePerm)
	case TokenBracketLeft:
		return p.parseSliceOrArray(basePerm)
	default:
		// We are searching the longest match here. If inside somethig else,
		// there might be syntax for the parent node after this - ignore that.
		return basePerm
	}
}

// @syntax basePermission ('o'|'r'|'w'|'R'|'W'|'m'|'l'|'v'|'a'|'n')+
func (p *Parser) parseBasePermission() BasePermission {
	var perm BasePermission
	tok := p.sc.Expect(TokenWord)

	for _, c := range tok.Value {
		switch c {
		case 'o':
			perm |= Owned
		case 'r':
			perm |= Read
		case 'w':
			perm |= Write
		case 'R':
			perm |= ExclRead
		case 'W':
			perm |= ExclWrite
		case 'm':
			perm |= Mutable
		case 'l':
			perm |= LinearValue
		case 'v':
			perm |= Value
		case 'a':
			perm |= Any
		case 'n':
			perm |= None
		default:
			panic(p.sc.wrapError(fmt.Errorf("Unknown permission bit or type: %c", c)))
		}
	}
	if perm.String() != tok.Value {
		fmt.Printf("Warning: Permission %v can be rewritten as %v\n", tok.Value, perm.String())
	}
	return perm
}

// @syntax func <- ['(' param List ')'] 'func' '(' [paramList] ')' ( [inner] |  '(' [paramList] ')')
func (p *Parser) parseFunc(bp BasePermission) Permission {
	var receiver []Permission
	var params []Permission
	var results []Permission

	// Try to parse the receiver
	if tok, _ := p.sc.Accept(TokenParenLeft, TokenFunc); tok.Type == TokenParenLeft {
		receiver = p.parseFieldList(TokenComma)
		p.sc.Expect(TokenParenRight)
		p.sc.Expect(TokenFunc)
	}

	// Pararameters
	p.sc.Expect(TokenParenLeft)
	if p.sc.Peek().Type != TokenParenRight {
		params = p.parseFieldList(TokenComma)
	}
	p.sc.Expect(TokenParenRight)

	// Results
	if tok, _ := p.sc.Accept(TokenParenLeft); tok.Type == TokenParenLeft {
		results = p.parseFieldList(TokenComma)
		p.sc.Expect(TokenParenRight)
	} else if tok := p.sc.Peek(); tok.Type == TokenWord {
		// permission starts with word. We peek()ed first, so we can backtrack.
		results = []Permission{p.parseInner()}
	}

	return &FuncPermission{BasePermission: bp, Receivers: receiver, Params: params, Results: results}
}

// @syntax paramList <- inner (',' inner)*
// @syntax fieldList <- inner (';' inner)*
func (p *Parser) parseFieldList(sep TokenType) []Permission {
	var tok Token
	var perms []Permission

	perm := p.parseInner()
	perms = append(perms, perm)

	for tok = p.sc.Peek(); tok.Type == sep; tok = p.sc.Peek() {
		p.sc.Scan()
		perms = append(perms, p.parseInner())
	}

	return perms
}

// @syntax sliceOrArray <- '[' [NUMBER] ']' inner
func (p *Parser) parseSliceOrArray(bp BasePermission) Permission {
	p.sc.Expect(TokenBracketLeft)
	_, isArray := p.sc.Accept(TokenNumber, TokenWildcard)
	p.sc.Expect(TokenBracketRight)

	rhs := p.parseInner()

	if isArray {
		return &ArrayPermission{BasePermission: bp, ElementPermission: rhs}
	}
	return &SlicePermission{BasePermission: bp, ElementPermission: rhs}
}

// @syntax chan <- 'chan' inner
func (p *Parser) parseChan(bp BasePermission) Permission {
	p.sc.Expect(TokenChan)
	rhs := p.parseInner()
	return &ChanPermission{BasePermission: bp, ElementPermission: rhs}
}

// @syntax chan <- 'interface' '{' [fieldList] '}'
func (p *Parser) parseInterface(bp BasePermission) Permission {
	var fields []Permission
	p.sc.Expect(TokenInterface)
	p.sc.Expect(TokenBraceLeft)
	if p.sc.Peek().Type != TokenBraceRight {
		fields = p.parseFieldList(TokenSemicolon)
	}
	p.sc.Expect(TokenBraceRight)
	return &InterfacePermission{BasePermission: bp, Methods: fields}
}

// @syntax map <- 'map' '[' inner ']' inner
func (p *Parser) parseMap(bp BasePermission) Permission {
	p.sc.Expect(TokenMap) // Map keyword

	p.sc.Expect(TokenBracketLeft)
	key := p.parseInner()
	p.sc.Expect(TokenBracketRight)

	val := p.parseInner()

	return &MapPermission{BasePermission: bp, KeyPermission: key, ValuePermission: val}
}

// @syntax pointer <- '*' inner
func (p *Parser) parsePointer(bp BasePermission) Permission {
	p.sc.Expect(TokenStar)
	rhs := p.parseInner()
	return &PointerPermission{BasePermission: bp, Target: rhs}
}

// @syntax struct <- 'struct' '{' fieldList '}'
func (p *Parser) parseStruct(bp BasePermission) Permission {
	p.sc.Expect(TokenStruct)
	p.sc.Expect(TokenBraceLeft)
	fields := p.parseFieldList(TokenSemicolon)
	p.sc.Expect(TokenBraceRight)
	return &StructPermission{BasePermission: bp, Fields: fields}
}
