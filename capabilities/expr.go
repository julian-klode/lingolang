// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"go/ast"

	"github.com/julian-klode/lingolang/permission"
)

// Borrowed describes a variable that had to be borrowed from, along
// with it's associated original permission.
type Borrowed struct {
	id   *ast.Ident
	perm permission.Permission
}

// VisitExpr abstractly interprets permission changes by the expression.
//
//
func (i *Interpreter) VisitExpr(st Store, e ast.Expr) (permission.Permission, []Borrowed, Store) {
	if e == nil {
		return permission.None, nil, st
	}
	switch e := e.(type) {
	case *ast.BadExpr:
		panic("Bad expression")
	case *ast.BasicLit:
		panic("basic literal")
	case *ast.BinaryExpr:
		return i.visitBinaryExpr(st, e)
	case *ast.CallExpr:
		panic("call")
	case *ast.CompositeLit:
		panic("composite literal")
	case *ast.FuncLit:
		panic("fun lit")
	case *ast.Ident:
		return i.visitIdent(st, e)
		//return st.Assign(e, to)
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
		panic("type Assert")
	case *ast.UnaryExpr:
		panic("unary")
	default:
		e.End()
	}
	return nil, nil, nil
}

// Release Releases all borrowed objects, and restores their previous permissions.
func (i *Interpreter) Release(node ast.Node, st Store, undo []Borrowed) Store {
	for _, b := range undo {
		st.SetEffective(b.id, b.perm)
	}
	return st
}

// Assert asserts that the base permissions of subject are a superset or the same as has.
func (i *Interpreter) Assert(node ast.Node, subject permission.Permission, has permission.BasePermission) {
	if has&^subject.GetBasePermission() != 0 {
		panic("Not good")
	}
}

func (i *Interpreter) visitIdent(st Store, e *ast.Ident) (permission.Permission, []Borrowed, Store) {
	perm := st.GetEffective(e)
	borrowed := []Borrowed{{e, perm}}
	st.SetEffective(e, &permission.WildcardPermission{})
	return perm, borrowed, st
}

// visitBinaryExpr - A binary expression is either logical, arithmetic, or a comparison.
func (i *Interpreter) visitBinaryExpr(st Store, e *ast.BinaryExpr) (permission.Permission, []Borrowed, Store) {
	lhs, ldeps, st := i.VisitExpr(st, e.X)
	rhs, rdeps, st := i.VisitExpr(st, e.Y)

	// Requires: LHS, RHS readable
	i.Assert(e.X, lhs, permission.Read)
	i.Assert(e.Y, rhs, permission.Read)
	// Ensures: Undo = nil
	st = i.Release(e, st, ldeps)
	st = i.Release(e, st, rdeps)

	return permission.Mutable, nil, st
}

// An index expression has the form A[B] and needs read permissions for both A and
// B. A and B will be borrowed as needed. If used in a getting-way, we we could always
// treat B as unowned, but in A[B] = C, B might need to be moved into A, therefore both
// A and B will be dependencies of the result, at least for maps.
func (i *Interpreter) visitIndexExpr(st Store, e *ast.IndexExpr) (permission.Permission, []Borrowed, Store) {
	p1, deps1, st := i.VisitExpr(st, e.X)
	p2, deps2, st := i.VisitExpr(st, e.Index)

	// Requires: LHS and RHS are readable.
	i.Assert(e.X, p1, permission.Read)
	i.Assert(e.Index, p2, permission.Read)

	switch p1 := p1.(type) {
	case *permission.ArrayPermission:
		// Ensures(array): Can only have integers here, no need to keep on to deps2
		st = i.Release(e, st, deps2)
		return p1.ElementPermission, deps1, st
	case *permission.SlicePermission:
		// Ensures(slice): Can only have integers here, no need to keep on to deps2
		st = i.Release(e, st, deps2)
		return p1.ElementPermission, deps1, st
	case *permission.MapPermission:
		if permission.CopyableTo(p2, p1.KeyPermission) || permission.MovableTo(p2, p1.KeyPermission) {
			return p1.ValuePermission, deps1, st
		}
	}

	panic("Indexing unknown type")
}
