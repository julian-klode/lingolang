# Static analysis of Go programs
Based on the operations described in the previous section, a static analyser can be written that ensures that the rules of linearity are respected. This static analysis can be done in the form of an abstract interpreter; that is, an interpreter that does not operate on concrete values, but abstract values and tries to interpret all possible paths through a program.

In the `github.com/julian-klode/lingolang` reference implementation, the permissions and operations are provided in thea package called `capabilities` for historic reasons. A better name might have been `interpreter`.

## The store
The store is an essential part of the interpreter. It maps variable names $\in V$ to:

1. An effective permission
2. A maximum permission
3. A use count

The maximum permission restricts the effective permission. It can be used to prevent re-assignment of variables by revoking the write permission there. The uses count will be used later when evaluating function literals to see which values have been captured in the closure of the literal.

The store has several operations:

1. `GetEffective`, written $S[v]$, returns the effective permission of $v$ in $S$.
1. `GetMaximum`, written $S[\overline{v}]$, returns the maximum permission of $v$ in $S$.
1. `Define`, written $S[v := p]$, is a new store where a new $v$ is defined if none is in the current block, otherwise, it's the same as $S[v = p]$.
1. `SetEffective`, written $S[v = p]$, is a new store where $v$'s effective permission is set to $intersect(p, S[\overline{v}])$
1. `SetMaximum`, written $S[v \overset{\wedge}{=} p]$, is a new store where $v$'s maximum permission is set to $p$. It also performs $S[v = intersect(p, S[\overline{v}])]$ to ensure that the effective permission is weaker than the maximum permission.
1. `Release`, written $S[=D]$, where $D$ is a set of tuples $(V, P)$ is the same as setting the effective permissions of all $(v, p) \in D$ in S. We call that _releasing_ D.
1. `BeginBlock`, written $S[+]$, is a store where a new block has begun
1. `EndBlock`, written $S[-]$, is S with the most recent block removed
1. `Merge`, written $S \cap S'$, where S and S' have the same length and variables in the same order, is the result of intersecting all permissions in S with the ones at the same position in S'.

In the code, the store is a slice of structs where each struct contains a name, an effective permission, a maximal permission, and the number of times the variable has been referenced so far. Defining a new variable or beginning a new block scope prepends to the store.

```go
type Store []struct {
	name string
	eff  permission.Permission
	max  permission.Permission
	uses int
}
```
The beginning of a block scope is marked by a struct where the fields have their zero values, that is `{"", 0, 0, 0}`. More specifically,
such a block marker is identified by checking if the name field is empty. When exiting a block, we simply find the first such marker, and then create a slice starting with the element following it.


## Expressions
The interpreter's function `func (i *Interpreter) VisitExpr(st Store, e ast.Expr) (permission.Permission, Owner, []Borrowed, Store)` visits an expression in a store, yielding a new permission, an owner, a set of variables borrowed by the expression, and a new store.

The types `Borrowed` and `Owner` are pairs of a variable name and a permission. The owner of an expression is the variable of which the expression is a part of; for example, the owner of `&array[1]` is `array`. Dependencies represented by `Borrowed` are other values that have been borrowed. For example, in the composite literal `T {a, b}`, `a` and `b` could be dependencies. There is a special `NoOwner` value of type `Owner` that represents that no owner exists for a particular expression.

The `Owner` vs `Borrowed` distinction is especially important with deferred function calls and the go statement. We will later see that the owner is the function (which may be a closure with a bound receiver), while any owners and dependencies of the arguments are forgotten.

Since the code is a bit long too read, it makes sense to provide a short, and hopefully more readable abstraction of it. The function `VisitExpr` essentially becomes the relation $\leadsto : Expr \times Store \to Permission \times (Variable, Permission) \times \text{set of } (Variable, Permission) \times Store$.

In the following, we will look at the individual expressions and check how they evaluate. The rules are written similar to typing rules in "Types and Programming Languages" by Benjamin C. Pierce [@tapl].


