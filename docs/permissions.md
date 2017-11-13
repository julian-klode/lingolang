# Permissions for Go
In the previous chapter, we saw monads, linear types, and the two generalisations of linear types as capabilities
and fractional permissions. This chapter introduces permissions for Go based on the concepts from 'Capabilities for Sharing'[@Boyland:2001:CSG:646158.680004],
and certain operations that will be useful to build a static analyser that checks permissions on a Go program.

The reasons for going with a capabilities-derived approach are simple: Monads don't work in Go, as Go does not
have generic types; and fractional permissions are less powerful: Capabilities allow you to define objects with
single-writer, multiple reader permissions (where there are two references, $ow\overline{W}$ for writing and,
$or$ for reading), which might come in handy later, or even just non-linear writeable values, which can be useful
for interaction with legacy code.

This approach is called _Lingo_ (short for linear Go). Permissions in Lingo are different from the original
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
    var a /* orR [] owW * owW */ = make([]int)
    var b = a
```

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

Instead of using a store mapping objects, and (object, field) tuples to capabilities, that is, (object, permission) pairs, Lingo employs a different approach in order to combat the limitations shown in the introduction.
Lingo's store maps a variable to a permission.
It does however, not just have the permission bits introduced earlier (from now on called _base permission_), but also _structured_ permissions, equivalent to types.
These structured permissions consist of a base permission and permissions for each child, target, etc.

Apart from primitive and structured permissions, there are also some special permissions:

* The untyped nil permission, representing the `nil` literal.
* The wildcard permission, written `_`. It is used in permission annotations whenever the default permission for a type should be used.

The full syntax for these permissions is given in listing \ref{syntax}. The base permission does not need to be specified for structured types, if absent, it is considered to be `om`. There also are some shortcuts for some common combinations:

* `m`, for _mutable_, is equivalent to `rwRW`
* `v`, for _value_, is equivalent to `rW`
* `l`, for _linear value_, is equivalent to `rRW` and a linear variant of value
* `n`, for _none_, is equivalent to, well, none bits set
* `a`, for _any_, is equivalent to all non-exclusive bits set, that is `orwRW`.

In the rest of the chapter, we will discuss permissions using a set based notation: The set of rights, or permissions bits is $R = \{o, r, w, R, W\}$. A base permission
$b \subset R$ (single lower case character) is a subset of all possible bits. The set $P$ is the set of all possible permissions, and a single upper case character
$A \subset P$ indicates any member in it.

## Assignments
Some of the core operations on permissions involve assignability: Given a source permission and a target permission, can I assign an object with the source permission to a variable of the target permission?

As a value based language, one of the most common forms of assignability is copying:
```go
var x /* or */ = 0
var y = x   // copy
```
Another one is referencing:
```go
var x /* or */ = 0
var y = &x   // reference
var z = y    // still a reference to x, so while we copy the pointer, we also reference x one more time
```
Finally, in order to implement linearity, we need a way to move things:
```go
var x /* om */ = 0
var y = &x   // have to move x, otherwise y and x both reach x
var z = y    // have to move the pointer from y to z, otherwise both reach x
```
(Though we are moving the permissions, not the values in that case, `y` still points to `x`)

In the following, the function $ass: P \times P \to bool$ describes whether a value of the left permission can be assigned to a location of
the right permission; it takes an implicit parameter describing the current mode of copying. The functions $cop$, $ref$,
and $mov$ are functions doing assignment in copy, reference, and move mode.

The base case for assigning is base permissions. For copying, the only requirement is that the source is readable (or it and target are empty). A move additionally requires that no more permissions are added - this is needed: If I move a pointer to a read-only object, I can't move it to a pointer to a writeable object, for example. When referencing, the same no-additional-permissions requirement persists, but both sides may not be linear - a linear value can only have one reference, so allowing to create another would be wrong.
\begin{align*}
    ass(a, b) &:\Leftrightarrow \begin{cases}
        r \in a \text{ or } a = b =  \emptyset                                           & \text{if copying} \\
        b  \subset a \text{ and } (r \in A \text{ or } a = b = \emptyset)                & \text{if moving} \\
        b  \subset a \text{ and } \text{ and not } lin(a) \text{ and not } lin(b)        & \text{if referencing}
    \end{cases} \\
    \text{where } & lin(a) :\Leftrightarrow r, R \in a \text{ or } w, W \in a
\end{align*}

Next up are permissions with value semantics: arrays, structs, and tuples (tuples are only used internally to represent multiple function results). They are assignable if all their children are assignable.
\begin{align*}
    ass(a\ [\_]A, b\ [\_]B) &:\Leftrightarrow ass(a, b) \text{ and } ass(A, B)     \\
    \begin{aligned}
        ass(&a \textbf{ struct } \{ A_0; \ldots; A_n \}, \\
            &b \textbf{ struct } \{ B_0; \ldots; B_m \})
    \end{aligned} &:\Leftrightarrow
        ass(a, b) \text{ and } ass(A_i, B_i)    \quad \forall 0 \le i \le n \\
    \begin{aligned}
        ass(a \ ( A_0, \ldots, A_n),
            b \ ( B_0, \ldots, B_m))
    \end{aligned} &:\Leftrightarrow
        ass(a, b) \text{ and } ass(A_i, B_i)    \quad \forall 0 \le i \le n
\end{align*}

Channels, slices, and maps are reference types. They behave like value types, except that copying is replaced by referencing.
\begin{align*}
    ass(a \textbf{ chan } A, b \textbf{ chan } B) &:\Leftrightarrow \begin{cases}
        ref(a, b) \text{ and } ref(A, B)    & \text{copy} \\
        ass(a, b) \text{ and } ass(A, B)    & \text{else}
    \end{cases} \\
    ass(a\ []A, b\ []B) &:\Leftrightarrow \begin{cases}
        ref(a, b) \text{ and } ref(A, B)    & \text{copy} \\
        ass(a, b) \text{ and } ass(A, B)    & \text{else}
    \end{cases} \\
    ass(a \textbf{ map }[A_0] A_1, b \textbf{ map }[B_0] B_1) &:\Leftrightarrow \begin{cases}
        ref(a, b) \text{ and } ref(A_0, B_0) \text{ and } ref(A_1, B_1)    & \text{copy} \\
        ass(a, b) \text{ and } ass(A_0, B_0) \text{ and } ass(A_1, B_1)    & \text{else} \\
    \end{cases}
\end{align*}

Function permissions are a fairly special case.
The base permission here indicates the permissions of elements in the closure, essentially.
A mutable function is thus a function that can have different results for the same immutable parameters.
The receiver of a function, it's parameters, and the closure are essentially parameters of the function,
and parameters are contravariant: I can pass a mutable object when a read-only object is expected, but I
can't pass more. For the closure, ownership is the exception: An owned function can be assigned to an
unowned function, but not vice versa:
\begin{align*}
    \begin{aligned}
        ass(&a\ (R) \textbf{ func } ( P_0 \ldots, P_n ) (R_0, \ldots, R_m), \\
            &b\ (R') \textbf{ func } ( P'_0 \ldots, P'_n ) (R'_0, \ldots, R'_m)
    \end{aligned} &:\Leftrightarrow  \begin{cases}
        ref(a \cap \{o\}, b \cap \{o\}) \\
                     \quad\text{and } ref(b \setminus \{o\}, a \setminus \{o\})  \\
                     \quad\text{and } mov(R, R) \\
                     \quad\text{and } mov(P'_i, P_i) \\
                     \quad\text{and } mov(R_j, R'_j)   & \text{copy}\\
        ass(a \cap \{o\}, b \cap \{o\}) \\
                     \quad\text{and } ass(b \setminus \{o\}, a \setminus \{o\})  \\
                     \quad\text{and } mov(R', R) \\
                     \quad\text{and } mov(P'_i, P_i) \\
                     \quad\text{and } mov(R_j, R'_j)   & \text{else}\\
        \end{cases} \\
        & \qquad \ \text{ for all } 0 \le i \le n, 0 \le j \le m
\end{align*}
TODO: Why do we use $mov$ for receivers, parameters, return values?

Pointers are another special case: When a pointer is copied, itself is copied, but the target is referenced (as we now have two pointers to the same target):
\begin{align*}
    ass(a * A, b * B) &:\Leftrightarrow \begin{cases}
        ass(a, b) \text{ and } ref(A, B)    & \text{copy} \\
        ass(a, b) \text{ and } ass(A, B)    & \text{else}
    \end{cases}
\end{align*}
There is one minor deficiency with this approach: A pointer `ol * om` cannot be moved into a pointer `om * om`, due to the rule about not adding any permissions. This is the correct behaviour when moving a reference to such a pointer, but when we have two pointer variables with these permissions, we should be able to move the value itself. That is, there should probably be two types of moving: moving by reference, and moving by value. It is unclear if it is worth the effort, though - it does mean that function parameters should not require `om * om` pointers, but rather 'ol * om', but that is a minor issue.

Interfaces are method sets that work like reference types, but the methods are always moved rather than referenced. TODO This actually seems wrong.
\begin{align*}
    \begin{aligned}
        ass(&a \textbf{ interface } \{ A_0; \ldots; A_n \}, \\
            &b \textbf{ interface } \{ B_0; \ldots; B_m \})
    \end{aligned} &:\Leftrightarrow  \begin{cases}
        ref(a, b) \text{ and } mov(A_{idx(B_i, A)}, B_i)    & \text{copy}\\
        ass(a, b) \text{ and } mov(A_{idx(B_i, A)}, B_i)    & \text{else}\\
        \end{cases} \\
        & \qquad \ \text{ for all } 0 \le i \le m
\end{align*}
where  $idx(B_i, A)$ determines the position of a method with the same name as $B_i$ in $A$.

Finally, we have some special cases: The wildcard and nil. The wildcard is not assignable, it's only used when writing permissions to mean "default". The `nil` permission is assignable to itself, to pointers, and permissions for reference and reference-like types.
\begin{align*}
        ass(\textbf{\_}, B)  &:\Leftrightarrow \text{ false } \\
        ass(\textbf{nil}, a * B)  &:\Leftrightarrow \text{ true } & ass(\textbf{nil}, a \textbf{ chan } B)  &:\Leftrightarrow \text{ true } \\
        ass(\textbf{nil}, a \textbf{ map } [B]C)  &:\Leftrightarrow \text{ true } &
        ass(\textbf{nil}, a []C)  &:\Leftrightarrow \text{ true } \\
        ass(\textbf{nil}, a \textbf{ interface } \{ \ldots \})  &:\Leftrightarrow \text{ true } &
        ass(\textbf{nil}, \textbf{nil})  &:\Leftrightarrow \text{ true }
\end{align*}

## Converting to a base permission
Converting a given permission to a base permission essentially replaces all base permissions in that permission with the specified one, except for some exception, which we'll see later. It's major use case is specifying an incomplete type, for example:

```go
var x /* @perm om */ *int
```
It's a pointer, but the permission is only for a base. We can convert the default permission for the type (we'll discuss them later) to `om`, giving us a complete permission. And in the next section, we'll extend conversion to arbitrary prefixes of the permission graph.

Another major use case is ensuring consistency of rules, like:

- Unwritable objects may not embed any writable objects
- Non-linear unwriteable objects may contain pointers to non-linear writable objects
- Linear unwritable objects may point to linear unwritable objects.

As every specified permission will be converted to its base type, we can ensure that every permission is consistent, and we don't end up with inconsistent permissions like `or * om` - a pointer that could be copied, but pointing to a linear object.

Most cases of to-base conversions are rather simple:
\begin{align*}
    ctb(a, b) &:= b \\
    ctb(\_, b) &:= b \\
    ctb(nil, b) &:= nil \\
    ctb(a \textbf{ chan } A, b) &:= ctb(a, b) \textbf{ chan } ctb(B, ctb(a, b)) \\
    ctb(a \textbf{ } []A, b) &:= ctb(a, b) \textbf{ } []ctb(B, ctb(a, b))       \\
    ctb(a \textbf{ } [\_]A, b) &:= ctb(a, b) \textbf{ } [\_]ctb(B, ctb(a, b))   \\
    ctb(a \textbf{ map} [A]B, b) &:= ctb(a, b) \textbf{ map} [ctb(A)]ctb(B, ctb(a, b))   \\
    ctb(a \textbf{ struct } \{ A_0; \ldots; A_n \}, b) &:= ctb(a, b) \textbf{ struct }  \{ ctb(A_0, ctb(a, b)); \ldots; ctb(A_n, ctb(a, b)) \}   \\
    ctb(a\ ( A_0; \ldots; A_n), b) &:= ctb(a, b)\ ( ctb(A_0, ctb(a, b)); \ldots; ctb(A_n, ctb(a, b)) )
\end{align*}

Functions and interfaces are special, again: Methods, and receivers, parameters, results of functions are converted to their own base permission:
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
    var x /* or */ func(int) *int
```
`x` should be `or`, but this does not mean that it should be `or func (or) or`. While the result seems OK here, the default for a function parameter should be unowned (and read-only).


