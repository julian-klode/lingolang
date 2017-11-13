# Implementation

The program is available in the ``github.com/julian-klode/lingolang` import, and provides three packages:

1. `permission` implements permission objects, parsing permissions, and operations involving permissions, like conversions, merges, assignability checks, and constructing permissions from types.
2. `capabilities` implements the store and the abstract interpreter
3. `lingolint` or rather `main`, implements a command-line binary

The permission library is thoroughly tested and achieves 100% line coverage. Likewise, the capabilities package is tested as far as possible, but it only achieves between 99% and 100% line coverage. The high coverage rate with the rule of not introducing any new changes that reduce coverage if possible, lead to a solid code base that can easily be extended without worrying much about breaking stuff.

The permission library is organized into several files, by function, rather than by types. There is one file dealing with checking copyability, one file dealing with convert-to-base operations, one file declaring all the types, one file contains the scanner, one file the parser, and so on.

## Parsing permission annotations
Permission annotations are stored in comments attached to functions, and declarations of variables. A comment line introducing a permission annotation starts with `@perm`, for example:

```go
// @perm om * om
var pointerToInt *int

var pointerToInt *int // @perm om * om
```

Go's excellent built-in AST package (located in `go/ast`) provides native support for associating comments to nodes in the syntax tree in a understandable and reusable way. We can simply walk the AST, and map each node to an existing annotation or `nil`.

The permission specification itself is then parsed using a hand-written scanner and a hand-written recursive-descent parser. The scanner operates on a stream of runes, and represents a stream of tokens with a buffer of one token for look-ahead. It provides the following functions to the parser:

* `func (sc *Scanner) Scan() Token` returns the next token in the token stream
* `func (sc *Scanner) Unscan(tok Token)` returns the last token returned from `Scan()` to the stream
* `func (sc *Scanner) Peek() Token` is equivalent to `Scan()` followed by `Unscan()`
* `func (sc *Scanner) Accept(types ...TokenType) (tok Token, ok bool)` takes a list of acceptable token types and returns the next token in the token stream and whether it matched. If the token did not match the expected token type, `Unscan()` is called before returning it.
* `func (sc *Scanner) Expect(types ...TokenType) Token` is like `Accept()` but errors out if the token does not match.

Error handling is not done by the usual approach of returning error values, because that made the parser code hard to read. Instead, when an error occurs, the built-in `panic()` is called with a `scannerError` object as an argument. This makes the scanner not very friendly to use outside the package, but it simplifies the parser, which calls `recover` in its outer `Parse()` method to recover any such error and return it as an error value.

```{#parse .go caption="The outer Parse() function of the parser" float=ht frame=tb}
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
```

With these functions, it is easy to write a recursive descent parser. For example, the code for parsing `<basePermission> * <permission>` for pointer permission is just this:

```go
func (p *Parser) parsePointer(bp BasePermission) Permission {
	p.sc.Expect(TokenStar)
	rhs := p.parseInner()
	return &PointerPermission{BasePermission: bp, Target: rhs}
}
```

Internally the scanner is implemented by a set of functions:

* `func (sc *Scanner) readRune() rune` returns the next Unicode run from the input string
* `func (sc *Scanner) unreadRune()` moves on rune back in the input stream
* `func (sc *Scanner) scanWhile(typ TokenType, acceptor func(rune) bool) Token` creates a token by reading and appending runes as long as the given acceptor returns true.

The main Scan() function calls `readRune` to read a rune and based on that rune decides the next step. For single character tokens, the token matching the rune is returned directly. If the rune is a character, then `unreadRune()` is called to put it back
and `sc.scanWhile(TokenWord, unicode.IsLetter)` is called to scan the entire word (including the unread rune). Then it is checked if the word is a keyword, and if so, the proper keyword token is returned, otherwise the word is returned as a token of type `Word` (which is used to represent permission bitsets, since the flags may appear in any order). Whitespace in the input is skipped:

```go
for {
    switch ch := sc.readRune(); {
    case ch == 0:
        return Token{}
    case ch == '(':
        return Token{TokenParenLeft, "("}
    ...
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
```

## Representation of permissions
Base permissions are implemented as integer bitfields. Intersection is the bitwise and operation; union is the bitwise or operation.

## Representation of the store
The store stores a slice of structs where each struct contains a name, an effective permission, a maximal permission, and the number of times
the variable has been referenced so far. Defining a new variable or beginning a new block scope prepends to the store.

```go
type Store []struct {
	name string
	eff  permission.Permission
	max  permission.Permission
	uses int
}
```

The beginning of a block scope is marked by a struct where the fields have their zero values, that is `{"", 0, 0, 0}`. More specifically,
such a block marker is identified by checking if the name field is empty.

When exiting a block, we simply find the first such marker, and then create a slice starting with the element following it.
