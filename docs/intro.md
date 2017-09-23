# Introduction

## The Go programming language
\vskip-3.5cm \hfill\includegraphics[height=3cm]{gopherbw}

Go[^Go] is an imperative programming language for concurrent programming created and mainly developed by Google, initially mostly by Robert Griesemer, Rob Pike, and Ken Thompson.
Design of the language started in 2007, and an initial version was released in 2009; with the first stable version, 1.0 released in 2012[@gofaq].


[^Go]: <https://golang.org> -	The Go gopher was designed by Renee French. (http://reneefrench.blogspot.com/)
     The design is licensed under the Creative Commons 3.0 Attributions license.
      Read this article for more details: https://blog.golang.org/gopher

Go has a C-like syntax (without a preprocessor), garbage collection, and, like it's predecessors devloped at Bell Labs -- Newsqueak (Rob Pike), Alef (Phil Winterbottom), and Inferno (Pike, Ritchie, et al) -- provides built-in support for concurrency using so-called goroutines and channels, a form of co-routines, based on the idea of Hoare's 'Communicating Sequential Processes'[@hoare1978communicating].

Go programs are organized in packages. A package is essentially a directory containing Go files. All files in a package share the same namespace, and there are two visibilities for symbols in a package: Symbols starting with an upper case character are visible to other packages, others are private to the package.

#### Types
Go has a fairly simple type system: There is no subtyping (but there are conversions), no generics, no polymorphic functions, and there are only a few basic categories of types:

1. base types: `int`, `int64`, `int8`, `uint`, `float`, etc.
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

    A named type is a (mostly) distinct type from the underlying type - an
    explicit conversion is required to use them (in most cases). In some cases,
    like if the underlying type is a number, operators like `+` do work on them.

Maps, slices, and channels are (mostly) reference types (or rather, structs containing pointers)
but all other types are passed by value. Especially arrays are copied entirely when passed around,
instead of just copying the pointer, like C does.

Constants are untyped. For example, while a named type is different from an underlying type, and an `int` can not be copied to a `type MyInt int`, the value `1` is untyped and can be copied into both. Only a rough kind of type is available, classifying a literal into integral, floating point, string, etc.

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

An object implements an interface if it implements all methods; for example, the following interface `MyMethoder` is implemented by `*SomeType` (notice the pointer), and values of `*SomeType` can thus be used as values of `MyMethoder`. The most basic interface is `interface{}`, that is an interface with an empty method set - any object satisfies that interface.
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
* Cases not fall through by default (no `break` needed), use `fallthrough` at the end of a block to fall through.
* The `for` loop can loop over ranges: `for key, val := range map { do something }`



