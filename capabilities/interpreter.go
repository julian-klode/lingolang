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
	typesInfo *types.Info
	curFunc   *permission.FuncPermission
}

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
	case *ast.BasicLit:
		return i.visitBasicLit(st, e)
	case *ast.BinaryExpr:
		return i.visitBinaryExpr(st, e)
	case *ast.CallExpr:
		return i.visitCallExpr(st, e)
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

// Release Releases all borrowed objects, and restores their previous permissions.
func (i *Interpreter) Release(node ast.Node, st Store, undo []Borrowed) Store {
	var err error
	for _, b := range undo {
		st, err = st.SetEffective(b.id.Name, b.perm)
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
	if e.Name == "nil" {
		return &permission.NilPermission{}, nil, st
	}
	if e.Name == "true" || e.Name == "false" {
		return permission.Mutable | permission.Owned, nil, st
	}
	perm := st.GetEffective(e.Name)
	if perm == nil {
		i.Error(e, "Cannot borow %s: Unknown variable", e)
	}
	borrowed := []Borrowed{{e, perm}}
	dead := permission.ConvertToBase(perm, permission.None)
	st, err := st.SetEffective(e.Name, dead)
	if err != nil {
		i.Error(e, "Cannot borrow identifier: %s", err)
	}
	return perm, borrowed, st
}

func (i *Interpreter) moveOrCopy(e ast.Node, st Store, from, to permission.Permission, deps []Borrowed) (Store, []Borrowed, error) {
	switch {
	// If the value can be copied into the caller, we don't need to borrow it
	case permission.CopyableTo(from, to):
		st = i.Release(e, st, deps)
		deps = nil
	// The value cannot be moved either, error out.
	case !permission.MovableTo(from, to):
		return nil, nil, fmt.Errorf("Cannot copy or move: Needed %#v, received %#v", to, from)

	// All borrows for unowned parameters are released after the call is done.
	case to.GetBasePermission()&permission.Owned == 0:
	// Write and exclusive permissions are stripped when converting a value from linear to non-linear
	case permission.IsLinear(from) && !permission.IsLinear(to):
		for i := range deps {
			deps[i].perm = permission.ConvertToBase(deps[i].perm, deps[i].perm.GetBasePermission()&^(permission.ExclRead|permission.ExclWrite|permission.Write))
		}
		st = i.Release(e, st, deps)
		deps = nil
	// The value was moved, so all its deps are lost
	default:
		deps = nil
	}

	return st, deps, nil

}

// visitBinaryExpr - A binary expression is either logical, arithmetic, or a comparison.
func (i *Interpreter) visitBinaryExpr(st Store, e *ast.BinaryExpr) (permission.Permission, []Borrowed, Store) {
	var err error
	// We first evaluate the LHS, and then RHS after the LHS. If this is a logical
	// operator, the result is the intersection of LHS & (RHS after LHS), because
	// it might be possible that only LHS is evaluated. Otherwise, both sides are
	// always evaluated, so the result is RHS after LHS.
	lhs, ldeps, stl := i.VisitExpr(st, e.X)
	stl = i.Release(e, stl, ldeps)
	i.Assert(e.X, lhs, permission.Read)

	rhs, rdeps, str := i.VisitExpr(stl, e.Y)
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

	return permission.Owned | permission.Mutable, nil, st
}

// An index expression has the form A[B] and needs read permissions for both A and
// B. A and B will be borrowed as needed. If used in a getting-way, we we could always
// treat B as unowned, but in A[B] = C, B might need to be moved into A, therefore both
// A and B will be dependencies of the result, at least for maps.
func (i *Interpreter) visitIndexExpr(st Store, e *ast.IndexExpr) (permission.Permission, []Borrowed, Store) {
	var err error

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

		st, deps2, err = i.moveOrCopy(e, st, p2, p1.KeyPermission, deps2)
		if err != nil {
			return i.Error(e, "Cannot move or copy from %s to %s: %s", p2, p1.KeyPermission, err)
		}
		return p1.ValuePermission, deps1, st
	}

	i.Error(e, "Indexing unknown type")
	return nil, nil, nil
}

