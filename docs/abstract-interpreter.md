# Static analysis of Go programs
Based on the operations described in the previous section, a static analyser can be written that ensures that the rules of linearity are respected. This static analysis can be done in the form of an abstract interpreter; that is, an interpreter that does not operate on concrete values, but abstract values and tries to interpret all possible paths through a program.

We will introduce a store mapping variables to permissions with some operations, an expression evaluator $\leadsto$, and a statement evaluator $\rightarrow$. There will also be several
important helper functions like $moc$ which moves or copies a value, depending on which action is applicable, and `defineOrAssign` which takes care of defining and assigning values. Most
of the chapter will be in the form of a operational semantics, though some more complex cases will be discussed in code form for better readability.

In the `github.com/julian-klode/lingolang` reference implementation, the permissions and operations are provided in thea package called `capabilities` for historic reasons. A better name might have been `interpreter`.

## The store
The store is an essential part of the interpreter. It maps variable names $\in V$ to: \label{sec:store}

1. An effective permission
2. A maximum permission
3. A use count

The maximum permission restricts the effective permission. It can be used to prevent re-assignment of variables by revoking the write permission there. The uses count will be used later when evaluating function literals to see which values have been captured in the closure of the literal.

The store has several operations:

1. `GetEffective`, written $S[v]$, returns the effective permission of $v$ in $S$.
1. `GetMaximum`, written $S[\overline{v}]$, returns the maximum permission of $v$ in $S$.
1. `Define`, written $S[v := p]$, is a new store where a new $v$ is defined if none is in the current block, otherwise, it's the same as $S[v = p]$.
1. `SetEffective`, written $S[v = p]$, is a new store where $v$'s effective permission is set to $merge_{\cap}(S[\overline{v}], p)$
1. `SetMaximum`, written $S[v \overset{\wedge}{=} p]$, is a new store where $v$'s maximum permission is set to $p$. It also performs $S[v = merge_{\cap}(p, S[\overline{v}])]$ to ensure that the effective permission is weaker than the maximum permission.
1. `Release`, written $S[=D]$, where $D$ is a set of tuples $V \times {\cal P}$ is the same as setting the effective permissions of all $(v, p) \in D$ in S. We call that _releasing_ D, because $D$ will be a set of dependencies we borrowed from the store.
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
The interpreter's function $\leadsto : Expr \times Store \to Permission \times (Variable, Permission) \times \text{set of } (Variable, Permission) \times Store$ (also called `VisitExpr` in the code) visits an expression in a store, yielding a new permission, an owner, a set of variables borrowed by the expression, and a new store. It takes care of abstractly interpreting the expression and checking the permissions.

The types `Borrowed` and `Owner` are pairs of a variable name and a permission. The owner of an expression is the variable of which the object the expression evaluates to is a part of; for example, the owner of `&array[1]` is `array`. Dependencies represented by `Borrowed` are other values that have been borrowed. For example, in the composite literal `T {a, b}`, the variables `a` and `b` could be dependencies. There is a special `NoOwner` value of type `Owner` that represents that no owner exists for a particular expression.

The `Owner` vs `Borrowed` distinction is especially important with deferred function calls and the go statement. We will later see that the owner is the function (which may be a closure with a bound receiver), while any owners and dependencies of the arguments are forgotten.

Since the code is a bit long too read, it makes sense to provide a short, and hopefully more readable abstraction of it. The function `VisitExpr` essentially becomes the relation .

