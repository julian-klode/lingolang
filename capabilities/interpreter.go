// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/julian-klode/lingolang/permission"
)

// Interpreter interprets a given statement or expression.
type Interpreter struct {
	typesInfo            *types.Info
	curFunc              *permission.FuncPermission
	fset                 *token.FileSet
	AnnotatedPermissions map[ast.Expr]permission.Permission
	typeMapper           permission.TypeMapper
}

// Borrowed describes a variable that had to be borrowed from, along
// with it's associated original permission.
type Borrowed struct {
	id   *ast.Ident
	perm permission.Permission
}

// Owner is an alias for Borrowed indicating that this is the owning object of the expression result.
type Owner Borrowed

// NoOwner is the zero value for Owner.
var NoOwner = Owner{}

// VisitExpr abstractly interprets permission changes by the expression.
//
//
func (i *Interpreter) VisitExpr(st Store, e ast.Expr) (permission.Permission, Owner, []Borrowed, Store) {
	if e == nil {
		return permission.None, NoOwner, nil, st
	}
	switch e := e.(type) {
	case *ast.BasicLit:
		return i.visitBasicLit(st, e)
	case *ast.BinaryExpr:
		return i.visitBinaryExpr(st, e)
	case *ast.CallExpr:
		return i.visitCallExpr(st, e, false)
	case *ast.CompositeLit:
		return i.visitCompositeLit(st, e)
	case *ast.FuncLit:
		return i.visitFuncLit(st, e)
	case *ast.Ident:
		return i.visitIdent(st, e)
		//return st.Assign(e, to)
	case *ast.IndexExpr:
		return i.visitIndexExpr(st, e)
	case *ast.ParenExpr:
		return i.VisitExpr(st, e.X)
	case *ast.SelectorExpr:
		return i.visitSelectorExpr(st, e)
	case *ast.SliceExpr:
		return i.visitSliceExpr(st, e)
	case *ast.StarExpr:
		return i.visitStarExpr(st, e)
	case *ast.TypeAssertExpr:
		return i.Error(e, "type assertion not yet implemented")
	case *ast.UnaryExpr:
		return i.visitUnaryExpr(st, e)
	}
	return i.Error(e, "Reached a bad expression %v - this should not happen", e)
}

// visitExprNoOwner is like VisitExpr(), but merges the owner into the borrowed
func (i *Interpreter) visitExprNoOwner(st Store, e ast.Expr) (permission.Permission, []Borrowed, Store) {
	perm, owner, deps, st := i.VisitExpr(st, e)
	if owner != NoOwner {
		deps = append(deps, Borrowed(owner))
	}
	return perm, deps, st
}

// Release Releases all borrowed objects, and restores their previous permissions.
func (i *Interpreter) Release(node ast.Node, st Store, undo []Borrowed) Store {
	var err error
	for _, b := range undo {
		if b == Borrowed(NoOwner) {
			continue
		}
		st, err = st.SetEffective(b.id.Name, b.perm)
		if err != nil {
			i.Error(node, "Cannot release borrowed variable %s: %s", b.id, err)
		}
	}
	return st
}

// Assert asserts that the base permissions of subject are a superset or the same as has.
func (i *Interpreter) Error(node ast.Node, format string, args ...interface{}) (permission.Permission, Owner, []Borrowed, Store) {
	panic(fmt.Errorf("%v: In %s: %s", node.Pos(), node, fmt.Sprintf(format, args...)))
}

func (i *Interpreter) Assert(node ast.Node, subject permission.Permission, has permission.BasePermission) {
	if has&^subject.GetBasePermission() != 0 {
		i.Error(node, "Required permissions %s, but only have %s", has, subject)
	}
}

func (i *Interpreter) visitIdent(st Store, e *ast.Ident) (permission.Permission, Owner, []Borrowed, Store) {
	if e.Name == "nil" {
		return &permission.NilPermission{}, NoOwner, nil, st
	}
	if e.Name == "true" || e.Name == "false" {
		return permission.Mutable | permission.Owned, NoOwner, nil, st
	}
	perm := st.GetEffective(e.Name)
	if perm == nil {
		i.Error(e, "Cannot borow %s: Unknown variable in %s", e, st)
	}
	owner := Owner{e, perm}
	dead := permission.ConvertToBase(perm, permission.None)
	st, err := st.SetEffective(e.Name, dead)
	if err != nil {
		i.Error(e, "Cannot borrow identifier: %s", err)
	}
	return perm, owner, nil, st
}

func (i *Interpreter) moveOrCopy(e ast.Node, st Store, from, to permission.Permission, owner Owner, deps []Borrowed) (Store, Owner, []Borrowed, error) {
	switch {
	// If the value can be copied into the caller, we don't need to borrow it
	case permission.CopyableTo(from, to):
		st = i.Release(e, st, []Borrowed{Borrowed(owner)})
		st = i.Release(e, st, deps)
		owner = NoOwner
		deps = nil
	// The value cannot be moved either, error out.
	case !permission.MovableTo(from, to):
		return nil, NoOwner, nil, fmt.Errorf("Cannot copy or move: Needed %s, received %s", to, from)

	// All borrows for unowned parameters are released after the call is done.
	case to.GetBasePermission()&permission.Owned == 0:
	// Write and exclusive permissions are stripped when converting a value from linear to non-linear
	case permission.IsLinear(from) && !permission.IsLinear(to):
		if owner != NoOwner {
			owner.perm = permission.ConvertToBase(owner.perm, owner.perm.GetBasePermission()&^(permission.ExclRead|permission.ExclWrite|permission.Write))
		}
		st = i.Release(e, st, []Borrowed{Borrowed(owner)})
		st = i.Release(e, st, deps)
		owner = NoOwner
		deps = nil
	// The value was moved, so all its deps are lost
	default:
		deps = nil
		owner = NoOwner
	}

	return st, owner, deps, nil

}

