# Linear typing for Go

This project aims to implements annotations for linear typing in Go, so that
Go programs can be written without worrying about accidentally sharing mutable
state between goroutines.

[![Build Status](https://travis-ci.org/julian-klode/lingolang.svg?branch=master)](https://travis-ci.org/julian-klode/lingolang)
[![codecov](https://codecov.io/gh/julian-klode/lingolang/branch/master/graph/badge.svg)](https://codecov.io/gh/julian-klode/lingolang)
[![GoDoc](https://godoc.org/github.com/julian-klode/lingolang?status.svg)](http://godoc.org/github.com/julian-klode/lingolang)
[![Go Report Card](https://goreportcard.com/badge/github.com/julian-klode/lingolang)](https://goreportcard.com/report/github.com/julian-klode/lingolang)

## Goal

Provide a set of comment-based annotations to Go programs that restrict the
permitted operations on values, without causing an otherwise semantic change
to the language (every program with annotations is a valid Go program, and
does not perform differently if the annotations are removed).

The idea is to implement a capability-based system, with several common
modifiers for linear constants, linear mutable values, pure values, and
possibly some read-only type (where others can have a write reference too).

The capability inference and checking runs after the standard Go type checker.

Checking should be incremental of some sort, so existing code can be annotated
without breaking it (or causing warnings). Either by making annotations turn on
checking of an entire package, or (within a package?) make annotations on one
function/variable require annotations on all callees/users.

## Problems

The annotation approach means that any capabilities are present only at
compile-time and cannot be recovered at run-time - in other words, the
capabilities are erased (like type erasure in Java). This means that it
is not possible to use type guards or type switches to decide between mutable
and non-mutable implementations of an interface, for instance. It seems unlikely
that we can even store both of them in the same interface{} value, though, so
it does not seem like a huge problem.

If interfaces can have methods that only run on mutable receivers, then it
would be helpful to only have to implement the constant methods where only
an interface with constant capabilities is requested. As our checker runs after
the Go type checker and does not affect the Go code at all, though, that is
not possible.

The capabilities model is defined for a reference-based type system. Go uses
values and pointers. Maps, Arrays, and Slices can be considered references,
though - or: they are basically structs containing pointers. In any case, it
seems there is no need to have an "identity" permission, although it might
make sense to have some permission for the operation of taking the address of
something (the & operator).

## Legal
Linear Typing for Go is licensed under the 2 clause BSD license; basically the
same license as Go, just with the third clause removed.
