# Basics
This chapter gives a quick overview of the Go programming language before discussing approaches to solving the problem of aliases of mutable objects.

## The Go programming language
\vskip-2cm \hfill\includegraphics[height=1.5cm]{gopherbw}

Go[^Go] is an imperative programming language for concurrent programming created mainly developed by Google, initially mostly by Robert Griesemer, Rob Pike, and Ken Thompson.
Design of the language started in 2007, and an initial version was released in 2009; with the first stable version, 1.0 released in 2012 [@gofaq].


[^Go]: <https://golang.org> -	The Go gopher was designed by Renee French. (http://reneefrench.blogspot.com/)
     The design is licensed under the Creative Commons 3.0 Attributions license.
      Read this article for more details: https://blog.golang.org/gopher

Go has a C-like syntax (without a preprocessor), garbage collection, and, like its predecessors devloped at Bell Labs -- Newsqueak (Rob Pike), Alef (Phil Winterbottom), and Inferno (Pike, Ritchie, et al.) -- provides built-in support for concurrency using so-called goroutines and channels, a form of co-routines, based on the idea of Hoare's 'Communicating Sequential Processes' [@hoare1978communicating].

Go programs are organised in packages. A package is essentially a directory containing Go files. All files in a package share the same namespace, and there are two visibilities for symbols in a package: Symbols starting with an upper case character are visible to other packages, others are private to the package:

```go
func PublicFunction() {
    fmt.Println("Hello world")
}

func privateFunction() {
    fmt.Println("Hello package")
}
```

#### Types
Go has a fairly simple type system: There is no subtyping (but there are conversions), no generics, no polymorphic functions, and there are only a few basic categories of types:

1. base types: `int`, `int64`, `int8`, `uint`, `float32`, `float64`, etc.
1. `struct`
1. `interface` - a set of methods
1. `map[K, V]` - a map from a key type to a value type
1. `[number]Type` - an array of some element type
1. `[]Type` - a slice (pointer to array with length and capability) of some type
1. `chan Type` - a thread-safe queue
1. pointer `*T` to some other type
1. functions
1. named type - aliases for other types that may have associated methods:
    ```go
    type T struct { foo int }
    type T *T
    type T OtherNamedType
    ```

    A named type is a mostly distinct type from the underlying type - an
    explicit conversion is required to use them, at least in most cases: In some cases,
    like if the underlying type is a number, some operators like `+` do work on them.

Maps, slices, and channels are mostly reference types (or rather, structs containing pointers)
but all other types are passed by value. Especially arrays are copied entirely when passed around,
instead of just copying the pointer, like C does.

##### Constants

Go has untyped literals and constants.

```
    1    // untyped integer literal
    const foo = 1 // untyped integer constant
    const foo int = 1 // int constant
```


Untyped values are classified into the following categories: `UntypedBool`, `UntypedInt`, `UntypedRune`, `UntypedFloat`, `UntypedComplex`, `UntypedString`, and `UntypedNil` (Go calls them _basic kinds_, other basic kinds are available for the concrete types like `uint8`). An untyped value can be assigned to a named type derived from a base type; for example:

```go
type someType int

const untyped = 2             // UntypedInt
const bar someType = untyped  // OK: untyped can be assigned to someType
const typed int = 2           // int
const bar2 someType = typed   // error: int cannot be assigned to someType
```

#### Interfaces and 'objects'
As mentioned before, interfaces are a set of methods.
Go is not an object-oriented language per se, but it has some support for associating methods with
named types: When declaring a function, a _receiver_ can be provided - a receiver is an additional
function argument that is passed before the function and involved in the function lookup, like this:

```go
type SomeType struct { ... }

func (s *SomeType) MyMethod() {
}

func main() {
    var s SomeType
    s.MyMethod()
}
```

An object implements an interface if it implements all methods; for example, the following interface `MyMethoder` is implemented by `*SomeType` (note the pointer), and values of `*SomeType` can thus be used as values of `MyMethoder`. The most basic interface is `interface{}`, that is an interface with an empty method set - any object satisfies that interface.
```go
type MyMethoder interface {
    MyMethod()
}
```
There are some restrictions on valid receiver types; for example, while a named type could be a pointer (for example, `type MyIntPointer *int`), such a type is not a valid receiver type.


#### Control flow
Go provides three primary statements for control flow: `if`, `switch`, and `for`.
The statements are fairly similar to their equivalent in other C-like languages, with some exceptions:

* There are no parentheses around conditions, so it's `if a == b {}`, not `if (a == b) {}`. The braces are mandatory.
* All of them can have initialisers, like this

    ```if result, err := someFunction(); err == nil { // use result }```

* The `switch` statement can use arbitrary expressions in cases
* The `switch` statement can switch over nothing (equals switching over true)
* Cases do not fall through by default (no `break` needed), use `fallthrough` at the end of a block to fall through.
* The `for` loop can loop over ranges: `for key, val := range map { do something }`



#### Goroutines
The keyword `go` spawns a new goroutine, a concurrently executed function. It can be used with any function call, even a function literal:

