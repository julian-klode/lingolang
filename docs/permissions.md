
# Permissions for Go
In the previous chapter, we saw monads, linear types, and the two generalisations of linear types as capabilities
and fractional permissions. This chapter introduces permissions for Go based on the concepts from 'Capabilities for Sharing'[@Boyland:2001:CSG:646158.680004],
and certain operations that will be useful to build a static analyser that checks permissions on a Go program. In the `github.com/julian-klode/lingolang` reference implementation, the permissions and operations are provided in the `permission` package.

The reasons for going with a capabilities-derived approach are simple: Monads don't work in Go, as Go does not
have generic types; and fractional permissions are less powerful, and we also need to deal with legacy code and perhaps could use some other permissions for describing Go-specific operations, like a permission for allowing a function to be executed as a goroutine.

## Structure of permissions
This approach to linear types in Go is called _Lingo_ (short for linear Go). Permissions in Lingo are different from the original
approach in 'Capabilities for Sharing' in a few points. First of all, because Go has no notion of identity,
since it uses pointers, the right for that is dropped. We end up with 5 rights, or permission bits:
`r` (read), `w` (write), `R` (exclusive read), `W` (exclusive write), and `o` (ownership).

A permission is called _linear_ iff it contains an exclusive right matched with its base right, that is, either
`rR` or `wW`.
Compared to the introduction of linear types, the ones to be introduced are not single-use values, but rather may only have a single reference at a time, which is conceptually equivalent to having the same parameter act as both input and output arguments, where passing a value makes a function use it and then write back a new one.
A linear object can only be referenced by or contained in other linear objects, in order to preserve linearity.
For example, in the following code, $b$ is an array of mutable pointers. If the array were non-linear, we
could copy it to $b$, creating two references to each linear element, which is not allowed.
```go
    var a /* @perm orR [] owW * owW */ = make([]int)
    var b = a
```
We will see later that this actually would not be a problem, as the checks are recursive and would prevent such an object from being copied, but it makes no real sense to have an object marked as non-linear contain a linear one - it would be nothing more than a lie.

Ownership plays an interesting role with linear objects: It determines whether a reference is _moved_ or _borrowed_ (temporarily moved). For example, when passing a linear map to a function expecting an owned map, the map will be moved inside the function; if the parameter is unowned, the map will be borrowed instead. Coming back to the analogy with in and out parameters, owned parameters are both in and out, whereas unowned parameters are only in.

```{#syntax caption="Permission syntax" float=t frame=tb}
main <- inner EOF
inner <- '_' | [[basePermission] [func | map | chan | pointer | sliceOrArray] | basePermission]
basePermission ('o'|'r'|'w'|'R'|'W'|'m'|'l'|'v'|'a'|'n')+
func <- ['(' param List ')'] 'func' '(' [paramList] ')'
        ( [inner] |  '(' [paramList] ')')
paramList <- inner (',' inner)*
fieldList <- inner (';' inner)*
sliceOrArray <- '[' [NUMBER|_] ']' inner
chan <- 'chan' inner
chan <- 'interface' '{' [fieldList] '}'
map <- 'map' '[' inner ']' inner
pointer <- '*' inner
struct <- 'struct' '{' fieldList '}'
```

Instead of using a store mapping objects, and (object, field) tuples to capabilities, that is, (object, permission) pairs, Lingo employs a different approach in order to combat the limitations shown in the introduction:
Lingo's store maps a variable to a permission.
In order to represents complex data structures, it does however not just have the permission bits introduced earlier (from now on called _base permission_), but also _structured_ permissions, which are similar to types.
These structured permissions consist of a base permission and permissions for each child, target, etc. - a "shadow" type system essentially.

There is one problem with the approach of one base permission and one permission per child: Reference types like maps or functions actually need two base permissions:
The permission of the reference (as in, "can I assign a different map to this variable") and the permission of the referenced value (as in, "can I insert something into this map").
We will see later that this causes some issues.

Apart from primitive and structured permissions, there are also some special permissions:

* The untyped nil permission, representing the `nil` literal. \label{sec:untyped-nil}
* The wildcard permission, written `_`. It is used in permission annotations whenever the default permission for a type should be used.

There also are some shortcuts for some common combinations:

* `m`, for _mutable_, is equivalent to `rwRW`
* `v`, for _value_, is equivalent to `rW`
* `l`, for _linear value_, is equivalent to `rRW` and a linear variant of value
* `n`, for _none_, is equivalent to, well, none bits set
* `a`, for _any_, is equivalent to all non-exclusive bits set, that is `orwRW`.

The full syntax for these permissions is given in listing \ref{syntax}. The base permission does not need to be specified for structured types, if absent, it is considered to be `om`.

In the rest of the chapter, we will discuss permissions using a set based notation: The set of rights, or permissions bits is ${\cal R} = \{o, r, w, R, W\}$. A base permission
is a subset  $\subset \cal R$ of it, that is an element in $2^{\cal R}$. The set $\cal P$ is the infinite set of all permissions:

$$ {\cal P} = 2^{\cal R} \cup \{p \textbf{ struct } \{P_0, ..., P_n \} | p \subset {\cal R}, P_i \in {\cal P} \} \cup \ldots \cup \{nil, \_\}$$

