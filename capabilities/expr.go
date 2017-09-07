// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"go/ast"

	"github.com/julian-klode/lingolang/permission"
)

// VisitExpr abstractly interprets permission changes by the expression.
func (i *Interpreter) VisitExpr(st Store, e ast.Expr) (permission.Permission, Store) {
	switch e := e.(type) {
	case *ast.BadExpr:
		panic("Bad expression")
	case *ast.BasicLit:
		panic("basic literal")
	case *ast.BinaryExpr:
		panic("binary")
	case *ast.CallExpr:
		panic("call")
	case *ast.CompositeLit:
		panic("composite literal")
	case *ast.FuncLit:
		panic("fun lit")
	case *ast.Ident:
		return st.GetEffective(e), st
	case *ast.IndexExpr:
		panic("index expr")
	case *ast.ParenExpr:
		return i.VisitExpr(st, e.X)
	case *ast.SelectorExpr:
		panic("index expr")
	case *ast.SliceExpr:
		panic("slice")
	case *ast.StarExpr:
		panic("star")
	case *ast.TypeAssertExpr:
		panic("type assert")
	case *ast.UnaryExpr:
		panic("unary")
	default:
		e.End()
	}
	return nil, nil
}
