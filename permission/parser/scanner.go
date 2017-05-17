// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package parser

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// TokenType is the type of a token. Valid values are given in the
// enumeration below.
type TokenType int

// Token types. These shall become private at some point.
const (
	TokenEndOfFile    TokenType = iota // End of file or error
	TokenParenLeft                     // Opening parenthesis.
	TokenParenRight                    // Closing parenthesis.
	TokenComma                         // A Comma
	TokenWord                          // A word (string of letters)
	TokenFunc                          // The word "func"
	TokenInterface                     // The word "interface"
	TokenMap                           // The word "map"
	TokenChan                          // The word "chan"
	TokenError                         // The word "error" (for testing)
	TokenStruct                        // The word "struct"
	TokenNumber                        // A number (string of digits)
	TokenStar                          // The character "*"
	TokenBracketLeft                   // The character "["
	TokenBracketRight                  // The character "]"
	TokenBraceLeft                     // The character '{'
	TokenBraceRight                    // The character '}'
	TokenSemicolon                     // The character ';'
)

var tokenTypeString = map[TokenType]string{
	TokenEndOfFile:    "end of file",
	TokenParenLeft:    "opening paren",
	TokenParenRight:   "closing paren",
	TokenComma:        "comma",
	TokenWord:         "word",
	TokenFunc:         "keyword 'func'",
	TokenInterface:    "keyword 'interface'",
	TokenMap:          "keyword 'map'",
	TokenChan:         "keyword 'chan'",
	TokenError:        "keyword 'error'",
	TokenStruct:       "keyword 'struct'",
	TokenNumber:       "number",
	TokenStar:         "operator '*'",
	TokenBracketLeft:  "operator '['",
	TokenBracketRight: "operator ']'",
	TokenBraceLeft:    "operator '{'",
	TokenBraceRight:   "operator '}'",
	TokenSemicolon:    "operator ';'",
}

func (typ TokenType) String() string {
	switch str := tokenTypeString[typ]; str {
	case "":
		return fmt.Sprintf("<unknown token type %d>", typ)
	default:
		return str
	}
}

// Token has a type and a value
type Token struct {
	Type  TokenType
	Value string
}

func (tok Token) String() string {
	return fmt.Sprintf("Token{Type: %s, Value: %#v}", tok.Type.String(), tok.Value)
}

// Scanner scans input for tokens used in the permission description language.
//
// It provides a single lookahead token to use.
type Scanner struct {
	input     string // Input string
	offset    int    // Offset in the input string
	start     int    // Start of the current rune
	buffer    Token  // A token that was unscanned
	hasBuffer bool   // Whether buffer contains an unscanned token
}

// An error in the error.
type scannerError struct {
	error
	offset int
}

func (err scannerError) Error() string {
	return fmt.Sprintf("At element ending at %d: %s", err.offset, err.error.Error())
}

// NewScanner creates a new scanner
func NewScanner(input string) *Scanner {
	return &Scanner{input: input}
}

// Scan the next token.
func (sc *Scanner) Scan() Token {
	// We put a token back, so let's give that out again.
	if sc.hasBuffer {
		tok := sc.buffer
		sc.buffer = Token{}
		sc.hasBuffer = false
		return tok
	}
	for {
		switch ch := sc.readRune(); {
		case ch == 0:
			return Token{}
		case ch == '(':
			return Token{TokenParenLeft, "("}
		case ch == ')':
			return Token{TokenParenRight, ")"}
		case ch == '*':
			return Token{TokenStar, "*"}
		case ch == '[':
			return Token{TokenBracketLeft, "["}
		case ch == ']':
			return Token{TokenBracketRight, "]"}
		case ch == '{':
			return Token{TokenBraceLeft, "{"}
		case ch == '}':
			return Token{TokenBraceRight, "}"}
		case ch == ',':
			return Token{TokenComma, ","}
		case ch == ';':
			return Token{TokenSemicolon, ";"}
		case unicode.IsLetter(ch):
			sc.unreadRune()
			tok := sc.scanWhile(TokenWord, unicode.IsLetter)
			assignKeyword(&tok)
			return tok
		case unicode.IsDigit(ch):
			sc.unreadRune()
			return sc.scanWhile(TokenNumber, unicode.IsDigit)
		case unicode.IsSpace(ch):
		default:
			panic(sc.wrapError(errors.New("Unknown character to start token: " + string(ch))))
		}
	}
}

// Unscan makes a token available again for Scan()
func (sc *Scanner) Unscan(tok Token) {
	sc.buffer = tok
	sc.hasBuffer = true
}

// Peek at the next token.
func (sc *Scanner) Peek() Token {
	tok := sc.Scan()

	sc.Unscan(tok)

	return tok
}

// Accept peeks the next token and if its type matches one of the types
// specified as an argument, scans and returns it.
func (sc *Scanner) Accept(types ...TokenType) (tok Token, ok bool) {
	tok = sc.Scan()

	for _, typ := range types {
		if tok.Type == typ {
			return tok, true
		}
	}

	sc.Unscan(tok)
	return tok, false
}

// Expect calls accept and panic()s if Accept fails
func (sc *Scanner) Expect(types ...TokenType) Token {
	tok, ok := sc.Accept(types...)
	if !ok {
		panic(sc.wrapError(fmt.Errorf("Expected one of %v, received %v", types, tok)))
	}
	return tok
}

// readRune calls ReadRune() on the reader and panics if it errors.
func (sc *Scanner) readRune() rune {
	r, size := utf8.DecodeRuneInString(sc.input[sc.offset:])
	switch {
	case r == utf8.RuneError && size == 0:
		return 0
	case r == utf8.RuneError:
		panic(sc.wrapError(fmt.Errorf("Encoding error")))
	default:
		sc.start = sc.offset
		sc.offset += size
		return r
	}
}

// unreadRune unreads a rune
func (sc *Scanner) unreadRune() {
	sc.offset = sc.start
}

// scanWhile scans a sequence of runes accepted by the given function.
func (sc *Scanner) scanWhile(typ TokenType, acceptor func(rune) bool) Token {
	start := sc.offset
	var ch rune
	for ch = sc.readRune(); acceptor(ch); ch = sc.readRune() {
	}
	if ch != 0 {
		sc.unreadRune()
	}

	return Token{typ, sc.input[start:sc.offset]}
}

// Annotate error with position information
func (sc *Scanner) wrapError(err error) error {
	return scannerError{err, sc.offset}
}

// assignKeyword looks at the value of a word token and if it is a keyword,
// replaces the Type field with the correct type for the keyword.
func assignKeyword(tok *Token) {
	switch tok.Value {
	case "func":
		tok.Type = TokenFunc
	case "interface":
		tok.Type = TokenInterface
	case "map":
		tok.Type = TokenMap
	case "chan":
		tok.Type = TokenChan
	case "error":
		tok.Type = TokenError
	case "struct":
		tok.Type = TokenStruct
	}
}
