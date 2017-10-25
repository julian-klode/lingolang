// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/julian-klode/lingolang/permission"
)

func recoverErrorOrFail(t *testing.T, message string) {
	if e := recover(); (e == nil && message != "") || (e != nil && message == "") || !strings.Contains(fmt.Sprint(e), message) {
		t.Fatalf("Unexpected error -- %s -- expected it to contain -- %s --", e, message)
	}
}

func runFuncRecover(t *testing.T, message string, block func()) {
	defer recoverErrorOrFail(t, message)
	block()
}

type tuplePermission []string

func newPermission(input interface{}) permission.Permission {
	switch input := input.(type) {
	case nil:
		return nil
	case string:
		perm, err := permission.NewParser(input).Parse()
		if err != nil {
			panic(err)
		}

		if iface, ok := perm.(*permission.InterfacePermission); ok {
			// Step 1: Replace receivers with pointer to iface, keep receivers in list
			originalReceivers := make([]permission.Permission, len(iface.Methods))
			for i := range iface.Methods {
				originalReceivers[i] = iface.Methods[i].(*permission.FuncPermission).Receivers[0]
				iface.Methods[i].(*permission.FuncPermission).Receivers[0] = perm
			}
			// Step 2: Convert receivers to specified ones.
			for i := range iface.Methods {
				iface.Methods[i].(*permission.FuncPermission).Receivers[0], err = permission.ConvertTo(perm, originalReceivers[i])
				if err != nil {
					panic(err)
				}
			}
		}
		return perm
	case permission.Permission:
		return input
	case tuplePermission:
		base := newPermission(input[0])
		var others []permission.Permission
		for _, s := range input[1:] {
			o := newPermission(s)
			others = append(others, o)
		}

		return &permission.TuplePermission{base.(permission.BasePermission), others}
	}
	panic(fmt.Errorf("Not reachable, unkown object %#v", input))
}

func TestVisitIdent(t *testing.T) {
	st := Store{
		{"x", newPermission("om[]om"), newPermission("om")},
	}
	i := &Interpreter{}
	runFuncRecover(t, "Unknown variable", func() {
		i.VisitExpr(st, ast.NewIdent("a"))
	})
	runFuncRecover(t, "Cannot restrict effective permission", func() {
		i.VisitExpr(st, ast.NewIdent("x"))
	})
}

func TestHelper(t *testing.T) {
	p := newPermission("ov interface{ ov (ov) func () }")
	spew.Printf("p = %v, receiver=%v\n", p, p.(*permission.InterfacePermission).Methods[0].(*permission.FuncPermission).Receivers)
}