// visitBinaryExpr - A binary expression is either logical, arithmetic, or a comparison.
func (i *Interpreter) visitBinaryExpr(st Store, e *ast.BinaryExpr) (permission.Permission, Owner, []Borrowed, Store) {
	var err error
	// We first evaluate the LHS, and then RHS after the LHS. If this is a logical
	// operator, the result is the intersection of LHS & (RHS after LHS), because
	// it might be possible that only LHS is evaluated. Otherwise, both sides are
	// always evaluated, so the result is RHS after LHS.
	lhs, ldeps, stl := i.visitExprNoOwner(st, e.X)
	stl = i.Release(e, stl, ldeps)
	i.Assert(e.X, lhs, permission.Read)

	rhs, rdeps, str := i.visitExprNoOwner(stl, e.Y)
	str = i.Release(e, str, rdeps)
	i.Assert(e.Y, rhs, permission.Read)

	switch e.Op {
	case token.LAND, token.LOR:
		st, err = stl.Merge(str)
		if err != nil {
			return i.Error(e, "Cannot merge different outcomes of logical operator")
		}
	default:
		st = str
	}

	return permission.Owned | permission.Mutable, NoOwner, nil, st
}

// An index expression has the form A[B] and needs read permissions for both A and
// B. A and B will be borrowed as needed. If used in a getting-way, we we could always
// treat B as unowned, but in A[B] = C, B might need to be moved into A, therefore both
// A and B will be dependencies of the result, at least for maps.
func (i *Interpreter) visitIndexExpr(st Store, e *ast.IndexExpr) (permission.Permission, Owner, []Borrowed, Store) {
	var err error

	p1, owner1, deps1, st := i.VisitExpr(st, e.X)
	p2, owner2, deps2, st := i.VisitExpr(st, e.Index)

	// Requires: LHS and RHS are readable.
	i.Assert(e.X, p1, permission.Read)
	i.Assert(e.Index, p2, permission.Read)

	switch p1 := p1.(type) {
	case *permission.ArrayPermission:
		// Ensures(array): Can only have integers here, no need to keep on to deps2
		st = i.Release(e, st, []Borrowed{Borrowed(owner2)})
		st = i.Release(e, st, deps2)
		return p1.ElementPermission, owner1, deps1, st
	case *permission.SlicePermission:
		// Ensures(slice): Can only have integers here, no need to keep on to deps2
		st = i.Release(e, st, []Borrowed{Borrowed(owner2)})
		st = i.Release(e, st, deps2)
		return p1.ElementPermission, owner1, deps1, st
	case *permission.MapPermission:
		// Ensures(map): If the key can be copied, we don't borrow it.
		st, owner2, deps2, err = i.moveOrCopy(e, st, p2, p1.KeyPermission, owner2, deps2)
		if err != nil {
			return i.Error(e, "Cannot move or copy from %s to %s: %s", p2, p1.KeyPermission, err)
		}
		return p1.ValuePermission, owner1, deps1, st
	}

	i.Error(e, "Indexing unknown type")
	return nil, NoOwner, nil, nil
}

func (i *Interpreter) visitStarExpr(st Store, e *ast.StarExpr) (permission.Permission, Owner, []Borrowed, Store) {
	p1, owner1, deps1, st := i.VisitExpr(st, e.X)
	i.Assert(e.X, p1, permission.Read)

	var typ types.Type

	if i.typesInfo != nil {
		typ = i.typesInfo.TypeOf(e.X)
	}

	switch p1 := p1.(type) {
	case *permission.PointerPermission:
		return p1.Target, owner1, deps1, st
	}

	return i.Error(e, "Trying to dereference non-pointer %v of type %v", p1, typ)
}
func (i *Interpreter) visitUnaryExpr(st Store, e *ast.UnaryExpr) (permission.Permission, Owner, []Borrowed, Store) {
	p1, owner1, deps1, st := i.VisitExpr(st, e.X)
	i.Assert(e.X, p1, permission.Read)

	switch e.Op {
	case token.AND:
		return &permission.PointerPermission{
			BasePermission: permission.Owned | permission.Mutable,
			Target:         p1}, owner1, deps1, st

	case token.ARROW:
		ch, ok := p1.(*permission.ChanPermission)
		if !ok {
			return i.Error(e.X, "Expected channel permission, received %v", ch)
		}
		st = i.Release(e, st, []Borrowed{Borrowed(owner1)})
		st = i.Release(e, st, deps1)
		return ch.ElementPermission, NoOwner, nil, st
	default:
		st = i.Release(e, st, []Borrowed{Borrowed(owner1)})
		st = i.Release(e, st, deps1)
		return permission.Owned | permission.Mutable, NoOwner, nil, st
	}
}

func (i *Interpreter) visitBasicLit(st Store, e *ast.BasicLit) (permission.Permission, Owner, []Borrowed, Store) {
	return permission.Owned | permission.Mutable, NoOwner, nil, st
}

func (i *Interpreter) visitCallExpr(st Store, e *ast.CallExpr, isDeferredOrGoroutine bool) (permission.Permission, Owner, []Borrowed, Store) {
	var err error

	fun, owner, funDeps, st := i.VisitExpr(st, e.Fun)

	var accumulatedUnownedDeps []Borrowed
	switch fun := fun.(type) {
	case *permission.FuncPermission:
		for j, arg := range e.Args {
			argPerm, argOwner, argDeps, store := i.VisitExpr(st, arg)
			st = store

			st, argOwner, argDeps, err = i.moveOrCopy(e, st, argPerm, fun.Params[j], argOwner, argDeps)
			if err != nil {
				return i.Error(arg, "Cannot copy or move to parameter: Needed %#v, received %#v", fun.Params[j], argPerm)
			}

			accumulatedUnownedDeps = append(accumulatedUnownedDeps, Borrowed(argOwner))
			accumulatedUnownedDeps = append(accumulatedUnownedDeps, argDeps...)
		}
		var deps []Borrowed
		if isDeferredOrGoroutine {
			deps = funDeps
			accumulatedUnownedDeps = nil
		} else {
			// Call is done, release function permissions
			st = i.Release(e, st, accumulatedUnownedDeps) // TODO(jak): Is order important?
			st = i.Release(e, st, funDeps)
			// For a normal function call, there's no point to hang on to the function owner.
			st = i.Release(e, st, []Borrowed{Borrowed(owner)})
			owner = NoOwner
		}

		if len(fun.Results) == 1 {
			return fun.Results[0], owner, deps, st
		}
		return &permission.TuplePermission{BasePermission: permission.Owned | permission.Mutable, Elements: fun.Results}, owner, deps, st
	default:
		return i.Error(e, "Cannot call non-function object %v", fun)
	}

}

