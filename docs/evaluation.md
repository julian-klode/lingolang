# Evaluation

## Testing the implementation
The implementation, since the beginning, has been subject to rigorous unit testing with continuous integration on [travis-ci.org](https://travis-ci.org/julian-klode/lingolang) and code coverage reports on [codecov.io](https://codecov.io/gh/julian-klode/lingolang).

![code coverage chart](coverage-chart.png)

The code coverage chart shows that it started out slightly below 100% line coverage, eventually reaching 100%, only to drop again - when the interpreter started coming together - there are quite a few places in the interpreter code that are unreachable conditions and would require constructing a lot of illegal AST objects to test.

Unfortunately, the Go tools only provide line coverage, and not branch or path coverage. This is somewhat problematic: For example, if we have an `if` statement without an `else` part, we can test if the if has been taken, but we usually cannot check whether it's not been taken: The if statement would eventually fall out of its block and back into the parent block, and thus all lines are executed.

### Testing the permissions package
The permissions package contains the parser and the rules for permissions described in the section _[Permissions for Go](#permissions-for-go)_.

#### The parser
The first functional component to be introduced were the scanner and the parser, along with its test suite. Actually, only the parser has a test suite initially: It already tested most of the scanner, except some error formatting code and the function to render a token type as a string for an invalid token type value - but these were also fixed later, leading to 100% coverage for both of them.

The commits after that enabled continuous integration. Test cases in Go are usually constructed in a table driven fashion: You define a table of test inputs and outputs, and a function iterating over
them.
In the case of the parser, the tests simply are a map from string in the permission syntax (listing \ref{syntax}) to the permission object that should
have been parsed:

```go
var testCasesParser = map[string]Permission{
	"123":     nil, // ...
	"\xc2":    nil, // incomplete rune at beginning
	"a\xc2":   nil, // incomplete rune in word
	"oe":      nil,
	"or":      Owned | Read,
    // ... other combinations ...
	"a":       Any,
	"on":      Owned,
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
Due to the parser being recursive descent, we don't have to test every possible base permission combination at every level, but testing it once is enough,
so the test for structured permissions can just pick any base permission and are still representative.
Combined with 100% line coverage (and no short circuiting logic operators in the code), that means that we can parse all valid permissions.
But the line coverage is also misleading: The code uses `Expect(tokenType, tokenType, ...)` and `Accept(tokenType, tokenType)`, so
the error checking is not represented in additional lines, so there could of course be errors lingering somewhere.

There are two slight issues with this specific table: First, a map has no order; and second, the tests have no names. The first is a bit annoying to deal with,
especially if you have logging to the code being tested, but it's not a real problem: Even if tests fail, the function running the tests executes all of them:
```go
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
by a single case statement in the switch statement switching over the type of the type representation:

```go
func (typeMapper TypeMapper) NewFromType(t0 types.Type) (result Permission) {
    // ... insert some code handling recursion by using a map ...
	switch t := t0.(type) {
	case *types.Array:
		return typeMapper.newFromArrayType(t)
	case *types.Slice:
		return typeMapper.newFromSliceType(t)
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
The tests for assigning were done as a simple quintuplet: Source permission, target permission, result for move, result for reference, result for copy:
```go
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
Merge and conversion tests look like this:
```go
{mergeIntersection, "om * om", "om", nil, "Cannot merge"}
```
The type of merge to perform, the two input parameters, the expected output or nil, and an error string (or empty string if no error should occur).

The tests are fairly similar to the assign tests: Both assignment and `merge and conversions` have their own recursing of the data structures, and thus
we need to test possible combinations of things like empty lists, non-empty lists, and so on.
Merge and convert are even safer than assign: They actually check that the length of function parameters are the same, rather than relying on Go doing it
for them. And thus they also have tests checking that you cannot merge a 2-parameter function to a one parameter function, for example.

One thing is special about tests for intersection and union: Since both are commutative, the test runner actually checks each test case in both directions,
ensuring that $A \cap B = B \cap A$ without twice the number of tests.

### Testing the interpreter package
The interpreter contains the store and abstract interpreter described in the section entitled _[Static analysis of Go programs](#static-analysis-of-go-programs)_.

#### Testing the store
The store really was some special thing to test. No table driven tests were used here, but rather some testing functions were written. The reason is simple: While
the other stuff to be tested were single functions with lots of cases; the store is a lot of small functions with few cases per function.

#### Abstract expression interpretation
#### Abstract statement interpretation

## Known issues
The implementation is _coarse_: If I borrow anything uniquely referred to by a variable, then the entire variable is marked as unusable, rather than just the part
of the permission that was borrowed. A solution to this problem would be to collect a path from a variable to an object (like, select field x, select field y), when
evaluating an expression - then we could create a new permission where the borrowed part is replaced by an unusable permission. But this leads to another problem:

The implementation is _shallow_: When looking at the requirements for operands, it only looks at the base permission of the operands. So if an expression requires an
operand to be readable, we only check whether its base permission contains the $r$ bit. That is fine so far, as we ensure consistency at some point in the program
by converting each permission to its own base permission, and thus a readable object can't have unreadable members. But it falls short if we actually want to allow
it. Luckily, there is an option: Instead of checking if $r \in base(A)$ we can check if $ass_{mov}(A, ctb(A, r))$, that is, we create a new structure where all base
permissions are replaced with $\{r\}$ and then we can check if the value is movable to it, which recursively checks that each base permission is movable to $\{r\}$.

The implementation is _ambiguous_: Permissions for types and values are stored in the same namespace. This is not a real problem, though, as Go ensures that we can't
use type in value contexts and vice versa.

It is also _incomplete_: We only have support for checking expressions and statements (including function literals), but we don't have support for declaring global
variables, functions, or importing other packages. These can be "hacked" in easily: A package could have a "package" permission holding permissions for all objects
declared in it, and the current package is simply a store, mapping types and values to their permissions. There are some problems with global mutable state, though:
We cannot really borrow objects in the package - it has multiple om functions.

Type assertion, conversion between named types and interfaces, and type switches are missing. These require gathering a set of methods for a given named type. But
we don't have an equivalent to named types in permissions, and adding it does not seem feasible anymore, as it would require substantial changes in the
interpreter. An alternative would be to simply attach a list of methods to the unnamed permissions, and when converting, take the left set of methods. When a
conversion to an interface is required, we could just build an interface out of these methods, and check if that interface is assignable to the interface.
