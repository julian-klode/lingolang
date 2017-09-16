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
	}
	panic("Not reachable")
}

func TestVisitIdent(t *testing.T) {
	st := Store{
		{ast.NewIdent("x"), newPermission("om[]om"), newPermission("om")},
	}
	i := &Interpreter{}
	runFuncRecover(t, "Unknown variable", func() {
		i.VisitExpr(st, ast.NewIdent("a"))
	})
	runFuncRecover(t, "Cannot restrict effective permission", func() {
		i.VisitExpr(st, st[0].ident)
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

		// Function calls
		{"a(b)", "call1", "om func(om * om) or", "om * om", "or", []string{}, "om func(om * om) or", "n * r"},
		{"a(b)", "call1", "om func(om) or", "om", "or", []string{}, "om func(om) or", "om"},
		{"a(b)", "call1", "om func(om)", "om", "n", []string{}, "om func(om)", "om"},
		{"a(b)", "call1", "om func(om) n", "om", "n", []string{}, "om func(om) n", "om"},
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
				st = st.Define(lhs, newPermission(test.lhs))
			}
			if rhs != nil {
				st = st.Define(rhs, newPermission(test.rhs))
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

			if lhs != nil && !reflect.DeepEqual(store.GetEffective(lhs), newPermission(test.lhsAfter)) {
				t.Errorf("Found lhs after = %v, expected %v", store.GetEffective(lhs), newPermission(test.lhsAfter))
			}
			if rhs != nil && !reflect.DeepEqual(store.GetEffective(rhs), newPermission(test.rhsAfter)) {
				t.Errorf("Found rhs after = %v, expected %v", store.GetEffective(rhs), newPermission(test.rhsAfter))
			}
		})

	}
}