```go
func main() {
    ...
    go func() {
        ...
    }()

    go some_function(some_argument)
}
```

#### Channels
Goroutines are often combined with channels to provide an extended form of Communicating Sequential Processes [@hoare1978communicating]. A channel is a concurrent-safe queue, and can be buffering or unbuffered:

```go
var unbuffered = make(chan int)  // sending blocks until value has been read
var buffered = make(chan int, 5) // may have up to 5 unread values queued
```

The `<-` operator is used to communicate with a single channel.

```go
valueReadFromChannel := <- channel
otherChannel <- valueToSend
```

The `select` statement allows communication with multiple channels:

```go
select {
    case incoming := <- inboundChannel:
        // A new message for me
    case outgoingChannel <- outgoing:
        // Could send a message, yay!
}
```

#### The `defer` statement
Go provides a `defer` statement that allows a function call to be scheduled for execution when the function exits. It can be used for resource clean-up, for example:

```go
func myFunc(someFile io.ReadCloser) {
    defer someFile.close()
    /* Do stuff with file */
}
```

It is of course possible to use function literals as the function to call, and any variables can be used as usually when writing the call.

#### Error handling
Go does not provide exceptions or structured error handling. Instead, it handles errors by returning them in a second or later return value:

```go
func Read(p []byte) (n int, err error)

// Built-in type:
type error interface {
        Error() string
}
```

Errors have to be checked in the code, or can be assigned to `_`:

```go
n0, _ := Read(Buffer)   // ignore error
n, err := Read(buffer)
if err != nil {
    return err
}
```

There are two functions to quickly unwind and recover the call stack, though: `panic()` and `recover()`.
When `panic()` is called, the call stack is unwound, and any deferred functions are run as normally.
When a deferred function invokes `recover()`, the unwinding stops, and the value given to `panic()` is returned.
If we are unwinding normally and not due to a panic, `recover()` simply returns `nil`.
In the example below, a function is deferred and any `error` value that was given to `panic()` is recovered and stored in an error return value.
Libraries sometimes use that approach to make highly recursive code like parsers more readable, while still maintaining the usual error return value for public functions.

```go
func Function() (err error) {
    defer func() {
        s := recover()
        switch s := s.(type) {  // type switch
            case error:
                err = s         // s has type error now
            default:
                panic(s)
        }
    }
}
```

#### Arrays and slices
As mentioned before, an array is a value type and a slice is a pointer into an array, created either by slicing an existing array or by using `make()` to create a slice, which will create an anonymous array to hold the elements.

```go
slice1 := make([]int, 2, 5) // 5 elements allocated, 2 initialized to 0
slice2 := array[:]          // sliced entire array
slice3 := array[1:]         // slice of array without first element
```

There are some more possible combinations for the slicing operator than mentioned above, but this should give a good first impression.

A slice can be used as a dynamically growing array, using the `append()` function.

```
slice = append(slice, value1, value2)
slice = append(slice, arrayOrSlice...)
```

Slices are also used internally to represent variable parameters in variable length functions.

##### Maps
Maps are simple key value stores and support indexing and assigning. They are *not* thread-safe.

```go
someValue := someMap[someKey]
someValue, ok := someMap[someKey]   // ok is false if key not in someMap
someMap[someKey] = someValue
```

## Approaches to dealing with mutability

Functional programming is a form of programming in which side effects do not exist.
That is, there is no such things as mutable data structures, or even I/O operations; only pure
transformations from one data structure to another.

### Monads and Linear Types
There is a way to express mutability or side effects in functional programming: Haskell and some other
languages use a construct called monads [@launchbury1995state].
A monad is a data structure that essentially represents a computation. For example, an array monad could have
operations 'modifying' an array that compute a new monad that when 'executed' produces a final array - it is essentially a form of embedded language.
For example, in Haskell, a monad is defined like this:
```haskell
class Monad m where
  return ::   a -> m a
  (>>=)  :: m a -> (a -> m b) -> m b
```
`return` returns a value in the monad, and `>>=` takes a value in the monad, and passes it to
a function which expects a raw value and returns a new value in the monad, yielding the new
value in the monad.

Monads solve the problem of referential transparency: A function with the same input will
produce the same output (a function operating on a monad produces a new monad describing any
computations to be made, and is thus pure). They have one major drawback however: They are not
easily combinable: As soon as you have more than one monad, you need to 'lift' monad operations into
other monads in order to use them together. This makes it hard to read code.

An alternative approach to representing mutability are linear types. A value of a linear type can only
be used once. In fact, traditionally a linear value must be used exactly once. So an array can be implemented
as a linear type with operations consuming one linear value and returning another. The compiler can then
optimize the operation to use the same array, because it knows that nobody else is accessing the 'old' array.