#### Identifier: `id`

If the identifier is `nil`, a `nil` permission is returned.
If it is `true` or `false`, the permission `om` is returned. Otherwise, the effective permission $e$ for the entry with the same name in the store is returned, and replaced
by $ctb(effective, \{n\})$. The owner of $e$ is $(identifier, e)$ - if code is done using the value referred to
by the identifier, it can "put it back" in the store:

\begin{align*}
    \langle \textbf{nil}, s \rangle &\leadsto (nil, NoOwner, \emptyset, s) & \text{(P-Nil)}\\
    \langle \textbf{true}, s \rangle &\leadsto (om, NoOwner, \emptyset, s) & \text{(P-True)}\\
    \langle \textbf{false}, s \rangle &\leadsto (om, NoOwner, \emptyset, s)  & \text{(P-False)}\\
    \langle id, s \rangle &\leadsto (s[id], (id, s[id]), \emptyset, s[id = ctb(s[id], n)])  & \text{(P-Ident)}
\end{align*}

For a comparison, the code implementing this is shown in listing \ref{visitIdent}.



```{#visitIdent .go caption="Abstract interpreter for identifiers" float=t frame=tb}
func (i *Interpreter) visitIdent(st Store, e *ast.Ident) (permission.Permission, Owner, []Borrowed, Store) {
	if e.Name == "nil" {
		return &permission.NilPermission{}, NoOwner, nil, st
	}
	if e.Name == "true" || e.Name == "false" {
		return permission.Mutable | permission.Owned, NoOwner, nil, st
	}
	perm := st.GetEffective(e.Name)
	if perm == nil {
		i.Error(e, "Cannot borow %s: Unknown variable in %s", e, st)
	}
	owner := Owner{e, perm}
	dead := permission.ConvertToBase(perm, permission.None)
	st, err := st.SetEffective(e.Name, dead)
	if err != nil {
		i.Error(e, "Cannot borrow identifier: %s", err)
	}
	return perm, owner, nil, st
}
```

TODO: Currently, the moving also happens for non-linear values. This seems rather pointless, and might complicate things a bit.


#### Star Expression: `*E`

If `expr` has a pointer permission, the result is the same as evaluating the pointer, with the permission in the
result replaced by the result's target permission:

\begin{align*}
    \frac{\langle E, s \rangle \leadsto (a * A, o, d, s') \text{ for some } a \in R, A \in P}{\langle *E, s \rangle \leadsto (A, o, d, s')} && \text{(P-Star)}
\end{align*}

#### Binary expression: `A op B`

First a is evaluated in $s$, yielding $(P_a, o_a, d_a, s_a)$. The borrowed objects in $o_a$ and $d_a$ are released, yielding $s_a'$, and then b is evaluated in $s_a'$, yielding: $(P_b, o_b, d_b, s_b)$.
Its dependencies and owner are released as well, yielding $s_b'$.

For logical operators, that is $op \in \{\text{\lstinline/&&/}, \text{\lstinline/||/} \}$, the resulting store is generated by intersecting the store $s_a'$ with $s_b'$. This ensures that any code
executed after `A && B` is valid both if only `A` was executed (and false), and if both `A` and `B` were executed (because A was true).
\begin{align*}
\frac{\langle A, s\rangle \leadsto (P_a, o_a, d_a, s_a)  \qquad \langle B, s_a[=d_a \cup \{o_a\}] \rangle \leadsto (P_b, o_b, d_b, s_b) \qquad r \in P_a, P_b}{\langle A \text{ op } B, s \rangle \leadsto (om, NoOwner, \emptyset, s_a[=d_a \cup \{o_a\}] \cap s_b[= d_b \cup \{o_b\}])}    && \text{(P-Logic)} \\
    (\text{for all short-circuiting binary operators } op)
\end{align*}

