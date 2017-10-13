// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"reflect"

	"github.com/julian-klode/lingolang/permission"
)

// Store associates variables with permissions for the abstract interpreter.
//
// Store essentially maps identifiers in the program to permissions; an
// effective, and a maximum one. As a special case, if ident is nil, the
// item acts marks the beginning of a new frame.
type Store []struct {
	name string
	eff  permission.Permission
	max  permission.Permission
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
		if v.name == "" {
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

	switch {
	case st == nil:
		return st2, nil
	case st2 == nil:
		return st, nil
	}

	for i, v := range st {
		if st[i].name != st2[i].name {
			return nil, fmt.Errorf("Invalid merge: Different identifiers %s vs %s at position %d", st[i].name, st2[i].name, i)
		}
		st3[i].name = st[i].name
		st3[i].eff, err = permission.Intersect(st[i].eff, st2[i].eff)
		if err != nil {
			return nil, fmt.Errorf("Cannot merge effective permissions %s and %s of %s: %s", st[i].eff, st2[i].eff, v.name, err)
		}
		st3[i].max, err = permission.Intersect(st[i].max, st2[i].max)
		if err != nil {
			return nil, fmt.Errorf("Cannot merge maximum permissions %s and %s of %s: %s", st[i].max, st2[i].max, v.name, err)
		}
	}
	return st3, nil
}

// Define defines an identifier in the current block. If the current block already contains
// a variable of the same name, no new variable is created, but the existing one is assigned
// by calling SetEffective().
func (st Store) Define(name string, perm permission.Permission) (Store, error) {
	// Do not allow a redefinition in the same block, make that an assignment instead. This matches
	// gos define operator.
	for _, item := range st {
		if item.name == name {
			return st.SetEffective(name, perm)
		}
		if item.name == "" {
			break
		}
	}
	var st2 = make(Store, len(st)+1)
	st2[0].name = name
	st2[0].max = perm
	st2[0].eff = perm
	for i, v := range st {
		st2[i+1] = v
	}
	return st2, nil
}

// SetEffective changes the permissions associated with an ident.
//
// The effective permission is limited to the maximum permission that the
// variable can have.
func (st Store) SetEffective(name string, perm permission.Permission) (Store, error) {
	st1 := make(Store, len(st))
	copy(st1, st)
	st = st1
	for i, v := range st {
		if v.name == name {
			eff, err := permission.Intersect(st[i].max, perm)
			if err != nil {
				return nil, fmt.Errorf("Cannot restrict effective permission of %s to new max: %s", v.name, err.Error())
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
func (st Store) SetMaximum(name string, perm permission.Permission) (Store, error) {
	st1 := make(Store, len(st))
	copy(st1, st)
	st = st1
	for i, v := range st {
		if v.name == name {
			eff, err := permission.Intersect(st[i].eff, perm)
			if err != nil {
				return nil, fmt.Errorf("Cannot restrict effective permission of %s to new max: %s", v.name, err.Error())
			}
			st[i].eff = eff
			st[i].max = perm
			return st, nil
		}
	}
	panic("Program error: Setting a nonexisting variable")
}

// GetEffective returns the effective permission for the identifier
func (st Store) GetEffective(name string) permission.Permission {
	for _, v := range st {
		if v.name == name {
			return v.eff
		}
	}
	return nil
}

// GetMaximum returns the maximum permission for the identifier
func (st Store) GetMaximum(name string) permission.Permission {
	for _, v := range st {
		if v.name == name {
			return v.max
		}
	}
	return nil
}