Consuming and returning linear values is a bit annoying, which is why some programming languages started
introducing shortcuts for it: A parameter with a special annotation serves as both an input
and an output parameter. For example, in Mercury, an efficient purely declarative logic programming language [@somogyi1995mercury][@dowd2000using, section 2.2], an operation could look like this:
```prolog
:- module hello.
:- interface.

:- import_module io.

:- pred main(io::di, io::uo) is det.

:- implementation.
main(!IO) :-
    write_string("Hello, world!\n", !IO).
```
The exclamation mark here is equivalent to an input and output parameter, so it is the same as:
```prolog
main(IO0, IO) :-
    write_string("Hello, world!\n", IO0, IO).
```
With such a notation, we immediately reach a level where the code basically just looks like imperative code but with all the same guarantees of purely functional code. We can also make this the only notation, effectively gaining an imperative language with the same guarantees as a functional one.

There are various names describing the same or fairly similar concepts as this: linear [@Baker:1995:LVL:199818.199860], unique [@achten1993high][@boyland2001alias], free [@hogg1991islands][@noble1998flexible], or unsharable[@minsky1996towards].

One programming language using linear types is Rust ([^Rust]). In Rust, the input/output annotation is basically the only variant - it looks and works like an imperative language. Linear values can be created and 'borrowed' for passing them to
another function, for example. Rust has no garbage collector, but a system of life times where each function parameter can be associated a named life time and the result can then refer to the names of the parameters. This allows it to be used even without a heap, at least in theory. Rust does not use linear types for I/O which is a bit unfortunate.

[^Rust]: <https://www.rust-lang.org>

### Capabilities for Sharing
The several implementations of linear types in different programming languages, are all slightly incompatible with each other, which is why 'Capabilities for Sharing' [@Boyland:2001:CSG:646158.680004] tried to introduce a common system for describing linearity.

It describes a simple reference based language with objects containing fields. A _capability_ is a pair of an address and a permission - a set of the following flags:

* $R$ - read
* $W$ - write
* $I$ - identity - the address can be compared
* $\overline{R}$ - exclusive read - no other capability can read that address
* $\overline{W}$ - exclusive write - no other capability can write that address
* $\overline{I}$ - exclusive identity - no other capability has identity access to the object
* $O$ - ownership

Exclusive read and write do not imply their non-exclusive counter parts; for example, an object $R\overline{W}$ prevents others from writing, but cannot write itself - it is essentially a read-only object.
Permissions also must be asserted: Other capabilities can have their conflicting access rights stripped at run-time.
Asserting the exclusive permissions of an unowned capability strips away incompatible permissions from other unowned capabilities, but asserting on an owned capability strips away all incompatible permissions on all other capabilities
- so for example, if there are two capabilities $A$ and $B$ for the location $x$, both with $R\overline{W}$, asserting the permissions one one will strip away the permission from the other.
If not asserted, exclusive permissions mean nothing: There could be multiple capabilities with exclusive reads for the same object in the program.

They provide small-step semantics of a tiny language which operates on a _store_ which maps addresses and (object, field) pairs to capabilities. One thing follows from that approach:

Given four objects `a`, `b`, `x`, `y`, with `a`, `b` each having a field `x` referencing `x`, and `x` having a field referencing `y`, the capability for `a.x.y` and `b.x.y` have to be the same:

* Evaluating `a.x.y`:
    1. $(A, X) \rightarrow (X, \text{permissions for } X \text{ in } A)$
    1. $(X, Y) \rightarrow (Y, \text{permissions for } Y \text{ in } X)$
* Evaluating `b.x.y`:
    1. $(B, X) \rightarrow (X, \text{permissions for } X \text{ in } B)$
    1. $(X, Y) \rightarrow (Y, \text{permissions for } Y \text{ in } X)$

This means that while `a` and `b` can have different _views_ on `x`, they must have the same one for `y`.
It's unclear if this was intended to keep things simple, or if it was not considered that it might be useful to have different permissions for `y`.

Permissions might also be overly flexible: Should we really care about exclusive identity, or values that have no permission at all? There are 7 flags with two values each, so for a primitive value we end up with $2^7 = 128$ possible permissions.

### Fractional permissions
Another approach to linear values is fractional permissions [@Boyland:2003:CIF:1760267.1760273] and fractional permissions without fractions [@Heule:2011:FPW:2076674.2076675]. Fractional permissions are of course, only permissions, they need to be associated with values somehow, compared to capabilities which also abstract away the object.
In the fractional permission world, an object starts out with a permission of 1, and each time it is borrowed, the permissions are split. A permission of 1 can write, other permissions can only read.

Fractional permissions have one advantage over the permission approach outline in the previous section: They can be recombined.
They are also less flexible, offering only linear writeable and non-linear read-only kinds of values, rather than $2^7$ possible combinations,
which might be an advantage or not, depending on what they are to be used for.

It seems possible to extend fractional permissions with some non-linear writeable object: Introduce infinity as a valid value, and define fractions
of infinity as infinity; and defining a writeable object as having a permission $\ge 1$, rather than equal to 1. This way, there could be an infinite
number of references to a writeable object.

Likewise, a linear read-only value could perhaps be introduced by introducing a special fraction that cannot be divided into smaller fractions.