func (i *Interpreter) visitSliceExpr(st Store, e *ast.SliceExpr) (permission.Permission, Owner, []Borrowed, Store) {
	arr, owner, arrDeps, st := i.VisitExpr(st, e.X)
	low, lowDeps, st := i.visitExprNoOwner(st, e.Low)
	high, highDeps, st := i.visitExprNoOwner(st, e.High)
	max, maxDeps, st := i.visitExprNoOwner(st, e.Max)

	if e.Low != nil {
		i.Assert(e.Low, low, permission.Read)
	}
	if e.High != nil {
		i.Assert(e.High, high, permission.Read)
	}
	if e.Max != nil {
		i.Assert(e.Max, max, permission.Read)
	}

	st = i.Release(e, st, maxDeps)
	st = i.Release(e, st, highDeps)
	st = i.Release(e, st, lowDeps)

	switch arr := arr.(type) {
	case *permission.ArrayPermission:
		return &permission.SlicePermission{BasePermission: permission.Owned | permission.Mutable, ElementPermission: arr.ElementPermission}, owner, arrDeps, st
	case *permission.SlicePermission:
		return arr, owner, arrDeps, st
	}
	return i.Error(e, "Cannot create slice of %v - not sliceable", arr)
}

func (i *Interpreter) visitSelectorExpr(st Store, e *ast.SelectorExpr) (permission.Permission, Owner, []Borrowed, Store) {
	selection := i.typesInfo.Selections[e]
	path := selection.Index()
	pathLen := len(path)
	lhs, owner, deps, st := i.VisitExpr(st, e.X)

	for depth, index := range path {
		kind := types.FieldVal
		if depth == pathLen-1 {
			kind = selection.Kind()
		}
		lhs, owner, deps, st = i.visitSelectorExprOne(st, e, lhs, index, kind, owner, deps)
	}
	return lhs, owner, deps, st
}

func (i *Interpreter) visitSelectorExprOne(st Store, e ast.Expr, p permission.Permission, index int, kind types.SelectionKind, owner Owner, deps []Borrowed) (permission.Permission, Owner, []Borrowed, Store) {
	var err error

	switch kind {
	case types.FieldVal:
		/* A field value might be accessed through a pointer, fix it */
		if ptr, ok := p.(*permission.PointerPermission); ok {
			p = ptr.Target
		}

		strct, ok := p.(*permission.StructPermission)
		if !ok {
			return i.Error(e, "Cannot read field %s of non-struct type %#v", index, p)
		}
		return strct.Fields[index], owner, deps, st
	case types.MethodVal:
		// TODO: NamedType
		switch p := p.(type) {
		case *permission.InterfacePermission:
			target := p.Methods[index].Receivers[0]
			if st, owner, deps, err = i.moveOrCopy(e, st, p, target, owner, deps); err != nil {
				return i.Error(e, spew.Sprintf("Cannot bind receiver: %s in %v", err, p))
			}

			perm := p.Methods[index]
			// If we are binding unowned, our function value must be unowned too.
			if perm.Receivers[0].GetBasePermission()&permission.Owned == 0 {
				perm = permission.ConvertToBase(perm, perm.GetBasePermission()&^permission.Owned).(*permission.FuncPermission)
			}

			return stripReceiver(perm), owner, deps, st
		default:
			return i.Error(e, "Incompatible or unknown type on left side of method value for index %d", index)
		}
	case types.MethodExpr:
		switch p := p.(type) {
		case *permission.InterfacePermission:
			st = i.Release(e, st, []Borrowed{Borrowed(owner)})
			st = i.Release(e, st, deps)
			return pushReceiverToParams(p.Methods[index]), NoOwner, nil, st
		default:
			return i.Error(e, "Incompatible or unknown type on left side of method value for index %d", index)
		}
	}
	return i.Error(e, "Invalid kind of selector expression")
}

// stripReceiver returns perm with an empty receiver list.
func stripReceiver(perm *permission.FuncPermission) *permission.FuncPermission {
	var perm2 permission.FuncPermission

	perm2.BasePermission = perm.BasePermission
	perm2.Name = perm.Name
	perm2.Params = perm.Params
	perm2.Results = perm.Results
	return &perm2
}

func pushReceiverToParams(perm *permission.FuncPermission) *permission.FuncPermission {
	var perm2 permission.FuncPermission

	perm2.BasePermission = perm.BasePermission
	perm2.Name = perm.Name
	perm2.Params = append(append([]permission.Permission(nil), perm.Receivers...), perm.Params...)
	perm2.Results = perm.Results
	return &perm2
}