Compare the syntax chart in listing \ref{syntax} for which permissions are possible.

Base permissions like $b \in 2^{\cal R}$ are usually denoted by lower case, other permissions (or generically, all permissions) are
denoted by uppercase characters like $P \in {\cal R}$.

### Excursus: Parsing the syntax
In the implementation, base permissions are stored as bitfields and structured permissions are structs matching the abstract syntax. Permission annotations are stored in comments attached to functions, and declarations of variables. A comment line introducing a permission annotation starts with `@perm`, for example:

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


## Assignment operations
Some of the core operations on permissions involve assignability: Given a source permission and a target permission, can I assign an object with the source permission to a variable of the target permission?

As a value based language, one of the most common forms of assignability is copying:
```go
var x /* @perm or */ = 0
var y = x   // copy
```
Another one is referencing:
```go
var x /* @perm or */ = 0
var y = &x   // reference
var z = y    // still a reference to x, so while we copy the pointer, we also reference x one more time
```
Finally, in order to implement linearity, we need a way to move things:
```go
var x /* @perm om */ = 0  // this was or before (!!!)
var y = &x   // have to move x, otherwise y and x both reach x
var z = y    // have to move the pointer from y to z, otherwise both reach x
```
(Though we are moving the permissions, not the values in `&x` - the value is still stored in the location of `x`)

In the following, the function $ass_\mu: P \times P \to bool$ describes whether a value of the left permission can be assigned to a location of the right permission. $\mu$ is the mode, it can be either
$cop$ for copy, $ref$ for reference, or $mov$ for move.

The base case for assigning is base permissions. For copying, the only requirement is that the source is readable (or it and target are empty). A move additionally requires that no more permissions are added - this is needed: If I move a pointer to a read-only object, I can't move it to a pointer to a writeable object, for example. When referencing, the same no-additional-permissions requirement persists, but both sides may not be linear - a linear value can only have one reference, so allowing to create another would be wrong.
\begin{align*}
    ass_\mu(a, b) &:\Leftrightarrow \begin{cases}
        r \in a \text{ or } a = b =  \emptyset                                           & \text{if } \mu = cop \\
        b  \subset a \text{ and } (r \in A \text{ or } a = b = \emptyset)                & \text{if } \mu = mov \\
        b  \subset a \text{ and } \text{ and not } lin(a) \text{ and not } lin(b)        & \text{if } \mu = ref
    \end{cases} \\
    \text{where } & lin(a) :\Leftrightarrow r, R \in a \text{ or } w, W \in a
\end{align*}\label{sec:assign}

The $r \in a \text{ or } a = b =  \emptyset$ requirement for $mov$ is not entirely correct. There really should be two kind of move operations: Moving a value (which needs to read the value), and
moving a reference to something as $b \subset a$ (like moving a pointer `om * om` does not require reading the `om` target). The latter is essentially similar to a subtype relationship if permissions were types.

In the code, the function is implemented like in listing \ref{assignabilitybase}:
```{#assignabilitybase caption="Base case of assignability, in code form" float=t frame=tb}
func (perm BasePermission) isAssignableTo(p2 Permission, state assignableState) bool {
	perm2, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	switch state.mode {
	case assignCopy:
		return perm&Read != 0 || (perm == 0 && perm2 == 0) // Either A readable, or both empty permissions (hack!)
	case assignMove:
		return perm2&^perm == 0 && (perm&Read != 0 || (perm == 0 && perm2 == 0)) // No new permission && copy
	case assignReference:
		return perm2&^perm == 0 && !perm.isLinear() && !perm2.isLinear() // No new permissions and not linear
	}
	panic(fmt.Errorf("Unreachable, assign mode is %v", state.mode))

}
```

Next up are permissions with value semantics: arrays, structs, and tuples (tuples are only used internally to represent multiple function results). They are assignable if all their children are assignable.
\begin{align*}
    ass_\mu(a\ [\_]A, b\ [\_]B) &:\Leftrightarrow ass_\mu(a, b) \text{ and } ass_\mu(A, B)     \\
    \begin{aligned}
        ass_\mu(&a \textbf{ struct } \{ A_0; \ldots; A_n \}, \\
            &b \textbf{ struct } \{ B_0; \ldots; B_m \})
    \end{aligned} &:\Leftrightarrow
        ass_\mu(a, b) \text{ and } ass_\mu(A_i, B_i)    \quad \forall 0 \le i \le n \\
    \begin{aligned}
        ass_\mu(a \ ( A_0, \ldots, A_n),
            b \ ( B_0, \ldots, B_m))
    \end{aligned} &:\Leftrightarrow
        ass_\mu(a, b) \text{ and } ass_\mu(A_i, B_i)    \quad \forall 0 \le i \le n
\end{align*}