func (i *Interpreter) visitStarExpr(st Store, e *ast.StarExpr) (permission.Permission, []Borrowed, Store) {
	p1, deps1, st := i.VisitExpr(st, e.X)
	i.Assert(e.X, p1, permission.Read)

	var typ types.Type

	if i.typesInfo != nil {
		typ = i.typesInfo.TypeOf(e.X)
	}

	switch p1 := p1.(type) {
	case *permission.PointerPermission:
		return p1.Target, deps1, st
	}

	return i.Error(e, "Trying to dereference non-pointer %v of type %v", p1, typ)
}
func (i *Interpreter) visitUnaryExpr(st Store, e *ast.UnaryExpr) (permission.Permission, []Borrowed, Store) {
	p1, deps1, st := i.VisitExpr(st, e.X)
	i.Assert(e.X, p1, permission.Read)

	switch e.Op {
	case token.AND:
		return &permission.PointerPermission{
			BasePermission: permission.Owned | permission.Mutable,
			Target:         p1}, deps1, st

	case token.ARROW:
		ch, ok := p1.(*permission.ChanPermission)
		if !ok {
			return i.Error(e.X, "Expected channel permission, received %v", ch)
		}
		st = i.Release(e, st, deps1)
		return ch.ElementPermission, nil, st
	default:
		st = i.Release(e, st, deps1)
		return permission.Owned | permission.Mutable, nil, st
	}
}

func (i *Interpreter) visitBasicLit(st Store, e *ast.BasicLit) (permission.Permission, []Borrowed, Store) {
	return permission.Owned | permission.Mutable, nil, st
}

func (i *Interpreter) visitCallExpr(st Store, e *ast.CallExpr) (permission.Permission, []Borrowed, Store) {
	var err error

	fun, funDeps, st := i.VisitExpr(st, e.Fun)

	var accumulatedUnownedDeps []Borrowed
	switch fun := fun.(type) {
	case *permission.FuncPermission:
		for j, arg := range e.Args {
			argPerm, argDeps, store := i.VisitExpr(st, arg)
			st = store

			st, argDeps, err = i.moveOrCopy(e, st, argPerm, fun.Params[j], argDeps)
			if err != nil {
				return i.Error(arg, "Cannot copy or move to parameter: Needed %#v, received %#v", fun.Params[j], argPerm)
			}

			accumulatedUnownedDeps = append(accumulatedUnownedDeps, argDeps...)
		}

		// Call is done, release function permissions
		st = i.Release(e, st, accumulatedUnownedDeps) // TODO(jak): Is order important?
		st = i.Release(e, st, funDeps)

		if len(fun.Results) == 1 {
			return fun.Results[0], nil, st
		}
		return &permission.TuplePermission{BasePermission: permission.Owned | permission.Mutable, Elements: fun.Results}, nil, st
	default:
		return i.Error(e, "Cannot call non-function object %v", fun)
	}

}

func (i *Interpreter) visitSliceExpr(st Store, e *ast.SliceExpr) (permission.Permission, []Borrowed, Store) {
	arr, arrDeps, st := i.VisitExpr(st, e.X)
	low, lowDeps, st := i.VisitExpr(st, e.Low)
	high, highDeps, st := i.VisitExpr(st, e.High)
	max, maxDeps, st := i.VisitExpr(st, e.Max)

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
		return &permission.SlicePermission{BasePermission: permission.Owned | permission.Mutable, ElementPermission: arr.ElementPermission}, arrDeps, st
	case *permission.SlicePermission:
		return arr, arrDeps, st
	}
	return i.Error(e, "Cannot create slice of %v - not sliceable", arr)
}

func (i *Interpreter) visitSelectorExpr(st Store, e *ast.SelectorExpr) (permission.Permission, []Borrowed, Store) {
	selection := i.typesInfo.Selections[e]
	path := selection.Index()
	pathLen := len(path)
	lhs, deps, st := i.VisitExpr(st, e.X)

	for depth, index := range path {
		kind := types.FieldVal
		if depth == pathLen-1 {
			kind = selection.Kind()
		}
		lhs, deps, st = i.visitSelectorExprOne(st, e, lhs, index, kind, deps)
	}
	return lhs, deps, st
}

