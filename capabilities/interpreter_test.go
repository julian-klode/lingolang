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
		{"a[b]", "mutableMapFreeze", "om map[or * or]om", "om * om", "om", []string{"a"}, "n map[n * r]n", "n * r"},
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
		{"T {1}", "compositeLit", "om", "om", errorResult("not yet implemented"), nil, nil, nil},
		{"func() {}", "funcLit", "om", "om", errorResult("not yet implemented"), nil, nil, nil},
		{"a.(b)", "type cast", "om", "om", errorResult("not yet implemented"), nil, nil, nil},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterface", "ov interface{ ov (ov) func () }", "_", "ov func ()", []string{}, "ov interface{ ov (ov) func () }", "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceUnowned", "ov interface{ ov (v) func () }", "_", "v func ()", []string{}, "ov interface{ ov (v) func () }", "_"},
		{scenario{"type a interface{ b()}", "a.b"}, "selectMethodExpressionInterfaceUnowned", "ov interface{ ov (v) func () }", "_", errorResult("not yet implemented"), []string{}, "ov interface{ ov (v) func () }", "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceCantBind", "ov interface{ ov (om) func () }", "_", errorResult("not bind receiver"), []string{}, "ov interface{ ov (ov) func () }", "_"},
		{scenario{"var a interface{ b()}", "a.b"}, "selectMethodValueInterfaceIncompatibleLHS", "_", "_", errorResult("unknown type on left side"), []string{}, "ov interface{ ov (ov) func () }", "_"},
		{scenario{"var a struct { b int }", "a.b"}, "selectStructMember", "ov struct { ov }", "_", "ov", []string{"a"}, "n struct { n }", "_"},
		{scenario{"var a struct { b int }", "a.b"}, "selectStructMemberNotStruct", "ov", "_", errorResult("non-struct"), []string{"a"}, "n struct { n }", "_"},
		{scenario{"type b struct { x, c int }\nvar a struct { b }", "a.c"}, "selectStructMemberEmbedded", "ov struct { ov struct { on; ov } }", "_", "ov", []string{"a"}, "n struct { n struct { n; n } }", "_"},
		{scenario{"type b struct { x, c int }\nvar a struct { *b }", "a.c"}, "selectStructMemberEmbeddedPointer", "ov struct { ov * ov struct { on; ov } }", "_", "ov", []string{"a"}, "n struct { n * v struct { n; v } }", "_"},
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
				st = st.Define(lhs.Name, newPermission(test.lhs))
			}
			if rhs != nil && test.rhs != nil {
				st = st.Define(rhs.Name, newPermission(test.rhs))
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
		st = st.Define("a", newPermission("om"))
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
