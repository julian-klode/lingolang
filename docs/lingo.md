# Linear Go
As explained in the introduction, Go is mostly a value-, rather than reference-based language.
Therefore, approaches like capabilities or fractional permissions cannot be used as is, but require some changes.

Since we are adapting an existing programming language, it seems that fractional permissions are ill-suited:
We want to have both linear types and non-linear types, for compatibility with existing unannotated code.

For capabilities it is much easier, we essentially only need to eliminate the identity permission, as we do not have such a concept.
It could be argued that taking a pointer to an addressable object is similar in concept, and it might be worthwhile exploring a permission bit for that, but I can not imagine any applications.

## Permissions in Go

For this approach, dubbed _Lingo_ (short for linear Go), we therefore end up with 5 permissions bits:
`r` (read), `w` (write), `R` (exclusive read), `W` (exclusive write), and `o` (ownership).

Out of the $2^5$ possible combinations of permissions bits we have a few built-in aliases:

* `m`, _mutable_, is equivalent to `rwRW`
* `v`, _value_, is equivalent to `rW`
* `l`, _linear value_, is equivalent to `rRW` (the term linear value actually conflicts with the usage in this thesis)
* `n`, _none_, is equivalent to, well, none bits set
* `a`, _any_, is equivalent to all non-exclusive bits set, that is `orwRW`.

An object is considered as _linear_ iff it has a read/write right with a matching exclusive right, that is, it must contain either `rR` or `wW`.
A linear object here may be used more than once, but it might have only a single reference at a time.
This is conceptually equivalent to having the same parameter act as both input and output arguments, where passing a value makes a function use it and then write back a new one.
A linear object can only be referenced by or contained in other linear objects, in order to preserve linearity.

Unwritable objects may not embed any writable objects, but they may store pointers to writable objects. For linear objects, a linear unwritable object may store a linear writable object. There are some odd corner cases with reference-like types like maps and slices, as they are conceptually embedded at the moment, and thus must be unwritable as well if the outer object is unwritable.

Ownership plays an interesting role with linear objects: It determines whether a reference is _moved_ or _borrowed_ (temporarily moved). For example, when passing a linear map to a function expecting an owned map, the map will be moved inside the function; if the parameter is unowned, the map will be borrowed instead. Coming back to the analogy with in and out parameters, owned parameters are both in and out, whereas unowned parameters are only in.

Instead of using a store mapping objects, and (object, field) tuples to capabilities, that is, (object, permission) pairs, Lingo employs a different approach in order to combat the limitations shown in the introduction.
Lingo's store maps a variable to a permission.
It does however, not just have the permission bits introduced earlier (from now on called _base permission_), but also _structured_ permissions, equivalent to types.
A structured permission consists of a base permission and permissions for child elements, and is essentially a graph with the same shape as the graph describing the type. Some examples:

* Primitive values (integers, strings, floats, etc.) have a base permission
* An object of type `*T` has the structured permission `<base> * <perm of T>`.
* An object of type `struct { x int; y int}` has the structured permission `<base> struct { <base>; <base>}`
* An object of type `[]int` has the structured permission `<base> [] <base>`

Apart from primitive and structured permissions, there are also some special permissions:

* The untyped nil permission, representing the `nil` literal.
* The wildcard permission, written `_`. It is used in permission annotations whenever the default permission for a type should be used.

**Summary**: Lingo has 5 permission flags to form base permissions. There are four kinds of permissions: base permissions, structured permissions, nil permissions, and wildcard permissions.

## Basic operations on permissions
Lingo was designed bottom-up, by starting with the permissions, and then defining operations on them that will be needed later on.

### Assignability
Some of the core operations on permissions involve assignability: Given a source permission and a target permission, can I assign an object with the source permission to a variable of the target permission?

Three forms of assigning have been identified: copying, moving, and referencing. The shape of source and target permissions must be the same, that means, for example, that a `om * om` cannot be assigned to an `om` value. Since the shape of a structured permission equals the shape of a type, that should always be the case in practice.

Moving

: An object can be moved if the source base permission includes the read bit, and the target base permission is a subset of the source base permission (no new bits may be added).
If there are children, they must be movable as well.

Copying

: An object can be copied if it is readable, and the children are copyable. If the object is of reference-like type, copying is equivalent to referencing. For non-reference-like objects, copying may add permission bits to the base permission. When a pointer is copied, it's target must be referenceable.

Referencing

: An object can be referenced if neither source nor target permission are linear, and the children can be referenced. As special cases, function receivers, parameters, return values must be movable instead; and functions in an interface must be movable as well.

Function permissions are a fairly special case. Here, the `w` flag does not really mean writable, but it means that the function may return different results even for the same input parameters. As such, a readable function may be used in places where a writable function is expected, which is exactly the opposite of all other permissions, and the checks for referencing and moving are thus done in reverse for a function's base permission, except for the ownership flag.

