// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
)

// StmtExit describes a possible exit point of a statement. It is a Store
// with an optional branch statement attached if (and only if) the statement
// performs an early exit of a (parent) block that needs to be handled in a
// parent block statement visitor. For example, a block that has been exited
// using break will have nil as it's exit value, unless that break is breaking
// an outer block.
type StmtExit struct {
	Store
	// exit might be either an *ast.BranchStmt or an *ast.ReturnStmt. The
	// latter needs to be forwarded to the function, the former to the block
	// statement handling it. Once it's been handled, nil should be returned
	// in its place to the caller.
	exit ast.Stmt
	// defers need to be propagated to the parent function call so we can
	// abstractly interpret them after the function execution. It's going to
	// be complicated...
	defers []ast.DeferStmt
}

// Interpreter interprets a given statement or expression.
type Interpreter struct {
	defers []ast.DeferStmt
}

func (i *Interpreter) VisitFuncDecl(st Store, fn *ast.FuncDecl) (Store, error) {
	var err error
	// TODO parameters and stuff
	exits := i.VisitBlockStmt(st, fn.Body)

	if len(exits) == 0 {
		panic(fmt.Errorf("No exit stores in function %v", fn))
	}

	st = exits[0].Store
	for i, value := range exits {
		if i > 0 {
			st, err = st.Merge(value.Store)
			if err != nil {
				return nil, err
			}
		}
	}
	return st, nil
}

func (i *Interpreter) VisitBlockStmt(st Store, blk *ast.BlockStmt) []StmtExit {
	var err error
	st = st.BeginBlock()
	for _, stmt := range blk.List {
		exits := i.VisitStmt(st, stmt)
		for _, exit := range exits {
			if exit.exit != nil {
				panic("Unhandled exit")
			}
			st, err = st.Merge(exit.Store)
			if err != nil {
				panic(err)
			}

		}
	}
	st = st.EndBlock()
	return []StmtExit{{st, nil, nil}}
}

func (i *Interpreter) VisitStmt(st Store, stmt ast.Stmt) []StmtExit {
	switch stmt := stmt.(type) {
	case *ast.BlockStmt:
		return i.VisitBlockStmt(st, stmt)
	}
	return nil
}