func (i *Interpreter) visitCompositeLit(st Store, e *ast.CompositeLit) (permission.Permission, Owner, []Borrowed, Store) {
	var err error
	// TODO: Types should be stored differently, possibly just wrapped in a *permission.Type or something.
	typPermAsPerm, deps, st := i.visitExprNoOwner(st, e.Type)
	typPerm, ok := typPermAsPerm.(*permission.StructPermission)
	st = i.Release(e, st, deps)
	deps = nil
	if !ok {
		return i.Error(e.Type, "Expected struct permission when constructing value via composite literal")
	}

	if i.typesInfo == nil {
		return i.Error(e, "Need typesInfo to evaluate composite literals")
	}
	typAndVal, ok := i.typesInfo.Types[e]
	if !ok {
		return i.Error(e, "Could not find type for composite literal")
	}

	for index, value := range e.Elts {
		// Translate a key value expression to an index, value pair
		if kve, ok := value.(*ast.KeyValueExpr); ok {
			key, ok := kve.Key.(*ast.Ident)
			if !ok {
				return i.Error(kve, "No key found\n")
			}

			strct := typAndVal.Type.Underlying().(*types.Struct)
			for index = 0; index <= strct.NumFields(); index++ {
				if strct.NumFields() == index {
					return i.Error(kve, "Could not lookup key %v", key)
				}

				if strct.Field(index).Name() == key.Name {
					break
				}

			}
			value = kve.Value
		}

		valPerm, valDeps, store := i.visitExprNoOwner(st, value)
		st = store

		if st, _, valDeps, err = i.moveOrCopy(e, st, valPerm, typPerm.Fields[index], NoOwner, valDeps); err != nil {
			return i.Error(value, spew.Sprintf("Cannot bind field: %s in %v", err, typPerm.Fields[index]))
		}
		// FIXME(jak): This might conflict with some uses of dependencies which use A depends on B as B contains A.
		deps = append(deps, valDeps...)
	}
	return typPerm, NoOwner, deps, st
}

func (i *Interpreter) visitFuncLit(st Store, e *ast.FuncLit) (permission.Permission, Owner, []Borrowed, Store) {
	oldCurFunc := i.curFunc
	if i.typeMapper == nil {
		i.typeMapper = permission.NewTypeMapper()
	}
	typ := i.typesInfo.TypeOf(e).(*types.Signature)
	i.curFunc = i.typeMapper.NewFromType(typ).(*permission.FuncPermission)
	defer func() {
		i.curFunc = oldCurFunc
	}()
	return i.buildFunction(st, e, typ, e.Body)
}

func (i *Interpreter) buildFunction(st Store, node ast.Node, typ *types.Signature, body *ast.BlockStmt) (permission.Permission, Owner, []Borrowed, Store) {
	var deps []Borrowed
	var err error
	origStore := st
	perm := i.typeMapper.NewFromType(typ).(*permission.FuncPermission)
	perm.BasePermission |= permission.Owned

	st = st.BeginBlock()
	if len(perm.Receivers) > 0 {
		for _, recv := range perm.Receivers {
			st, err = st.Define(typ.Recv().Name(), recv)
			if err != nil {
				i.Error(node, "Cannot define receiver %s", err)
			}
		}
	}

	params := typ.Params()
	for j := 0; j < params.Len(); j++ {
		param := params.At(j)
		st, err = st.Define(param.Name(), perm.Params[j])
		if err != nil {
			i.Error(node, "Cannot define parameter %d called %s: %s", j, param.Name(), err)
		}
		log.Printf("Defined %s to %s", param.Name(), perm.Params[j])
	}

	exits := i.visitStmtList(st, body.List, false)
	st = append(NewStore(), origStore...)
	for _, exit := range exits {
		exit.Store = exit.Store.EndBlock()
		if len(exit.Store) != len(origStore) {
			i.Error(node, "Store in wrong state after exit: expected %d, received %d", len(origStore), len(exit.Store))
		}

		for j := range exit.Store {
			if exit.Store[j].name != origStore[j].name {
				i.Error(node, "Invalid behavior: Function literal changes name of %d from %s to %s", j, origStore[j].name, exit.Store[j].name)
			}
			if exit.Store[j].eff != origStore[j].eff {
				i.Error(node, "Invalid behavior: Function literal changes permission of borrowed value %s from %s to %s", exit.Store[j].name, origStore[j].eff, exit.Store[j].eff)
			}
			if exit.Store[j].eff == nil {
				continue
			}
			if exit.Store[j].uses <= origStore[j].uses {
				continue
			}
			log.Printf("Borrowing %s = %s", st[j].name, exit.Store[j].eff)
			deps = append(deps, Borrowed{ast.NewIdent(st[j].name), st[j].eff})
			st[j].eff = permission.ConvertToBase(st[j].eff, 0)
			log.Printf("Borrowed %s is now %s", st[j].name, st[j].eff)
			st[j].uses = exit.Store[j].uses

			if exit.Store[j].eff.GetBasePermission()&permission.Owned == 0 && perm.GetBasePermission()&permission.Owned != 0 {
				log.Printf("Converting function to unowned due to %s", exit.Store[j].name)
				perm = permission.ConvertToBase(perm, perm.GetBasePermission()&^permission.Owned).(*permission.FuncPermission)
			}
		}
	}

	log.Printf("Function %s has deps %s", perm, deps)

	return perm, NoOwner, deps, st
}

// StmtExit is a store with an optional field specifying any early exit from a block, like
// a return, goto, or a continue. The idea is simple: Each block handler checks if it should
// handle such a branch, and do that or pass it up to the upper layer.
type StmtExit struct {
	Store
	branch ast.Stmt // *ReturnStmt or *BranchStmt, or nil if normal exit
}