For pointers, it is important to add one thing: There are two types of conversions: Normal ones and strict ones. The difference is simple: While the normal one works combines the old target's permission with the permission being converted to, strict conversion just converts the target to the specified permission. Strict conversions will become important when converting (in the type sense) a value to interfaces:
```go
var x /* om * or */ *int
var y /* om interface {} */ = x
var z /* om * om */ = y.(*int)     // um, target is mutable now?
```
Converting to an interface is a lossy operation: We can only maintain the outer permission. But we cannot allow the case above to happen: We just converted a pointer to read-only data to a pointer to writeable data. Not good. One way to solve this is to ensure that a permission can be assigned to it's strict permission, gathered by strictly converting the type-default permission to the current permissions base permission:
$$
y = x \Leftrightarrow  ass(perm(x), ctb_{strict}(perm(typeof(x)), base(perm(x)) \text { and } ass(base(x), base(y))
$$
\begin{samepage}
The rules for converting a pointer permission to a base permission are thus a bit complex:
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
1. If the new base permission has no exclusive read right, but the new target has exclusive write and write flags (is linearly writable), these flags are dropped.
2. If the new base permission has no exclusive read right, but the new target has exclusive read and read flags (is linearly readable), the exclusive read flag is dropped.

Steps 1 and 2 make it consistent: Without them, we could have a non-linear pointer pointing to a linear target. Since the target could only have one reference, but the pointer appears to be copyable (it's not, as the assignability rules also work recursively), we get the impression that we could have two pointers for the same target. It also allows us to just gather linearity info from the outside: If the base permission of a value is non-linear, it cannot contain linear values - this can be used to simplify some checks.

#### Theorem: $cbt_b(A) = cbt(A, b)$ is idempotent
_Theorem:_ Conversion to base, $cbt$ is idempotent, or rather $cbt_b(A) = cbt(A, b)$ is. That is, for all $A \in P, b \in R$: $ctb_b(A) = ctb(A, b) = ctb(ctb(A, b), b) = ctb_b(ctb_b(A))$.

_Background:_ This theorem is important because we generally assume that $ctb(A, base(A)) = A$ for all $A \in P$ that have been converted once (what is called consistent, and is the case for
all permissions the static analysis works with).

_Proof._

1. Simple cases:
    \begin{align*}
        ctb(ctb(a, b), b) &= ctb(b, b) = b = ctb(a, b) \\
        ctb(ctb(\_, b), b) &= ctb(b, b) = b = ctb(\_, b)\\
        ctb(ctb(nil, b), b) &= ctb(nil, b) = nil = ctb(nil, b)\\
    \end{align*}
1. Channels, slices, arrays, maps, structs, and tuples basically have the same rules: All children are converted to the same base permission as well. It suffices to prove one of them. Let us pick channels:
    \begin{align*}
        ctb(ctb(a \textbf{ chan } A, b), b) &= ctb(ctb(a, b) \textbf{ chan } ctb(A, ctb(a, b)), b) & \text{(def chan)} \\
                                            &= ctb(ctb(a, b), b) \textbf{ chan } ctb(ctb(A, ctb(a, b)), b) & \text{(def chan)}\\
                                            &= ctb(b, b) \textbf{ chan } ctb(ctb(A, b), b) & (ctb(a, b) = b) \\
                                            &= b \textbf{ chan } ctb(A, b)  & \text{other case} \\
                                            &= ctb(a, b) \textbf{ chan } ctb(A, ctb(a, b)) & (ctb(a, b) = b) \\
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
1. Pointers are more complicated. Let $ctb(a * A, b) = a' * ctb(A, t_2)$ with $a' = ctb(a,b)$ and a $t_2$ according to the definition. And  ctb(ctb(a * A, b), b) = ctb(a' * ctb(A, t_2), b) = a'' * ctb(A, t_2')$. We have to show that $a' = a''$ and $t_2 = t_2'$.

    1. $a' = ctb(a, b) = ctb(ctb(a, b), b) = a''$.
    2. $t_2$ essentially has the form $t_0 \setminus X = base(A) \setminus \{o\} \cup (a \cap \{o\}) \setminus X$ for some set $X \in \{\{R\}, \{w, W\}, \{R, w, W\}\}$; depending on the value of A.

    Given that $a = a'$, it follows that:

    \begin{align*}
        t_0' &= ( \underbrace{base(ctb(A, t_2)}_{= t_2}) \setminus \{o\}) \cup (\underbrace{a''}_{= a'} \cap \{o\}) \\
            &= ( t_2 \setminus \{o\}) \cup (a' \cap \{o\}) \\
            &=  t_2 \\
            &= ( (base(A) \underbrace{\setminus \{o\} \cup (a' \cap \{o\})}_{\text{no effect due to } \setminus \{o\} \text{ later}} \setminus X)  \setminus \{o\}) \cup (a' \cap \{o\}) \\
            &= ( (base(A) \setminus X)  \setminus \{o\}) \cup (a' \cap \{o\}) \\
            &= ( base(A) \setminus \{o\}) \cup (a' \cap \{o\}) \setminus X & \text{because }  o \not\in X\\
            &= t_0 \setminus X = t_2
    \end{align*}

    Now, let's look at $t_1'$ and $t_2'$. There are two variants each: $t_i = t_{i-1}$ and $t_i = t_{i-1} \setminus X$ for some $X$ if  $R \not\in a'$ and some other condition holds. Therefore, for $R \in a'$,
    it trivially follows that $t_0 = t_1 = t_2$ and $t_0' = t_1' = t_2'$. Let's assume $R \not\in a'$, and thus . For $t_1'$, this means:
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
    The first case cannot happen: $t_0' \supset \{w, W\} \Rightarrow ( t_2 \setminus \{o\}) \cup (a' \cap \{o\}) \supset \{w, W\} \Rightarrow t_2 \supset \{w, W\}$. But that can't be the case as $t_2 \subset t_1$, and $t_1$ is $t_0$ with $\{r, W\}$ removed if they were part of it. Therefore, $t_1' = t_0' = t_2$.
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
    And again, the first case cannot happen, it leads to a contradiction: $t_1' = t_2 \supset \{r, R\} \Rightarrow t_1 \supset \{r, R\} \Rightarrow t_2 = t_1 \setminus \{R\} \Rightarrow t_2 \not\supset \{r, R\}$.

    Therefore, $t_2' = t_1' = t_0' = t_2$, and thus $ctb(ctb(a * A, b), b) = ctb(a * A, b)$.
1. The strict case of pointers is trivial and can be proven like channels.

In conclusion, $ctb(ctb(A, b), b) = ctb(A, b)$ for all $A \in P, b \in R$, as was to be shown.  $\qed$


## Merging and Converting
The idea of conversion to base permissions from the previous paragraph can be extended to converting between structured types. When converting between two structured types, replace all base permissions in the source with the base permissions in the same position in the target, and when the source permission is structured and the target is base, it just switches to a to-base conversion.

There are two more kinds of recursive merge operations: intersection and union.
These are essentially just recursive relaxations of intersection and union on the base permissions, that is, they simply perform intersection and union on all base types
in the structure.

Except for functions of course: An intersection of a function requires union for parameters and receivers, because just like with subtyping (in languages that have it), parameters and receivers are contravariant:
If one function expects `orw` and another expects `or` a place that needs either of those functions (an intersection) needs a function that accepts $orw \cup or = orw$ - because passing a writable object to a function only needing a read-only one would work, but passing a read-only value to a function that needs a writable one would not be legal.

Intersections are sort of a parallel to phi nodes in a program's static single assignment form. They can effectively be used to join different paths:

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

As a special exception to the recursive relaxation and same-shape rules, when either side of a merge is a wildcard permission, the result is the other side - the wildcard permission acts as the neutral element. [^monoid]

[^monoid]: I believe that makes permissions with these operations monoids (a semi group (set with associative operation) with a neutral element), but proofing associativity for this recursive operation is a bit too involved

As a further special exception, if either side is a nil permission and the other side a pointer permission, the pointer permission is the result. For conversions, this does make sense: Given that I can assign nil to any pointer, I can also convert nil permissions to any pointer permission. For union and intersection, consider the classical examples:

* For union, the question is: Can $p \cup nil$ be used in place of both $p$ and $nil$? Technically the answer is no, because $p$ cannot be used where $nil$ is expected. But nil permissions are only ever
  used for $nil$ literals (they cannot even be specified, there is no syntax for them), so we never reach that situation.
* For intersection, the question is: Can values of $p$ or $nil$ be assigned to $p \cap nil$. Yes, they can be, $nil$ is assignable to every pointer, and $p$ is assignable to itself.


## Creating a new permission from a type
Since permissions have a similar shape as types and Go provides a well-designed types package, we can easily navigate type structures and create structured permissions for them with some defaults. Currently, it just places maximum `om`
permissions in all base permission fields.
