// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"go/token"

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
		i.Error(e, "Bad expression")
	case *ast.BasicLit:
		i.Error(e, "basic literal")
	case *ast.BinaryExpr:
		return i.visitBinaryExpr(st, e)
	case *ast.CallExpr:
		i.Error(e, "call")
	case *ast.CompositeLit:
		i.Error(e, "composite literal")
	case *ast.FuncLit:
		i.Error(e, "fun lit")
	case *ast.Ident:
		return i.visitIdent(st, e)
		//return st.Assign(e, to)
	case *ast.IndexExpr:
		return i.visitIndexExpr(st, e)
	case *ast.ParenExpr:
		return i.VisitExpr(st, e.X)
	case *ast.SelectorExpr:
		i.Error(e, "index expr")
	case *ast.SliceExpr:
		i.Error(e, "slice")
	case *ast.StarExpr:
		return i.visitStarExpr(st, e)
	case *ast.TypeAssertExpr:
		i.Error(e, "type Assert")
	case *ast.UnaryExpr:
		return i.visitUnaryExpr(st, e)
	default:
		e.End()
	}
	return nil, nil, nil
}

// Release Releases all borrowed objects, and restores their previous permissions.
func (i *Interpreter) Release(node ast.Node, st Store, undo []Borrowed) Store {
	var err error
	for _, b := range undo {
		st, err = st.SetEffective(b.id, b.perm)
		if err != nil {
			i.Error(node, "Cannot release borrowed variable %s: %s", b.id, err)
		}
	}
	return st
}

// Assert asserts that the base permissions of subject are a superset or the same as has.
func (i *Interpreter) Error(node ast.Node, format string, args ...interface{}) (permission.Permission, []Borrowed, Store) {
	panic(fmt.Errorf("%v: In %s: %s", node.Pos(), node, fmt.Sprintf(format, args...)))
}

func (i *Interpreter) Assert(node ast.Node, subject permission.Permission, has permission.BasePermission) {
	if has&^subject.GetBasePermission() != 0 {
		i.Error(node, "Required permissions %s, but only have %s", has, subject)
	}
}

func (i *Interpreter) visitIdent(st Store, e *ast.Ident) (permission.Permission, []Borrowed, Store) {
	perm := st.GetEffective(e)
	if perm == nil {
		i.Error(e, "Cannot borow %s: Unknown variable", e)
	}
	borrowed := []Borrowed{{e, perm}}
	dead, err := permission.ConvertTo(perm, permission.None)
	if err != nil {
		i.Error(e, "Cannot borrow identifier: %s", err)
	}
	st, err = st.SetEffective(e, dead)
	if err != nil {
		i.Error(e, "Cannot borrow identifier: %s", err)
	}
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
	return permission.Owned | permission.Mutable, nil, st
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
		// Ensures(map): If the key can be copied, we don't borrow it.
		copy := permission.CopyableTo(p2, p1.KeyPermission)
		if copy {
			st = i.Release(e, st, deps2)
		}

		if copy || permission.MovableTo(p2, p1.KeyPermission) {
			return p1.ValuePermission, deps1, st
		}

		i.Error(e, "Cannot move or copy from %s to %s", p2, p1.KeyPermission)
	}

	i.Error(e, "Indexing unknown type")
	return nil, nil, nil
}

func (i *Interpreter) visitStarExpr(st Store, e *ast.StarExpr) (permission.Permission, []Borrowed, Store) {
	p1, deps1, st := i.VisitExpr(st, e.X)
	i.Assert(e.X, p1, permission.Read)

	switch p1 := p1.(type) {
	case *permission.PointerPermission:
		return p1.Target, deps1, st
	}

	return i.Error(e, "Trying to dereference non-pointer %v", p1)
}
func (i *Interpreter) visitUnaryExpr(st Store, e *ast.UnaryExpr) (permission.Permission, []Borrowed, Store) {
	p1, deps1, st := i.VisitExpr(st, e.X)
	i.Assert(e.X, p1, permission.Read)

	switch e.Op {
	case token.AND:
		return &permission.PointerPermission{
			BasePermission: permission.Owned | permission.Mutable,
			Target:         p1}, deps1, st
	default:
		st = i.Release(e, st, deps1)
		return permission.Owned | permission.Mutable, nil, st
	}
}