Another special case with assignability is the untyped nil permission: It can be assigned to itself, and to any pointer permission.

While copying and moving requires read permissions on the source, neither operation requires write permission on the target. The reason for that is simple: We also want to be able to initialize read-only variables or construct read-only objects. There are limited ways of doing (re-)assignments in Go, and adding a special check that the target is writable there does not seem like a huge burden.

Referencing values does not require read permissions. This makes sense, given that we do not copy or move the value, and thus do not have to read it, but only need to move or copy the reference. For example, when copying a pointer, we need to be able to read the pointer, but we do not need read permissions on the target of the pointer. The target could be inaccessible for all we care.

### Converting to base permission
Another set of operations, closely related to the ones coming up next, are conversions to a base permission. Given a permission and a base permission, return a new permission that replaces all base permissions in the input permission with the given base permission.

There are two variants of conversion: The strict one, which replaces all base permissions except for function receivers, parameters, and return values, and a more relaxed one that converts a pointer target differently:

Instead of converting the pointer target to the given base permission, a new base permission for the pointer target is constructed as follow:

1. The owned flag from the old target base permission is replaced with the owned flag from the given base permission
2. If the new base permission has no exclusive read right, but the new target has exclusive write and write flags (is linearly writable), these flags are dropped. (TODO: Bug)
3. If the new base permission has no exclusive read right, but the new target has exclusive read and read flags (is linearly readable), these flags are dropped.

or in code:
```go
nextTarget := p.Target.GetBasePermission()&^Owned |
              (next.BasePermission & Owned)
// Strip linear write rights.
if (next.BasePermission&ExclRead) == 0
    && (nextTarget&(ExclWrite|Write)) == (ExclWrite|Write) {
    nextTarget &^= Write | ExclWrite
}
// Strip linearity from linear read rights
if (next.BasePermission&ExclRead) == 0
    && (nextTarget&(ExclRead|Read)) == (ExclRead|Read) {
    nextTarget &^= ExclRead
}
```

For example, a pointer `ol * om = orRW * om` converted to `ol` yields `ol * om`, but with strict conversion it yields `ol * ol`.

Converting an untyped nil permission to a base permission yields the untyped nil permission.

### Merging and Converting
The idea of conversion to base types from the previous paragraph can be extended to converting between structured types. When converting between two structured types, replace all base permissions in the source with the base permissions in the same position in the target, and when the source permission is structured and the target is base, it just switches to a to-base conversion.

There are two more kinds of recursive merge operations: intersection and union.
These are essentially just recursive relaxations of intersection and union on the base permissions.

Except for functions of course: An intersection of a function requires union of parameters and receivers, because just like with subtyping (in languages that have it), parameters and receivers are contravariant:
If one function expects `orw` and another expects `or` a place that needs either of those functions (an intersection) needs a function that accepts $orw \cup or = orw$ - because passing a writable object to a function only needing a read-only one would work, but passing a read-only value to a function that needs a writable one would lead to funny results.

Intersections are sort of a parallel to phi nodes in a program's static single assignment form. They can effectively be used to join different paths:

```go
if (...) {
    myfun = function expecting mutable value
} else {
    myfun = function expection read-only value
}

myfun = intersect(myfun in first branch, my fun in second branch)
```

In this example, after the if/else block has been evaluated, the permissions of `myfun` are an intersection of the permission it would have in both branches.

As a special exception to the recursive relaxation and same-shape rules, when either side of a merge is a wildcard permission, the result is the other side - the wildcard permission acts as the neutral element. [^monoid]

[^monoid]: I believe that makes permissions with these operations monoids (a semi group (set with associative operation) with a neutral element), but proofing associativity for this recursive operation is a bit too involved

As a further special exception, if either side is a nil permission and the other side a pointer permission, the pointer permission is the result. For conversions, this does make sense: Given that I can assign nil to any pointer, I can also convert nil permissions to any pointer permission. For union and intersection, consider the classical examples:

* For union, the question is: Can $p \cup nil$ be used in place of both $p$ and $nil$? Technically the answer is no, because $p$ cannot be used where $nil$ is expected. But nil permissions are only ever
  used for $nil$ literals (they cannot even be specified, there is no syntax for them), so we never reach that situation.
* For intersection, the question is: Can values of $p$ or $nil$ be assigned to $p \cap nil$. Yes, they can be, $nil$ is assignable to every pointer, and $p$ is assignable to itself.


### Creating a new permission from a type
Since permissions have a similar shape as types and Go provides a well-designed types package, we can easily navigate type structures and create structured permissions for them with some defaults. Currently, it just places maximum `om`
permissions in all base permission fields.

