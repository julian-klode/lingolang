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

func TestVisitBinaryExpr(t *testing.T) {
	st := NewStore()
	i := &Interpreter{}
	e, _ := parser.ParseExpr("a + b")

	t.Run("ok", func(t *testing.T) {
		st = st.Define(e.(*ast.BinaryExpr).X.(*ast.Ident), permission.Read)
		st = st.Define(e.(*ast.BinaryExpr).Y.(*ast.Ident), permission.Read)

		perm, deps, store := i.VisitExpr(st, e)

		if len(deps) != 0 {
			t.Errorf("Expected no dependencies, received %v", deps)
		}
		if perm != permission.Mutable {
			t.Errorf("Expected mutable, received %v", perm)
		}
		if !store.Equal(st) {
			t.Errorf("Expected no change to store but was %v and is %v", st, store)
		}
	})

	t.Run("lhsUnreadable", func(t *testing.T) {
		defer recoverErrorOrFail(t, "In a: Required permissions r, but only have w")

		st = st.Define(e.(*ast.BinaryExpr).X.(*ast.Ident), permission.Write)
		st = st.Define(e.(*ast.BinaryExpr).Y.(*ast.Ident), permission.Read)

		i.VisitExpr(st, e)
	})
	t.Run("rhsUnreadable", func(t *testing.T) {
		defer recoverErrorOrFail(t, "In b: Required permissions r, but only have w")

		st = st.Define(e.(*ast.BinaryExpr).X.(*ast.Ident), permission.Read)
		st = st.Define(e.(*ast.BinaryExpr).Y.(*ast.Ident), permission.Write)

		i.VisitExpr(st, e)
	})
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

type errorResult string

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

func TestVisitIndexExpr(t *testing.T) {
	testCases := []struct {
		name         string
		lhs          interface{}
		rhs          interface{}
		result       interface{}
		dependencies []string
		lhsAfter     interface{}
		rhsAfter     interface{}
	}{
		{"mutableSlice", "om[]om", "om", "om", []string{"a"}, "n[]n", "om"},
		{"mutableArray", "om[_]om", "om", "om", []string{"a"}, "n[_]n", "om"},
		{"mutableMap", "om map[om]om", "om", "om", []string{"a"}, "n map[n]n", "om"},
		// mutable map, non-copyable key: Item was moved into the map, it's gone now.
		{"mutableMapNoCopyKey", "om map[om * om]om", "om * om", "om", []string{"a"}, "n map[n * r]n", "n * r"},
		// a regular writable pointer is copyable. but beware: that's unsafe.
		{"mutableMapCopyablePointerKey", "om map[orw * orw]om", "orw * orw", "om", []string{"a"}, "n map[n * rw]n", "orw * orw"},
		// we pass a mutable key where we only need r/o, the key is consumed.
		{"mutableMapFreeze", "om map[or * or]om", "om * om", "om", []string{"a"}, "n map[n * r]n", "n * r"},
		{"notIndexable", "or", "ov", errorResult("Indexing unknown"), nil, "", ""},
		{"keyNotReadable", "om[] om", "ow", errorResult("Required permission"), nil, "", ""},
		{"indexableNotReadable", "on[] on", "or", errorResult("Required permission"), nil, "", ""},
		{"mutableMapInvalidKey", "or map[om * om]ov", "ov * ov", errorResult("move or copy"), nil, "", ""},
	}

	st := NewStore()
	i := &Interpreter{}
	e, _ := parser.ParseExpr("a[b]")
	lhs := e.(*ast.IndexExpr).X.(*ast.Ident)
	rhs := e.(*ast.IndexExpr).Index.(*ast.Ident)

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			st = st.Define(lhs, newPermission(test.lhs))
			st = st.Define(rhs, newPermission(test.rhs))

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

			if !reflect.DeepEqual(store.GetEffective(lhs), newPermission(test.lhsAfter)) {
				t.Errorf("Found lhs after = %v, expected %v", store.GetEffective(lhs), newPermission(test.lhsAfter))
			}
			if !reflect.DeepEqual(store.GetEffective(rhs), newPermission(test.rhsAfter)) {
				t.Errorf("Found lhs after = %v, expected %v", store.GetEffective(rhs), newPermission(test.rhsAfter))
			}
		})

	}
}

func TestVisitStarExpr(t *testing.T) {
	testCases := []struct {
		name         string
		lhs          interface{}
		rhs          interface{}
		result       interface{}
		dependencies []string
		lhsAfter     interface{}
		rhsAfter     interface{}
	}{
		{"mutablePointer", "", "om * om", "om", []string{"b"}, "", "n * r"},
		{"mutablePointerReadTarget", "", "om * or", "or", []string{"b"}, "", "n * r"},
		{"readOnlyPointer", "", "or * or", "or", []string{"b"}, "", "n * r"},
		{"noPointer", "", "or", errorResult("non-pointer"), []string{"b"}, "", "n * r"},
	}

	st := NewStore()
	i := &Interpreter{}
	e, _ := parser.ParseExpr("*b")
	rhs := e.(*ast.StarExpr).X.(*ast.Ident)

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			st = st.Define(rhs, newPermission(test.rhs))

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

			if !reflect.DeepEqual(store.GetEffective(rhs), newPermission(test.rhsAfter)) {
				t.Errorf("Found lhs after = %v, expected %v", store.GetEffective(rhs), newPermission(test.rhsAfter))
			}
		})

	}
}