func (i *Interpreter) visitSelectorExprOne(st Store, e ast.Expr, p permission.Permission, index int, kind types.SelectionKind, deps []Borrowed) (permission.Permission, []Borrowed, Store) {
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
		return strct.Fields[index], deps, st
	case types.MethodVal:
		// TODO: NamedType
		switch p := p.(type) {
		case *permission.InterfacePermission:
			target := p.Methods[index].(*permission.FuncPermission).Receivers[0]
			if st, deps, err = i.moveOrCopy(e, st, p, target, deps); err != nil {
				return i.Error(e, spew.Sprintf("Cannot bind receiver: %s in %v", err, p))
			}

			perm := p.Methods[index].(*permission.FuncPermission)
			// If we are binding unowned, our function value must be unowned too.
			if perm.Receivers[0].GetBasePermission()&permission.Owned == 0 {
				perm = permission.ConvertToBase(perm, perm.GetBasePermission()&^permission.Owned).(*permission.FuncPermission)
			}

			return stripReceiver(perm), deps, st
		default:
			return i.Error(e, "Incompatible or unknown type on left side of method value for index %d", index)
		}
	case types.MethodExpr:
		switch p := p.(type) {
		case *permission.InterfacePermission:
			st = i.Release(e, st, deps)
			return pushReceiverToParams(p.Methods[index].(*permission.FuncPermission)), nil, st
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

func (i *Interpreter) visitCompositeLit(st Store, e *ast.CompositeLit) (permission.Permission, []Borrowed, Store) {
	var err error
	// TODO: Types should be stored differently, possibly just wrapped in a *permission.Type or something.
	typPermAsPerm, deps, st := i.VisitExpr(st, e.Type)
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

		valPerm, valDeps, store := i.VisitExpr(st, value)
		st = store

		if st, valDeps, err = i.moveOrCopy(e, st, valPerm, typPerm.Fields[index], valDeps); err != nil {
			return i.Error(value, spew.Sprintf("Cannot bind field: %s in %v", err, typPerm.Fields[index]))
		}
		// FIXME(jak): This might conflict with some uses of dependencies which use A depends on B as B contains A.
		deps = append(deps, valDeps...)
	}
	return typPerm, deps, st
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
	default:
		i.Error(stmt, "Unknown type of statement")
		panic(nil)
	}
}

func (i *Interpreter) visitBlockStmt(st Store, stmt *ast.BlockStmt) []StmtExit {
	st = st.BeginBlock()
	res := i.visitStmtList(st, stmt.List)
	for i := range res {
		res[i].Store = res[i].Store.EndBlock()
	}
	return res
}

func (i *Interpreter) visitCaseClause(st Store, stmt *ast.CaseClause) []StmtExit {
	var err error
	var mergedStore Store
	for _, e := range stmt.List {
		perm, deps, store := i.VisitExpr(st, e)
		st = store
		i.Assert(e, perm, permission.Read)
		st = i.Release(e, st, deps)
		if mergedStore, err = mergedStore.Merge(st); err != nil {
			i.Error(e, "Could not merge with previous results: %s", err)
		}
	}
	return i.visitStmtList(mergedStore, stmt.Body)
}