For other operators, that is $op \not\in \{\text{\lstinline/&&/}, \text{\lstinline/||/} \}$, the resulting store is just the store after executing B, as there is no "short circuiting".
\begin{align*}
\frac{\langle A, s\rangle \leadsto (P_a, o_a, d_a, s_a)  \qquad \langle B, s_a[=d_a \cup \{o_a\}] \rangle \leadsto (P_b, o_b, d_b, s_b) \qquad r \in P_a, P_b}{\langle A \text{ op } B, s \rangle \leadsto (om, NoOwner, \emptyset, s_b[= d_b \cup \{o_b\}])}    && \text{(P-binary)} \\
    (\text{for all not short-circuiting binary operators } op)
\end{align*}


In both cases, the result has no owner and no dependencies, and the permission `om`, as all binary operators produce primitive values, either boolean or numeric.



#### Index expression: `A[B]`

We first evaluate the left side, then the index. If the left side is an array or a slice, the right side is a primitive value, so we can release its owner
and dependency if any (it's just an offset into an array), and then return the permission for the elements in the array or slice, with `A` being the owner
(as `A[B]` is part of A).
\begin{align*}
    \frac{
        \langle A, s \rangle \leadsto (p_a [] P_a, o_a, d_a, s_a)
        \quad \langle B, s_a \rangle \leadsto (P_b, o_b, d_b, s_b)
        \quad r \in p_a, p_b
    }{
        \langle A[B], s \rangle \leadsto (P_a, o_a, d_a, s_b[= o_b \cup d_b])
    } && \text{(P-SIdx)} \\
    \frac{
        \langle A, s \rangle \leadsto (p_a [\_] P_a, o_a, d_a, s_a)
        \quad \langle B, s_a \rangle \leadsto (P_b, o_b, d_b, s_b)
        \quad r \in p_a, p_b
    }{
        \langle A[B], s \rangle \leadsto (P_a, o_a, d_a, s_b[= o_b \cup d_b])
    } && \text{(P-AIdx)} \\
\end{align*}


If the left hand side is a map, the right hand side might be more complicated. Since this expression may appear on the left hand side of an assignment, that is,
when writing a value into the map, and the key might be arbitrarily complicated, we need to treat this as if we are assigning the key to somewhere.


For cases of assigning things, we can define a helper function, called `moveOrCopy` (listing \ref{moveOrCopy}), or short $moc$ (cases checked in order, top to bottom):

\begin{align*}
    moc(st, F, T, o, d) := \begin{cases}
        (st[= d \cup \{o\}], NoOwner, \emptyset) & \text{ if } cop(F, T) \\
        \bot & \text{ if not } mov(F, T) \\
        (st, o, d)  & \text{ if } o \not\in base(T) \\
        (st[= d \cup \{unlinear(o)\}], NoOwner, \emptyset) & \text{ if } lin(F) \text{ and not } lin(T) \\
        (st, NoOwner, \emptyset) & \text{ else } \\
    \end{cases} \\
    \textbf{where }  unlinear(o) := \begin{cases}
        o & \text{ if } o = NoOwner \\
        (v, ctb(p, base(p) \setminus \{R,W,w\})) & \text{else, where for some $v$, $p$:} o = (v, p)
    \end{cases}
\end{align*}

Explanations for each case:

1. The permission $F$ is copyable to $T$. This means that we can release the owner and dependencies back to the store - they no longer need to be borrowed.
2. The permission $F$ is not movable. This is an error case, all later cases require movability.
3. The target permission is unowned. We are just temporarily borrowing the object, so we keep the owner and dependencies around
4. We are moving a linear value to a non-linear value.
   This means we need to "freeze" the value, and any object containing it, that is make them immutable and non-linear.
5. We are doing any other kind of move. Since we handled copying in case 1, this means that we are moving a linear value to a linear value (a non-linear value
   would be copyable). Therefore, the owner and dependencies borrowed for $F$ will be forgotten, ensuring we cannot reach $F$
   via an alias once we moved it to $T$.

```{#moveOrCopy .go caption="The essential \lstinline|moveOrCopy| helper function" float=!hbt frame=tb}
func (i *Interpreter) moveOrCopy(e ast.Node, st Store, from, to permission.Permission, owner Owner, deps []Borrowed) (Store, Owner, []Borrowed, error) {
    switch {
    // If the value can be copied into the caller, we don't need to borrow it
    case permission.CopyableTo(from, to):
        st = i.Release(e, st, []Borrowed{Borrowed(owner)})
        st = i.Release(e, st, deps)
        owner = NoOwner
        deps = nil
    // The value cannot be moved either, error out.
    case !permission.MovableTo(from, to):
        return nil, NoOwner, nil, fmt.Errorf("Cannot copy or move: Needed %s, received %s", to, from)

    // All borrows for unowned parameters are released after the call is done.
    case to.GetBasePermission()&permission.Owned == 0:
    // Write and exclusive permissions are stripped when converting a value from linear to non-linear
    case permission.IsLinear(from) && !permission.IsLinear(to):
        if owner != NoOwner {
            owner.perm = permission.ConvertToBase(owner.perm, owner.perm.GetBasePermission()&^(permission.ExclRead|permission.ExclWrite|permission.Write))
        }
        st = i.Release(e, st, []Borrowed{Borrowed(owner)})
        st = i.Release(e, st, deps)
        owner = NoOwner
        deps = nil
    // The value was moved, so all its deps are lost
    default:
        deps = nil
        owner = NoOwner
    }

    return st, owner, deps, nil
}
```

With $moc$ defined, we are able to define $\leadsto$ for indexing maps:
\begin{align*}
    \frac{
        \langle A, s \rangle \leadsto (p_a \textbf{map}[K] V, o_a, d_a, s_a)
        \quad \langle B, s_a \rangle \leadsto (P_b, o_b, d_b, s_b)
        \quad r \in p_a, p_b
    }{
        \langle A[B], s \rangle \leadsto (V, o_a, d_a, s_b') \text{ where } s_b', o_b', d_b' = moc(s_b, P_b, K, o_b, d_b)
    } && \text{(P-MIdx)}
\end{align*}
As can be seen, if any owner and dependencies are remaining for $B$ after $moc$, these are forgotten too. This only affects
maps with unowned keys, as $moc$ otherwise always returns no owner and an empty dependency set.
TODO: Big issue?

One thing missing is indexing strings, which yields characters. Strings are represented as base permissions currently, but
probably deserve their own permission type.

#### Unary expressions
We have already seen one unary expression, the star expression. For unknown reasons, it is its own category of syntax node
in Go, while all the other unary expressions share a common type.

The first unary expression to discuss is `&E`, the _address-of_ operator. Taking the address of `E` constructs a pointer to it.
At the moment, this is done like that:
\begin{align*}
    \frac{
        \langle E, s \rangle \leadsto (P_e, o_e, d_e, s_e)
    } {
        \langle \&E, s \rangle \leadsto (om * P_e, o_e, d_e, s_e)
    } && \text{(P-Addr)}
\end{align*}
There is a problem with that approach and how assignment is handled: Given a variable `v` that is mutable, `&v` moves the
permission from the store to the expression, but only the effective permission is moved, as we saw in the definition for
identifiers before. This causes problems later when introducing assignment statements: They check the maximum permissions.
What should happen here is that we also take away the maximum permission of the owner, preventing any future re-assignment
and restoration of its effective permissions (that's a TODO).


The next operation is `<-E`, the channel receive operation. The expression `E` is a channel, and the next value in it
is to be retrieved. We can immediately release any owner and dependencies of `E` once we got the value, after all, the
value is not owned by the channel, it's owned by the receiver:

\begin{align*}
    \frac{
        \langle E, s \rangle \leadsto (p_e \textbf{ chan } P_e, o_e, d_e, s_e)
    } {
        \langle \leftarrow E, s \rangle \leadsto (P_e, NoOwner, \emptyset, s_e[= d_e \cup \{o_e\}])
    } && \text{(P-Recv)}
\end{align*}


Finally we have the "boring" case of other unary operators, like plus and minus. These are just working on primitive values, so
we can just return a new primitive owned mutable permission:
\begin{align*}
    \frac{
        \langle E, s \rangle \leadsto (P_e, o_e, d_e, s_e)
    } {
        \langle op\ E, s \rangle \leadsto (om, NoOwner, \emptyset, s_e[= d_e \cup \{o_e\}])
    } && \text{(P-Unary)}
\end{align*}

#### Basic literals
A basic literal `lit` just evaluates to an `om` permission, and obviously has no owner or dependencies.
\begin{align*}
    \langle lit, s \rangle \leadsto (om, NoOwner, \emptyset, s) && \text{(P-Lit)}
\end{align*}

#### Function calls
A function call $E(A_0, \ldots, A_n)$ is fairly simple: First of all, the function is evaluated, then the arguments, from left to right.
Owners and dependencies are collected until the end of the call, when they can be released. A special case in a function call are `go` statements and defer statements:
Here, no permissions are released, and the owner and the dependencies of the function become the owner and dependencies of the function expression.
This is needed: For deferred statement, the arguments are bound in place of the statement, but the function is only executed when unwinding the call stack,
hence these parameters need to be unreachable in the function where the statement is located. The Go statement is similar, except that execution is not
done when the stack unwinds, but on a different goroutine.

Given:
\begin{align*}
    \langle E, s \rangle &\leadsto (e \textbf{ func } (P_0, \ldots, P_n) (R_0, \ldots, R_r), o, d, s_{-1}) \\
    \langle A_i, s_{i-1} \rangle &\leadsto (P_{A_i}, o'_{i}, d'_{i}, s'_{i})   \\
    s_{i}, o_i, d_i &= moc(s'_{i}, P_{A_i}, P_i, o'_i, d'_i) \\
    \intertext{Then:}
    \langle E(A_0, \ldots, A_n), s \rangle &\leadsto (results, o, d, s_{n})  & \text{if deferred or go}  \\
    \langle E(A_0, \ldots, A_n), s \rangle &\leadsto (results, NoOwner, \emptyset, rel(s_{n}))  & \text{else} \\
    \text{where } rel(s) &= s\left[= \{o\} \cup d \cup \bigcup\limits_{i=0}^{n} (d_i \cup \{o_i\})\right] \\
     results &= \begin{cases}
        R_0         & \text{if } r = 0 \\
        (R_0, \ldots, R_n) & \text{else}
    \end{cases}
\end{align*}

One seemingly odd result of the rules is that an argument is moved into a deferred call or go call even if the parameter is unowned. Under normal circumstances binding an argument to an unowned parameter would just temporarily borrow it. But with a deferred call or go call, there is no clear point at which the call ends, hence it's not possible to release them again:

```go
// Case 1: deferred function call or go statement
defer function(arg)     // arg is linear and not copyable

// should not be able to use arg here, even if function parameter is unowned

// Case 2: Normal function call
function(arg)     // arg is linear and not copyable

// arg released (if parameter is unowned), should be able to use it
```

#### Slice expressions
The slice expression `A[L:H:M]` (where `L`, `H`, `M` are optional) is simply evaluated left to right. For the purpose of this definition, let us assume that they all are specified. If either are unspecified, their owner would be `NoOwner`, their dependencies empty, and the store identical to the left-hand-side store.

\begin{align*}
\intertext{Therefore, if:}
    \langle A, s \rangle   &\leadsto (P_a, o_a, d_a, s_a) \\
    \langle L, s_a \rangle &\leadsto (P_l, o_l, d_l, s_l)      \qquad \text{ and } r \in base(P_l) \\
    \langle H, s_l \rangle &\leadsto (P_h, o_h, d_h, s_h)     \qquad \text{ and } r \in base(P_h)\\
    \langle M, s_m \rangle &\leadsto (P_m, o_m, d_m, s_m)      \qquad \text{ and } r \in base(P_m)\\
\intertext{Then, f $P_a$ is an array (has the form $a\ [\_]E$), the result becomes a slice of $E$s:}
    \langle A[L:H:M], s \rangle &\leadsto (om []E, o_m, d_m, s_m[= d_l \cup d_h \cup d_m \cup \{o_l, o_h, o_m\}]) \\
\intertext{Otherwise, if $P_a$ is a slice already, it stays one:}
    \langle A[L:H:M], s \rangle &\leadsto (P_a, o_m, d_m, s_m[= d_l \cup d_h \cup d_m \cup \{o_l, o_h, o_m\}])
\end{align*}

This is overly pessimistic: All owners and dependencies are only released at the end, they could be released earlier (TODO).


##### Selector expression
The selector expression `E.I`, where `E` is an expression and `I` is an identifier seems simple. But Go allows embedding structs in others and accessing the fields of embedded structs without explicitly referencing the embedded struct:
```
type T struct { x int }
type U struct { T }

var u U

// u.x is u.T.x
```

Hence any such expression actually has to traverse a path.


Go defines three types of selections:

1. `FieldVal`, field values - a value indexed by a field name
2. `MethodVal`, method values - a value indexed by a method name
3. `MethodExpr`, method expressions - a type indexed by a method name

The type of selection really only applies to the last element in the path though: When selecting a path like `u.T.x` above, `T` must be a struct - a function (which the other cases would produce) cannot be selected.

It also provides a library function to translate a selection expression to a path of integers, referencing the fields / methods in the object that is being selected from. Let this function
produce a path $index_i$ ($0 \le i < n$) and $selectionKind$, one of the three selection kinds.


First of all, if we can evaluate the statement:

$$\langle E, s \rangle \leadsto (P_{-1}, o_{-1}, d_{-1}, s_{-1})$$

And next, can, for each step $i \ge 0$ in the path, perform a selection:

\begin{align*}
    (P_i, o_i, d_i, s_i) :&= selectOne(s_{i-1}, P_{i-1}, index_{i}, kind, o_{i-1}, d_{i-1}) \\
    \text{where } kind &= \begin{cases}
        selectionKind    & \text{if } i == n - 1 \\
        \text{\lstinline|FieldVal|}         & \text{else (as explained before)}
    \end{cases} \\
    \text{and } & selectOne() \text{ will be defined later}
\end{align*}

Then:
$$\langle E.I, s \rangle \leadsto (P_{n-1}, o_{n-1}, d_{n-1}, s_{n-1})$$

Now, $selectOne$ is tricky. Field values are simple, with one complication: Pointers are handled transparently - selecting a field in a pointer selects a field in the struct, as the language requires that.

\begin{align*}
    selectOne&(s, a * A, idx, \text{\lstinline|FieldVal|}, o, d)  \\
    &:= selectOne(s, A, idx, \text{\lstinline|FieldVal|}, o, d) \\
    selectOne&(s, a \textbf{ struct } \{A_0, \ldots, A_n\}, idx, \text{\lstinline|FieldVal|}, o, d) \\
         &:= (A_{idx}, o, d, s) \\
\intertext{Method expressions simply find the method, and prepend the receiver permission to the parameter permissions}
    selectOne&(s, a \textbf{ interface } \{A_0, \ldots, A_n\}, idx, \text{\lstinline|MethodExpr|}, o, d) \\
         &:= (recvToParams(A_{idx}), NoOwner, \emptyset, s[= d \cap \{o\}]) \\
         \text{where } & recvToParams(a (R) \textbf{ func } (P_0, \ldots, P_n) (R_0, \ldots, R_r)) \\
            &:= a \textbf{ func } (R, P_0, \ldots, P_n) (R_0, \ldots, R_r) \\
\intertext{For method values, we reuse the $moc$ function defined earlier to move or copy the lhs into the receiver. If we are binding an unowned receiver, the bound method value will be unowned too, to ensure we don't store an unowned value in an owned function value, as they have different lifetimes.}
    selectOne&(s, \overbrace{a \textbf{ interface } \{A_0, \ldots, A_n\}}^{= In}, idx, \text{\lstinline|MethodVal|}, o, d) \\
         &:= (stripRecv(maybeUnowned(A_{idx})), o', d', s') \\
         \text{where } & stripRecv(a (R) \textbf{ func } (P_0, \ldots, P_n) (R_0, \ldots, R_r)) \\
            &:= a \textbf{ func } (P_0, \ldots, P_n) (R_0, \ldots, R_r) \\
            & maybeUnowned(In, R) \\
            &:= \begin{cases}
                In & \text{if } o \in base(R) \\
                ctb(In, base(In) \setminus \{o\}) & \text{if } o \not\in base(R)
            \end{cases} \\
             (s', o', d') &:= moc(s, In, R, o, d)
\end{align*}

Method values and method expressions also exist on named types, but named types do not have an equivalent permission yet, and therefore, they are not implemented for these types. Introducing a permission equivalent to named types would have caused significant changes in the abstract interpreter and it was too late for that. An alternative would have been to associate method sets from named types with the permissions for their underlying types (for example, a pointer to struct). It is unclear how they would have to be handled on assignments, though. Can they be just ignored? More work is needed here. TODO


#### Composite Literals

A composite literal `T {E_0, ..., E_n}` is fairly similar to a function calls. All values are moved or copied to their relevant position in the generated struct. However, dependencies are not released. There naturally is no owner, since the result is not part of a value reached from a variable.

There is one complication: The individual expressions may also be key-value expressions. Let's just assume that we have two functions $index(E_i)$ returning the index in the generated struct (which is $i$ if it is not a key-value expression, and $value(E_i)$ representing the permission of the value of the expression (which is just $E_i$ if it is not a key-value expression). These functions can be trivially implemented by looking at the type information provided by the `go/types` package.

Then it follows that, if:
\begin{align*}
    \langle T, s \rangle          &\leadsto (\overbrace{a \textbf{ struct } \{ A_0, \ldots, A_k \}}^{= P}, o_{-1}, d_{-1}, s_{-1})  \qquad k \ge n\\
    \langle value(E_i), s'_{i-1} \rangle &\leadsto (P_i, o_i, d_i, s_i)
\end{align*}
that:
\begin{align*}
    \langle T \{ E_0, \ldots, E_n\}, s \rangle &\leadsto \left(P, NoOwner, \bigcup\limits_{i=0}^{i \le n} d'_i \cup \{o'_i\}, s'_{n} \right) \\
        \text{where } (s'_{-1}, o'_{-1} d'_{-1}) &= (s_{-1}[= d_{-1} \cup \{o_{-1}\}], NoOwner, \emptyset) \\
                      (s'_i, o'_i, d'_i) &= moc(s_i, P_i, A_{index(E_i)}, o_i, d_i)
\end{align*}

Or in short:

1. Evaluate the type
2. Evaluate each argument, and move it to the corresponding position in the struct
3. Return the type's permission, with dependencies collected from the parameters.

An alternative approach that was considered is to construct the struct permission simply using the arguments, without the type info - just evaluate each argument and make a list of their permissions, essentially. This fails in two ways however: A composite literal may be incomplete, so the struct would have less fields then expected, and would thus not be assignable to the permission for the real type. Also, key-value expressions could not be handled without type info: It would not be clear where to put the permissions in the struct permission.

#### Function literals

Function literals will be handled in the statement section, since they require a lot of knowledge about statements.


## Statements
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