func (i *Interpreter) visitStmt(st Store, stmt ast.Stmt) []StmtExit {
	switch stmt := stmt.(type) {
	case nil:
		return []StmtExit{{st, nil}}
	case *ast.BlockStmt:
		return i.visitBlockStmt(st, stmt)
	case *ast.LabeledStmt:
		return i.visitLabeledStmt(st, stmt)
	case *ast.CaseClause:
		return i.visitCaseClause(st, stmt)
	case *ast.BranchStmt:
		return i.visitBranchStmt(st, stmt)
	case *ast.ExprStmt:
		return i.visitExprStmt(st, stmt)
	case *ast.IfStmt:
		return i.visitIfStmt(st, stmt)
	case *ast.ReturnStmt:
		return i.visitReturnStmt(st, stmt)
	case *ast.IncDecStmt:
		return i.visitIncDecStmt(st, stmt)
	case *ast.SendStmt:
		return i.visitSendStmt(st, stmt)
	case *ast.EmptyStmt:
		return i.visitEmptyStmt(st, stmt)
	case *ast.AssignStmt:
		return i.visitAssignStmt(st, stmt)
	case *ast.RangeStmt:
		return i.visitRangeStmt(st, stmt)
	case *ast.SwitchStmt:
		return i.visitSwitchStmt(st, stmt)
	case *ast.SelectStmt:
		return i.visitSelectStmt(st, stmt)
	case *ast.CommClause:
		return i.visitCommClause(st, stmt)
	case *ast.ForStmt:
		return i.visitForStmt(st, stmt)
	case *ast.DeferStmt:
		return i.visitDeferStmt(st, stmt)
	case *ast.GoStmt:
		return i.visitGoStmt(st, stmt)
	case *ast.DeclStmt:
		return i.visitDeclStmt(st, stmt)
	default:
		i.Error(stmt, "Unknown type of statement")
		panic(nil)
	}
}

func (i *Interpreter) visitBlockStmt(st Store, stmt *ast.BlockStmt) []StmtExit {
	st = st.BeginBlock()
	res := i.visitStmtList(st, stmt.List, false)
	for i := range res {
		res[i].Store = res[i].Store.EndBlock()
	}
	return res
}

func (i *Interpreter) visitCaseClause(st Store, stmt *ast.CaseClause) []StmtExit {
	var err error
	var mergedStore Store
	// List of alternatives A, B, C, ...
	for _, e := range stmt.List {
		perm, deps, store := i.visitExprNoOwner(st, e)
		st = store
		i.Assert(e, perm, permission.Read)
		st = i.Release(e, st, deps)
		if mergedStore, err = mergedStore.Merge(st); err != nil {
			i.Error(e, "Could not merge with previous results: %s", err)
		}
	}
	return i.visitStmtList(mergedStore, stmt.Body, false)
}

func (i *Interpreter) visitExprStmt(st Store, stmt *ast.ExprStmt) []StmtExit {
	_, deps, st := i.visitExprNoOwner(st, stmt.X)
	st = i.Release(stmt.X, st, deps)
	return []StmtExit{{st, nil}}
}

func (i *Interpreter) visitIfStmt(st Store, stmt *ast.IfStmt) []StmtExit {
	st = st.BeginBlock()
	exits := i.visitStmt(st, stmt.Init)
	if len(exits) != 1 {
		i.Error(stmt.Init, "Initializer to if statement has %d exits", len(exits))
	}
	st = exits[0].Store // assert len(exits) == 1
	perm, deps, st := i.visitExprNoOwner(st, stmt.Cond)
	i.Assert(stmt.Cond, perm, permission.Read)
	st = i.Release(stmt.Cond, st, deps)

	exitsThen := i.visitStmt(st, stmt.Body)
	exitsElse := i.visitStmt(st, stmt.Else)

	for i := range exitsThen {
		exitsThen[i].Store = exitsThen[i].Store.EndBlock()
	}
	for i := range exitsElse {
		exitsElse[i].Store = exitsElse[i].Store.EndBlock()
	}
	log.Printf("then a is now: %v", exitsThen[0].GetEffective("a"))
	log.Printf("else a is now: %v", exitsElse[0].GetEffective("a"))

	out := append(exitsThen, exitsElse...)

	if len(out) < 2 { // TODO: unreachable.
		i.Error(stmt, "If has less than 2 exits", len(exits))
	}

	return out
}

type work struct {
	Store
	int
}

type blockManager struct {
	todo  []work
	seen  []work
	exits []StmtExit
}

func (bm *blockManager) isDuplicate(start work) bool {
	for _, sn := range bm.seen {
		if sn.int == start.int && sn.Store.Equal(start.Store) {
			return true
		}
	}
	bm.seen = append(bm.seen, start)
	return false
}

func (bm *blockManager) nextWork() (work, Store) {
	last := (bm.todo)[len(bm.todo)-1]
	bm.todo = (bm.todo)[:len(bm.todo)-1]
	return last, last.Store
}

func (bm *blockManager) addWork(todo ...work) {
	for _, w := range todo {
		if !bm.isDuplicate(w) {
			bm.todo = append(bm.todo, w)
		}
	}
}

func (bm *blockManager) hasWork() bool {
	return len(bm.todo) > 0
}

func (bm *blockManager) addExit(exits ...StmtExit) {
	bm.exits = append(bm.exits, exits...)
}

func collectLabels(stmts []ast.Stmt) map[string]int {
	labels := make(map[string]int)
	for k, stmt := range stmts {
		if l, ok := stmt.(*ast.LabeledStmt); ok {
			labels[l.Label.Name] = k
		}
	}
	return labels
}