func TestVisitExpr(t *testing.T) {
	type errorResult string

	type scenario struct {
		setup string
		expr  string
	}

	valueInterface := newPermission("ov interface{ ov (ov) func () }")
	valueMethodExpr := newPermission("ov func(ov)")
	valueMethodExpr.(*permission.FuncPermission).Params[0] = valueInterface

	unownedValueInterface := newPermission("ov interface{ ov (v) func () }")
	unownedValueMethodExpr := newPermission("ov func(v)")
	unownedValueMethodExpr.(*permission.FuncPermission).Params[0] = unownedValueInterface.(*permission.InterfacePermission).Methods[0].(*permission.FuncPermission).Receivers[0]

	testCases := []struct {
		expr         interface{}
		name         string
		lhs          interface{}
		rhs          interface{}
		result       interface{}
		dependencies []string
		lhsAfter     interface{}
		rhsAfter     interface{}
	}{
		// ------------------- Binary expressions ----------------------------
		{"b&&b", "binarySingleIdent", "", "om", "om", []string{}, "", "om"},
		{"a+b", "binaryOk", "or", "or", "om", []string{}, "or", "or"},
		{"a+b", "binaryLhsUnreadable", "ow", "or", errorResult("In a: Required permissions r, but only have ow"), []string{}, "or", "or"},
		{"a+b", "binaryLhsUnreadable", "or", "ow", errorResult("In b: Required permissions r, but only have ow"), []string{}, "or", "or"},
		// ------------------- Indexing ----------------------------
		{"a[b]", "mutableSlice", "om[]om", "om", "om", []string{"a"}, "n[]n", "om"},
		{"a[b]", "mutableArray", "om[_]om", "om", "om", []string{"a"}, "n[_]n", "om"},
		{"a[b]", "mutableMap", "om map[om]om", "om", "om", []string{"a"}, "n map[n]n", "om"},
		// mutable map, non-copyable key: Item was moved into the map, it's gone now.
		{"a[b]", "mutableMapNoCopyKey", "om map[om * om]om", "om * om", "om", []string{"a"}, "n map[n * r]n", "n * r"},
		// a regular writable pointer is copyable. but beware: that's unsafe.
		{"a[b]", "mutableMapCopyablePointerKey", "om map[orw * orw]om", "orw * orw", "om", []string{"a"}, "n map[n * rw]n", "orw * orw"},
		// we pass a mutable key where we only need r/o, the key is consumed.
		{"a[b]", "mutableMapFreeze", "om map[or * or]om", "om * om", "om", []string{"a"}, "n map[n * r]n", "or * or"},
		{"a[b]", "notIndexable", "or", "ov", errorResult("Indexing unknown"), nil, "", ""},
		{"a[b]", "keyNotReadable", "om[] om", "ow", errorResult("Required permission"), nil, "", ""},
		{"a[b]", "indexableNotReadable", "on[] on", "or", errorResult("Required permission"), nil, "", ""},
		{"a[b]", "mutableMapInvalidKey", "or map[om * om]ov", "ov * ov", errorResult("move or copy"), nil, "", ""},
		// ------------------- Star expressions ----------------------------
		{"*b", "mutablePointer", "", "om * om", "om", []string{"b"}, "", "n * r"},
		{"*b", "mutablePointerReadTarget", "", "om * or", "or", []string{"b"}, "", "n * r"},
		{"*b", "readOnlyPointer", "", "or * or", "or", []string{"b"}, "", "n * r"},
		{"*b", "noPointer", "", "or", errorResult("non-pointer"), []string{"b"}, "", "n * r"},
		{scenario{"var b *int", "*b"}, "stareWithTypeInfo", "", "or", errorResult("non-pointer"), []string{"b"}, "", "n * r"},
		// Unary expressions
		{"<-b", "unaryChannelRead", nil, "om chan om", "om", []string{}, nil, "om chan om"},
		{"<-b", "unaryChannelReadLinear", nil, "om chan ol", "ol", []string{}, nil, "om chan ol"},
		{"<-b", "unaryChannelReadNotChan", nil, "om", errorResult("xpected channel"), []string{}, nil, "om chan ol"},
		{"-b", "mutableNegation", nil, "om", "om", []string{}, nil, "om"},
		{"&b", "mutableNegation", nil, "om", "om * om", []string{"b"}, nil, "n"},
		{"&b", "mutableNegation", nil, "or", "om * or", []string{"b"}, nil, "n"},
		// Parens
		{"(b)", "parenMut", nil, "om", "om", []string{"b"}, nil, "n"},
		{"(b)", "parenPoint", nil, "om * om", "om * om", []string{"b"}, nil, "n * r"},
		// Function calls
		{"a(b)", "callMutableNoCopy", "om func(om * om) or", "om * om", "or", []string{}, "om func(om * om) or", "n * r"},
		{"a(b, b)", "callMutableNoCopy", "om func(om * om, om * om) or", "om * om", errorResult("Cannot copy or move"), []string{}, "om func(om * om, om * om) or", "n * r"},
		{"a(b)", "callMutableNoCopyUnowned", "om func(m * m) or", "om * om", "or", []string{}, "om func(m * m) or", "om * om"},
		{"a(b)", "callMutableNoCopyUnownedToUnownedReadable", "om func(r * r) or", "om * om", "or", []string{}, "om func(r * r) or", "om * om"},
		{"a(b, b)", "callMutableNoCopyUnownedToUnownedReadable", "om func(r * r, r * r) or", "om * om", errorResult("Cannot copy or move"), []string{}, "om func(r * r, r * r) or", "om * om"},
		{"a(b)", "callMutableNoCopyUnownedToOwnedReadable", "om func(or * or) or", "om * om", "or", []string{}, "om func(or * or) or", "or * or"},
		{"a(b, b)", "callMutableNoCopyUnownedToOwnedReadable", "om func(or * or, or * or) or", "om * om", "or", []string{}, "om func(or * or, or * or) or", "or * or"},
		{"a(b)", "callMutableNoCopyUnownedToOwnedReadable", "om func(or * owr) or", "om * om", "or", []string{}, "om func(or * owr) or", "or * or"},
		{"a(b)", "callMutableCopy", "om func(om) or", "om", "or", []string{}, "om func(om) or", "om"},
		{"a(b)", "callMutableNoRet", "om func(om)", "om", tuplePermission{"om"}, []string{}, "om func(om)", "om"},
		{"a(b)", "callMutableNoRet", "om func(om) (ov, oa)", "om", tuplePermission{"om", "ov", "oa"}, []string{}, "om func(om) (ov, oa)", "om"},
		{"a(b)", "callMutableIncompat", "om func(om * om)", "or * or", errorResult("not copy or move"), []string{}, "om func(om)", "or * or"},

		{"a(b)", "callRetValue", "om func(om) n", "om", "n", []string{}, "om func(om) n", "om"},
		{"a(b)", "callNotAFunction", "om", "om", errorResult("non-function"), []string{}, "om", "om"},
		// Basic lit
		{"127", "basicLitInt", nil, nil, "om", []string{}, nil, nil},
		{"127.1", "basicLitFloat", nil, nil, "om", []string{}, nil, nil},
		{"0i", "basicLitImag", nil, nil, "om", []string{}, nil, nil},
		{"'c'", "basicLitChar", nil, nil, "om", []string{}, nil, nil},
		{"\"string\"", "basicLitString", nil, nil, "om", []string{}, nil, nil},
		// Slice
		{"a[:]", "sliceAllArr", "om [_]ov", nil, "om []ov", []string{"a"}, "n [_]n", nil},
		{"a[:]", "sliceAllSlice", "om []ov", nil, "om []ov", []string{"a"}, "n []n", nil},
		{"a[:]", "sliceAllSliceRo", "or []or", nil, "or []or", []string{"a"}, "n []n", nil},
		{"a[:b]", "sliceHigh", "om []ov", "om", "om []ov", []string{"a"}, "n []n", "om"},
		{"a[b:2:3]", "sliceMin", "om []ov", "om", "om []ov", []string{"a"}, "n []n", "om"},
		{"a[1:2:b]", "sliceMax", "om []ov", "om", "om []ov", []string{"a"}, "n []n", "om"},
		{"a[1:2:b]", "sliceInvalid", "om map[ov]ov", "om", errorResult("not sliceable"), []string{"a"}, "n []n", "om"},
		// TODO
		{"func() {}", "funcLit", "om", "om", errorResult("not yet implemented"), nil, nil, nil},
		{"a.(b)", "type cast", "om", "om", errorResult("not yet implemented"), nil, nil, nil},

		// Selectors (1): Method values
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterface", "ov interface{ ov (ov) func () }", "_", "ov func ()", []string{}, "ov interface{ ov (ov) func () }", "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceUnowned", "ov interface{ ov (v) func () }", "_", "v func ()", []string{}, "ov interface{ ov (v) func () }", "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceUnowned", "m interface{ om (m) func () }", "_", "m func ()", []string{"a"}, permission.ConvertToBase(newPermission("m interface{ om (m) func () }"), newPermission("n").GetBasePermission()), "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceCantBind", "ov interface{ ov (om) func () }", "_", errorResult("not bind receiver"), []string{}, "ov interface{ ov (ov) func () }", "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceIncompatibleLHS", "_", "_", errorResult("unknown type on left side"), []string{}, "ov interface{ ov (ov) func () }", "_"},
		// Selectors (2): Structs
		{scenario{"var a struct { b int }", "a.b"}, "selectStructMember", "ov struct { ov }", "_", "ov", []string{"a"}, "n struct { n }", "_"},
		{scenario{"var a struct { b int }", "a.b"}, "selectStructMemberNotStruct", "ov", "_", errorResult("non-struct"), []string{"a"}, "n struct { n }", "_"},
		{scenario{"type b struct { x, c int }\nvar a struct { b }", "a.c"}, "selectStructMemberEmbedded", "ov struct { ov struct { on; ov } }", "_", "ov", []string{"a"}, "n struct { n struct { n; n } }", "_"},
		{scenario{"type b struct { x, c int }\nvar a struct { *b }", "a.c"}, "selectStructMemberEmbeddedPointer", "ov struct { ov * ov struct { on; ov } }", "_", "ov", []string{"a"}, "n struct { n * v struct { n; v } }", "_"},
		// Selectors (3): Method expressions
		{scenario{"type a interface{ b()}", "a.b"}, "selectMethodExprInterface", valueInterface, "_", valueMethodExpr, []string{}, valueInterface, "_"},
		{scenario{"type a interface{ b()}", "a.b"}, "selectMethodExprInterfaceUnowned", unownedValueInterface, "_", unownedValueMethodExpr, []string{}, unownedValueInterface, "_"},
		// Composite literals
		{scenario{"type a struct { x int }\nvar b int", "a{b}"}, "compositeLitIndexed", "ov struct { ov }", "ov", "ov struct { ov }", []string{}, "ov struct { ov }", "ov"},
		{scenario{"type a struct { x int }\nvar b int", "a{x: b}"}, "compositeLitKeyed", "ov struct { ov }", "ov", "ov struct { ov }", []string{}, "ov struct { ov }", "ov"},
		{scenario{"type a struct { x int }\nvar b int", "a{x: b}"}, "compositeLitKeyedUnowned", "v struct { v }", "ov", "v struct { v }", []string{}, "v struct { v }", "ov"},
		{scenario{"type a struct { x int }\nvar b int", "a{b}"}, "compositeLitIndexedOwnedMutable", "om struct { om * om }", "om * om", "om struct { om * om }", []string{}, "om struct { om * om }", "n * r"},
		{scenario{"type a struct { x int }\nvar b int", "a{b}"}, "compositeLitIndexedUnownedMutable", "m struct { m * m }", "om * om", "m struct { m * m }", []string{"b"}, "m struct { m * m }", "n * r"},
		{scenario{"type a struct { x int }\nvar b int", "a{b}"}, "compositeLitErrorCannotBind", "m struct { m }", "_", errorResult("not bind field"), nil, nil, nil},
		{scenario{"type a struct { x int }\nvar b int", "a{b}"}, "compositeLitErrorNoStruct", "m", "_", errorResult("xpected struct"), nil, nil, nil},
		{"a{b}", "compositeLitErrorNoTypesInfo", "m struct { m }", "m", errorResult("typesInfo"), nil, nil, nil},
		// Nil
		{"nil", "nilJust", nil, nil, &permission.NilPermission{}, []string{}, nil, nil},
		{"a(nil)", "nilCall", "om func(om * om) ov", nil, "ov", []string{}, "om func (om * om) ov", nil},
		{"a(nil)", "nilPointer", "om func(or * or) ov", nil, "ov", []string{}, "om func (or * or) ov", nil},
		// Booleans
		{"true", "true", nil, nil, "om", []string{}, nil, nil},
		{"false", "false", nil, nil, "om", []string{}, nil, nil},
	}

	for _, test := range testCases {

		t.Run(test.name, func(t *testing.T) {
			defer recoverErrorOrFail(t, "")
			st := NewStore()
			i := &Interpreter{}

			var lhs *ast.Ident
			var rhs *ast.Ident
			var e ast.Expr

			switch expr := test.expr.(type) {
			case string:
				e, _ = parser.ParseExpr(expr)
			case scenario:
				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, "test", "package test\n\n"+expr.setup+"\n\nvar x="+expr.expr, 0)
				if err != nil {
					t.Fatalf("Could not parse setup: %s", err)
				}
				info := types.Info{
					Defs:       make(map[*ast.Ident]types.Object),
					Selections: make(map[*ast.SelectorExpr]*types.Selection),
					Types:      make(map[ast.Expr]types.TypeAndValue),
				}
				config := &types.Config{}
				_, err = config.Check("test", fset, []*ast.File{file}, &info)
				i.typesInfo = &info
				if err != nil {
					t.Fatalf("Could not parse setup: %s", err)
				}
				e = file.Decls[len(file.Decls)-1].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0]
			default:
				panic(fmt.Errorf("Unsupported value %s, %v", e, e))
			}

			ast.Inspect(e, func(node ast.Node) bool {
				if ident, ok := node.(*ast.Ident); ok {
					switch ident.Name {
					case "a":
						lhs = ident
					case "b":
						rhs = ident
					}
				}
				return true
			})

			if lhs != nil && test.lhs != nil {
				st, _ = st.Define(lhs.Name, newPermission(test.lhs))
			}
			if rhs != nil && test.rhs != nil {
				st, _ = st.Define(rhs.Name, newPermission(test.rhs))
			}

			if eResult, ok := test.result.(errorResult); ok {
				runFuncRecover(t, string(eResult), func() {
					i.VisitExpr(st, e)
				})
				return
			}

			perm, deps, store := i.VisitExpr(st, e)

			if !reflect.DeepEqual(newPermission(test.result), perm) {
				t.Errorf("Evaluated to %v, expected %v", perm, newPermission(test.result))
			}

			// Check dependencies
			depsAsString := make([]string, len(deps))
			for i := range deps {
				depsAsString[i] = deps[i].id.Name
			}

			if !reflect.DeepEqual(depsAsString, test.dependencies) {
				t.Errorf("Found dependencies %v, expected %v", depsAsString, test.dependencies)
			}

			if lhs != nil && !reflect.DeepEqual(store.GetEffective(lhs.Name), newPermission(test.lhsAfter)) {
				t.Error(spew.Errorf("Found lhs after = %v, expected %v", store.GetEffective(lhs.Name), newPermission(test.lhsAfter)))
			}
			if rhs != nil && !reflect.DeepEqual(store.GetEffective(rhs.Name), newPermission(test.rhsAfter)) {
				t.Error(spew.Errorf("Found rhs after = %v, expected %v", store.GetEffective(rhs.Name), newPermission(test.rhsAfter)))
			}
		})

	}
}