Channels, slices, and maps are reference types. They behave like value types, except that copying is replaced by referencing.
\begin{align*}
    ass_\mu(a \textbf{ chan } A, b \textbf{ chan } B) &:\Leftrightarrow \begin{cases}
        ass_{ref}(a, b) \text{ and } ass_{ref}(A, B)    & \mu = cop \\
        ass_\mu(a, b) \text{ and } ass_\mu(A, B)    & \text{else}
    \end{cases} \\
    ass_\mu(a\ []A, b\ []B) &:\Leftrightarrow \begin{cases}
        ass_{ref}(a, b) \text{ and } ass_{ref}(A, B)    & \mu = cop \\
        ass_\mu(a, b) \text{ and } ass_\mu(A, B)    & \text{else}
    \end{cases} \\
    ass_\mu(a \textbf{ map }[A_0] A_1, b \textbf{ map }[B_0] B_1) &:\Leftrightarrow \begin{cases}
        ass_{ref}(a, b) \text{ and } ass_{ref}(A_0, B_0) \text{ and } ass_{ref}(A_1, B_1)    & \mu = cop \\
        ass_\mu(a, b) \text{ and } ass_\mu(A_0, B_0) \text{ and } ass_\mu(A_1, B_1)    & \text{else} \\
    \end{cases}
\end{align*}

Interfaces work the same, but methods are looked up by name:
\begin{align*}
    \begin{aligned}
        ass_\mu(&a \textbf{ interface } \{ A_0; \ldots; A_n \}, \\
            &b \textbf{ interface } \{ B_0; \ldots; B_m \})
    \end{aligned} &:\Leftrightarrow  \begin{cases}
        ass_{ref}(a, b) \text{ and } ass_{ref}(A_{idx(B_i, A)}, B_i)    & \mu = cop\\
        ass_\mu(a, b) \text{ and } ass_\mu(A_{idx(B_i, A)}, B_i)    & \text{else}\\
        \end{cases} \\
        & \qquad \ \text{ for all } 0 \le i \le m
\end{align*}
where  $idx(B_i, A)$ determines the position of a method with the same name as $B_i$ in $A$.

