// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"go/parser"
	"reflect"
	"strings"
	"testing"

	"github.com/julian-klode/lingolang/permission"
)

func recoverErrorOrFail(t *testing.T, message string) {
	if e := recover(); e == nil || !strings.Contains(fmt.Sprint(e), message) {
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
	panic("Not reachable")
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

func TestVisitExpr(t *testing.T) {
	type errorResult string

	testCases := []struct {
		expr         string
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
		{"a.b", "selector", "om", "om", errorResult("not yet implemented"), nil, nil, nil},
		{"a.(b)", "selector", "om", "om", errorResult("not yet implemented"), nil, nil, nil},
	}

	for _, test := range testCases {

		t.Run(test.name, func(t *testing.T) {
			st := NewStore()
			i := &Interpreter{}

			var lhs *ast.Ident
			var rhs *ast.Ident
			e, _ := parser.ParseExpr(test.expr)
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

			if lhs != nil {
				st = st.Define(lhs.Name, newPermission(test.lhs))
			}
			if rhs != nil {
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
				t.Errorf("Found lhs after = %v, expected %v", store.GetEffective(lhs.Name), newPermission(test.lhsAfter))
			}
			if rhs != nil && !reflect.DeepEqual(store.GetEffective(rhs.Name), newPermission(test.rhsAfter)) {
				t.Errorf("Found rhs after = %v, expected %v", store.GetEffective(rhs.Name), newPermission(test.rhsAfter))
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
