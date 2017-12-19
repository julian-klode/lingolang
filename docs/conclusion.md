# Conclusion
We have seen that linearity can be used to model mutability in a safe way, and that it can prevent certain race conditions when used in concurrent
programs. We noted that Go is a good language to adapt linear permissions for, as it is both small, and features native primitives like channels
and lightweight "goroutines" for constructing concurrent programs.

Our adaption of linear types happened in the form of permissions. We defined a set of base permissions consisting of read, write, exclusive read, and exclusive write, and
extended that to the shape of types like $a \textbf{ struct } { A, B }$ for some base permission $a$ and some other permissions $A, B$. This allows us to describe permissions for an
entire Go program in a way that closely resembles types, and could probably be natively integrated into the native type system.

The adaption was designed in a sort-of bottom-up approach. First, we created the permission package and operations related to assigning values. It quickly became clear that
an abstract interpreter seems like a good approach for checking the permissions, and we extended the permission package with further functionality that could become useful
for the interpreter.

The bottom-up approach led to some mismatches: Most importantly, the inability to render parts of an object unusable - when a part like `v.x.y` is borrowed, `v` is currently
completely borrowed. On the other hand, it led to a very well tested permission package which can easily be extended with new things without having to fear breaking existing
functionality greatly.

Another mismatch is the named type and interface handling: While Go allows converting named types to interfaces and vice versa, the permissions have no support
for that. The realisation that a named permission could be needed came too late in the process, after most of the interpreter had already been written, and would
have required significant changes in the interpreter which were not feasible anymore in the time alloted to this thesis.

The bottom-up approach also was the reason for requiring read permissions on the source of a copy or move assignment. This was added to fix a problem in the
interpreter, but ended up breaking a few other use cases.

The implementation did not reach completion. While we can interpret almost all statements, we do not have support for complete functions, packages, and
imports, meaning that an entire Go program cannot be checked yet. Support for annotations is mostly theoretical, too. While the annotation parsing works just fine
and is heavily relied upon in testing, we did not actually combine the stage that retrieves annotations with the interpreter that checks the permissions - we only
have a map from identifier to annotation so far (seen in \fref{sec:annotated-permissions}), but we never filled it.

Apart from completing the implementation, further work is also needed in the usability department: Deepy nested permissions, permissions with a lot of direct children,
and cycles in permissions means that errors are hard to understand. it is not entirely clear how to optimise that, though.

There's also the question on how to handle compatibility: While it is possible to handle existing Go calls in an unsafe manner (just specify parameters to request no
permissions, and return the maximum permission for return values), this does not seem satisfactory: If we can just have unsafe parts like that, the checking can easily
be circumvented. Adding an "unsafe" base permission would help here.

Overall, we can be happy with the permission package of the implementation - it is a high quality code base with 100% code coverage, and a fairly clear approach on what
can be done with permissions. It seems easily reusable in a better abstract interpreter or other checking framework. it is also easily extensible, especially when writing
new functions that check a property for two permissions (like "can I assign A to B") or that produce a new permission from two permissions (like, "intersect A and B"), as
their implementation is very generic and support for some new operations can easily be inserted.

We cannot be as happy with the interpreter package: it is not always easy to follow, it is not fully tested, and it has quite a few bugs. But that was to be expected, given
that it was written after the permission package, and time eventually ran out. The approach of using an abstract interpreter, and a store with effective and maximum permissions
seems worth pursuing, though.