## Static analysis
Based on the operations described in the previous section, a static analyser can be written that ensures that the rules of linearity are respected. This static analysis can be done in the form of an abstract interpreter; that is, an interpreter that does not operate on concrete values, but abstract values and tries to interpret all possible paths through a program.

The abstract interpreter has a store $S: V \rightarrow P$ that maps variables to permissions.
A variable, in this case is simply a string with the variables name.
The store is ordered, and grouped into blocks, in order to implement scoping (TODO: Scoping is incomplete).
It is immutable, changes are done by creating a new store with the changes in it, thus making it easy to implement branching in the interpreter.

### Interpreting expressions
Expression evaluation functions take an expression and a store, and return a new store, a new permission, and a slice of dependencies.
The new store contains all changes that the execution would perform on the input store when executed, and the new permission is the permission of the object the expression would compute to.
The slice of dependencies is more interesting:

Go has several addressable expressions. For example, an element in the array is addressable and thus a pointer to it can be created: `&array[1]` is a pointer to the first element in array.

When `array` is evaluated, a permission for the array is returned and the dependencies contains a pair of (array, old permissions of array) - the permissions for `array` in the store are moved into the expression result, effectively by perform `store[array] convertToBase(store[array], n)` - the variable is effectively marked as unusable.
When the index and address-of operation are then evaluated, the dependencies stay the same.
When we now assign `&array[1]` to another variable, the permissions for `array` are gone, so we can't accidentally refer to `array[1]` via two different references.

When the expression is bound to a owned variable, access to `array` is lost - the dependencies are dropped. When the result is bound to an unowned variable, access to `array` will be restored when the variable goes out of scope.
In order to simplify the implementation, binding a value with dependencies to an unowned place is only allowed at initialization time (or when constructing an unowned object). This ensures that we can drop any unowned variables when the block (or for function calls, the call) ends. (TODO: Something is a bit off)

TODO: Currently, the moving also happens for non-linear values. This seems rather pointless, and might complicate things a bit.

### Interpreting statements
Statements are slightly more complex than expressions, (1) because they allow introducing new variable bindings in the current scope; and (2) because they allow jumping: There can be `return`, `goto`, `break`, `fallthrough` and all other kinds of statements in there.

There are two common ways to handle "early" exits in an interpreter:

1. Raise an exception (for example, a ContinueException, or BreakException)
2. Return a value for a block statement describing where the statement exited, and pass that through

In an abstract interpreter, option 1 does not work - there may be multiple exits involved; some leaving the block normally (by falling out of it), some with a branching statement.
The second option is applicable, with the change that instead of returning one value we return multiple ones. Each statement visitor returns a pair of

1. a new store, with the changes the statement made
2. an indicator of how the block was left (in this implementation, it is either nil or a pointer to the `ReturnStmt` or `BranchStmt` (`goto`, `break`, `continue`, `falltrough`))

Handling `goto`, `break`, and `continue` in a block means we cannot just iterate over the block and return, but might need to iterate multiple times, at potentially different start positions in a block (`goto` to labelled statement). The algorithm for that is simple:

1. `labels` := a map of label to position in the block
1. `work` := a list of (store, position in block) pairs, empty
1. `seen` := a list of stores, empty
1. Push `store, 0` to the block
1. While there is work:
    1. `store, position` := Pop one item out of work
    2. If `store` in `seen`: continue
    1. append `store` to `seen`
    1. `exits` := Visit the statement
    1. For each `exit` in `exits`:
        1. If `exit` has `branch` reference and `position` = lookup label in map is in this block:
            1. Add (store of `exit`, `position`) to work
        1. Else, If it is a break statement, add (store of `exit`, `nil`) to `exits`
        1. Otherwise, add `exit` to `exits`

This algorithm ensures that every possible store that is generated by the loop and could be used in another iteration will be used in another iteration:
Loops and blocks are iterated until we have seen all possible outcomes.

It is however suboptimal in one aspect: Some unreachable code is not checked.
In order to prevent unchecked statements, we can simply keep track of which statements we visited, and then walk over all statements again and warn about any unvisited statements.

The concrete implementation of the algorithm varies:

* For block statements, it is as written.
* In a `for` or `range` loop, the position is gone, it's only possible to jump to the beginning (the body is a block statement, so `goto` is handled there)
* A `for` loop:
    * executes an initialization statement before pushing the initial store
    * before the body is visited, the condition is visited, and the resulting store appended to exits (handling the not entering case)
    * after the execution of the body, a post statement is executed on all stores to be appended to work
* A `range` loop:
    * before the initial store is pushed, the value to be ranged above is borrowed
    * for each iteration after the initial one, before the seen check, the current store is appended to the list of exits.
    * for each iteration, after the seen check, the key, and value on the lhs of the range loop are instantiated
    * after the iterations, the borrowed value is released on all exits if it was unowned.
