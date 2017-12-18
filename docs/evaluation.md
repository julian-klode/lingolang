# Evaluation
Having discussed the design and implementation, the time has come to evaluate the project according to our criteria from \fref{sec:criteria}, that is:

Completeness
: All syntactic constructs are tested

Correctness
: Only valid programs are allowed

Preciseness
: We do not reject useful programs because our rules are too broad.

Usability
: If there is a problem, is it understandable

Compatibility
: A compiling Lingo program behaves the same as a Go program with all Lingo annotations are removed

Coverage
: The implementation should be well-tested with unit tests

## Completeness
The implementation is unfortunately not complete.

We only have support for checking expressions and statements (including function literals), but we do not have support for declaring global variables, functions, or importing other packages.
Adding support for global declarations requires handling global state. While we could just create a "package" permission holding permissions for all objects declared in it, and make the current package a store, it is unclear how any mutable global state should interact with functions.
There does not seem to be a safe solution for global mutable state: If two functions want to access the same global, mutable, variable, how do we handle that? Marking each function's closure as mutable is not enough: Their closure is the same. Two solutions might be possible: Identify groups of functions with the same closure and only allow one of them to be used at a time, or just forbid two functions from using the same global mutable variable; essentially making global mutable state function-specific.

The support for expressions and statements is slightly incomplete as well:
Type assertion, conversion between named types and interfaces, and type switches are missing. These require gathering a set of methods for a given named type. But
we do not have an equivalent to named types in permissions, and adding it does not seem feasible anymore, as it would require substantial changes in the
interpreter. An alternative would be to simply attach a list of methods to the unnamed permissions, and when converting, take the left set of methods. When a
conversion to an interface is required, we could just build an interface out of these methods, and check if that interface is assignable to the interface.

Also as mentioned in \fref{sec:index-string}, strings can be indexed, but we do not support it - strings are just base permissions, but should be their
own permission kind. This seems easily solvable however.

## Correctness
The implementation is shallow: When looking at the requirements for operands, it only looks at the base permission of the operands. So, for example, if an expression requires an
operand to be readable, we only check whether its base permission contains the $r$ bit. That should be fine so far, as we ensure consistency at some point in the program
by converting each permission to its own base permission, and thus a readable object cannot have unreadable members. But it falls short if we actually want to allow
it. There might be an option: Instead of checking if $r \in base(A)$ we can check if $ass_{mov}(A, ctb(A, r))$, that is, we create a new structure where all base
permissions are replaced with $\{r\}$ and then we can check if the value is movable to it, which recursively checks that each base permission is movable to $\{r\}$.

As shown in \fref{sec:address-of}, taking the address of a pointer does not modify the store, except for the evaluation of the pointer, of course. As mentioned there,
this is wrong: Taking the address of a pointer should (usually) consume the maximum permission of the owner. Let's look at an example: Let's say we have a variable `a`
that is `om * om`, and we take a pointer to it: `p = &a` - p is now `om * om * om`. But `a` can be reassigned: `a = a new pointer` and regain its effective permission,
meaning we now have two usable references to `a`. This happens because assignment checks against the maximum  permission (\fref{sec:assign-set-value-max}), and thus, taking the maximum permission away
instead of the effective permission would solve the issue.


## Preciseness
The implementation is coarse: If I borrow anything uniquely referred to by a variable (for example, when freezing it, as in \fref{sec:coarse-part-move}), then the entire variable is marked as unusable, rather than just the part
of the permission that was borrowed. A solution to this problem would be to collect a path from a variable to an object (like, select field x, select field y), when
evaluating an expression - then we could create a new permission where the borrowed part is replaced by an unusable permission. But this then leads to the shallowness
problem mentioned above.

The implementation is ambiguous: Permissions for types and values are stored in the same namespace. This is not a real problem, though, as Go ensures that we cannot
use type in value contexts and vice versa.

As seen in \fref{sec:visitIdent}, moving a value out of the store also happens for non-linear values. This is overly broad: If a value is not linear, it should be
copyable. Therefore it might make sense to leave that out.

We have seen in \fref{sec:index-map} that indexing a map also moves the key into the map (its dependencies are forgotten) if the map has unowned keys (the map
is unowned). This might break some code that should actually work.

In \fref{sec:slice} we saw that slicing only releases the permissions of the slice arguments after they have all been evaluated, which is overly coarse. It prevents
a call like `a[x:x]` for some linear `x`. Instead, slicing should release each argument's dependencies as soon as it has been evaluated - the result will be an
integer and its dependencies thus do not matter.

In \fref{sec:two-moves} we saw that there should actually be two types of move operations, and \fref{sec:two-moves-func} showed that, because we only have the
one definition that requires the source to be readable, functions with parameters of permission `n` can not be assigned elsewhere. Also, \fref{sec:two-moves-ptr}
showed that we have similar problems for pointers: A `ol * om` cannot be passed to function expecting an `om * om` pointer, although this would be harmless - both
are linear pointers, and the target is the same in both cases.