#### Goroutines
The keyword `go` spawns a new go-routine, a concurrently executed function. It can be used with any function call, even a function literal:

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
Goroutines are often combined with channels to provide an extended form of CSP [@hoare1978communicating]. A channel is a concurrent-safe queue, and can be buffering or unbuffered:

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
n, err := Read(buffer)
if err != nil {
    return err
}
```

There are two functions to quickly unwind and recover the call stack, though: `panic()` and `recover()`.
When panic is called, the call stack is unwinded, and any deferred functions are run as normally.
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
As mentioned before, an array is a value type and a slice is a pointer into an array, created either by slicing an existing array or using an anonymous backing store:

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

## State in functional programming and linear types
(Purely) Functional programming is a form of programming in which side effects do (should) not exist.
That is, there should be no such things as mutable data structures, or even I/O operations, only pure
transformations from one data structure to another.

There is a way to express mutability or side effects in functional programming: Haskell and some other
languages use a construct called monads[@launchbury1995state].
A monad is a data structur  e that essentially represents a computation. For example, an array monad could have
operations 'modifying' an array that compute a new monad that when 'executed' produces a final array - it is essentially a form of embedded language. TODO: How is a monad composed

Monads solve the problem of referential transparency: A function with the same input will
produce the same output (a function operating on a monad produces a new monad describing any
computations to be made, and is thus pure). They have one major drawback however: They are not
easily combinable: As soon as you have more than one Monad, you need to 'lift' Monad operations into
other monads in order to use them together. This makes it hard to read code.

An alternative approach to representing mutability are linear types. A value of a linear type can only
be used once. In fact, traditionally a linear value must be used exactly once. So an array can be implemented
as a linear type with operations consuming one linear value and returning another. The compiler can then
optimize the operation to use the same array, because it knows that nobody else is accessing the 'old' array.

Consuming and returning linear values is a bit annoying, which is why some programming languages started
introducing shortcuts for it (TODO: Source): A parameter with a special annotation serves as both an input
and an output parameter. For example, in TODO language, an operation could look like this:
```
TODO example
```
With such a notation, we immediately reach a level where the code basically just looks like imperative code but with all the same guarantees of purely functional code.

One programming language using linear types is Rust ([^Rust]). In Rust, the input/output annotation is basically the only variant - it looks and works like an imperative language. Linear values can be created and 'borrowed' for passing them to
another function, for example. Rust has no garbage collector, but a system of life times where each function parameter can be associated a named life time and the result can then refer to the names of the parameters. This allows it to be used even without a heap, at least in theory. Rust does not use linear types for I/O which is a bit unfortunate.

[^Rust]: <https://www.rust-lang.org>

## Capabilities for Sharing
The several implementations of linear types in different programming languages, are all slightly incompatible with each other, which is why 'Capabilities for Sharing'[@Boyland:2001:CSG:646158.680004] tried to introduce a common system for describing linearity.

It describes a simple reference based language with objects containing fields. A _capability_ is a pair of an address and a permission - a set of the following flags:

* $R$ - read
* $W$ - write
* $I$ - identity - the address can be compared
* $\overline{R}$ - exclusive read - no other capability can read that address
* $\overline{W}$ - exclusive write - no other capability can write that address
* $\overline{I}$ - exclusive identity - no other capability has identity access to the object
* $O$ - ownership

Exclusive read and write do not imply their non-exclusive counter parts, allowing to create absolutely read-only object ($R\overline{W}$). Permissions also must be asserted: Other capabilities can have their conflicting access rights stripped at run-time. Asserting the permissions of an unowned object strips away incompatible permissions from other unowned objects, but asserting on an owned object strips away all incompatible permissions elsewhere.
If not asserted, exclusive permissions mean nothing: There could be multiple capabilities with exclusive reads for the same object in the program.

They provide a small-step evaluation of a tiny language which operates on a _store_ which maps addresses and (object, field) pairs to capabilities. It's not entirely clear if this was intended, but this approach has a somewhat serious drawback:

Given four objects `a`, `b`, `x`, `y`, with `a`, `b` having fields `x` referencing `x`, and `x` having a field referencing `y`, the capability for `a.x.y` and `b.x.y` have to be the same:

* Evaluating `a.x.y`:
    1. $(A, X) \rightarrow (X, \text{permissions for } X \text{ in } A)$
    1. $(X, Y) \rightarrow (Y, \text{permissions for } Y \text{ in } X)$
* Evaluating `b.x.y`:
    1. $(B, X) \rightarrow (X, \text{permissions for } X \text{ in } B)$
    1. $(X, Y) \rightarrow (Y, \text{permissions for } Y \text{ in } X)$

This means that while `a` and `b` can have different _views_ on `x`, they must have the same one for `y`.

Permissions might also be overly flexible: Should we really care about exclusive identity, or values that have no permission at all? There are 7 flags with two values each, so for a primitive value we end up with $2^7 = 128$ possible permissions.

## Fractional permissions
Another approach to linear values is fractional permissions[@Boyland:2003:CIF:1760267.1760273] and fractional permissions without fractions[@Heule:2011:FPW:2076674.2076675].
In the fractional permission world, an object starts out with a permission of 1, and each time it is borrowed, the permissions are split. A permission of 1 can write, other permissions can only read.

Fractional permissions have one advantage over the permission approach outline in the previous section: They can be recombined. The approach is otherwise far less flexible though, offering only 2 possible kinds of values (writable and not-writable) rather than the $2^7$ possible combinations of permissions.

## Tying it all together
Go's easy-to-use abstraction of concurrent programming with goroutines and channels are incredibly useful. One of the common proverbs in Go is "Don't communicate by sharing memory, share memory by communicating." (Rob Pike), that is
instead of directly sharing data structures send pointers via channels.

Linear values allow us to prevent what I like to call _use-after-send_ issues: If we just sent a pointer to our mutable data to another goroutine, we should not access it anymore, unless it's read-only. If we did, we would end up with data races. With linear values, the value is _moved_ into the channel (and then into the receiving goroutine), and rendered inaccessible in the sender.

The next chapter discusses applying permissions as used in capabilities to Go to describe linear values and at the same time also introduce read-only values to Go as a side-effect.