func (i *Interpreter) visitStmtList(initStore Store, stmts []ast.Stmt, isASwitch bool) []StmtExit {
	var bm blockManager

	if len(stmts) == 0 {
		return []StmtExit{{initStore, nil}}
	} else if isASwitch {
		bm.addExit(StmtExit{initStore, nil})
		for i := range stmts {
			bm.addWork(work{initStore, i})
		}
	} else {
		bm.addWork(work{initStore, 0})
	}
	labels := collectLabels(stmts)

	for bm.hasWork() {
		item, _ := bm.nextWork()

		log.Printf("Visiting statement %d of %d in %v", item.int, len(stmts), item.Store.GetEffective("a"))
		exits := i.visitStmt(item.Store, stmts[item.int])
		log.Printf("Leaving statement with %d exits at %d outputs and %d work", len(exits), len(bm.exits), len(bm.todo))
		for _, exit := range exits {
			switch branch := exit.branch.(type) {
			case nil:
				if len(stmts) > item.int+1 && !isASwitch {
					bm.addWork(work{exit.Store, item.int + 1})
				} else {
					bm.addExit(StmtExit{exit.Store, nil})
				}
			case *ast.ReturnStmt:
				bm.addExit(exit) // Always exits the block
			case *ast.BranchStmt:
				branchingThis := (branch.Label == nil || branch.Label.Name == "" /* | TODO current label */)
				switch {
				case isASwitch && branch.Tok == token.BREAK && branchingThis:
					bm.addExit(StmtExit{exit.Store, nil})
				case isASwitch && branch.Tok == token.FALLTHROUGH:
					bm.addWork(work{exit.Store, item.int + 1})
				case branch.Tok == token.GOTO:
					if target, ok := labels[branch.Label.Name]; ok {
						bm.addWork(work{exit.Store, target})
					} else {
						bm.addExit(exit)
					}
				default:
					bm.addExit(exit)
				}
			}
		}
		log.Printf("Left statement with %d exits at %d outputs and %d work", len(exits), len(bm.exits), len(bm.todo))
	}

	return bm.exits
}

func (i *Interpreter) visitBranchStmt(st Store, s *ast.BranchStmt) []StmtExit {
	return []StmtExit{{st, s}}
}

func (i *Interpreter) visitReturnStmt(st Store, s *ast.ReturnStmt) []StmtExit {
	if len(s.Results) != len(i.curFunc.Results) {
		i.Error(s, "Different numbers of return values")
	}

	for k := 0; k < len(s.Results); k++ {
		// TODO: Named return values are not accurately presented. We need to map index
		// to name in the interpreter (random name for unnamed results) and then look them
		// up in the store.
		perm, owner, deps, store := i.VisitExpr(st, s.Results[k])
		store, owner, _, err := i.moveOrCopy(s, store, perm, i.curFunc.Results[k], owner, deps)
		if err != nil {
			i.Error(s, "Cannot bind return value %d: %s", k, err)
		}
		st = store
	}
	// A return statement is a singular exit.
	return []StmtExit{{st, s}}
}

func (i *Interpreter) visitIncDecStmt(st Store, stmt *ast.IncDecStmt) []StmtExit {
	p, deps, st := i.visitExprNoOwner(st, stmt.X)
	i.Assert(stmt.X, p, permission.Read|permission.Write)
	st = i.Release(stmt.X, st, deps)
	return []StmtExit{{st, nil}}
}

func (i *Interpreter) visitSendStmt(st Store, stmt *ast.SendStmt) []StmtExit {
	chanRaw, chanDeps, st := i.visitExprNoOwner(st, stmt.Chan)
	chn, isChan := chanRaw.(*permission.ChanPermission)
	if !isChan {
		i.Error(stmt.Chan, "Expected channel, received %v", chanRaw)
	}
	i.Assert(stmt.Chan, chn, permission.Write)

	val, valOwner, valDeps, st := i.VisitExpr(st, stmt.Value)

	st, valOwner, valDeps, err := i.moveOrCopy(stmt, st, val, chn.ElementPermission, valOwner, valDeps)
	if err != nil {
		i.Error(stmt, "Cannot send value: %v", err)
	}

	st = i.Release(stmt.Value, st, []Borrowed{Borrowed(valOwner)})
	st = i.Release(stmt.Value, st, valDeps)
	st = i.Release(stmt.Chan, st, chanDeps)

	return []StmtExit{{st, nil}}

}

func (i *Interpreter) visitLabeledStmt(st Store, stmt *ast.LabeledStmt) []StmtExit {
	return i.visitStmt(st, stmt.Stmt)
}

func (i *Interpreter) visitEmptyStmt(st Store, stmt *ast.EmptyStmt) []StmtExit {
	return []StmtExit{{st, nil}}
}

func (i *Interpreter) visitAssignStmt(st Store, stmt *ast.AssignStmt) []StmtExit {
	return []StmtExit{{i.defineOrAssignMany(st, stmt, stmt.Lhs, stmt.Rhs, stmt.Tok == token.DEFINE, false), nil}}
}