func TestVisitExpr_bad(t *testing.T) {
	var i Interpreter
	var st Store
	runFuncRecover(t, "bad expr", func() {
		i.VisitExpr(st, &ast.BadExpr{})
	})
}

func TestRelease(t *testing.T) {
	var i Interpreter
	var st Store
	runFuncRecover(t, "not release borrowed variable", func() {
		st, _ = st.Define("a", newPermission("om"))
		i.Release(ast.NewIdent("a"), st, []Borrowed{
			{ast.NewIdent("a"), newPermission("om * om")},
		})
	})
}

func TestVisitSelectorExprOne_impossible(t *testing.T) {
	i := &Interpreter{}
	runFuncRecover(t, "nvalid kind", func() {
		i.visitSelectorExprOne(nil, ast.NewIdent("error"), nil, -1, -42, nil)
	})
}

func TestVisitCompositeLit_errors(t *testing.T) {
	i := &Interpreter{}
	var st Store

	st, _ = st.Define("T", newPermission("om struct {om}"))
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test", "package test\n\n type T struct { x int }\n\nvar x= T { x: 5 }", 0)
	if err != nil {
		t.Fatalf("Could not parse setup: %s", err)
	}
	info := types.Info{
		Defs:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Types:      make(map[ast.Expr]types.TypeAndValue),
	}
	config := &types.Config{}
	_, err = config.Check("test", fset, []*ast.File{file}, &info)
	i.typesInfo = &info
	if err != nil {
		t.Fatalf("Could not parse setup: %s", err)
	}
	e := file.Decls[len(file.Decls)-1].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0].(*ast.CompositeLit)

	runFuncRecover(t, "not find type", func() {
		i.typesInfo = &types.Info{Defs: info.Defs, Selections: info.Selections}
		i.visitCompositeLit(st, e)
	})

	runFuncRecover(t, "o key found", func() {
		i.typesInfo = &info
		e.Elts[0].(*ast.KeyValueExpr).Key = &ast.BadExpr{}
		i.visitCompositeLit(st, e)
	})

	runFuncRecover(t, "ot lookup key", func() {
		i.typesInfo = &info
		e.Elts[0].(*ast.KeyValueExpr).Key = ast.NewIdent("foobar")
		i.visitCompositeLit(st, e)
	})
}

