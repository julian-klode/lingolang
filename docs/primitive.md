# Introduction
Lingo defines several types of permissions, the base permission and product permissions for structs, functions, and so on. Product permissions always inherit the rules for base permissions.

The following permissions are defined:

* `o` or *owned*
* `r` or *read*
* `w` or *write*
* `R` or *exclusive read*
* `W` or *exclusive write*

The **owned** permission bit limits which values can be stored as part of which other values: Owned values may only embed or reference owned values.

The exclusive permissions do not include their non-exclusive variants: A permission set `rW` cannot be written too, but it asserts that no other reference can write to it.

A permission is said to be _linear_ iff it combines a exclusive permission with its non-exclusive counter part.

There are some important shortcuts:

* `n` (short for _none_) is no flags
* `l` (short for _linear value_) is `rRW`
* `m` (short for _mutable_) is `rwRW`
* `v` (short for _value_) is `rW`

## Wildcard permissions
A wildcard permission `_` can be specified in place of a real permission. A
wildcard permission cannot be assigned from or assigned to, and it is neither
linear nor non-linear. Wildcard permissions can be merged with or converted
from/to other permissions, yielding the other permission - they are the neutral
element of permission operations. They are useful in annotations only, and can
be used to say: "keep the default here".

## Primitive operations
Lingo defines 6 primitive operations on which the checking is build. This subsection describes how the apply to primitive values, the next subsection describes additional requirements on more complex values.

The _move_ operation moves a value from one location to another; as long as the destination has a less-wide set of permissions. It might not actually move a value though, it's more an indication if it is allowed.

The _copy_ operation copies values and thus allows new permissions to be added to a value.

The _reference_ operation is similar to _move_, but does not work on linear values.

The _convert_ operation takes two permission structures and replaces all base permissions in the first one with their corresponding ones in the second.
Its use case is annotations: An annotation might be incomplete (for example, the `om struct { or }`, where the type is actually `type T struct { next * T}`. A matching permission would be `p = om struct { om * p }`. If we take such a complete permission and convert it to the annotation, we gain a complete permission matching the intention of the annotation:
        `p = om struct { or * p0 }`
where `p0 = or struct { or * p0}` (because the inner reference has a new base permission, it needs to be expanded one).

The _merge_ operation (in its _intersect_ and _union_ variants) takes two
permissions and returns a new one. For intersection, the returned permission
is assignable-to both inputs, so it can be used to merge two different code
paths (sort of like a $\phi$ node). For union, the returned permission is
assignable-from both inputs. It is used for parameters when intersecting
functions: `intersect(om func(om) or, or func(or) or) = om func(om) or` - after
all, we could not pass a read-only argument to a function exception something
mutable.

Finally, given a type, a permission set can be generated that matches the shape of the type. This structure contains the maximum set of permissions possible (except ownership).
The generated permission set can be restricted with an annotation, by first normalizing the annotation, and then converting the type-generated one to the normalized annotation.

## Primitive operations on complex types

### Pointers
```go
// @cap <base> * <permission>
type PointerType * OtherType
pointer := &value
```
A pointer declares a reference to another memory location (the target).

A pointer can be _moved_ if its target can be moved, and _copied_ or _referenced_ if its target can be referenced. Note that an expression like `&value` might either move or reference `value` depending on whether it is linear or not.

If a linear pointer is _converted_ to a non-linear pointer, any linearity is stripped from the target as well, and if it was write-linear, the write permission is stripped as well.
For example, converting `orwRW * orwRW` to `or` yields `or * orR`, but converting it to `orR` yields `orR * orwRW`.

### Channels
```go
// @cap <base> chan <permission>
type ChannelType chan ElementType
channel <- value       // send value to channel
channel -> value       // receive value from channel
```

A channel is a concurrency-safe queue of items used for communicating between goroutines.

Channels cannot be _copied_, they can however be moved or referenced, if their elements can be moved or referenced, respectively.

### Maps
```go
// @cap <base> map[<permission>] <permission>
type MapType map[KeyType] ValueType
amap[key] = value       // Assign
value = amap[key]       // Retrieve (zero value if not found)
value, ok = amap[key]   // Retrieve (ok = true if found)
```

A map is a mapping from a key to a value type.

Maps cannot be _copied_, they can however be moved or referenced, if their elements can be moved or referenced, respectively.

### Structs
```go
// @cap <base> struct { <permission list> }
type StructType struct {
        int             // Embedding
        first int
        second int
}
strct.first = value
value = struct.first
```

A struct is a product of multiple types. It is one contiguous memory area consisting of its fields in the order they appear. Apart from normal fields, a struct can also embed other types, in this case, it can be passed where the embedded type is expected (and the embedded object is passed instead).

A struct can be _copied_, _moved_, or _referenced_ if its elements can be.

### Arrays
```go
// @cap <base> [<integer>]<permission>
type ArrayType [1]ElementType
array[0] = value
value = array[0]
```
An array is a fixed-size sequence of objects placed in one contiguous memory region.

An array can be _copied_, _moved_, or _referenced_ if its elements can be.

### Slices
```go
// @cap <base> []<permission>
type SliceType []ElementType
slice = array[from:to]
slice = make(SliceType, size, capability)
slice[0] = value
value = slice[0]
```
A slice is a reference to part of an array.
It can be generated by slicing an array, or using make().

As slices are references, they cannot be copied. "Copying" a slice simply generates a new reference to the underlying array.

They can be _referenced_ or _moved_ if their elements can be.

### Functions
```go
type FunctionType func foo(param1 int) (res1 int)
type MethodType (receiver Type) func bar(param1 int) (res1 int)

foo(5)
receiver.bar(5)
```
Functions can have a list of parameters, zero or more return values (optionally named), and a receiver (they are called methods then).

Functions are essentially reference types, and cannot be copied.

For functions, the permissions take on different meanings: A writeable function is a function that can change its return value for different inputs.

Functions can be moved or referenced, but the rules are more complex than for other types:

* Ownership needs to be preserved, as usual
* Otherwise, permissions can only be added, not removed. For example, So for example, `or func()` can be used as an `ow func()`, but not vice versa.
* Receivers and parameters are reverse-operated, they need to be moved/referenced from the destination to the src.
For example, `or func (or)` can be moved/referenced to/as a `or func(orw)` (additional permissions on argumentsare simply discarded).
* Results are simply recursively moved/referenced.

The same rules apply to merging functions: The base permission, parameters,
and receivers merge with the opposite operation (that is, union when
intersecting and vice versa).

### Interfaces
Interfaces essentially are a set of methods. They have the same requirements as methods.
