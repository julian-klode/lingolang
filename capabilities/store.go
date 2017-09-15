// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"reflect"

	"github.com/julian-klode/lingolang/permission"
)

// Store associates variables with permissions for the abstract interpreter.
//
// Store essentially maps identifiers in the program to permissions; an
// effective, and a maximum one. As a special case, if ident is nil, the
// item acts marks the beginning of a new frame.
type Store []struct {
	ident *ast.Ident
	eff   permission.Permission
	max   permission.Permission
}

// NewStore returns a new, empty Store
func NewStore() Store {
	return nil
}

// Equal checks if two Stores are equal
func (st Store) Equal(ot Store) bool {
	if st == nil {
		return ot == nil || len(ot) == 0
	}
	if ot == nil {
		return st == nil || len(st) == 0
	}
	return reflect.DeepEqual(st, ot)
}

// BeginBlock copies the Store, prepending a frame marker at the beginning.
func (st Store) BeginBlock() Store {
	var st2 = make(Store, len(st)+1)
	for i, v := range st {
		st2[i+1] = v
	}
	return st2
}

// EndBlock returns a slice of the input describing the parent block.
func (st Store) EndBlock() Store {
	for i, v := range st {
		if v.ident == nil {
			return st[i+1:]
		}
	}
	panic("Program error: Not inside a block, so cannot end one")
}

// Merge merges two Stores describing two different branches in the code. The
// Stores must be defined in the same order.
func (st Store) Merge(st2 Store) (Store, error) {
	var err error
	st3 := make(Store, len(st))

	for i, v := range st {
		if st[i].ident != st2[i].ident {
			return nil, fmt.Errorf("Invalid merge: Different identifiers %s vs %s at position %d", st[i].ident, st2[i].ident, i)
		}
		st3[i].ident = st[i].ident
		st3[i].eff, err = permission.Intersect(st[i].eff, st2[i].eff)
		if err != nil {
			return nil, fmt.Errorf("Cannot merge effective permissions %s and %s of %s: %s", st[i].eff, st2[i].eff, v.ident, err)
		}
		st3[i].max, err = permission.Intersect(st[i].max, st2[i].max)
		if err != nil {
			return nil, fmt.Errorf("Cannot merge maximum permissions %s and %s of %s: %s", st[i].max, st2[i].max, v.ident, err)
		}
	}
	return st3, nil
}

// Define defines an identifier in the current block.
func (st Store) Define(ident *ast.Ident, perm permission.Permission) Store {
	var st2 = make(Store, len(st)+1)
	st2[0].ident = ident
	st2[0].max = perm
	st2[0].eff = perm
	for i, v := range st {
		st2[i+1] = v
	}
	return st2
}

// SetEffective changes the permissions associated with an ident.
//
// The effective permission is limited to the maximum permission that the
// variable can have.
func (st Store) SetEffective(ident *ast.Ident, perm permission.Permission) (Store, error) {
	st1 := make(Store, len(st))
	copy(st1, st)
	st = st1
	for i, v := range st {
		if v.ident == ident {
			eff, err := permission.Intersect(st[i].max, perm)
			if err != nil {
				return nil, fmt.Errorf("Cannot restrict effective permission of %s to new max: %s", v.ident, err.Error())
			}
			st[i].eff = eff
			return st, nil
		}
	}
	panic("Program error: Setting a nonexisting variable")
}

// SetMaximum changes the maximum permissions associated with an ident.
//
// Lowering the maximum permission also lowers the effective permission if
// they would otherwise exceed the maximum.
//
func (st Store) SetMaximum(ident *ast.Ident, perm permission.Permission) (Store, error) {
	st1 := make(Store, len(st))
	copy(st1, st)
	st = st1
	for i, v := range st {
		if v.ident == ident {
			eff, err := permission.Intersect(st[i].eff, perm)
			if err != nil {
				return nil, fmt.Errorf("Cannot restrict effective permission of %s to new max: %s", v.ident, err.Error())
			}
			st[i].eff = eff
			st[i].max = perm
			return st, nil
		}
	}
	panic("Program error: Setting a nonexisting variable")
}

// GetEffective returns the effective permission for the identifier
func (st Store) GetEffective(ident *ast.Ident) permission.Permission {
	for _, v := range st {
		if v.ident == ident {
			return v.eff
		}
	}
	return nil
}

// GetMaximum returns the maximum permission for the identifier
func (st Store) GetMaximum(ident *ast.Ident) permission.Permission {
	for _, v := range st {
		if v.ident == ident {
			return v.max
		}
	}
	return nil
}