func (i *Interpreter) visitExprStmt(st Store, stmt *ast.ExprStmt) []StmtExit {
	_, deps, st := i.VisitExpr(st, stmt.X)
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
	perm, deps, st := i.VisitExpr(st, stmt.Cond)
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

func (i *Interpreter) visitStmtList(st Store, stmts []ast.Stmt) []StmtExit {
	labels := make(map[string]int)
	work := []struct {
		Store
		int
	}{{st, 0}}

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

nextWork:
	for len(work) != 0 {
		// We treat work as a stack. Semantically, it feels like a queue would be a better
		// fit, but it does not make any difference IRL, and by using a stack approach we
		// do not end up with indefinitely growing arrays (because we'd slice off of the first
		// element and then append a new one)
		start := work[len(work)-1]
		work = work[:len(work)-1]
		st := start.Store
		log.Printf("Visiting statement %d of %d", start.int, len(stmts))

		// Hey guys, we've already been here, discard that path.
		for _, sn := range seen {
			if sn.int == start.int && sn.Store.Equal(st) {
				log.Printf("Rejecting statement %d in store %v", start.int, st)
				continue nextWork
			}
		}
		seen = append(seen, start)

		stmt := stmts[start.int]
		exits := i.visitStmt(st, stmt)

		for _, exit := range exits {
			st := exit.Store
			switch branch := exit.branch.(type) {
			case nil:
				if len(stmts) > start.int+1 {
					addWork(st, start.int+1)
				} else {
					output = append(output, StmtExit{st, nil})
				}
			case *ast.ReturnStmt:
				// This exits the block
				output = append(output, exit)

			case *ast.BranchStmt:
				switch {
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
		perm, deps, store := i.VisitExpr(st, s.Results[k])
		store, _, err := i.moveOrCopy(s, store, perm, i.curFunc.Results[k], deps)
		if err != nil {
			i.Error(s, "Cannot bind return value %d: %s", k, err)
		}
		st = store
	}
	// A return statement is a singular exit.
	return []StmtExit{{st, s}}
}

func (i *Interpreter) visitIncDecStmt(st Store, stmt *ast.IncDecStmt) []StmtExit {
	p, deps, st := i.VisitExpr(st, stmt.X)
	i.Assert(stmt.X, p, permission.Read|permission.Write)
	st = i.Release(stmt.X, st, deps)
	return []StmtExit{{st, nil}}
}

func (i *Interpreter) visitSendStmt(st Store, stmt *ast.SendStmt) []StmtExit {
	chanRaw, chanDeps, st := i.VisitExpr(st, stmt.Chan)
	chn, isChan := chanRaw.(*permission.ChanPermission)
	if !isChan {
		i.Error(stmt.Chan, "Expected channel, received %v", chanRaw)
	}
	i.Assert(stmt.Chan, chn, permission.Write)

	val, valDeps, st := i.VisitExpr(st, stmt.Value)

	st, valDeps, err := i.moveOrCopy(stmt, st, val, chn.ElementPermission, valDeps)
	if err != nil {
		i.Error(stmt, "Cannot send value: %v", err)
	}

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
		rhs0, rdeps, store := i.VisitExpr(st, stmt.Rhs[0])
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
			perm, depsThis, store := i.VisitExpr(st, expr)
			log.Printf("Visited expr %#v in store %v", expr, st)
			st = store
			rhs = append(rhs, perm)

			// Screw this. This is basically creating a temporary copy or (non-temporary, really) move of the values, so we
			// can have stuff like a, b = b, a without it messing up.
			store, depsThis, err := i.moveOrCopy(expr, st, perm, perm, depsThis)
			st = store

			if err != nil {
				i.Error(expr, "Could not move value: %s", err)
			}

			deps = append(deps, depsThis...)
		}
	}

	if len(rhs) != len(stmt.Lhs) {
		i.Error(stmt, "Expected same number of arguments on both sides of assignment (or one function call on the right)")
	}

	for j, lhs := range stmt.Lhs {
		st = i.defineOrAssign(st, stmt, lhs, rhs[j], stmt.Tok == token.DEFINE)
	}

	st = i.Release(stmt, st, deps)

	return []StmtExit{{st, nil}}
}

func (i *Interpreter) defineOrAssign(st Store, stmt ast.Stmt, lhs ast.Expr, rhs permission.Permission, isDefine bool) Store {
	var err error

	// Define or set the effective permission of the left hand side to the right hand side. In the latter case,
	// the effective permission will be restricted by the specified maximum (initial) permission.
	if ident, ok := lhs.(*ast.Ident); ok {
		if isDefine {
			st, err = st.Define(ident.Name, rhs)
		} else {
			st, err = st.SetEffective(ident.Name, rhs)
		}

		if err != nil {
			i.Error(lhs, "Could not assign or define: %s", err)
		}
	} else if isDefine {
		i.Error(lhs, "Cannot define: Left hand side is not an identifier")
	}

	// Ensure we can do the assignment. If the left hand side is an identifier, this should always be
	// true - it's either Defined to the same value, or set to something less than it in the previous block.

	perm, _, _ := i.VisitExpr(st, lhs)    // We just need to know permission, don't care about borrowing.
	i.Assert(lhs, perm, permission.Owned) // Make sure it's owned, so we don't move unowned to it.

	// Input deps are nil, so we can ignore them here.
	st, _, err = i.moveOrCopy(lhs, st, rhs, perm, nil)
	if err != nil {
		i.Error(lhs, "Could not assign or define: %s", err)
	}

	return st
}