There also is a sister function, `VisitExprOwnerToDeps` which does not return a owner, but instead inserts the owner into the list of dependencies. This is helpful in places where the owner is not interesting (it's not used in the formal notation, but will be seen in some code excerpts later).

In the following, we will look at the individual expressions and check how they evaluate. The rules are written similar to typing rules in "Types and Programming Languages" by Benjamin C. Pierce [@tapl].


#### Identifier: `id`

There are three cases of identifies:

1. `nil` evaluates to the `nil` permission, it was created just for `nil` literals, since nil literals can be assigned to any nilable value (\fref{sec:ass-nil}).
2. `true` and `false` evaluate to the `om` permission, since they are just primitive values. They could just as well evaluate to any other (readable) base permission, but `om` is the strongest one, so to speak.
3. Any other identifier $id$ evaluates to the effective permission $e$ in the store. The effective permission in the store is replaced with an unusable one (converted to `n`), and the owner becomes $(id, e)$. The owner can later be released when
   the variable is no longer needed.

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
The star expression dereferences a pointer. Therefore we must evaluate the expression `E` and then dereference
the permission it returns, that is, return the target permission, for example:
```go
// E has permission om * l
*E  // permission l
```

\begin{align*}
    \frac{
        \langle E, s \rangle \leadsto (a * A, o, d, s') \text{ for some } a \subset {\cal R}, A \in {\cal P}}
    {
        \langle *E, s \rangle \leadsto (A, o, d, s')} && \text{(P-Star)
    }
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

Examples:

```go
5 + 5       // om, no owner
a + b       // om, no owner
```


#### Index expression: `A[B]`
The index operator indexes an array, a slice, or a map (by the key type of the map). It can appear on the left-hand side of an assignment expression,
and it is also addressable (except for maps): It's legal to take it's address with the `&` operator. Having it appear on the left-hand side means that
a map expression must move or copy the key into the map - we are storing a new value after all.
```go
someMap[someKey] = someValue        // someKey is either moved or copied into the map, as is someValue
```

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

In order to handle indexing maps, we need to take care of the left-hand-side situation mentioned above: The key must be copied or moved into the map. Since
we do not know whether the expression is on a left-hand or right-hand side, we must be conservative and assume it is on the left-hand side.

We can define a helper function, called `moveOrCopy` (listing \ref{moveOrCopy}), or short $moc$ (cases checked in order, top to bottom). $moc$ tries to
copy first, and then falls back to moving it or (in case of assigning an object of mutable linear permission to a non-linear one), making it immutable and then copying the value.

\begin{align*}
    moc(st, F, T, o, d) := \begin{cases}
        (st[= d \cup \{o\}], NoOwner, \emptyset) & \text{ if } ass_{cop}(F, T) \\
        \bot & \text{ if not } ass_{mov}(F, T) \\
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
    // If the value can be copied into the caller, we do not need to borrow it
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

With $moc$ defined, we are able to define $\leadsto$ for indexing maps. After evaluating the map and the key, we use $moc$
to copy or move the key into the map, and the permission for values of the map is returned (and the owner is the map).
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

The first unary expression to discuss is `&E`, the _address-of_ operator. Taking the address of `E` constructs a pointer to it,
currently simply by wrapping it in an `om *`.
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
is to be retrieved. The owner and the dependencies of the channel are essentially irrelevant: The value received from
the channel is not owned by the channel, but its owned by whatever is receiving it; the owner and dependencies can
thus be released immediately after evaluating the expression.

\begin{align*}
    \frac{
        \langle E, s \rangle \leadsto (p_e \textbf{ chan } P_e, o_e, d_e, s_e)
    } {
        \langle \leftarrow E, s \rangle \leadsto (P_e, NoOwner, \emptyset, s_e[= d_e \cup \{o_e\}])
    } && \text{(P-Recv)}
\end{align*}


Finally we have the "boring" case of other unary operators, like plus and minus. These are just working on primitive values, so
we can just return a new primitive owned mutable permission `om` and have no owner, since these construct new primitive values.
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
\end{align*}\label{sec:functioncall}

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
\intertext{For method values, we reuse the $moc$ function defined earlier to move or copy the lhs into the receiver. If we are binding an unowned receiver, the bound method value will be unowned too, to ensure we do not store an unowned value in an owned function value, as they have different lifetimes.}
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
The second option is applicable, with the change that instead of returning one value we return multiple ones. Each statement visitor returns pairs of

1. a new store, with the changes the statement made
2. an indicator of how the block was left (in this implementation, it is either nil or a pointer to the `ReturnStmt` or `BranchStmt` (`goto`, `break`, `continue`, `falltrough`))

Most statements return just one such pair, but if control flow is involved, there might be multiple, representing the individual paths.

Evaluating statements also needs access to the current function's permission. As such, we evaluate triplets of statement, store, and a function permission. The following sections refer to the statement evaluation function as

$$\rightarrow : Stmt \times Store \times \underbrace{FuncPermission}_\text{active function} \to \text{ set of } (\underbrace{Store}_\text{resulting store} \times \underbrace{(Stmt \cup \{nil\})}_{\text{jump/return, if any}}).$$

Some parts (assignments, blocks, and loops) are not easily to define in such a formal notation. They are explained as Go functions in literate programming style.

#### Simple statements
An expression statement simply evaluates the expressions, releases owners and dependencies afterwards, and returns the new store yielded by the expression.
\begin{align*}
    \frac{
        \langle E, s \rangle \leadsto (P, o, d, s')
    } {
        \langle ExprStmt(E), s, f \rangle \rightarrow \{(s'[= d \cup \{o\}], nil)\}
    }   && \text{(P-ExprStmt)}
\intertext{An increase/decrease statement like \lstinline|E++| needs read and write permissions for the expression \lstinline|E|. It evaluates \lstinline|E|, then releases the owner and dependencies.}
    \frac{
        \langle E, s \rangle \leadsto (P, o, d, s') \qquad r, w \in base(E)
    } {
        \langle E++, s, f \rangle, \langle E--, s, f \rangle \rightarrow \{(s'[= d \cup \{o\}], nil)\}
    } && \text{(P-IncDecStmt)}
\intertext{A labeled statement \lstinline|x: S| for a statement \lstinline|S| just evaluates \lstinline|S|.}
    \frac{
        \langle S, s \rangle \rightarrow X
    } {
        \langle x: S, s, f \rangle \rightarrow X   \quad    \text{for all label names } x
    } && \text{(P-LabeledStmt)}
\intertext{An empty statement does nothing.}
    \langle , s, f \rangle \rightarrow \{(s, nil)\} && \text{(P-EmptyStmt)}
\end{align*}

#### Assignments and declarations
There are essentially two forms of assignment and declarations:

1. Assign statements: `a := b` and `a = b` (the former defines the variable if not defined in the current block)
2. Declaration statement: `var a = b`, `var a` (the latter creates zero values)

Both of them share most of the implementation in the form of two functions: `defineOrAssign` which handles a single LHS expression[^lhs-expression] and a single RHS permission, and `defineOrAssignMany` which takes care of defining or assigning multiple (or zero) RHS to one or more LHS values.

The function `defineOrAssign` is the core function responsible for evaluating definitions and assignment.
The function starts by checking that the left-hand side is an identifier and defining it as necessary (or, if the identifier is `_`, by returning). Afterwards it evaluates the left-hand side (which is now defined), and then performs a move-or-copy from the right permission to the left.
```go
func (i *Interpreter) defineOrAssign(st Store, stmt ast.Stmt, lhs ast.Expr, rhs permission.Permission, owner Owner, deps []Borrowed, isDefine bool, allowUnowned bool) (Store, Owner, []Borrowed) {
	var err error

    <<define or assign lhs>>

	// Ensure we can do the assignment. If the left-hand side is an identifier, this should always be
	// true - it's either Defined to the same value, or set to something less than it in the previous block.

	perm, _, _ := i.visitExprOwnerToDeps(st, lhs) // We just need to know permission, do not care about borrowing.
	if !allowUnowned {
		i.Assert(lhs, perm, permission.Owned) // Make sure it's owned, so we do not move unowned to it.
	}

	// Input deps are nil, so we can ignore them here.
	st, owner, deps, err = i.moveOrCopy(lhs, st, rhs, perm, owner, deps)
	if err != nil {
		i.Error(lhs, "Could not assign or define: %s", err)
	}

	log.Println("Assigned", lhs, "in", st)

	return st, owner, deps
}
```

[^lhs-expression]: While the abstract syntax tree just has expressions in general on the left-hand side of an assignment, only certain expressions are allowed in practice (variables, indexing, field access, pointer dereference, wildcard).

The code handling defining or assigning the permission of the lhs first handles the underscore case, and then handles the define or assign:
```go
<<define or assign lhs>>=
// Define or set the effective permission of the left-hand side to the right-hand side. In the latter case,
// the effective permission will be restricted by the specified maximum (initial) permission.
if ident, ok := lhs.(*ast.Ident); ok {
    <<handle _>>

    if isDefine {
        <<define value>>
    } else {
        <<set value>>
    }

    if err != nil {
        i.Error(lhs, "Could not assign or define: %s", err)
    }
} else if isDefine {
    i.Error(lhs, "Cannot define: Left-hand side is not an identifier")
}
```

Handling the underscore case is simple: An assignment to `_` would be equivalent to just executing the expression on the right-hand side, that is, we can release owner and dependencies - it's not going to be moved anywhere.
```go
<<handle _>>=
if ident.Name == "_" {
    i.Release(stmt, st, []Borrowed{Borrowed(owner)})
    i.Release(stmt, st, deps)
    return st, NoOwner, nil
}
```

If we are evaluating a define statement, an annotation may be present. In that case, the actual RHS permission is converted (recall  \fref{sec:ctb} and \fref{sec:convert}) to the annotated permission to create the LHS permission. Otherwise, the LHS permission is the RHS permission (possibly subject to limits if the variable is already defined and we are in fact reassigning).
```go
<<define value>>=
log.Println("Defining", ident.Name)
if ann, ok := i.AnnotatedPermissions[ident]; ok {
    if ann, err = permission.ConvertTo(rhs, ann); err != nil {
        st, err = st.Define(ident.Name, ann)
    }
} else {
    st, err = st.Define(ident.Name, rhs)
}
```

Otherwise, when assigning rather than defining, we just set the effective permission to either the maximum permission the variable can hold (if it can be copied to it) or to the RHS permission (limited by the maximum permission, see \fref{sec:store}). The maximum case allows us to add permissions when copying, which is a property copying was designed to have (see \fref{sec:assign}).
```go
<<set value>>=
if permission.CopyableTo(rhs, st.GetMaximum(ident.Name)) {
    st, err = st.SetEffective(ident.Name, st.GetMaximum(ident.Name))
} else {
    st, err = st.SetEffective(ident.Name, rhs)
}
```

The function `defineOrAssign` allows us to do 1:1 definitions and assignments, that is, cases where we have one value and one variable. There are more cases though: Tuples representing multiple return values can be unpacked, and there might simply be no values at all when defining (which would create zero values for their respective type, like `null` for a pointer type).

The function `defineOrAssignMany` takes care of that. It takes multiple LHS expressions and multiple RHS expressions, and then unpacks the RHS expressions into a list of permissions and a list of dependencies.
If there are less RHS expressions than LHS expressions, the missing ones are substituted with permissions for zero values, so we can handle cases like `var x int` where no values are specified.
Finally, `defineOrAssign` is called for each pair of LHS expression and `RHS` permissions.

```go
func (i *Interpreter) defineOrAssignMany(st Store, stmt ast.Stmt, lhsExprs []ast.Expr, rhsExprs []ast.Expr, isDefine bool, allowUnowned bool) Store {
	var deps []Borrowed
	var rhs []permission.Permission
	if len(rhsExprs) == 1 && len(lhsExprs) > 1 {
		<<unpack tuple>>
	} else {
        <<unpack multiple>>
	}

    <<fill rhs up with zero values>>
	if len(rhs) != len(lhsExprs) {
		i.Error(stmt, "Expected same number of arguments on both sides of assignment (or one function call on the right): Got rhs=%d lhs=%d", len(rhs), len(lhsExprs))
	}

	for j, lhs := range lhsExprs {
		st, _, _ = i.defineOrAssign(st, stmt, lhs, rhs[j], NoOwner, nil, isDefine, allowUnowned)
	}

	st = i.Release(stmt, st, deps)

	return st
}
```

Unpacking a tuple is simple: We evaluate the single RHS expression, and then take the tuple elements as the RHS dependencies. Since a tuple is the result of a function call (you might have noticed that is the only evaluation that can cause a tuple permission to be created) we do not really need to care about owners or dependencies, so we just collect them for releasing later.
```go
<<unpack tuple>>=
// These really can't have owners.
rhs0, rdeps, store := i.visitExprOwnerToDeps(st, rhsExprs[0])
st = store
tuple, ok := rhs0.(*permission.TuplePermission)
if !ok {
    i.Error(stmt, "Left side of assignment has more than one element but right hand only one, expected it to be a tuple")
}
deps = append(deps, rdeps...)
rhs = tuple.Elements
```

When unpacking multiple RHS expressions, the idea is that all RHS values are asssigned to a temporary variable, and the temporary variables are assigned to the final ones later. The reason for that is that an operation -- like `a, b = b, a` (or `(a,b) = (b,a)` in languages with more syntax) -- swaps the variables, rather than making both be `b` afterwards.
```go
<<unpack multiple>>=
for _, expr := range rhsExprs {
    log.Printf("Visiting expr %#v in store %v", expr, st)
    perm, ownerThis, depsThis, store := i.VisitExpr(st, expr)
    log.Printf("Visited expr %#v in store %v", expr, st)
    st = store
    rhs = append(rhs, perm)

    // Screw this. This is basically creating a temporary copy or (non-temporary, really) move of the values, so we
    // can have stuff like a, b = b, a without it messing up.
    store, ownerThis, depsThis, err := i.moveOrCopy(expr, st, perm, perm, ownerThis, depsThis)
    st = store

    if err != nil {
        i.Error(expr, "Could not move value: %s", err)
    }

    deps = append(deps, Borrowed(ownerThis))
    deps = append(deps, depsThis...)
}
```

Finally, we fill up missing elements on the right with default permissions for their type - these will be zero values, as mentioned before.
```go
<<fill rhs up with zero values>>=
// Fill up the RHS with zero values if it has less elements than the LHS. Used for var x, y int; for example.
for elem := len(rhs); elem < len(lhsExprs); elem++ {
    var perm permission.Permission

    perm = i.typeMapper.NewFromType(i.typesInfo.TypeOf(lhsExprs[elem]))
    perm = permission.ConvertToBase(perm, perm.GetBasePermission()|permission.Owned)

    rhs = append(rhs, perm)

}
```

Now, as for the actual statements:

Assign and define (`=` and `:=`) are equivalent to `defineOrAssignMany`. It sets the `isDefine` paramater to true if the token of the statement was `:=`, otherwise it is false. Unowned values are not allowed.

```go
func (i *Interpreter) visitAssignStmt(st Store, stmt *ast.AssignStmt) []StmtExit {
	return []StmtExit{{i.defineOrAssignMany(st, stmt, stmt.Lhs, stmt.Rhs, stmt.Tok == token.DEFINE, false), nil}}
}
```

Declaration statements of the form `var x, y = a, b` or just `var x, y` are more complicated. The declaration has a list of specifications,and each specification has a list of names and values. We therefore need to call `defineOrAssignMany` once per specification (there could also be other specifications). Unowned values are not allowed, and `isDefine` is always true.
```go
func (i *Interpreter) visitDeclStmt(st Store, stmt *ast.DeclStmt) []StmtExit {
    <<boring setup code>>

	for _, spec := range decl.Specs {
		switch spec := spec.(type) {
        case *ast.ValueSpec:
            names := <<convert spec.Names into a slice of expressions>>
			st = i.defineOrAssignMany(st, stmt, names, spec.Values, true, false)
		default:
			continue
		}
	}
	return []StmtExit{{st, nil}}
}
```

#### Special function calls: Go / Deferred
The go and defered statements are fairly simple. They consist of a call expression which is evaluated in go/defer
mode. If you recall the definition of function calls from \fref{sec:functioncall}, you will remember that the
dependencies of a deferred call are the dependencies of the function.

Now, there is one difference between `go` and `defer` calls: While neither release their owner, a `defer` call
releases unowned dependencies. As we have established before in \fref{sec:functioncall}, the dependencies of
a `go` or `defer` call are the dependencies of the function.

This has one important effect: If the function called is a function literal, the dependencies will correspond
to the captured variables. We can thus continue to work with unowned parameters in the function containing the
`defer` statement, even if the deferred call's literal captures the parameter.
```go
// @perm func(m * m)
function fooDefer(bar *int) {
    defer function() {
        ... bar ... // captured
    }
    ... bar ... // still works
}
// @perm func(m * m)
function fooGo(bar *int) {
    go function() {
        ... bar ... // captured
    }
    ... bar ... // does not work: bar is unusable after the go statement
}
```
This is legal because any unowned parameter will still be usable when the function exits.

\begin{align*}
\frac{
    \langle E(A_0, \ldots, A_n), s \rangle \leadsto (P, o, d, s')
} {
    \langle \textbf{go } E(A_0, \ldots, A_n), s, f \rangle \rightarrow \{(s', nil)\}
}   && \text{(P-GoStmt)} \\
\frac{
    \langle E(A_0, \ldots, A_n), s \rangle \leadsto (P, o, d, s')
} {
    \langle \textbf{defer } E(A_0, \ldots, A_n), s, f \rangle \rightarrow \{(s'[= \{(v, p) \in d | o \not\in p\}], nil)\}
}   && \text{(P-DeferStmt)}
\end{align*}

#### Branch and return statements
A branch statement is one of the statements `break`, `continue`, `goto`, or `fallthrough`. Evaluating it has no effect on the
store, but the statement is the statement in the result pair of $\rightarrow$.
\begin{align*}
    \langle B, s, f \rangle \rightarrow \{(s, B)\} \text{ for all branch statements } B    && \text{(P-BranchStmt)}
\end{align*}
A return statement is more complex: It might have 0 or more arguments, corresponding to the number of return values of the current
function. Each argument is moved-or-copied into the return "position" before the next argument is evaluated. The result is one pair:
the store after evaluating the last argument, and the return statement.

\begin{align*}
\frac{
    \langle A_i, s_{i-1} \rangle \leadsto (P_i, o'_i, d'_i, s'_i)   \quad s_i, o_i, d_i = moc(s_i', P_i, receivers(f)_i, o'_i, d'_i)
} {
    \langle \underbrace{\textbf{return } A_0, \ldots, A_n}_{=B}, s_{-1}, f \rangle \rightarrow \{(s_{n}, B)\}
}   && \text{(P-ReturnStmt)}
\end{align*}


#### Block statements
A block is generally evaluated like this: First begin a new block scope, then evaluate the statement list, and finally remove the block scope from all exits:

\begin{align*}
\cfrac{
    (s[+], [S_0, \ldots, S_n], false) \xrightarrow{visitStmtList} exits
}{
    \langle \{S_0; \ldots; S_n\}, s, f \rangle \rightarrow \{(s[-], b) | (s, b) \in exits\}
} && \text{(P-BlockStmt)}
\end{align*}

Now, interpreting the list of statements is difficult: The presence of `goto` statements can cause us to jump within the block, and `break`, `continue`, or `return` statements can jump out of the block. Instead of just iterating the statements in the block, we maintain a work stack and a seen set, each containing pairs of store and position (we call that the block manager. The block manager uses the seen set to determine if it should add work to the work stack, it will only add work it has not seen yet). As mentioned before, formal definition of `visitStmtList` will not be provided, only a literate programming style one.

The execution of blocks, switches, and loops all follow the same approach: First some setup is performed, and then, while there is work, the statement refered to by the work item is executed (after some pre and before some post code). One example is `visitStmtList` which handles non-loop lists of statements (block bodies, list of case clauses in switches and select statements):
```go
func (i *Interpreter) visitStmtList(initStore Store, stmts []ast.Stmt, isASwitch bool) []StmtExit {
    var bm blockManager // A helper type providing helper stuff

    <<setup>>
    for bm.hasWork() {
        item := bm.nextWork()   // pop a work item from the stack
        <<pre>>
        exits := i.visitStmt(work.Store, <<stmt>>) // interpret the statement
        <<post>>
    }

    return bm.exits
}
```
The `<<post>>` part usually splits the list of statement exits into exits of the block we are evaluating and further work, which gets added to the work stack (if not seen already). This means that it will evaluate the block for all possible inputs: For example, if we are evaluating a block with a conditional jump back to the beginning, it will follow the jump back and evaluate the block again for the state of the store before the jump. Since only unseen entries are added, the code will also terminate eventually.

The `<<setup>>` part just exits with the initial store if it there are no statements to execute. If it is a switch/select statement, the entry point is added as an exit, and each statement (which corrrespond to case clauses) is added to the work state. If it is just a normal block, we enter statement 0 in the block. We also collect all labels, building a map of strings to indices in the statement list.
```go
if len(stmts) == 0 {
    return []StmtExit{{initStore, nil}}
} else if isASwitch {
    bm.addExit(StmtExit{initStore, nil})
    for i := range stmts {
        bm.addWork(work{initStore, i})
    }
} else {
    bm.addWork(work{initStore, 0})
}
labels := collectLabels(stmts)
```

The `<<pre>>` part is empty.

The `<<stmt>>` is `stmts[item.int]`, that is, the statement in the list of statements at the index stored in the work item.

The `<<post>>` part is exciting. Each exit of the evaluated statement (inner exit) either becomes additional work or an exit of the current block. It depends on what the branch statement is:
```
    switch branch := exit.branch.(type) {
        <<case 1>>
        <<case 2>>
        <<case 3>>
    }
```
The cases are:

1.
    If the inner exit contains no branch or return statement, the next statement is added as work if we are not in a switch/select and there is at least one more statement in the list. Otherwise, the inner exit becomes an exit.
    ```go
    case nil:
        if len(stmts) > item.int+1 && !isASwitch {
                bm.addWork(work{exit.Store, item.int + 1})
            } else {
                bm.addExit(StmtExit{exit.Store, nil})
            }
    ```
2.
    If the inner exit contains a return statement, it becomes an exit of the current block:
    ```go
    case *ast.ReturnStmt:
        bm.addExit(exit) // Always exits the block
    ```
3.
    If the inner exit contains a branch statement:

    1. We are in a switch, it is a break, and the break applies to the current branch: Add the inner exit (without the branch statement) as an exit.
    1. We are in a switch, it is a fallthrough: The next case clause is added as work.
    1. It is a goto: If the target is in the current block, add it as work; otherwise, add the inner exit as an exit
    1. Otherwise, add the inner exit as an exit.

    ```go
    case *ast.BranchStmt:
        branchingThis := (branch.Label == nil || branch.Label.Name == "" /* | TODO current label */)
        switch {
        case isASwitch && branch.Tok == token.BREAK && branchingThis:
            bm.addExit(StmtExit{exit.Store, nil})
        case isASwitch && branch.Tok == token.FALLTHROUGH:
            bm.addWork(work{exit.Store, item.int + 1})
        case branch.Tok == token.GOTO:
            if target, ok := labels[branch.Label.Name]; ok {
                bm.addWork(work{exit.Store, target})
            } else {
                bm.addExit(exit)
            }
        default:
            bm.addExit(exit)
        }
    ```

#### The `for` loop
The evaluation of loops, either `for` or `range` statements
```go
for x := 0; x < 5; x++ { ... }
for key, val := range something { ... }
```
works the same way as `visitStmtList` from blocks, just with different `<<setup>>`, `<<pre>>` and `<<post>>` phases.

For the `for` statement, the `<<setup>>` consists of beginning a new block, and then visiting the initializing statement (`x := 0` in the example above).

```go
	initStore = initStore.BeginBlock()

	// Evaluate the container specified on the right-hand side.
	for _, entry := range i.visitStmt(initStore, stmt.Init) {
		if entry.branch != nil {
			i.Error(stmt.Init, "Initializer exits uncleanly")
		}
		bm.addWork(work{entry.Store, 0})
    }
```

The `<<pre>>` part consists of evaluating the condition in the store of the iteration, releasing its owner and dependencies (`visitExprOwnerToDeps` simply merges owner into dependencies), and adding the state after condition evaluation as an exit.
```go
    // Check condition
    perm, deps, st := i.visitExprOwnerToDeps(st, stmt.Cond)
    i.Assert(stmt.Cond, perm, permission.Read)
    st = i.Release(stmt.Cond, st, deps)
    // There might be no more items, exit
    bm.addExit(StmtExit{st, nil})
```

The `<<stmt>>` that will be evaluated is always `stmt.Body`, the body of the for loop. The position stored in the work item is thus not used at all.

The `<<post>>` part consists of classifying exits of the loop body into more iterations and direct exits, running the post statement for each next iteration in its store, and adding the direct exits to the loop's list of exits, after ending the block.
```go
    nextIterations, exits := i.collectLoopExits(exits)
    for _, nextIter := range nextIterations {
        for _, nextExit := range i.visitStmt(nextIter.Store, stmt.Post) {
            if nextExit.branch != nil {
                i.Error(stmt.Init, "Post exits uncleanly")
            }
            bm.addWork(work{nextExit.Store, 0})
        }
    }
    i.endBlocks(exits)
    bm.addExit(exits...)
```

The `collectLoopExits` function splits the exits of a loop's body into further iterations and exits of a loop.
```go
func (i *Interpreter) collectLoopExits(exits []StmtExit) ([]work, []StmtExit) {
	var nextIterations []work
	var realExits []StmtExit

	for _, exit := range exits {
		switch branch := exit.branch.(type) {
            <<case 1>>
            <<case 2>>
            <<case 3>>
		}
	}

	return nextIterations, realExits
}
```
The cases are:

1. No branch statement: The exit is another iteration

    ```go
    case nil:
        nextIterations = append(nextIterations, work{exit.Store, 0})
    ```
2. A return statement: The exit is a real exit

    ```go
    case *ast.ReturnStmt:
        realExits = append(realExits, exit)
    ```

3. A branch statement. A break of the current loop is an exit, a continue of the current loop a next iteration, anything else an exit.

    ```go
    case *ast.BranchStmt:
        branchingThis := branch.Label == nil || branch.Label.Name == "" /* | TODO current label */
        switch {
        case branch.Tok == token.BREAK && branchingThis:
            realExits = append(realExits, StmtExit{exit.Store, nil})
        case branch.Tok == token.CONTINUE && branchingThis:
            nextIterations = append(nextIterations, work{exit.Store, 0})
        default:
            realExits = append(realExits, exit)
        }
    ```

#### The `range` loop
The `range` statement is extremely similar to the `for` statement. It shares the `collectLoopExits` function, but there are a few differences related to it assigning values from a container. Notably, the block begins new for each iteration, and is released for all exits in the statement.

The `<<setup>>` part adds an exit for not entering the range (which will not evaluate the right-hand side at all, which might be wrong). It then evaluates the right-hand side, and defers the release of its dependencies until the end of the block. Finally, the permissions for key and values are extraced from the RHS, and the initial work is added.
```go
bm.addExit(StmtExit{initStore, nil})

// Evaluate the container specified on the right-hand side.
perm, deps, initStore := i.visitExprOwnerToDeps(initStore, stmt.X)
defer func() {
    if canRelease {
        for j := range rangeExits {
            rangeExits[j].Store = i.Release(stmt, rangeExits[j].Store, deps)
        }
    }
}()
i.Assert(stmt.X, perm, permission.Read)

var rkey permission.Permission
var rval permission.Permission

<<rhs permissions>>

bm.addWork(work{initStore, 0})
```

where `<<rhs permissions>>` determines the permissions for the key and value of what is being iterated:
```go
switch perm := perm.(type) {
case *permission.ArrayPermission:
    rkey = permission.Mutable
    rval = perm.ElementPermission
case *permission.SlicePermission:
    rkey = permission.Mutable
    rval = perm.ElementPermission
case *permission.MapPermission:
    rkey = perm.KeyPermission
    rval = perm.ValuePermission
}
```

The ``<<pre>>`` part begins a new block, and then uses `defineOrAssign` to define or assign (depending on whether it is a `for k, v := range` or a `for k, v = range` loop) the variables representing keys and values. This also modifies a `canRelease` variable that is used in the function deferred in `<<setup>>` - basically, if the LHS are owned, they are bound indefinitely, and we cannot release the container we are iterating over later.

```go
st = st.BeginBlock()
if stmt.Key != nil {
    st, _, _ = i.defineOrAssign(st, stmt, stmt.Key, rkey, NoOwner, nil, stmt.Tok == token.DEFINE, stmt.Tok == token.DEFINE)
    if ident, ok := stmt.Key.(*ast.Ident); ok {
        log.Printf("Defined %s to %s", ident.Name, st.GetEffective(ident.Name))
        if ident.Name != "_" {
            canRelease = canRelease && (st.GetEffective(ident.Name).GetBasePermission()&permission.Owned == 0)
        }
    } else {
        canRelease = false
    }
}
if stmt.Value != nil {
    st, _, _ = i.defineOrAssign(st, stmt, stmt.Value, rval, NoOwner, nil, stmt.Tok == token.DEFINE, stmt.Tok == token.DEFINE)
    if ident, ok := stmt.Value.(*ast.Ident); ok {
        log.Printf("Defined %s to %s", ident.Name, st.GetEffective(ident.Name))
        if ident.Name != "_" {
            canRelease = canRelease && (st.GetEffective(ident.Name).GetBasePermission()&permission.Owned == 0)
        }
    } else {
        canRelease = false
    }
}
```

As with `for`, the `<<stmt>>` is just the loop's body: `stmt.Body`.

The `<<post>>` step is shorter than for for loops, as there are no post statements. We can immediately end the block we began in `<<pre>>` (meaning the key and value variables are gone again), collect the exits. One difference here is that every next iteration is also a valid exit, since we might have just "reached" the end of the container.

```go
i.endBlocks(exits)
nextIterations, exits := i.collectLoopExits(exits)
bm.addExit(exits...)
// Each next iteration is also possible work. This might generate duplicate exits, but we have
// to do it this way, as we might otherwise miss some exits
for _, iter := range nextIterations {
    bm.addExit(StmtExit{iter.Store, nil})
}
bm.addWork(nextIterations...)
```

#### If statements and switches

An if statement consists of an initializer, a condition, a body, and an else case:
```go
if foo := 5; foo > bar {
    then
} else ...      // the ... is a statement as well. An else is optional
```

An if statement is evaluated as follows:

1. Create a new block
1. Evaluate the initializing statement. It must produce one exit (the syntax does not allow more, this simplifies thing)
1. Evaluate the condition expression. It must be readable, and it's owner and dependencies are released.
1. Evaluate body and else part in the store that resulted from evaluating the condition.
1. Combine the exits of body and else to form the result, but remove the block we started at the beginning from their stores.


\begin{align*}
\cfrac{
    \langle S_0, s[+] \rangle \rightarrow \{(s_0, b_0)\} \qquad
    \langle E_1, s_0  \rangle \leadsto (P_1, o_1, d_1, s_1)    \quad r \in P_1
}{
    \cfrac{
        \quad \langle S_2, s_2, f \rangle \rightarrow exits_{then}
        \quad \langle S_3, s_2, f \rangle \rightarrow exits_{else}
    }{
        \langle \textbf{if } S_0; E_1 S_2 S_3, s, f \rangle \rightarrow \{(s[-], e) | (s, e) \in exits_{then} \cup exits_{else}\}
    }
} && \text{(P-IfStmt)}\\
\text{where }s_2 = s_1[= d_1 \cup \{o_1\}]
\end{align*}


A switch statement `switch INIT; TAG { BODY }`, where `BODY` is a list of case clauses, is evaluated like this:

1. Create a new scoping block
1. Evaluate the initializing statement.
1. For each possible exit: Evaluate tag in the exit's store and the body (with the cases being not entered, and entered for each case clause) in the store resulting from evaluating the tag.
1. For each exit of the body: Release the dependencies of the tag, and undo the block from the start.

\begin{align*}
\cfrac{
    \langle INIT, s[+] \rangle \rightarrow \{(s_0, b_0), \ldots, (s_n, b_n) \}
}{
    \cfrac{
        \langle TAG, s_i \rangle \leadsto (P_i, o_i, d_i, s'_i) \quad \forall 0 \le i \le n, r \in P_i
    }
    {
        \cfrac{
            (s'_i, BODY, true) \xrightarrow{visitStmtList} exits'_i
            exits_i := \{(s[= d_i \cup \{o_i\}][-], e) | (s, e) \in exits'_i \}
        }{
            \langle \textbf{switch } INIT; TAG \{ BODY \}, s, f \rangle \rightarrow \bigcup\limits_{0 \le i \le n} exits_i
        }
    }
} && \text{(P-SwitchStmt)}
\end{align*}

It might be more readable in code form, as shown in listing \ref{visitSwitchStmt}.

```{#visitSwitchStmt .go caption="Abstract interpreter for switch statements" float=t frame=tb}
func (i *Interpreter) visitSwitchStmt(st Store, stmt *ast.SwitchStmt) []StmtExit {
	var exits []StmtExit

	st = st.BeginBlock()
	for _, exit := range i.visitStmt(st, stmt.Init) {
		st := exit.Store
		perm, deps, st := i.visitExprOwnerToDeps(st, stmt.Tag)
		if stmt.Tag != nil {
			i.Assert(stmt.Tag, perm, permission.Read)
		}

		for _, exit := range i.visitStmtList(st, stmt.Body.List, true) {
			exit.Store = i.Release(stmt.Tag, exit.Store, deps)
			exits = append(exits, exit)
		}
	}

	for i := range exits {
		exits[i].Store = exits[i].Store.EndBlock()
	}
	return exits
}
```


A case clause has the form `case E_0, ..., E_n: BODY` for some expression `E` and some statement `BODY`. The expressions are evaluated from left to right and the resulting stores are merged. Then the body is executed in the resulting store. Alternatively, the body could be executed for each store, but executing it once should be faster than executing it $n+1$ times.

\begin{align*}
\cfrac{
    \langle E_i, s_{i-1} \rangle \leadsto (P, o'_i, d'_i, s'_i)
    \quad r \in P
    \quad s_i = s'_i[=d'_i \cup \{o'_i\}]
}{
    \cfrac{
        \left(\bigcap\limits_{0 \le i \le n} s_i, BODY, false\right) \xrightarrow{visitStmtList} exits
    }{
        \langle \textbf{case } E_0, \ldots, E_n: BODY, s, f \rangle \rightarrow exits
    }
} && \text{(P-CaseClause)}
\end{align*}



#### Channel statements

The easiest channel statement is the send statement. Given a writable channel, and an object, we move-or-copy the object permission into the channel element permission. Any dependency and owner left over by $moc$ are then released.

\begin{align*}
\cfrac{
    \langle E_0, s \rangle \leadsto (c \textbf{ chan } C, o_0, d_0, s_0)
}{
\cfrac{
    \quad \langle E_1, s \rangle \leadsto (V, o'_1, d'_1, s'_1)
    \quad s_1, o_1, d_1 := moc(s'_1, V, C, o'_1, d'_1)
} {
    \langle E_0 \text{ \lstinline|<-| } E_1, s, f \rangle \rightarrow \{(s_1[= d_1 \cup \{o_1\}], nil)\}
}
}    && \text{(P-SendStmt)}
\end{align*}

There is arguably a bug here. I can send to a unowned channel containing unowned values, and the dependencies would be released. A channel containing unowned values should not exist in the first place, though, but sadly, it is entirely possible to define one at the moment.


The `select` statement allows waiting for non-blocking send/receive availability on a set of channels. It is thus similar to (and probably named after) the `select(2)` system call from 4.2BSD and POSIX.1-2001. The resulting exits will be the initial store (for the not entered case), and those gained by jumping into each of the comm clauses in the body from the store (which is what `visitStmtList` does).

\begin{align*}
\cfrac{
    (s[+], BODY, true) \xrightarrow{visitStmtList} exits
} {
    \langle \textbf{select } \{ BODY \}, s, f \rangle \rightarrow \cup \{(s[-], e) for (s, e) \in exits\})\}
}
\end{align*}


A comm clause has the form `case STMT: BODY` for some statements `STMT` and `BODY`. First the statement is evaluated, and then the body is evaluated for each exit of the statement.

\begin{align*}
\cfrac{
    \langle STMT, s, f \rangle \rightarrow exits_0 \qquad
    exits := \bigcup\limits_{e \in exits_0} visitStmtList(store(e), BODY, false)
} {
    \langle \textbf{case } STMT: BODY, s, f \rangle \rightarrow exits
}
\end{align*}