func TestVisitStmt(t *testing.T) {
	type storeItemDesc struct {
		key   string
		value interface{}
	}
	type exitDesc struct {
		items []storeItemDesc
		pos   int
	}

	type testCase struct {
		name   string
		input  []storeItemDesc
		code   string
		output []exitDesc
		error  string
	}

	testCases := []testCase{
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
		{"returnOne",
			[]storeItemDesc{
				{"a", "om * om"},
				{"main", "om func (om * om) om * om"},
			},
			"func main(a *int) *int { return a }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "n * r"}}, 40},
			},
			"",
		},
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
		{"goto",
			[]storeItemDesc{
				{"a", "om * om"},
				{"nil", "om * om"},
				{"main", "om func (om * om) om * om"},
			},
			"func main(a *int) *int { x: if a != nil { return a }; goto x }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "n * r"}}, 57},
			},
			"",
		},
		{"gotoInfinite",
			[]storeItemDesc{
				{"main", "om func (om * om) om * om"},
			},
			"func main(a *int) *int { x: goto x }",
			[]exitDesc{},
			"",
		},
		{"gotoReturn",
			[]storeItemDesc{
				{"main", "om func (om * om) om * om"},
			},
			"func main(a *int) *int { x: goto x; return a }",
			[]exitDesc{},
			"",
		},
		{"conditionalMove",
			[]storeItemDesc{
				{"a", "om * om"},
				{"nil", "om * om"},
				{"main", "om func (om * om) om * om"},
				{"f", "om func (om * om) n"},
			},
			"func main(a *int, f func(a *int)) *int {  if a != nil { f(a); }; return a }",
			nil,
			"Cannot bind return value", // Might be entering return after having entered the if body
		},
		{"conditionalGoto",
			[]storeItemDesc{
				{"a", "om * om"},
				{"nil", "om * om"},
				{"main", "om func (om * om) om * om"},
				{"f", "om func (om * om) n"},
			},
			"func main(a *int, f func(a *int)) *int {  x: if a != nil { f(a); goto x }; return a }",
			nil,
			"63: In a: Required permissions r", // Fails in second iteration as a has been borrowed.
		},
		{"conditionalGotoMustContinueLoopNotBreakIt",
			[]storeItemDesc{
				{"a", "om * om"},
				{"b", "om * om"},
				{"main", "om func (om * om) om * om"},
				{"f", "om func (om * om) n"},
			},
			// This prevents a regression from where we used "break" instead of "continue" when trying
			// to skip a situation we already encountered.
			"func main(a *int, b *int, f func(a *int)) *int {  if a != nil { f(a); }; if b != nil { f(b) }; x: if 1 != 1 { goto x }; return nil}",
			[]exitDesc{
				{[]storeItemDesc{{"a", "om * om"}, {"b", "om * om"}}, 135},
				{[]storeItemDesc{{"a", "om * om"}, {"b", "n * r"}}, 135},
				{[]storeItemDesc{{"a", "n * r"}, {"b", "om * om"}}, 135},
				{[]storeItemDesc{{"a", "n * r"}, {"b", "n * r"}}, 135},
			},
			"",
		},
		{"emptyStmt",
			[]storeItemDesc{
				{"a", "om * om"},
				{"main", "om func (om * om) om * om"},
			},
			"func main(a *int) *int { ; return a }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "n * r"}}, 42},
			},
			"",
		},
		{"incDecStmt",
			[]storeItemDesc{
				{"a", "om"},
				{"main", "om func (om) om"},
			},
			"func main(a int) int { a++; return a; }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "om"}}, 43},
			},
			"",
		},
		{"incDecStmt",
			[]storeItemDesc{
				{"a", "o"},
				{"main", "om func (om) om"},
			},
			"func main(a int) int { a++; return a; }",
			[]exitDesc{},
			"Required permissions a",
		},
		{"sendStmt",
			[]storeItemDesc{
				{"a", "om chan om"},
				{"b", "om"},
				{"main", "om func (om, om) om"},
			},
			"func main(a chan int, b int) int { a <- b; return b }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "om chan om"}, {"b", "om"}}, 58},
			},
			"",
		},
		{"sendStmtValue",
			[]storeItemDesc{
				{"a", "om chan om"},
				{"b", "ov"},
				{"main", "om func (om, ov) om"},
			},
			"func main(a chan int, b int) int { a <- b; return b }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "om chan om"}, {"b", "ov"}}, 58},
			},
			"",
		},
		{"sendStmtNotAChannel",
			[]storeItemDesc{
				{"a", "om [] om"},
				{"b", "on"},
				{"main", "om func (om, ov) om"},
			},
			"func main(a chan int, b int) int { a <- b; return b }",
			[]exitDesc{},
			"xpected channel",
		},
		{"sendStmtIncompatibleValue",
			[]storeItemDesc{
				{"a", "om chan om"},
				{"b", "on"},
				{"main", "om func (om, ov) om"},
			},
			"func main(a chan int, b int) int { a <- b; return b }",
			[]exitDesc{},
			"Cannot send value: Cannot copy or move",
		},
		{"sendStmtLinear",
			[]storeItemDesc{
				{"a", "om chan om * om"},
				{"b", "om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(a chan *int, b *int) *int { a <- b; return nil }",
			[]exitDesc{
				{[]storeItemDesc{{"a", "om chan om * om"}, {"b", "n * r"}}, 61},
			},
			"",
		},
		{"sendStmtLinearError",
			[]storeItemDesc{
				{"a", "om chan om * om"},
				{"b", "om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(a chan *int, b *int) *int { a <- b; return b }",
			[]exitDesc{},
			"Cannot bind return value",
		},
		{"assignStmtMutableDefine",
			[]storeItemDesc{
				{"b", "om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b *int) *int  { a := b;  return a }",
			[]exitDesc{
				{[]storeItemDesc{{"b", "n * r"}}, 50},
			},
			"",
		},
		{"assignStmtSwap",
			[]storeItemDesc{
				{"a", "om * om"},
				{"b", "om * om"},
				{"main", "om func (om) n"},
			},
			"func main(a, b *int)  { a, b = b, a }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om * om"},
					{"b", "om * om"},
				}, -1},
			},
			"",
		},
		{"assignStmtSwap",
			[]storeItemDesc{
				{"a", "om * om"},
				{"b", "om * om"},
				{"main", "om func (om) n"},
			},
			"func main(a, b *int)  { a, b = b, a }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om * om"},
					{"b", "om * om"},
				}, -1},
			},
			"",
		},

		{"rangeStmtOwnedGone",
			[]storeItemDesc{
				{"a", "om []om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a []*float64, f func(*float64)) *float64 { for _, x := range a { f(x) }; return a[0] }",
			[]exitDesc{},
			"equired permissions r",
		},
		{"rangeStmtOwnedGoneAssign",
			[]storeItemDesc{
				{"a", "om []om * om"},
				{"x", "om"},
				{"y", "om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a []*float64, f func(*float64), x int, y *float64) *float64 { for x, y = range a { f(y) }; return a[0] }",
			[]exitDesc{},
			"equired permissions r",
		},
		{"rangeStmtUnownedNotGone",
			[]storeItemDesc{
				{"a", "m []m * m"},
				{"f", "om func (m * m) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a []*float64, f func(*float64)) *float64 { for _, x := range a { f(x) }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "m []m * m"},
				}, 98},
			},
			"",
		},

		{"rangeStmtReturnInIteration",
			[]storeItemDesc{
				{"a", "om []om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a []*float64, f func(*float64)) *float64 { for _, x := range a { return x }; return a[0] }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "n []n * r"},
				}, 90},
				{[]storeItemDesc{
					{"a", "n []n * r"},
				}, 102},
			}, // This one is essentially like an if: We either exit the loop and consume a, or we don't.
			"",
		},
		{"rangeStmtReturnInIterationMap",
			[]storeItemDesc{
				{"a", "om map[om]om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a map[string]*float64, f func(*float64)) *float64 { for _, x := range a { return x }; return a[\"0\"] }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "n map[n]n * r"},
				}, 99},
				{[]storeItemDesc{
					{"a", "n map[n]n * r"},
				}, 111},
			},
			"",
		},
		{"rangeStmtBreak",
			[]storeItemDesc{
				{"a", "om map[om]om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a map[string]*float64, f func(*float64)) *float64 { for range a { break }; return a[\"0\"] }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "n map[n]n * r"},
				}, 100},
			},
			"",
		},
		{"switchStmtSameExits",
			[]storeItemDesc{
				{"a", "om map[om]om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a map[string]*float64, f func(*float64)) *float64 { switch { case true: f(a[\"x\"]); case false: f(a[\"x\"]) }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "n map[n]n * r"},
				}, 133},
				{[]storeItemDesc{
					{"a", "om map[om]om * om"},
				}, 133},
			},
			"",
		},
		{"switchStmtFallthrough",
			[]storeItemDesc{
				{"a", "om map[om]om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a map[string]*float64, f func(*float64)) *float64 { switch { case true: f(a[\"x\"]); fallthrough; case false: f(a[\"x\"]) }; return nil }",
			[]exitDesc{},
			"135: In a: Required permissions r",
		},
		{"switchStmtBreak",
			[]storeItemDesc{
				{"a", "om map[om]om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a map[string]*float64, f func(*float64)) *float64 { switch { case true: break; f(a[\"x\"]); case false: break; f(a[\"x\"]) }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om map[om]om * om"},
				}, 147},
			},
			"",
		},
		{"switchStmtReturnEverywhere",
			[]storeItemDesc{
				{"a", "om map[om]om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a map[string]*float64, f func(*float64)) *float64 { switch { case true: return a[\"x\"]; case false: return nil }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om map[om]om * om"},
				}, 124},
				{[]storeItemDesc{
					{"a", "n map[n]n * r"},
				}, 97},
				{[]storeItemDesc{
					{"a", "om map[om]om * om"},
				}, 138},
			},
			"",
		},
		{"switchStmtOneReturn",
			[]storeItemDesc{
				{"a", "om map[om]om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a map[string]*float64, f func(*float64)) *float64 { switch { case true: return a[\"x\"]; case false: f(a[\"x\"]) }; return a[\"x\"] }",
			[]exitDesc{},
			"144: In a: Required permissions r",
		},
		{"selectStmt",
			[]storeItemDesc{
				{"a", "om chan om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(a chan *float64) *float64 { select { case x := <- a: return x; case x := <- a: return x;  }; return <- a }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om chan om * om"},
				}, 104},
				{[]storeItemDesc{
					{"a", "om chan om * om"},
				}, 78},
				{[]storeItemDesc{
					{"a", "om chan om * om"},
				}, 118},
			},
			"",
		},
		// Oh, oh, three exits, but all at the same position! We should be merging those...
		{"selectStmtAssign",
			[]storeItemDesc{
				{"a", "om chan om * om"},
				{"x", "om * om"},
				{"y", "om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a chan *float64, x, y *float64, f func(*float64)) *float64 { f(x); f(y); select { case x = <- a: ; case y = <- a: ;  }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om chan om * om"},
					{"x", "om * om"},
					{"y", "n * r"},
				}, 145},
				{[]storeItemDesc{
					{"a", "om chan om * om"},
					{"x", "n * r"},
					{"y", "om * om"},
				}, 145},
				{[]storeItemDesc{
					{"a", "om chan om * om"},
					{"x", "n * r"},
					{"y", "n * r"},
				}, 145},
			},
			"",
		},
		{"selectStmtAssignAndUseUp",
			[]storeItemDesc{
				{"a", "om chan om * om"},
				{"x", "om * om"},
				{"y", "om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a chan *float64, x, y *float64, f func(*float64)) *float64 { f(x); f(y); select { case x = <- a: f(x); case y = <- a: f(y);  }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om chan om * om"},
					{"x", "n * r"},
					{"y", "n * r"},
				}, 153},
			},
			"",
		},
		// This is equivalent to selectStmtAssign, the function calls are unreachable.
		{"selectStmtAssignBreakAndUseUp",
			[]storeItemDesc{
				{"a", "om chan om * om"},
				{"x", "om * om"},
				{"y", "om * om"},
				{"f", "om func (om * om) n"},
				{"main", "om func (om) om * om"},
			},
			"func main(a chan *float64, x, y *float64, f func(*float64)) *float64 { f(x); f(y); select { case x = <- a: break; f(x); case y = <- a: break; f(y);  }; return nil }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", "om chan om * om"},
					{"x", "om * om"},
					{"y", "n * r"},
				}, 167},
				{[]storeItemDesc{
					{"a", "om chan om * om"},
					{"x", "n * r"},
					{"y", "om * om"},
				}, 167},
				{[]storeItemDesc{
					{"a", "om chan om * om"},
					{"x", "n * r"},
					{"y", "n * r"},
				}, 167},
			},
			"",
		},
		{"forStmtIterateToError",
			[]storeItemDesc{
				{"b", "om * om"},
				{"f", "om func (om * om) om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b *int, f func(*int)) { for a := 0; a < 12345; a++ { f(b) }   }",
			nil,
			"80: In b:",
		},
		// This has two identical exits: (1) Never entered loop (2) Entered loop, but broken
		{"forStmtBreakEvil",
			[]storeItemDesc{
				{"b", "om * om"},
				{"f", "om func (om * om) om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b *int, f func(*int)) { for a := b; *a < 12345; (*a)++ { break; f(a) }   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"a", nil},
				}, -1},
				{[]storeItemDesc{
					{"a", nil},
				}, -1},
			},
			"",
		},
		// Test for the next case
		{"goStmtNoGo",
			[]storeItemDesc{
				{"b", "om * om"},
				{"c", "om * om"},
				{"f", "om func (om * om, m * m) om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b, c *int, f func(*int, *int)) { f(b, c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", "n * r"},
					{"c", "om * om"},
				}, -1},
			},
			"",
		},
		{"goStmt",
			[]storeItemDesc{
				{"b", "om * om"},
				{"c", "om * om"},
				{"f", "om func (om * om, m * m) om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b, c *int, f func(*int, *int)) { go f(b, c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", "n * r"},
					{"c", "n * r"},
				}, -1},
			},
			"",
		},
		{"deferStmt",
			[]storeItemDesc{
				{"b", "om * om"},
				{"c", "om * om"},
				{"f", "om func (om * om, m * m) om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b, c *int, f func(*int, *int)) { defer f(b, c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", "n * r"},
					{"c", "n * r"},
				}, -1},
			},
			"",
		},
		{"deferStmtOwner",
			[]storeItemDesc{
				{"b", "om interface{ om (om) func (om * om) n }"},
				{"c", "om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b interface { f(*int) } , c *int) { defer b.f(c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", permission.ConvertToBase(newPermission("om interface{ om (om) func (om * om) n }"), 0)},
					{"c", "n * r"},
				}, -1},
			},
			"",
		},
		{"deferStmtUnownedOwner",
			[]storeItemDesc{
				{"b", "om interface{ om (m) func (om * om) n }"},
				{"c", "om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b interface { f(*int) } , c *int) { defer b.f(c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", permission.ConvertToBase(newPermission("om interface{ om (m) func (om * om) n }"), 0)},
					{"c", "n * r"},
				}, -1},
			},
			"",
		},
		{"deferStmtCompletelyUnownedOwner",
			[]storeItemDesc{
				{"b", "m interface{ om (m) func (om * om) n }"},
				{"c", "om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b interface { f(*int) } , c *int) { defer b.f(c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", "m interface{ om (m) func (om * om) n }"}, // TODO: Broken.
					{"c", "n * r"},
				}, -1},
			},
			"",
		},
		{"deferStmtUnownedArg",
			[]storeItemDesc{
				{"b", "om interface{ om (m) func (m * m) n }"},
				{"c", "om * om"},
				{"main", "om func (om) om * om"},
			},
			"func main(b interface { f(*int) } , c *int) { defer b.f(c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", permission.ConvertToBase(newPermission("om interface{ om (m) func (m * m) n }"), 0)},
					{"c", "n * r"},
				}, -1},
			},
			"",
		},
		{"deferStmtCompletelyUnownedArg",
			[]storeItemDesc{
				{"b", "om interface{ om (m) func (m * m) n }"},
				{"c", "m * m"},
				{"main", "om func (om) om * om"},
			},
			"func main(b interface { f(*int) } , c *int) { defer b.f(c)   }",
			[]exitDesc{
				{[]storeItemDesc{
					{"b", permission.ConvertToBase(newPermission("om interface{ om (m) func (m * m) n }"), 0)},
					{"c", "n * r"},
				}, -1},
			},
			"",
		},
	}

	for _, cs := range testCases {
		t.Run(cs.name, func(t *testing.T) {
			if cs.error != "" {
				defer recoverErrorOrFail(t, cs.error)
			}

			i := &Interpreter{}
			var st Store
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test", "package test\n\n"+cs.code, 0)
			if err != nil {
				t.Fatalf("Could not parse setup: %s", err)
			}
			info := types.Info{
				Defs:       make(map[*ast.Ident]types.Object),
				Selections: make(map[*ast.SelectorExpr]*types.Selection),
				Types:      make(map[ast.Expr]types.TypeAndValue),
			}
			config := &types.Config{}
			_, err = config.Check("test", fset, []*ast.File{file}, &info)
			i.typesInfo = &info
			i.fset = fset
			if err != nil {
				t.Fatalf("Could not parse setup: %s", err)
			}

			for _, input := range cs.input {
				st, err = st.Define(input.key, newPermission(input.value))
				if err != nil {
					t.Fatalf("Could not define input %s: %s", input.key, err)
				}
			}

			i.curFunc = st.GetEffective("main").(*permission.FuncPermission)
			exits := i.visitStmt(st, file.Decls[len(file.Decls)-1].(*ast.FuncDecl).Body)

			if len(exits) != len(cs.output) {
				t.Fatalf("Expected %d result, got %d => %v", len(cs.output), len(exits), exits)
			}

			for k, output := range cs.output {
				exit := exits[k]
				for _, item := range output.items {
					act := exit.GetEffective(item.key)
					exp := newPermission(item.value)
					if !reflect.DeepEqual(act, exp) {
						t.Error(spew.Errorf("exit %d: key %s: Expected %v, received %v", k, item.key, exp, act))
					}
				}

				if (output.pos >= 0) != (exit.branch != nil) {
					t.Errorf("Expected branch statement = %v, Got branch statement = %v", output.pos >= 0, exit.branch != nil)
				} else if output.pos > 0 && int(exit.branch.Pos()) != output.pos {
					t.Error(spew.Errorf("exit %d: Expected %v, received %v", k, output.pos, exit.branch.Pos()))
				}
			}
		})
	}

}
