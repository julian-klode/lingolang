// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"go/ast"
	"go/parser"
	"testing"

	"github.com/julian-klode/lingolang/permission"
)

func TestExpr(t *testing.T) {
	st := NewStore()
	i := &Interpreter{}

	e, _ := parser.ParseExpr("a + a")
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
}
