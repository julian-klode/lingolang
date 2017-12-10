# Introduction
Strong, static typing is a great thing: It prevents a whole class of errors. But there still are errors other
than type errors. For example, consider the following Go program:

```go
func sendAPointer(aChannelOfIntPointers chan *int, anIntPointer *int) {
    aChannelOfIntPointers <- anIntPointer
}
```
It sends a pointer through a channel to another thread. Now, if both threads have write access to the pointer
target, there could be race condition. It would be nice to detect and prevent such race conditions.

A first useful step would be to introduce read-only permissions:
```go
// Requires: anIntPointer points to read-only memory
func sendAPointer(aChannelOfIntPointers chan *int, anIntPointer *int) {
    aChannelOfIntPointers <- anIntPointer
}
```
But what if another pointer points to the same location, but writable?

This generally boils down to aliasing: If we can ensure that the value we are sending over the channel does
not have any aliases, we can implement a solution that _moves_ the value through the channel, rather than
copying. We need something that says this:

```
// Requires: anIntPointer must not have any aliases
// Ensures: anIntPointer cannot be used afterwards
func sendAPointer(aChannelOfIntPointers chan *int, anIntPointer *int) {
    aChannelOfIntPointers <- anIntPointer
}
```

This thesis discusses an approach of linear types, in the sense of types that can only have a single (active)
reference to them. It implements linear types as a general form of permissions like read, write, exclusive read,
and some others.

This thesis is structured as follows:
Chapter 2 will give an introduction to Go and linear types, chapter 3 will introduce an approach to permissions
for Go, and chapter 4 will introduce an abstract interpreter that statically checks a Go program for permission violations.
Finally, chapters 5 and 6 discuss how the implementation was tested, and what the issues are.
