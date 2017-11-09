# Introduction
Consider the following code: A function takes a channel (a thread-safe queue) of pointers to integers,
and two pointers to integers. Then it sends one pointer through the channel -- an idiom emphasised by the proverb
"Don't communicate by sharing memory, share memory by communicating." (Rob Pike), and writes through the other.
```go
func sendAPointer(aChannelOfIntPointers chan *int,
                  anIntPointer *int,
                  anotherIntPointer *int) {
    aChannelOfIntPointers <- anIntPointer
    *anotherIntPointer = 5
}
```
Seems harmless, right? But what if `anotherIntPointer` and `anIntPointer` point to the same value, are _aliases_?
We just sent the pointer to that integer somewhere, but then modified it - a _use-after-send_ issue, if you want
to call it that.
This does not seem to be the intention of the code and in a concurrent program, it could lead to a race condition:
If one thread runs this function, and another reads from the channel, it's not clear whether the other thread will
see the target as `5` or the old value; or first the old and then the new value.

Aliases can occur naturally in various parts of the code, and sometimes it is not clear which variables alias each
other in a complex code base.
As such, it is not possible to reliably tell which effect writing through such an alias has: In the worst case, it
could affect any other variable with the same type.

If we can annotate variables that have no aliases, we can safely write through them and rest assured that the write
only changed that variable, and not any other.

But that's only half of the story. We also need a way to mark objects as constant; that is, a guarantee that while
other aliases exist, at least none of them can write to it.

Go is particularly useful to look at, because it only contains writeable values, and is designed for use in concurrent
applications, with sharing memory by communicating as shown above - since memory is shared by communicating, we can
conceptually move the value to the other process, instead of sharing it, and thus prevent two threads having access
to the same writeable object at the same time.
