// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"go/parser"
	"strings"
	"testing"

	"github.com/julian-klode/lingolang/permission"
)

func recoverErrorOrFail(t *testing.T, message string) {
	if e := recover(); e == nil || !strings.Contains(fmt.Sprint(e), message) {
		t.Fatalf("Unexpected error -- %s -- expected it to contain -- %s --", e, message)
	}
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
