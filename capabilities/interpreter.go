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
		return i.Error(e, "function literals not yet implemented")
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
			target := p.Methods[index].(*permission.FuncPermission).Receivers[0]
			if st, owner, deps, err = i.moveOrCopy(e, st, p, target, owner, deps); err != nil {
				return i.Error(e, spew.Sprintf("Cannot bind receiver: %s in %v", err, p))
			}

			perm := p.Methods[index].(*permission.FuncPermission)
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
			return pushReceiverToParams(p.Methods[index].(*permission.FuncPermission)), NoOwner, nil, st
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

func (i *Interpreter) visitStmtList(st Store, stmts []ast.Stmt, isASwitch bool) []StmtExit {
	if len(stmts) == 0 {
		return []StmtExit{{st, nil}}
	}
	labels := make(map[string]int)
	var work []struct {
		Store
		int
	}

	seen := []struct {
		Store
		int
	}{}

	addWork := func(st Store, pos int) {
		work = append(work, struct {
			Store
			int
		}{st, pos})
	}

	var output []StmtExit

	for k, stmt := range stmts {
		if l, ok := stmt.(*ast.LabeledStmt); ok {
			labels[l.Label.Name] = k
		}
	}

	if isASwitch {
		for i := range stmts {
			addWork(st, i)
		}
	} else {
		addWork(st, 0)
	}

nextWork:
	for len(work) != 0 {
		// We treat work as a stack. Semantically, it feels like a queue would be a better
		// fit, but it does not make any difference IRL, and by using a stack approach we
		// do not end up with indefinitely growing arrays (because we'd slice off of the first
		// element and then append a new one)
		start := work[len(work)-1]
		work = work[:len(work)-1]
		st := start.Store
		log.Printf("Visiting statement %d of %d in %v", start.int, len(stmts), st.GetEffective("a"))

		// Hey guys, we've already been here, discard that path.
		for _, sn := range seen {
			if sn.int == start.int && sn.Store.Equal(st) {
				log.Printf("Rejecting statement %d in store %v", start.int, st.GetEffective("a"))
				continue nextWork
			}
		}
		seen = append(seen, start)

		stmt := stmts[start.int]
		exits := i.visitStmt(st, stmt)

		log.Printf("Leaving statement with %d exits at %d outputs and %d work", len(exits), len(output), len(work))

		for _, exit := range exits {
			st := exit.Store
			switch branch := exit.branch.(type) {
			case nil:
				if len(stmts) > start.int+1 && !isASwitch {
					addWork(st, start.int+1)
				} else {
					output = append(output, StmtExit{st, nil})
				}
			case *ast.ReturnStmt:
				// This exits the block
				output = append(output, exit)

			case *ast.BranchStmt:
				switch {
				case isASwitch && branch.Tok == token.BREAK:
					if branch.Label == nil || branch.Label.Name == "" /* | TODO current label */ {
						output = append(output, StmtExit{exit.Store, nil})
					} else {
						output = append(output, exit)
					}
				case isASwitch && branch.Tok == token.FALLTHROUGH:
					addWork(st, start.int+1)
				case branch.Tok == token.GOTO:
					if target, ok := labels[branch.Label.Name]; ok {
						addWork(st, target)
					} else {
						output = append(output, exit)
					}
				default:
					output = append(output, exit)
				}
			}
		}
		log.Printf("Left statement with %d exits at %d outputs and %d work", len(exits), len(output), len(work))
	}

	return output
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
	var deps []Borrowed
	var rhs []permission.Permission
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
		// These really can't have owners.
		rhs0, rdeps, store := i.visitExprNoOwner(st, stmt.Rhs[0])
		st = store
		tuple, ok := rhs0.(*permission.TuplePermission)
		if !ok {
			i.Error(stmt, "Left side of assignment has more than one element but right hand only one, expected it to be a tuple")
		}
		deps = append(deps, rdeps...)
		rhs = tuple.Elements
	} else if len(stmt.Rhs) != len(stmt.Lhs) {
		i.Error(stmt, "Expected same number of arguments on both sides of assignment (or one function call on the right)")
	} else {
		for _, expr := range stmt.Rhs {
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

	if len(rhs) != len(stmt.Lhs) {
		i.Error(stmt, "Expected same number of arguments on both sides of assignment (or one function call on the right)")
	}

	for j, lhs := range stmt.Lhs {
		st = i.defineOrAssign(st, stmt, lhs, rhs[j], stmt.Tok == token.DEFINE, false)
	}

	st = i.Release(stmt, st, deps)

	return []StmtExit{{st, nil}}
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

func (i *Interpreter) visitRangeStmt(st Store, stmt *ast.RangeStmt) (rangeExits []StmtExit) {
	var seen []Store
	var work = []Store{}
	var output = []StmtExit{{st, nil}}
	var canRelease = true

	// Evaluate the container specified on the right hand side.
	perm, deps, st := i.visitExprNoOwner(st, stmt.X)
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
	log.Printf("Borrowed container, a is now %s", st.GetEffective("a"))

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

	work = append(work, st)
nextWork:
	for iter := 0; len(work) != 0; iter++ {
		// We treat work as a stack. Semantically, it feels like a queue would be a better
		// fit, but it does not make any difference IRL, and by using a stack approach we
		// do not end up with indefinitely growing arrays (because we'd slice off of the first
		// element and then append a new one)
		st = work[len(work)-1]
		work = work[:len(work)-1]

		log.Printf("Iterating %s", st.GetEffective("a"))
		/* Exit helper */
		if iter == 42 {
			i.Error(stmt, "Too many loops, aborting")
		}

		// There might be no more items, exit
		if iter > 0 {
			output = append(output, StmtExit{st, nil})
		}

		for _, sn := range seen {
			if sn.Equal(st) {
				continue nextWork
			}
		}

		seen = append(seen, st)

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

		nextIterations, exits := i.endBlocksAndCollectLoopExits(i.visitStmt(st, stmt.Body), true)
		output = append(output, exits...)
		work = append(work, nextIterations...)
	}

	log.Printf("Leaving range statement with %d exits", len(output))

	return output
}

// endBlocksAndCollectLoopExits splits a given set of block exits into exits out of the current loop (breaks, returns, etc)
// and further iterations of the loop.
//
// If endAllBlocks is true, all inputs will have EndBlock() called on them, otherwise, only the returned []StmtExit
// will have EndBlock() called on them.
func (i *Interpreter) endBlocksAndCollectLoopExits(exits []StmtExit, endAllBlocks bool) ([]Store, []StmtExit) {
	var nextIterations []Store
	var realExits []StmtExit
	if endAllBlocks {
		for j := range exits {
			exits[j].Store = exits[j].Store.EndBlock()
		}
	}
	log.Printf("VIsiting exits")
	for _, exit := range exits {
		switch branch := exit.branch.(type) {
		case nil:
			nextIterations = append(nextIterations, exit.Store)
		case *ast.ReturnStmt:
			realExits = append(realExits, exit)
		case *ast.BranchStmt:
			switch branch.Tok {
			case token.BREAK:
				if branch.Label == nil || branch.Label.Name == "" /* | TODO current label */ {
					realExits = append(realExits, StmtExit{exit.Store, nil})
				} else {
					realExits = append(realExits, exit)
				}
			case token.CONTINUE:
				if branch.Label == nil || branch.Label.Name == "" /* | TODO current label */ {
					nextIterations = append(nextIterations, exit.Store)
				} else {
					realExits = append(realExits, exit)
				}
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
		exits = append(exits, exit)
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

	exits = append(exits, StmtExit{st, nil})
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

func (i *Interpreter) visitForStmt(st Store, stmt *ast.ForStmt) (rangeExits []StmtExit) {
	var seen []Store
	var work = []Store{}
	var output = []StmtExit{}

	st = st.BeginBlock()

	// Evaluate the container specified on the right hand side.
	for _, entry := range i.visitStmt(st, stmt.Init) {
		if entry.branch != nil {
			i.Error(stmt.Init, "Initializer exits uncleanly")
		}
		work = append(work, entry.Store)
	}

nextWork:
	for iter := 0; len(work) != 0; iter++ {
		// We treat work as a stack. Semantically, it feels like a queue would be a better
		// fit, but it does not make any difference IRL, and by using a stack approach we
		// do not end up with indefinitely growing arrays (because we'd slice off of the first
		// element and then append a new one)
		st := work[len(work)-1]
		work = work[:len(work)-1]

		log.Printf("for: Told to iterate %v", st)
		/* Exit helper */
		if iter == 42 {
			i.Error(stmt, "Too many loops, aborting")
		}

		for _, sn := range seen {
			if sn.Equal(st) {
				log.Printf("for: Skipping to iterate %v", st)
				continue nextWork
			}
		}

		seen = append(seen, st)

		// Check condition
		perm, deps, st := i.visitExprNoOwner(st, stmt.Cond)
		i.Assert(stmt.Cond, perm, permission.Read)
		st = i.Release(stmt.Cond, st, deps)

		// There might be no more items, exit
		output = append(output, StmtExit{st, nil})

		nextIterations, exits := i.endBlocksAndCollectLoopExits(i.visitStmt(st, stmt.Body), false)

		log.Printf("for: Iteration has %d more works", len(nextIterations))
		log.Printf("for: Iteration has %d more exits", len(exits))

		for _, nextIter := range nextIterations {
			for _, nextExit := range i.visitStmt(nextIter, stmt.Post) {
				if nextExit.branch != nil {
					i.Error(stmt.Init, "Post exits uncleanly")
				}
				work = append(work, nextExit.Store)
			}
		}

		output = append(output, exits...)
	}

	log.Printf("Leaving range statement with %d exits", len(output))

	return output
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