func (i *Interpreter) defineOrAssignMany(st Store, stmt ast.Stmt, lhsExprs []ast.Expr, rhsExprs []ast.Expr, isDefine bool, allowUnowned bool) Store {
	var deps []Borrowed
	var rhs []permission.Permission
	if len(rhsExprs) == 1 && len(lhsExprs) > 1 {
		// These really can't have owners.
		rhs0, rdeps, store := i.visitExprNoOwner(st, rhsExprs[0])
		st = store
		tuple, ok := rhs0.(*permission.TuplePermission)
		if !ok {
			i.Error(stmt, "Left side of assignment has more than one element but right hand only one, expected it to be a tuple")
		}
		deps = append(deps, rdeps...)
		rhs = tuple.Elements
	} else {
		for _, expr := range rhsExprs {
			log.Printf("Visiting expr %#v in store %v", expr, st)
			perm, ownerThis, depsThis, store := i.VisitExpr(st, expr)
			log.Printf("Visited expr %#v in store %v", expr, st)
			st = store
			rhs = append(rhs, perm)

			// Screw this. This is basically creating a temporary copy or (non-temporary, really) move of the values, so we
			// can have stuff like a, b = b, a without it messing up.
			store, ownerThis, depsThis, err := i.moveOrCopy(expr, st, perm, perm, ownerThis, depsThis)
			st = store

			if err != nil {
				i.Error(expr, "Could not move value: %s", err)
			}

			deps = append(deps, Borrowed(ownerThis))
			deps = append(deps, depsThis...)
		}
	}

	// Fill up the RHS with zero values if it has less elements than the LHS. Used for var x, y int; for example.
	for elem := len(rhs); elem < len(lhsExprs); elem++ {
		var perm permission.Permission

		perm = i.typeMapper.NewFromType(i.typesInfo.TypeOf(lhsExprs[elem]))
		perm = permission.ConvertToBase(perm, perm.GetBasePermission()|permission.Owned) //FIXME

		rhs = append(rhs, perm)

	}
	if len(rhs) != len(lhsExprs) {
		i.Error(stmt, "Expected same number of arguments on both sides of assignment (or one function call on the right): Got rhs=%d lhs=%d", len(rhs), len(lhsExprs))
	}

	for j, lhs := range lhsExprs {
		st = i.defineOrAssign(st, stmt, lhs, rhs[j], isDefine, allowUnowned)
	}

	st = i.Release(stmt, st, deps)

	return st
}
func (i *Interpreter) defineOrAssign(st Store, stmt ast.Stmt, lhs ast.Expr, rhs permission.Permission, isDefine bool, allowUnowned bool) Store {
	var err error

	// Define or set the effective permission of the left hand side to the right hand side. In the latter case,
	// the effective permission will be restricted by the specified maximum (initial) permission.
	if ident, ok := lhs.(*ast.Ident); ok {
		if ident.Name == "_" {
			return st
		}
		if isDefine {
			log.Println("Defining", ident.Name)
			if ann, ok := i.AnnotatedPermissions[ident]; ok {
				if ann, err = permission.ConvertTo(rhs, ann); err != nil {
					st, err = st.Define(ident.Name, ann)
				}
			} else {
			}
			st, err = st.Define(ident.Name, rhs)
		} else {
			if permission.CopyableTo(rhs, st.GetMaximum(ident.Name)) {
				st, err = st.SetEffective(ident.Name, st.GetMaximum(ident.Name))
			} else {
				st, err = st.SetEffective(ident.Name, rhs)
			}
		}

		if err != nil {
			i.Error(lhs, "Could not assign or define: %s", err)
		}
	} else if isDefine {
		i.Error(lhs, "Cannot define: Left hand side is not an identifier")
	}

	// Ensure we can do the assignment. If the left hand side is an identifier, this should always be
	// true - it's either Defined to the same value, or set to something less than it in the previous block.

	perm, _, _ := i.visitExprNoOwner(st, lhs) // We just need to know permission, don't care about borrowing.
	if !allowUnowned {
		i.Assert(lhs, perm, permission.Owned) // Make sure it's owned, so we don't move unowned to it.
	}

	// Input deps are nil, so we can ignore them here.
	st, _, _, err = i.moveOrCopy(lhs, st, rhs, perm, NoOwner, nil)
	if err != nil {
		i.Error(lhs, "Could not assign or define: %s", err)
	}

	log.Println("Assigned", lhs, "in", st)

	return st
}

func (i *Interpreter) visitRangeStmt(initStore Store, stmt *ast.RangeStmt) (rangeExits []StmtExit) {
	var bm blockManager
	var canRelease = true

	bm.addExit(StmtExit{initStore, nil})

	// Evaluate the container specified on the right hand side.
	perm, deps, initStore := i.visitExprNoOwner(initStore, stmt.X)
	defer func() {
		// TODO: canRelease = true
		if canRelease {
			for j := range rangeExits {
				log.Printf("Releasing %s", deps)
				rangeExits[j].Store = i.Release(stmt, rangeExits[j].Store, deps)
			}
		}
	}()
	i.Assert(stmt.X, perm, permission.Read)
	log.Printf("Borrowed container, a is now %s", initStore.GetEffective("a"))

	var rkey permission.Permission
	var rval permission.Permission

	switch perm := perm.(type) {
	case *permission.ArrayPermission:
		rkey = permission.Mutable
		rval = perm.ElementPermission
	case *permission.SlicePermission:
		rkey = permission.Mutable
		rval = perm.ElementPermission
	case *permission.MapPermission:
		rkey = perm.KeyPermission
		rval = perm.ValuePermission
	}

	bm.addWork(work{initStore, 0})

	for bm.hasWork() {
		_, st := bm.nextWork()
		log.Printf("Iterating %s", st.GetEffective("a"))

		st = st.BeginBlock()
		if stmt.Key != nil {
			st = i.defineOrAssign(st, stmt, stmt.Key, rkey, stmt.Tok == token.DEFINE, stmt.Tok == token.DEFINE)
			if ident, ok := stmt.Key.(*ast.Ident); ok {
				log.Printf("Defined %s to %s", ident.Name, st.GetEffective(ident.Name))
				if ident.Name != "_" {
					canRelease = canRelease && (st.GetEffective(ident.Name).GetBasePermission()&permission.Owned == 0)
				}
			} else {
				canRelease = false
			}
		}
		if stmt.Value != nil {
			st = i.defineOrAssign(st, stmt, stmt.Value, rval, stmt.Tok == token.DEFINE, stmt.Tok == token.DEFINE)
			if ident, ok := stmt.Value.(*ast.Ident); ok {
				log.Printf("Defined %s to %s", ident.Name, st.GetEffective(ident.Name))
				if ident.Name != "_" {
					canRelease = canRelease && (st.GetEffective(ident.Name).GetBasePermission()&permission.Owned == 0)
				}
			} else {
				canRelease = false
			}
		}

		exits := i.visitStmt(st, stmt.Body)

		nextIterations, exits := i.endBlocksAndCollectLoopExits(exits, true)
		bm.addExit(exits...)
		// Each next iteration is also possible work. This might generate duplicate exits, but we have
		// to do it this way, as we might otherwise miss some exits
		for _, iter := range nextIterations {
			log.Printf("Appending output with a = %s", st.GetEffective("a"))
			bm.addExit(StmtExit{iter.Store, nil})
		}
		bm.addWork(nextIterations...)
	}

	log.Printf("Leaving range statement with %d exits", len(bm.exits))

	return bm.exits
}

