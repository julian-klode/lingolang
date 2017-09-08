// +build ignore
// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"go/ast"

	"github.com/julian-klode/lingolang/permission"
)

// VisitExpr abstractly interprets permission changes by the expression.
//
//
func (i *Interpreter) VisitExpr(st Store, e ast.Expr, to permission.Permission) (permission.Permission, Store) {
	if e == nil {
		return permission.None, st
	}
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
		return st.Assign(e, to)
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
	return nil, nil, nil
}

func (i *Interpreter) VisitIndexExpr(st Store, e *ast.IndexExpr, p permission.Permission) (permission.Permission, []*ast.Ident, Store) {
	p1, st := i.VisitExpr(st, e.X, p.)
	p2, st := i.VisitExpr(st, e.Index)

	switch p1 := p1.(type) {
	case *permission.ArrayPermission:
		return p1.ElementPermission, deps1, st
	case *permission.SlicePermission:
		return p1.ElementPermission, deps1, st
	case *permission.MapPermission:
		if permission.CopyableTo(p2, p1.KeyPermission) {
			return p1.ValuePermission, deps1, st
		} else if permission.MovableTo(p2, p1.KeyPermission) {
			return p1.ValuePermission, append(deps1, deps2...), st
		}
	}

	panic("Indexing unknown type")
}