## Usability
Usability is bad, really bad. The structured permissions are incredibly powerful, but this power comes at a price: Error messages are not readable. There are two
reasons for that: First of all, there can be a lot of nesting and a lot of wide permissions (like structs with a lot of elements), leading to long and hard to
read permissions. Secondly, there can be cycles, and the cycles do not always appear at the same stage: For example a permission "A = om * A" could be stored
as $A = om * A$ or it could be stored as $A' = om * om * A$ - they are still compatible. This makes it hard to figure out the actual error when two permissions
are incompatible.


## Compatibility
On the semantic front, if a Lingo program compiles it will behave exactly like a Go program.
This is a side effect of going with comment-based annotations and a simple checker that does not generate a modified program.

On the actual use part, while interfacing with legacy code could be made possible simply by using $n$ permissions for parameters, and $om$ or $orw$
for return values, this seems a bit unsafe. Permissions should be annotated with an unsafe bit, and conversions between unsafe permissions and safe
permissions should produce a warning. There should also be a way to annotate that a certain conversion is safe.

## Code coverage / Unit testing
The implementation, since the beginning, has been subject to rigorous unit testing with continuous integration on [travis-ci.org](https://travis-ci.org/julian-klode/lingolang) and code coverage reports on [codecov.io](https://codecov.io/gh/julian-klode/lingolang).

![code coverage chart](coverage-chart.png)

The code coverage chart shows that it started out slightly below 100% line coverage, eventually reaching 100%, only to drop again - when the interpreter started coming together - there are quite a few places in the interpreter code that are unreachable conditions and would require constructing a lot of illegal AST objects to test.

To be precise, the coverage for the permission package itself stayed at 100%, and the store also has 100% coverage, the interpreter, however only has about 92% coverage.

Unfortunately, the Go tools only provide line coverage, and not branch or path coverage. This is somewhat problematic: For example, if we have an `if` statement without an `else` part, we can test if the if has been taken, but we usually cannot check whether it has not been taken: The if statement would eventually fall out of its block and back into the parent block, and thus all lines are executed.

Like the code, this section is split in two parts: First, we will discuss how the permissions package was tested - the package includes the parser and the permission op
erations discussed in \fref[plain]{chap:permissions}. Afterwards, we will discuss testing the implementations of the store and abstract interpreter described in \fref[plain]{chap:interpreter}.

\clearpage

### Testing the permissions package
The permissions package contains the parser and the rules for permissions described in the section _[Permissions for Go](#permissions-for-go)_. Coverage is 100%.

#### The parser
The first functional component to be introduced were the scanner and the parser, along with its test suite. Actually, only the parser has a test suite initially: It already tested most of the scanner, except some error formatting code and the function to render a token type as a string for an invalid token type value - but these were also fixed later, leading to 100% coverage for both of them.

The commits after that enabled continuous integration. Test cases in Go are usually constructed in a table driven fashion: You define a table of test inputs and outputs, and a function iterating over
them.
In the case of the parser, the tests simply are a map from string in the permission syntax (listing \ref{syntax}) to the permission object that should
have been parsed:

```{#lst:testCasesParser .go caption="Parser test cases" frame=tb}
var testCasesParser = map[string]Permission{
	"123":     nil, // ...
	"\xc2":    nil, // incomplete rune at beginning
	"a\xc2":   nil, // incomplete rune in word
	"oe":      nil,
	"or":      Owned | Read,
    // ... other combinations ...
	"n":       None,
    "m [":     nil, // ...
	"m [] a": &SlicePermission{
		BasePermission:    Mutable,
		ElementPermission: Any,
	},
	"m [1] a": &ArrayPermission{
		BasePermission:    Mutable,
		ElementPermission: Any,
    },
    // .. more tests ...
	"m struct {v; l}": &StructPermission{
		BasePermission: Mutable,
		Fields: []Permission{
			Value,
			LinearValue,
		},
	},
	"_": &WildcardPermission{},
}
```
As can be seen, the map starts with some error cases, followed by base permissions, and then followed by more complex permissions (with error cases again).
Due to the parser being recursive descent, we do not have to test every possible base permission combination at every level, but testing it once is enough,
so the test for structured permissions can just pick any base permission and are still representative.
Combined with 100% line coverage (and no short circuiting logic operators in the code), that means that we can parse all valid permissions.
But the line coverage is also misleading: The code uses `Expect(tokenType, tokenType, ...)` and `Accept(tokenType, tokenType)`, so
the error checking is not represented in additional lines, so there could of course be errors lingering somewhere.

There are two slight issues with this specific table: First, a map has no order; and second, the tests have no names. The first is a bit annoying to deal with,
especially if you have logging to the code being tested, but it is not a real problem: Even if tests fail, the function running the tests executes all of them:
```{#lst:TestParser .go caption="Parser test runner" frame=tb}
func TestParser(t *testing.T) {
	for input, expected := range testCasesParser {
		input := input
		expected := expected
		t.Run(input, func(t *testing.T) {
			perm, err := NewParser(input).Parse()
			if !reflect.DeepEqual(perm, expected) {
				t.Errorf("Input %s: Unexpected permission %v, expected %v - error: %v", input, perm, expected, err)
			}
		})
	}
```
For the names, `t.Run()` is passed `input` as the first argument - which determines the name of the subset being run. Unfortunately, Go replaces spaces with underscores,
so searching for tests that failed is not that easy. Later tests for the interpreter have names for the test cases, as they just got to complex to use the input.

#### Type mapping
The type mapper was a rather simple piece of code. It had 100% coverage as well, but it was written with some wrong assumptions. For some background: Basic types, like
integer types, untyped constant types (untyped int, untyped float, untyped nil) are represented as one `types.Basic` type in Go's `go/types` package, and were thus handled
by a single case statement in the switch statement switching over the type of the type representation, as
seen in listing \ref{lst:NewFromType}.


```{#lst:NewFromType .go caption="NewFromType" float=!hbt frame=tb}
func (typeMapper TypeMapper) NewFromType(t0 types.Type) (result Permission) {
    // ... insert some code handling recursion by using a map ...
	switch t := t0.(type) {
	case *types.Array:
		return typeMapper.newFromArrayType(t)
    // ... other cases ...
	case *types.Basic:
		return basicPermission  // constant defined elsewhere
```
As can be seen, it assumed that basic types always map to base permissions, which is normally true - but one basic type is _untyped nil_.
So, untyped nil values received a permission for atom types like integers, and thus could not be assigned to pointers, maps, and other nil-able values.
This led to the nil permission being added, and of course a regression test that ensures that an untyped nil basic type always translates to an untyped nil permission.

Testing the other cases was mostly a matter of picking one specific example: Consider the pointer:
```go
func (typeMapper TypeMapper) newFromPointerType(t *types.Pointer) Permission {
	perm := &PointerPermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	perm.Target = typeMapper.NewFromType(t.Elem())
	return perm
}
```
it was tested by this:
```go
	"*interface{}": &PointerPermission{
		BasePermission: Mutable,
		Target: &InterfacePermission{
			BasePermission: Mutable,
		},
	},
```

#### Assignability tests
The tests for assigning were done as a simple quintuplet: Source permission, target permission, result for move, result for reference, result for copy; as the example in listing \ref{lst:testcasesAssignableTo} show.

```{#lst:testcasesAssignableTo .go caption="Test cases for assignability" float=!hbt frame=tb}
var testcasesAssignableTo = []assignableToTestCase{
	// Basic types
	{"om", "om", true, false, true},
	{"ow", "ow", false, true, false},
    // ...
	{&NilPermission{}, "om chan om", true, true, true}, // ... and all other nilable targets ...
	// Incompatible types
	{"om", "om func ()", false, false, false},  // ... and so on ...
	// tests with a helper type for tuples, since there is no syntax for them.
	{tuplePermission{"om", "om"}, tuplePermission{"om", "ov"}, true, false, true},
    // ....
}
```
And again, since the function were defined recursively, and we have successfully tested the base combinations, all that was left to do was to test some possible
combinations: For example, a function with no parameters, a function with parameters (though usually just one). The code is not precise, though: A function with
two parameters can be assigned to a function with one parameter, for example. The code relies on Go filtering such impossible cases out earlier when doing type
checking, and hence they are not always handled, and thus not tested.

#### Merge and conversion
Merge and conversion tests look like this: The type of merge to perform, the two input parameters, the expected output or nil, and an error string (or empty string if no error should occur). For example:
```go
{mergeIntersection, "om * om", "om", nil, "Cannot merge"}
```

The tests are fairly similar to the assign tests: Both assignment and `merge and conversions` have their own recursing of the data structures, and thus
we need to test possible combinations of things like empty lists, non-empty lists, and so on.
Merge and convert are even safer than assign: They actually check that the length of function parameters are the same, rather than relying on Go doing it
for them. And thus they also have tests checking that you cannot merge a 2-parameter function with a one parameter function, for example.

One thing is special about tests for intersection and union: Since both are commutative, the test runner actually checks each test case in both directions,
ensuring that $A\ merge_\cap\ B = B\ merge_\cap\ A$ (and the same for $\cup$) (for tested $A, B$) without twice the number of tests.

\clearpage

### Testing the interpreter package
The interpreter contains the store and abstract interpreter described in the section entitled _[Static analysis of Go programs](#static-analysis-of-go-programs)_,
coverage is about 93%.

This subsection is split on 3 pages: This page covers the store, the second covers expressions, and the third covers statements.

#### Testing the store
The store really was some special thing to test. No table driven tests were used here, but rather some testing functions were written. The reason is simple: While
the other stuff to be tested were single functions with lots of cases; the store is a lot of small functions with few cases per function.

An example test case for the store is the definition of values, as shown in listing \ref{lst:TestStore_Define}. We simply define a variable, check that the permission is correct, and then redefine it, which should reset the value (since `:=` can both define new variables, but also set existing variables defined in the same block).

```{#lst:TestStore_Define .go caption="Test case for definining values in the store" float=!h frame=tb}
func TestStore_Define(t *testing.T) {
	store := NewStore()
	store, _ = store.Define("a", permission.Mutable)
	if len(store) != 1 {
		t.Fatalf("Length is %d should be %d", len(store), 1)
	}
	if store.GetEffective("a") != permission.Mutable {
		t.Errorf("Should be mutable, is %v", store.GetEffective("a"))
	}
	store, _ = store.Define("a", permission.Read)
	if len(store) != 1 {
		t.Errorf("Length is %d should be %d", len(store), 1)
	}
	if store.GetEffective("a") != permission.Read {
		t.Errorf("Should be read-only, is %v", store.GetEffective("a"))
	}
}
```

The store has 100% line coverage, and all error handling is explicit with if statements, so the coverage should be more conclusive here than for the parser, for example, as all error cases need to be explictly tested here to achieve this level of coverage.

#### Abstract expression interpretation
The expression tests are table-driven again. As we can seen, we check a given expression (or a scenario, which contains a setup code part and an expression part),
with a name, and a permission for `a`, a permission for `b`, the result permission, the result owner, the result dependencies, and the after state of the `a` and
`b` permissions in the store.

```{#lst:testCasesExpr .go caption="Test case for expressions" float=!h frame=tb}
	testCases := []struct {
		expr         interface{}		// expression/scenario to test
		name         string				// name for test case
		lhs          interface{}		// permission for 'a' in store
		rhs          interface{}		// permission for 'b' store
		result       interface{}		// permission returned
		owner        string				// owner returned
		dependencies []string			// dependencies returned
		lhsAfter     interface{}		// permission of a in store after test
		rhsAfter     interface{}		// permission of b in store after test
	}{
		....
		{"a[b:2:3]", "sliceMin", "om []ov", "om", "om []ov", "a", []string{}, "n []n", "om"},
		{"a[1:2:b]", "sliceMax", "om []ov", "om", "om []ov", "a", []string{}, "n []n", "om"},
		{"a[1:2:b]", "sliceInvalid", "om map[ov]ov", "om", errorResult("not sliceable"), "a", []string{}, "n []n", "om"},
		// TODO
		//{scenario{"", "func() {}"}, "funcLit", "om", "om", "", "", nil, nil, nil},
		{"a.(b)", "type cast", "om", "om", errorResult("not yet implemented"), "", nil, nil, nil},

		// Selectors (1): Method values
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterface", "ov interface{ ov (ov) func () }", "_", "ov func ()", "", []string{}, "ov interface{ ov (ov) func () }", "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceUnowned", "ov interface{ ov (v) func () }", "_", "v func ()", "", []string{}, "ov interface{ ov (v) func () }", "_"},
		....
	}
```

Every implemented expression is tested, but the coverage of each expression is not fully 100%.

\clearpage

#### Abstract statement interpretation
Statement interpretation testing is table-driven as well. Each test case consists of the name, an description of an input store, some code to execute (in the form of a function), a slice of exit descriptions (a store description and a position of the branch statement that is returned - as described in \fref{sec:exit-1}), and an error string that is checked against a returned error (empty string means no error expected).

```go
	type testCase struct {
		name   string
		input  []storeItemDesc	// structs variable name -> permission
		code   string
		output []exitDesc		// struct with []storeItemDesc and int for position
		error  string
	}
```

One test case is the empty block - It simply falls through, hence the result has no exit statement (indicated by position being -1):
```go
		{"emptyBlock",
			[]storeItemDesc{
				{"main", "om func (om * om) om * om"},
			},
			"func main() {  }",
			[]exitDesc{
				{nil, -1},
			},
			"",
		},
```

A more complicated statement is `if`. In the following example we have two exits, one is the return inside the if, the
other is the return after it:
```go
		{"if",
			[]storeItemDesc{
				{"a", "om * om"},
				{"nil", "om * om"},
				{"main", "om func (om * om) om * om"},
			},
			"func main(a *int) *int { if a != nil { return a }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "n * r"}}, 54},
				{[]storeItemDesc{{"a", "om * om"}}, 66},
			},
			"",
		},
```

Every implemented statement is tested, again not for all possible cases.