// endBlocksAndCollectLoopExits splits a given set of block exits into exits out of the current loop (breaks, returns, etc)
// and further iterations of the loop.
//
// If endAllBlocks is true, all inputs will have EndBlock() called on them, otherwise, only the returned []StmtExit
// will have EndBlock() called on them.
func (i *Interpreter) endBlocksAndCollectLoopExits(exits []StmtExit, endAllBlocks bool) ([]work, []StmtExit) {
	var nextIterations []work
	var realExits []StmtExit
	if endAllBlocks {
		for j := range exits {
			exits[j].Store = exits[j].Store.EndBlock()
		}
	}

	for _, exit := range exits {
		switch branch := exit.branch.(type) {
		case nil:
			nextIterations = append(nextIterations, work{exit.Store, 0})
		case *ast.ReturnStmt:
			realExits = append(realExits, exit)
		case *ast.BranchStmt:
			branchingThis := branch.Label == nil || branch.Label.Name == "" /* | TODO current label */
			switch {
			case branch.Tok == token.BREAK && branchingThis:
				realExits = append(realExits, StmtExit{exit.Store, nil})
			case branch.Tok == token.CONTINUE && branchingThis:
				nextIterations = append(nextIterations, work{exit.Store, 0})
			default:
				realExits = append(realExits, exit)
			}
		}
	}
	if !endAllBlocks {
		for j := range realExits {
			realExits[j].Store = realExits[j].Store.EndBlock()
		}
	}

	return nextIterations, realExits
}

func (i *Interpreter) visitSwitchStmt(st Store, stmt *ast.SwitchStmt) []StmtExit {
	var exits []StmtExit

	st = st.BeginBlock()
	for _, exit := range i.visitStmt(st, stmt.Init) {
		st := exit.Store
		perm, deps, st := i.visitExprNoOwner(st, stmt.Tag)
		if stmt.Tag != nil {
			i.Assert(stmt.Tag, perm, permission.Read)
		}

		for _, exit := range i.visitStmtList(st, stmt.Body.List, true) {
			exit.Store = i.Release(stmt.Tag, exit.Store, deps)
			exits = append(exits, exit)
		}
	}

	for i := range exits {
		exits[i].Store = exits[i].Store.EndBlock()
	}
	return exits
}

func (i *Interpreter) visitSelectStmt(st Store, stmt *ast.SelectStmt) []StmtExit {
	var exits []StmtExit

	st = st.BeginBlock()

	for _, exit := range i.visitStmtList(st, stmt.Body.List, true) {
		exit.Store = exit.Store.EndBlock()
		exits = append(exits, exit)
	}

	return exits
}

func (i *Interpreter) visitCommClause(st Store, stmt *ast.CommClause) []StmtExit {
	var exits []StmtExit
	for _, e := range i.visitStmt(st, stmt.Comm) {
		exits = append(exits, i.visitStmtList(e.Store, stmt.Body, false)...)
	}
	return exits
}

func (i *Interpreter) visitForStmt(initStore Store, stmt *ast.ForStmt) (rangeExits []StmtExit) {
	var bm blockManager

	initStore = initStore.BeginBlock()

	// Evaluate the container specified on the right hand side.
	for _, entry := range i.visitStmt(initStore, stmt.Init) {
		if entry.branch != nil {
			i.Error(stmt.Init, "Initializer exits uncleanly")
		}
		bm.addWork(work{entry.Store, 0})
	}

	for bm.hasWork() {
		_, st := bm.nextWork()
		log.Printf("for: Told to iterate %v", st)
		// Check condition
		perm, deps, st := i.visitExprNoOwner(st, stmt.Cond)
		i.Assert(stmt.Cond, perm, permission.Read)
		st = i.Release(stmt.Cond, st, deps)
		// There might be no more items, exit
		bm.addExit(StmtExit{st, nil})

		exits := i.visitStmt(st, stmt.Body)

		nextIterations, exits := i.endBlocksAndCollectLoopExits(exits, false)
		log.Printf("for: Iteration has %d more works, %d more exits", len(nextIterations), len(exits))
		for _, nextIter := range nextIterations {
			for _, nextExit := range i.visitStmt(nextIter.Store, stmt.Post) {
				if nextExit.branch != nil {
					i.Error(stmt.Init, "Post exits uncleanly")
				}
				bm.addWork(work{nextExit.Store, 0})
			}
		}
		bm.addExit(exits...)
	}

	log.Printf("Leaving range statement with %d exits", len(bm.exits))

	return bm.exits
}

func (i *Interpreter) visitGoStmt(st Store, stmt *ast.GoStmt) []StmtExit {
	// Deps are gone
	perm, _, _, st := i.visitCallExpr(st, stmt.Call, true)
	i.Assert(stmt.Call, perm, permission.Read)
	return []StmtExit{{st, nil}}
}

func (i *Interpreter) visitDeferStmt(st Store, stmt *ast.DeferStmt) []StmtExit {
	// All deps are gone, except for captured unowned variables, they can be released
	// again, since they will by definition be available at the end of the function
	// when the call is to be executed.
	_, _, deps, st := i.visitCallExpr(st, stmt.Call, true)
	for _, dep := range deps {
		if dep.perm.GetBasePermission()&permission.Owned == 0 {
			st = i.Release(stmt.Call, st, []Borrowed{dep})
		}
	}

	return []StmtExit{{st, nil}}
}

func (i *Interpreter) visitDeclStmt(st Store, stmt *ast.DeclStmt) []StmtExit {
	decl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok {
		i.Error(stmt, "Expected general declaration")
	}
	if i.typeMapper == nil {
		i.typeMapper = permission.NewTypeMapper()
	}

	for _, spec := range decl.Specs {
		switch spec := spec.(type) {
		case *ast.ValueSpec:
			names := make([]ast.Expr, 0, len(spec.Names))
			for _, name := range spec.Names {
				names = append(names, name)
			}
			st = i.defineOrAssignMany(st, stmt, names, spec.Values, true, false)
		default:
			continue
		}
	}
	return []StmtExit{{st, nil}}
}
