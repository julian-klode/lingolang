# Introduction
Go is a strongly statically typed programming language created by Google. It places a heavy emphasis on _goroutines_, a form of concurrent processes, ligthweight threads, that can commmunicate with each other via channels - a concurrent-safe queue.

As a static programming language, Go can prevent certain type mismatches at compile time; for example,
a pointer cannot be sent where a value is expected. Being strongly typed, it does not perform implicit conversions - a weak language could allow you to pass an integer where a string is expected, and implcitly convert the integer to a string.

Go's support for static typing is very rudimentary: There are few primitive base types, structures, pointers, and interfaces - a collection of methods, essentially. Importantly, there is no support for read-only objects - all objects are writable.

Placing a heavy emphasis on concurrency and communication between goroutines, one of Go's proverbs is "Don't communicate by sharing memory, share memory by communicating." (Rob Pike), meaning that instead of having a pointer that is visible to all goroutines, send the pointer via a channel. For example, given a channel of int pointers, we can send an int pointer on it:

```go
    aChannelOfIntPointers <- anIntPointer
```

But `anIntPointer` and its target are always writable. By sending the value to the channel, two goroutines can now have access to it, and there could be race conditions if we are not careful. It would be nice to be able to send values over channels without having to fear race conditions like that.

A first useful step would be to introduce read-only permissions:
```go
    // Requires: anIntPointer points to read-only memory
    aChannelOfIntPointers <- anIntPointer
```
But how can we ensure that there is no other pointer pointing to the same location as `anIntPointer`, but writable? Restricting read-only pointers to be created only from read-only locations seems like a bad idea. Surely I want to be able to build a struct (for example) in a mutable fashion, but then return it as a read-only value.

We could use monads, a concept introduced by Haskell, to encapsulate mutability of a specific value. But no, monads require generic types for implementation, and Go does not provide them, so we have to look for something different.

So this seems to generally boil down to aliasing: If we can ensure that the value we are sending over the channel does
not have any (writeable) aliases, we can implement a solution that _moves_ the value through the channel, rather than
copying. We need something that says this:

```
    // Requires: anIntPointer must not have any aliases
    // Ensures: anIntPointer cannot be used afterwards
    aChannelOfIntPointers <- anIntPointer
```

Linear types allow doing just that: We can declare that an object of a given type only has a single reference to it. We can easily check linearity at runtime if we use reference counting: When a linear value is expected, but the reference count is larger than 1, we throw a runtime error. But this seems like a bad choice: Go is statically typed, and if we can check linearity statically, we can prevent a lot of race conditions from even compiling, reducing the amount of bugs in a running program, even in the face of untested code paths.

The remaining chapters of the thesis are structured as follows:
Chapter 2 will give an introduction to Go, monads, linear types, and some generalisations;
chapter 3 will introduce an approach to permissions for Go;
chapter 4 will introduce an abstract interpreter that statically checks a Go program for permission violations;
and chapters 5 and 6 discuss how the implementation was tested, and what the issues are.


What properties should our implementation optimally achieve?:\label{sec:criteria}

An important one is _completeness_: All syntactic constructs are tested. Or rather: An unknown syntactic construct should be rejected.

Another is _correctness_: Only valid programs are allowed. That means that if something is wrong, the program should not be considered valid.

There is also _preciseness_: We do not reject useful programs because our rules are too broad. As a non-Go example, a literal value of "1" should be able to be stored in all integer sizes, not just whatever type a literal has.

A further property is _usability_: Error messages should be understandable. If programmers do not understand the error, it makes it hard to fix it.

We should also consider _compatibility_ - A compiling Lingo program behaves the same as a Go program with all Lingo annotations are removed. By storing all annotations in comments, and just passing the program to the Go compiler after performing our checks, we can easily ensure that.

Finally, the implementation should be well tested. As a useful metric we can use _code coverage_. Optimally, it should be 100%, but that might not be entirely realistic - sometimes unreachable code needs to be written to future proof code, for example, checking that a method returns true that currently always returns true but might eventually return false.