Function permissions are a fairly special case.
The base permission here essentially indicates the permission of (elements in) the closure.
A mutable function is thus a function that can have different results for the same immutable parameters.
The receiver of a function, its parameters, and the closure are essentially parameters of the function,
and parameters are contravariant: I can pass a mutable object when a read-only object is expected, but I
can't pass a read-only object to a mutable object. For the closure, ownership is the exception: An owned function can be assigned to an
unowned function, but not vice versa:
\begin{align*}
    \begin{aligned}
        ass_\mu(&a\ (R) \textbf{ func } ( P_0 \ldots, P_n ) (R_0, \ldots, R_m), \\
            &b\ (R') \textbf{ func } ( P'_0 \ldots, P'_n ) (R'_0, \ldots, R'_m))
    \end{aligned} &:\Leftrightarrow  \begin{cases}
        ass_{ref}(a \cap \{o\}, b \cap \{o\}) \\
                     \quad\text{and } ass_{ref}(b \setminus \{o\}, a \setminus \{o\})  \\
                     \quad\text{and } ass_{mov}(R', R) \\
                     \quad\text{and } ass_{mov}(P'_i, P_i) \\
                     \quad\text{and } ass_{mov}(R_j, R'_j)   & \mu = cop\\
        ass_\mu(a \cap \{o\}, b \cap \{o\}) \\
                     \quad\text{and } ass_\mu(b \setminus \{o\}, a \setminus \{o\})  \\
                     \quad\text{and } ass_{mov}(R', R) \\
                     \quad\text{and } ass_{mov}(P'_i, P_i) \\
                     \quad\text{and } ass_{mov}(R_j, R'_j)   & \text{else}\\
        \end{cases} \\
        & \qquad \ \text{ for all } 0 \le i \le n, 0 \le j \le m
\end{align*}

$mov$ is used for the receiver, parameters, and return values, due to containing the sub-permission-of semantic. That it also contains the
read requirements was unintended, and can cause some issues here: A function with an unreadable argument cannot be copied (usually, unless both
source and target parameter permissions are simply $n$), for example.

Pointers are another special case: When a pointer is copied, the pointer itself is copied, but the target is referenced (as we now have two pointers to the same target):
\begin{align*}
    ass_\mu(a * A, b * B) &:\Leftrightarrow \begin{cases}
        ass_\mu(a, b) \text{ and } ass_{ref}(A, B)    & \mu = cop \\
        ass_\mu(a, b) \text{ and } ass_\mu(A, B)    & \text{else}
    \end{cases}
\end{align*}
There is one minor deficiency with this approach: A pointer `a` with permission `ol * om` cannot be moved into a pointer `b` with permission `om * om`, due to the rule about not adding any permissions. But that's not
always correct, which brings us back to the moving values vs moving references: `b = a` should be possible, but it should not be possible to assign a pointer to `a` (e.g. `om * ol * om`) to a pointer to b (e.g. `om * om * om`) - now we could access b with more permissions than we created it with. This issue means that a function should probably accept `ol * om` pointers rather than `om * om`, but that seems a minor issue.

Finally, we have some special cases: The wildcard and nil. The wildcard is not assignable, it is only used when writing permissions to mean "default". The `nil` permission is assignable to itself, to pointers, and permissions for reference and reference-like types.
\begin{align*}
        ass_\mu(\textbf{\_}, B)  &:\Leftrightarrow \text{ false } \\
        ass_\mu(\textbf{nil}, a * B)  &:\Leftrightarrow \text{ true } & ass_\mu(\textbf{nil}, a \textbf{ chan } B)  &:\Leftrightarrow \text{ true } \\
        ass_\mu(\textbf{nil}, a \textbf{ map } [B]C)  &:\Leftrightarrow \text{ true } &
        ass_\mu(\textbf{nil}, a []C)  &:\Leftrightarrow \text{ true } \\
        ass_\mu(\textbf{nil}, a \textbf{ interface } \{ \ldots \})  &:\Leftrightarrow \text{ true } &
        ass_\mu(\textbf{nil}, \textbf{nil})  &:\Leftrightarrow \text{ true }
\end{align*}\label{sec:ass-nil}

## Conversions to base permissions
Converting a given permission to a base permission essentially replaces all base permissions in that permission with the specified one, except for some exceptions like functions, which we'll see later in this section. Its major use case is specifying an incomplete type, for example:\label{sec:ctb}

```go
var x /* @perm om */ *int
```
It's a pointer, but the permission is only for a base. We can convert the default permission for the type (we'll discuss type default permissions later in \fref{sec:new-from-type}) to `om`, giving us a complete permission. And in the next section, we'll extend conversion to arbitrary prefixes of a permission.

Another major use case is ensuring consistency of rules, like:

- Unwriteable objects may not embed any writeable objects
- Non-linear unwriteable objects may contain pointers to non-linear writeable objects
- Linear unwriteable objects may point to linear writeable objects.

(That is, while unwriteable objects cannot contain writeable objects directly, they can point to them as long as linearity is respected)

As every specified permission will be converted to its base type, we can ensure that every permission is consistent, and we don't end up with inconsistent permissions like `or * om` - a pointer that could be copied, but pointing to a linear object.

The function $ctb : {\cal P} \times 2^{\cal R} \to {\cal P}$ is the convert-to-base function. It's simple cases are:
\begin{align*}
    ctb(a, b) &:= b \\
    ctb(\_, b) &:= b \\
    ctb(nil, b) &:= nil \\
    ctb(a \textbf{ chan } A, b) &:= ctb(a, b) \textbf{ chan } ctb(A, ctb(a, b)) \\
    ctb(a \textbf{ } []A, b) &:= ctb(a, b) \textbf{ } []ctb(A, ctb(a, b))       \\
    ctb(a \textbf{ } [\_]A, b) &:= ctb(a, b) \textbf{ } [\_]ctb(A, ctb(a, b))   \\
    ctb(a \textbf{ map} [A]B, b) &:= ctb(a, b) \textbf{ map} [ctb(A)]ctb(B, ctb(a, b))   \\
    ctb(a \textbf{ struct } \{ A_0; \ldots; A_n \}, b) &:= ctb(a, b) \textbf{ struct }  \{ ctb(A_0, ctb(a, b)); \ldots; ctb(A_n, ctb(a, b)) \}   \\
    ctb(a\ ( A_0; \ldots; A_n), b) &:= ctb(a, b)\ ( ctb(A_0, ctb(a, b)); \ldots; ctb(A_n, ctb(a, b)) )
\end{align*}\label{sec:ctb-nil}

For comparison, this is how the first case looks in the reference implementation:
```go
func (perm BasePermission) convertToBaseBase(perm2 BasePermission) BasePermission {
	return perm2
}
```

The rules are problematic in some sense, though: All children have the same base permission as their parent. This kind of makes sense for non-reference
values like structs containing integers - after all, they are in one memory location; but for reference types, it is somewhat confusing: For example, a struct
cannot have both a mutable (`om map...`) and a read-only map (`or map...`) as their base permissions are different. As mentioned before, these really need
a second base permission for the object being referenced (like a pointer, see below). Then both maps could be (linear) read-only references, one referencing
a mutable map, one referencing a read-only map.

Functions and interfaces are special, again: methods, and receivers, parameters, results of functions are converted to their own base permission:
\begin{align*}
    ctb(a\ (R) \textbf{ func } ( P_0, \ldots, P_n ) (R_0, \ldots, R_m), b) &:=&&  ctb(a, b)\ (ctb(R, base(R))) \textbf{ func }  \\
                                                                             &&&  ( ctb(P_0, base(P_0)), \ldots, ctb(P_n, base(P_n)) )  \\
                                                                             &&&  (ctb(R_0, base(R_0)), \ldots, ctb(R_m, base(R_m)))  \\
    ctb(a \textbf{ interface } \{ A_0; \ldots; A_n \}, b) &:=&& ctb(a, b) \textbf{ interface } \{ \\
                &&&\quad ctb(A_0, base(A_0)) \\
                &&&\quad\ldots; \\
                &&&\quad ctb(A_n, base(A_n)) \\
                &&&\}
\end{align*}
The reason for this is simple: Consider the following example:
```go
    var x /* om */ func(*int) *int
```
`x` should be `om`, but this does not mean that it should be `om func (om * om) om` just because the closure might be mutable - a function parameter should have the least permissions possible, so you can pass as many things as possible into it. The default also should be unowned, so a function does not consume it if it is linear, but releases it again later, so it can be used again in the caller.


For pointers, it is important to add one thing: There are two types of conversions: Normal ones and strict ones. The difference is simple: While the normal one combines the old target's permission with the permission being converted to, strict conversion just converts the target to the specified permission. Strict conversions will become important when doing a type conversion (recall \fref{sec:conversions}), for example, value to interface:
```go
var x /* om * or */ *int
var y /* om interface {} */ = x
var z /* om * om */ = y.(*int)     // um, target is mutable now?
```
Converting to an interface is a lossy operation: We can only maintain the outer permission. But we cannot allow the case above to happen: We just converted a pointer to read-only data to a pointer to writeable data. Not good. One way to solve this is to ensure that a permission can be assigned to it is strict permission, gathered by strictly converting the type-default permission to the current permissions base permission:
$$
y = x \Leftrightarrow  ass_\mu(perm(x), ctb_{strict}(perm(typeof(x)), base(perm(x)) \text { and } ass_\mu(base(x), base(y))
$$
\begin{samepage}
The rules for converting a pointer permission to a base permission are therefore a bit complicated:
\begin{align*}
    &&ctb(a * A, b)                  :&= a' * ctb(A, t_2)\\
    &&\quad \text { where }  a' &= ctb(a, b) \\
    &&                       t_0 &= (base(A) \setminus \{o\}) \cup (a' \cap \{o\}) \\
    &&                       t_1 &= \begin{cases}
                                    t_0 \setminus \{w, W\} & \text{if } R \not\in a' \text{ and } t_0 \supset \{w,W\} \\
                                    t_0 & \text{else} \\
                                    \end{cases} \\
    &&                       t_2 &= \begin{cases}
                                    t_1 \setminus \{R\} & \text{if } R \not\in a' \text{ and } t_1 \supset \{r, R\} \\
                                    t_1 & \text{else} \\
                                    \end{cases} \\
    &&ctb_{strict}(a * A, b)   :&= ctb_{strict}(a, b) * ctb_{strict}(A, ctb_{strict}(a, b)) \\
\end{align*}
\end{samepage}
The steps $t_0, t_1, t_2$ do the following:

0. The owned flag from the old target base permission is replaced with the owned flag from the given base permission. This is needed to ensure that we don't accidentally convert `om * om` to `m * om`. Keeping ownership the same throughout pointers also simplifies some other aspects in later code.
1. If the new base permission has no exclusive read right, but the new target has exclusive write and write flags (is linearly writeable), these flags are dropped.
2. If the new base permission has no exclusive read right, but the new target has exclusive read and read flags (is linearly readable), the exclusive read flag is dropped.

Steps 1 and 2 make it consistent: Without them, we could have a non-linear pointer pointing to a linear target. Since the target could only have one reference, but the pointer appears to be copyable (it is not, as the assignability rules also work recursively), we get the impression that we could have two pointers for the same target. It also allows us to just gather linearity info from the outside: If the base permission of a value is non-linear, it cannot contain linear values - this can be used to simplify some checks.

#### Theorem: $ctb_b(A) = ctb(A, b)$ is idempotent
_Theorem:_ Conversion to base, $ctb$ is idempotent, or rather $ctb_b(A) = ctb(A, b)$ is. That is, for all $A \in {\cal P}, b \subset {\cal R}$: $ctb_b(A) = ctb(A, b) = ctb(ctb(A, b), b) = ctb_b(ctb_b(A))$.

_Background:_ This theorem is important because we generally assume that $ctb(A, base(A)) = A$ for all $A \in {\cal P}$ that have been converted once (what is called consistent, and is the case for
all permissions the static analysis works with).

_Proof._ This only proves $ctb()$, not $ctb_{strict}()$, but the only difference is the pointer case, which can be proven like channels below.

1. Simple cases:
    \begin{align*}
        ctb(ctb(a, b), b) &= ctb(b, b) = b = ctb(a, b) \\
        ctb(ctb(\_, b), b) &= ctb(b, b) = b = ctb(\_, b)\\
        ctb(ctb(nil, b), b) &= ctb(nil, b) = nil = ctb(nil, b)\\
    \end{align*}
1. Channels, slices, arrays, maps, structs, and tuples basically have the same rules: All children are converted to the same base permission as well. It suffices to show the proof for one of them. Let us pick channels:
    \begin{align*}
        & ctb(ctb(a \textbf{ chan } A, b), b) \\
                                            &= ctb(ctb(a, b) \textbf{ chan } ctb(A, ctb(a, b)), b) & \text{(def chan)} \\
                                            &= ctb(ctb(a, b), b) \textbf{ chan } ctb(ctb(A, ctb(a, b)), b) & \text{(def chan)}\\
                                            &= ctb(b, b) \textbf{ chan } ctb(ctb(A, b), b) & (ctb(a, b) = b) \\
                                            &= b \textbf{ chan } ctb(A, b)  &  (ctb(a, b) = b, \text{other case}) \\
                                            &= ctb(a, b) \textbf{ chan } ctb(A, ctb(a, b)) & (ctb(a, b) = b, \text{other case}) \\
                                            &= ctb(a \textbf{ chan } A, b) & \text{(def chan)}
    \end{align*}
1. Functions and interfaces convert their child permissions to their own bases. We can proof the property for the special case of an interface with one method without loosing genericity, since these are structured the same.
    \begin{align*}
        &ctb(ctb(a \textbf{ interface } \{ A_0 \}, b), b) \\
        =& ctb(ctb(a, b) \textbf{ interface } \{  ctb(A_0, base(A_0))  \}, b) & \text{by definition}  \\
        =& \underbrace{ctb(ctb(a, b), b)}_{= ctb(a, b)} \textbf{ interface } \{ ctb(ctb(A_0, base(A_0)), \underbrace{base(ctb(A_0, base(A_0))))}_{= base(A_0) \text{(trivial)}} \}  & \text{by definition}\\
        =& ctb(a, b) \textbf{ interface } \{  \underbrace{ctb(ctb(A_0, base(A_0)), base(A_0))}_{\text{case of $ctb(ctb(A, b), b)$}} \} \\
        =& ctb(a, b) \textbf{ interface } \{ ctb(A_0, base(A_0)) \} \\
        =& ctb(a \textbf{ interface } \{ A_0 \}, b) & \text{by definition}
    \end{align*}
1. Pointers are more complicated:

    Let $ctb(a * A, b) = a' * ctb(A, t_2)$ with $a' = ctb(a,b)$ and a $t_2$ according to the definition. And  $ctb(ctb(a * A, b), b) = ctb(a' * ctb(A, t_2), b) = a'' * ctb(A, t_2')$. We have to show that $a' = a''$ and $t_2 = t_2'$.

    We can write $$t_2 = t_0 \setminus X = \overbrace{base(A) \setminus \{o\} \cup (a' \cap \{o\})}^{= t_0} \setminus X   \qquad\qquad \text{(short } t_2 \text{)}$$ for some set $X \in \{\{R\}, \{w, W\}, \{R, w, W\}\}$; depending on the value of A, and likewise for $t_2'$.

    Given that $a' = a''$, it follows that:

    \begin{align*}
        t_0' &= ( \underbrace{base(ctb(A, t_2)}_{= t_2}) \setminus \{o\}) \cup (\underbrace{a''}_{= a'} \cap \{o\}) \\
            &= ( t_2 \setminus \{o\}) \cup (a' \cap \{o\}) \\
            &=  t_2 & \text{(short } t_2 \text{)}
    \end{align*}

    Now, let's look at $t_1'$ and $t_2'$. There are two variants each: $t_i = t_{i-1}$ and $t_i = t_{i-1} \setminus X$ for some $X$ if  $R \not\in a'$ and some other condition holds. Therefore, for $R \in a'$,
    it trivially follows that $t_0 = t_1 = t_2$ and $t_0' = t_1' = t_2'$. Let's assume $R \not\in a'$. For $t_1'$, this means:
    \begin{align*}
                            t_1' &= \begin{cases}
                                        t_0' \setminus \{w, W\} & \text{if } R \not\in a'' \text{ and } t_0' \supset \{w,W\} \\
                                        t_0' & \text{else} \\
                                        \end{cases} \\
                                    &= \begin{cases}
                                        t_0' \setminus \{w, W\} & \text{if } t_0' \supset \{w,W\} \\
                                        t_0' & \text{else} \\
                                        \end{cases}
    \end{align*}
    The first case cannot happen: If $t_0' \supset \{w, W\} \Rightarrow t_2 \supset \{w, W\} \xRightarrow{t_2 \subset t_1} t_1 \supset \{w, W\} \Rightarrow t_0 \supset \{w,W\} \Rightarrow t_1 = t_0 \setminus \{w, W\} \Rightarrow t_1 \not\supset \{w, W\}$ - a contradication.
    Thus $t_1' = t_0' = t_2$.

    Now consider $t_2'$:
    \begin{align*}
                            t_2' &= \begin{cases}
                                        t_1' \setminus \{R\} & \text{if } R \not\in a'' \text{ and } t_1' \supset \{r,R\} \\
                                        t_1' & \text{else} \\
                                        \end{cases} \\
                                    &= \begin{cases}
                                        t_1' \setminus \{R\} & \text{if } t_1' \supset \{r,R\} \\
                                        t_1' & \text{else} \\
                                        \end{cases}
    \end{align*}
    And again, the first case cannot happen, it leads to a contradiction: If $t_1' = t_2 \supset \{r, R\} \Rightarrow t_1 \supset \{r, R\} \Rightarrow t_2 = t_1 \setminus \{R\} \Rightarrow t_2 \not\supset \{r, R\}$.

    Therefore, $t_2' = t_1' = t_0' = t_2$, and thus $ctb(ctb(a * A, b), b) = ctb(a * A, b)$.

In conclusion, $ctb(ctb(A, b), b) = ctb(A, b)$ for all $A \in {\cal P}, b \subset {\cal R}$, as was to be shown. It also follows that
$ctb_{strict}(ctb_{strict}(A, b), b) = ctb_{strict}(A, B)$ because the functions are the same, except for the diverging
pointer case, but that one is trivial to proof (like channels). \qed


## Other conversions and merges
The idea of conversion to base permissions from the previous paragraph can be extended to converting between structured types. When converting between two structured types, replace all base permissions in the source with the base permissions in the same position in the target, and when the source permission is structured and the target is base, it just switches to a to-base conversion. \label{sec:convert}

There are two more kinds of recursive merge operations: intersection and union.
These are essentially just recursive relaxations of intersection and union on the base permissions, that is, they simply perform intersection and union on all base types
in the structure.
A static analyser could use intersections to join the results of different branches, for example:

```go
if (...) {
    myfun = function expecting mutable value
} else {
    myfun = function expection read-only value
}

myfun = intersect(myfun in first branch, my fun in second branch)
```

In this example, after the if/else block has been evaluated, the permissions of `myfun` are an intersection of the permission it would have in both branches.

The function performing merges and conversions is $merge_\mu: P \times P \to P$. $\mu$ is the mode, which can be either union ($\cup$),
intersection ($\cap$), conversion ($ctb$), or strict conversion ($ctb_{strict}$).

In essence, $merge_\mu$ just extends an underlying function $\mu: 2^{\cal R} \times 2^{\cal R} \to {\cal P}$ ($\cap$ and $\cup$) or $\mu: P \times 2^{\cal R} \to {\cal P}$
($ctb$ and $ctb_{strict}$) to a function ${\cal P} \times {\cal P} \to {\cal P}$. In the latter case, we directly use $\mu(A,b)$
for all structured permissions $A$ and base permissions $b$, so the function can do special handling for the structured permission in the first
argument.
\begin{align*}
    merge_\mu(A, b)     &:=  \mu(A,b) =\begin{cases}
                            ctb(A, b)   & \text{if } \mu = ctb \\
                            ctb_{strict}(A, b)   & \text{if } \mu = ctb_{strict}
                        \end{cases}  \\
                    &\text{for all } \mu: P \times 2^{\cal R} \to {\cal P} \text{ and } A  \in {\cal P} \setminus 2^{\cal R}
\end{align*}


The wildcard exists just as a placeholder for annotation purposes, so merging it with anything should yield the other value. For nil permissions, merging them with a nilable
permission (a chan, func, interface, map, nil, pointer, or slice permission) yields the other permission:
\begin{align*}
    merge_\mu(\_, B)    &:= \_ &&& merge_\mu(A, \_)     &:= \_ \\
    merge_\mu(N, nil)   &:= N   &&& merge_\mu(nil, N)   &:= N  & \text{ for all nilable } N \in {\cal P} \text{ and } N = nil
\end{align*}\label{sec:merge-nil}
Regarding the soundness of the merging nils with nilable permissions:

* For union, the question is: Can $merge_{\cup}(N, nil)$ be used in place of both $N$ and $nil$? Technically the answer is no, because $N$ cannot be used where $nil$ is expected. But nil permissions are only ever
  used for $nil$ literals (they cannot even be specified, there is no syntax for them), so we never reach that situation.
* For intersection, the question is: Can values of $N$ or $nil$ be assigned to $merge_{\cap}(N, nil)$. Yes, they can be, $nil$ is assignable to every pointer, and $p$ is assignable to itself.

Otherwise, the base case for a merge is merging primitive values, and the rules for that are quite simple:
\begin{align*}
    merge_\mu(a, b)     &:= \mu(a,b) = \begin{cases}
                            ctb(a,b) & \text{if } \mu = ctb \text{ or } \\
                            ctb_{strict}(a,b) & \text{if } \mu = ctb_{strict} \\
                            a \cap b & \text{if } \mu = \cap       \\
                            a \cup b & \text{if } \mu = \cup
                        \end{cases}
\end{align*}

In the code, this is implemented as a function on some special `mergeState` type (listing \ref{mergebase}). This state happens to record the mode
of operation for the merge function, so the recursion does not need to be duplicated for each of them. It also has another
use case, to which we will get back later, at the end of the chapter. The fu
```{#mergebase caption="Base case of merge, in code form" float=htb frame=tb}
func (state *mergeState) mergeBase(p1, p2 BasePermission) BasePermission {
	switch state.action {
	case mergeConversion, mergeStrictConversion:
		return p1.convertToBaseBase(p2) // call ctb base case for type reasons
	case mergeIntersection:
		return p1 & p2
	case mergeUnion:
		return p1 | p2
	}
	panic(fmt.Errorf("Invalid merge action %d", state.action))
}
```

Pointers, channels, arrays, slices, maps, tuples, structs, and interfaces are trivial (structs and interfaces must have same number of members / methods):
\begin{align*}
    merge_\mu(a * A, b * B)     &:= merge_\mu(a, b) * merge_\mu(A, B) \\
    merge_\mu(a \textbf{ chan } A, b \textbf{ chan } B)  &:= merge_\mu(a, b) \textbf{ chan } merge_\mu(b, B) \\
    merge_\mu(a [\_] A, b [\_] B)  &:= merge_\mu(a, b) [\_] merge_\mu(b, B) \\
    merge_\mu(a [] A, b [] B)  &:= merge_\mu(a, b) [] merge_\mu(b, B) \\
    merge_\mu(a \textbf{ map}[A_0]\ A_1, b \textbf{ map}[B_0]\ B_1)  &:= merge_\mu(a, b) \textbf{ map}[merge_\mu(A_0, B_0)]\ merge_\mu(A_1, B_1) \\
    merge_\mu(a ( A_0, \ldots, A_n ), b (B_0, \ldots, B_n ) ) &:= merge_\mu(a, b) (merge_\mu(A_0, B_0), \ldots, merge_\mu(A_n, B_n) ) \\
    merge_\mu(a \textbf{ struct } \{A_0, \ldots, A_n \}, \\
          \qquad b \textbf{ struct } \{B_0, \ldots, B_n \} )
        &:= merge_\mu(a, b) \textbf{ struct } \{merge_\mu(A_0, B_0), \ldots,  \\
                                           & \qquad merge_\mu(A_n, B_n) \} \\
    merge_\mu(a \textbf{ interface } \{A_0, \ldots, A_n \}, \\
          \qquad b \textbf{ interface } \{B_0, \ldots, B_n \} )
        &:= merge_\mu(a, b) \textbf{ interface } \{merge_\mu(A_0, B_0), \ldots,  \\
                                           & \qquad merge_\mu(A_n, B_n) \}
\end{align*}

Functions are more difficult: An intersection of a function requires union for closure, receivers, and parameters, because just like with subtyping (in languages that have it), parameters and receivers are contravariant:
If one function expects `orw` and another expects `or` a place that needs either of those functions (an intersection) needs a function that accepts $orw \cup or = orw$ - because passing a writeable object to a function only needing a read-only one would work, but passing a read-only value to a function that needs a writeable one would not be legal.

For that, let
$$
mergeContra_\mu(A, B) := \begin{cases}
    merge_{\cap}(A, B) & \text{if } \mu = \cup \\
    merge_{\cup}(A, B) & \text{if } \mu = \cap \\
    merge_\mu(A, B) & \text{else}
\end{cases}
$$
be a helper function that merges contravariant things after swapping union and intersection modes.

Then merging functions is:
\begin{align*}
    merge_\mu(&a (R) \textbf{ func } (P_0, \ldots, P_n) (R_0, \ldots, R_n),  b (R') \textbf{ func } (P'_0, \ldots, P'_n) (R'_0, \ldots, R'_n)) \\
       := &mergeContra_\mu(a, b) (mergeContra_\mu(R, R')) \textbf{ func } \\
          &\qquad (mergeContra_\mu(P_0, P'_0), \ldots, mergeContra_\mu(P_n, P'_n)) \\
          &\qquad (merge_\mu(R_0, R'_0), \ldots, merge_\mu(R_n, R'_n))
\end{align*}

## Creating a new permission from a type
\label{sec:new-from-type}
Since permissions have a similar shape as types and Go provides a well-designed types package, we can easily navigate type structures and create structured permissions for them with some defaults. Currently, it just places maximum `m`
permissions in all base permission fields. And the interpreter, discussed in the next section, converts to owned as needed, using $ctb()$.

One special case exists: If a type is not understood, we try to create the permission from it is _underlying type_. For example, `type Foo int` is a named type, but we don't support named types, so we use the underlying type, `int`, for creating the permission.

## Handling cyclic permissions
So far, we have only looked at permissions without cycles. In the real world, permissions can have cycles, because types can have cycles too,
for example, `type T []T` is a type that is a slice of itself. The functions discussed so far transparently handle cycles with a simple caching
mechanism. Essentially, all functions seen so far recurse via a wrapper function that first checks the cache for the given arguments and returns the cached value if it exists,
and only calls the real function if the arguments were not seen yet.

For predicate functions, that is, the assignability functions, this wrapper function does all the work, including registering the arguments in the cache:
```go
func assignableTo(A, B Permission, state assignableState) bool {
	key := assignableStateKey{A, B, state.mode}
	isMovable, ok := state.values[key]

	if !ok {
		state.values[key] = true
		isMovable = A.isAssignableTo(B, state)
		state.values[key] = isMovable
	}

	return isMovable
}
```

For producer functions, that is, functions producing permissions, it is similar. For example, for `convertToBase`:
```go
func convertToBase(perm Permission, goal BasePermission, state *convertToBaseState) Permission {
	key := mergeStateKey{perm, goal, state.action}
	result, ok := state.state[key]
	if !ok {
		result = perm.convertToBase(goal, state)
	}
	return result
}
```
The actual registering of the expected output in the cache does not happen here, though. We need to do this in the concrete
methods, as we need to construct a new permission first. Hence, all `merge` and `convertToBase` methods start with something like this:
```go

func (p *SlicePermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &SlicePermission{}
	state.register(next, p, p2)

    // convertToBase(p, p2, state) returns next now
```

This also means that the proof for $ctb$ holds even in the face of cycles. For example, if we have the permission $A := a []A$ corresponding
to our type above, then, when converting this to b, the inner $ctb(A, b)$ will be the result of the outer permission. As long as the rule holds
for a cycle free permission, it thus also holds for a permission with cycles.

Also noteworthy: For `merge`, it is this wrapper function that handles the fallback to `convertToBase` by checking if we are converting a non-base
permission to a base-permission and then calling `convertBase` instead. This avoids having to implement a "to-base" case for each type.
