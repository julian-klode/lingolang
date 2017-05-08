// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package parser

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"unicode"
)

// TokenType is the type of a token. Valid values are given in the
// enumeration below.
type TokenType int

// Token types. These shall become private at some point.
const (
	EndOfFile  TokenType = iota // End of file or error
	Left                        // Opening parenthesis.
	Right                       // Closing parenthesis.
	Comma                       // A Comma
	Word                        // A word (string of letters)
	Func                        // The word "func"
	Map                         // The word "map"
	Chan                        // The word "chan"
	Error                       // The word "error" (for testing)
	Number                      // A number (string of digits)
	Star                        // The character "*"
	SliceOpen                   // The character "["
	SliceClose                  // The character "]"
)

func (typ TokenType) String() string {
	switch typ {
	case EndOfFile:
		return "end of file"
	case Left:
		return "opening paren"
	case Right:
		return "closing paren"
	case Comma:
		return "comma"
	case Word:
		return "word"
	case Func:
		return "keyword 'func'"
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
	case SliceOpen:
		return "operator '['"
	case SliceClose:
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
	reader *bufio.Reader
	buffer *Token
}

// NewScanner creates a new scanner
func NewScanner(rd io.Reader) *Scanner {
	return &Scanner{reader: bufio.NewReader(rd)}
}

// Scan the next token.
func (sc *Scanner) Scan() (Token, error) {
	if sc.buffer != nil {
		tok := *sc.buffer
		sc.buffer = nil
		return tok, nil
	}
	switch ch, _, err := sc.reader.ReadRune(); {
	case ch == 0:
		return Token{}, nil
	case err != nil:
		return Token{EndOfFile, "<error>"}, err
	case ch == '(':
		return Token{Left, "("}, nil
	case ch == ')':
		return Token{Right, ")"}, nil
	case ch == '*':
		return Token{Star, "*"}, nil
	case ch == '[':
		return Token{SliceOpen, "["}, nil
	case ch == ']':
		return Token{SliceClose, "]"}, nil
	case ch == ',':
		return Token{Comma, ","}, nil
	case unicode.IsLetter(ch):
		sc.reader.UnreadRune()
		tok, err := sc.scanWhile(Word, unicode.IsLetter)
		if err != nil {
			return Token{}, err
		}
		assignKeyword(&tok)
		return tok, nil
	case unicode.IsDigit(ch):
		sc.reader.UnreadRune()
		return sc.scanWhile(Number, unicode.IsDigit)
	case unicode.IsSpace(ch):
		return sc.Scan()
	default:
		return Token{Value: "<error>"}, errors.New("Unknown character to start token: " + string(ch))
	}
}

// Unscan makes a token available again for Scan()
func (sc *Scanner) Unscan(tok Token) {
	sc.buffer = &tok
}

// Peek at the next token.
func (sc *Scanner) Peek() (Token, error) {
	tok, err := sc.Scan()

	sc.Unscan(tok)

	return tok, err
}

// Accept peeks the next token and if its type matches one of the types
// specified as an argument, scans and returns it.
func (sc *Scanner) Accept(types ...TokenType) (Token, error) {
	tok, err := sc.Peek()
	if err != nil {
		return Token{}, err
	}
	for _, typ := range types {
		if tok.Type == typ {
			return sc.Scan()
		}
	}
	return Token{}, fmt.Errorf("Expected one of %v, received %v", types, tok)
}

// scanWhile scans a sequence of runes accepted by the given function.
func (sc *Scanner) scanWhile(typ TokenType, acceptor func(rune) bool) (Token, error) {
	word := make([]rune, 0)
	var ch rune
	var err error
	for ch, _, err = sc.reader.ReadRune(); acceptor(ch); ch, _, err = sc.reader.ReadRune() {
		word = append(word, ch)
	}

	if len(word) > 0 {
		sc.reader.UnreadRune()
		return Token{typ, string(word)}, nil
	}

	return Token{Value: "<error>"}, err
}

// assignKeyword looks at the value of a word token and if it is a keyword,
// replaces the Type field with the correct type for the keyword.
func assignKeyword(tok *Token) {
	switch tok.Value {
	case "func":
		tok.Type = Func
	case "map":
		tok.Type = Map
	case "chan":
		tok.Type = Chan
	case "error":
		tok.Type = Error
	}
}
