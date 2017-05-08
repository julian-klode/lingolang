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
	EndOfFile    TokenType = iota // End of file or error
	ParenLeft                     // Opening parenthesis.
	ParenRight                    // Closing parenthesis.
	Comma                         // A Comma
	Word                          // A word (string of letters)
	Func                          // The word "func"
	Interface                     // The word "interface"
	Map                           // The word "map"
	Chan                          // The word "chan"
	Error                         // The word "error" (for testing)
	Number                        // A number (string of digits)
	Star                          // The character "*"
	BracketLeft                   // The character "["
	BracketRight                  // The character "]"
)

func (typ TokenType) String() string {
	switch typ {
	case EndOfFile:
		return "end of file"
	case ParenLeft:
		return "opening paren"
	case ParenRight:
		return "closing paren"
	case Comma:
		return "comma"
	case Word:
		return "word"
	case Func:
		return "keyword 'func'"
	case Interface:
		return "keyword 'interface'"
	case Map:
		return "keyword 'map'"
	case Chan:
		return "keyword 'chan'"
	case Error:
		return "keyword 'error'"
	case Number:
		return "number"
	case Star:
		return "operator '*'"
	case BracketLeft:
		return "operator '['"
	case BracketRight:
		return "operator ']'"
	default:
		return fmt.Sprintf("<unknown token type %d>", typ)
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
	switch ch := sc.readRune(); {
	case ch == 0:
		return Token{}
	case ch == '(':
		return Token{ParenLeft, "("}
	case ch == ')':
		return Token{ParenRight, ")"}
	case ch == '*':
		return Token{Star, "*"}
	case ch == '[':
		return Token{BracketLeft, "["}
	case ch == ']':
		return Token{BracketRight, "]"}
	case ch == ',':
		return Token{Comma, ","}
	case unicode.IsLetter(ch):
		sc.unreadRune()
		tok := sc.scanWhile(Word, unicode.IsLetter)
		assignKeyword(&tok)
		return tok
	case unicode.IsDigit(ch):
		sc.unreadRune()
		return sc.scanWhile(Number, unicode.IsDigit)
	case unicode.IsSpace(ch):
		return sc.Scan()
	default:
		panic(sc.wrapError(errors.New("Unknown character to start token: " + string(ch))))
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

// TryAccept peeks the next token and if its type matches one of the types
// specified as an argument, scans and returns it.
func (sc *Scanner) TryAccept(types ...TokenType) (Token, error) {
	tok := sc.Peek()

	for _, typ := range types {
		if tok.Type == typ {
			return sc.Scan(), nil
		}
	}
	return Token{}, sc.wrapError(fmt.Errorf("Expected one of %v, received %v", types, tok))
}

// Accept is like TryAccept, but panics instead of returning an error.
func (sc *Scanner) Accept(types ...TokenType) Token {
	tok, err := sc.TryAccept(types...)
	if err != nil {
		panic(err)
	}
	return tok
}

// readRune calls ReadRune() on the reader and panics if it errors.
func (sc *Scanner) readRune() (r rune) {
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
		tok.Type = Func
	case "interface":
		tok.Type = Interface
	case "map":
		tok.Type = Map
	case "chan":
		tok.Type = Chan
	case "error":
		tok.Type = Error
	}
}
