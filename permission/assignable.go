// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import "fmt"

// This file defines when permissions are considered movable from one
// reference to another.
//
// TODO: Annotate functions with logic rules

type assignableMode int

const (
	assignMove      assignableMode = 0
	assignCopy      assignableMode = 1
	assignReference assignableMode = 2
)

type assignableStateKey struct {
	A, B Permission
	mode assignableMode
}
type assignableState struct {
	values map[assignableStateKey]bool
	mode   assignableMode
}

func newAssignableState(mode assignableMode) assignableState {
	return assignableState{make(map[assignableStateKey]bool), mode}
}

// copyAsReference converts a copy state into a reference state.
func copyAsReference(state assignableState) assignableState {
	switch state.mode {
	case assignCopy:
		return assignableState{state.values, assignReference}
	default:
		return state
	}
}

// MovableTo checks that a capability of permission A can be moved to
// a capability with permission B.
//
// A move is only allowed if the permissions of the target are narrower
// than the permission of the source. Values can be copied, however: A
// copy of a value might have a larger set of permissions - see CopyableTo()
func MovableTo(A, B Permission) bool {
	return assignableTo(A, B, newAssignableState(assignMove))
}

// RefcopyableTo checks whether an object with permission A can also be
// referenced by a permission B. It is generally like move, with two
// differences:
//
// 1. A may be unreadable
// 2. A and B may not be linear
func RefcopyableTo(A, B Permission) bool {
	return assignableTo(A, B, newAssignableState(assignReference))
}

// CopyableTo checks whether an object with permission A be copied into
// an object with permission B. The base requirement is that A is readable,
// or both have no permissions.
func CopyableTo(A, B Permission) bool {
	return assignableTo(A, B, newAssignableState(assignCopy))
}

func movableTo(A, B Permission, state assignableState) bool {
	return assignableTo(A, B, assignableState{state.values, assignMove})
}

func assignableTo(A, B Permission, state assignableState) bool {
	// Oh dear, this is our entry point. We need to ensure we can do recursive
	// permissions correctly.
	key := assignableStateKey{A, B, state.mode}
	isMovable, ok := state.values[key]

	if !ok {
		state.values[key] = true
		isMovable = A.isAssignableTo(B, state)
		state.values[key] = isMovable
	}

	return isMovable
}

func (perm BasePermission) isAssignableTo(p2 Permission, state assignableState) bool {
	perm2, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	switch state.mode {
	case assignCopy:
		return perm&Read != 0 || (perm == 0 && perm2 == 0) // Either A readable, or both empty permissions (hack!)
	case assignMove:
		return perm2&^perm == 0 && (perm&Read != 0 || (perm == 0 && perm2 == 0)) // No new permission && copy
	case assignReference:
		return perm2&^perm == 0 && !perm.isLinear() && !perm2.isLinear() // No new permissions and not linear
	}
	panic(fmt.Errorf("Unreachable, assign mode is %v", state.mode))

}

func (p *PointerPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	copyAsReferenceState := copyAsReference(state)
	switch p2 := p2.(type) {
	case *PointerPermission:
		return assignableTo(p.BasePermission, p2.BasePermission, state) && assignableTo(p.Target, p2.Target, copyAsReferenceState)
	default:
		return false
	}
}

func (p *ChanPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	state = copyAsReference(state)
	switch p2 := p2.(type) {
	case *ChanPermission:
		return assignableTo(p.BasePermission, p2.BasePermission, state) && assignableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

func (p *ArrayPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		return assignableTo(p.BasePermission, p2.BasePermission, state) && assignableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

func (p *SlicePermission) isAssignableTo(p2 Permission, state assignableState) bool {
	state = copyAsReference(state)
	switch p2 := p2.(type) {
	case *SlicePermission:
		return assignableTo(p.BasePermission, p2.BasePermission, state) && assignableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

func (p *MapPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	state = copyAsReference(state)
	switch p2 := p2.(type) {
	case *MapPermission:
		return assignableTo(p.BasePermission, p2.BasePermission, state) && assignableTo(p.KeyPermission, p2.KeyPermission, state) && assignableTo(p.ValuePermission, p2.ValuePermission, state)
	default:
		return false
	}
}

func (p *StructPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *StructPermission:
		if !assignableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Fields); i++ {
			if !assignableTo(p.Fields[i], p2.Fields[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *FuncPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	state = copyAsReference(state)
	switch p2 := p2.(type) {
	case *FuncPermission:
		// Ownership needs to be respected
		if p.BasePermission&Owned == 0 && p2.BasePermission&Owned != 0 {
			return false
		}
		// We cannot assign a mutable function to a value function, but vice
		// versa. This is reversed, because essentially
		if !assignableTo(p2.BasePermission&^Owned, p.BasePermission&^Owned, state) {
			return false
		}
		// Receivers and parameters are contravariant: If f2 takes argument
		// of a permission p2, and we want to assign f1 with permission p1,
		// then that permission may be more narrow.
		for i := 0; i < len(p.Receivers); i++ {
			if !movableTo(p2.Receivers[i], p.Receivers[i], state) {
				return false
			}
		}
		for i := 0; i < len(p.Params); i++ {
			if !movableTo(p2.Params[i], p.Params[i], state) {
				return false
			}
		}
		// Results are covariant
		for i := 0; i < len(p.Results); i++ {
			if !movableTo(p.Results[i], p2.Results[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *InterfacePermission) isAssignableTo(p2 Permission, state assignableState) bool {
	state = copyAsReference(state)
	switch p2 := p2.(type) {
	case *InterfacePermission:
		if !assignableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}

		for i := 0; i < len(p2.Methods); i++ {
			var l, r *FuncPermission
			r = p2.Methods[i]
			if i < len(p.Methods) {
				l = p.Methods[i]
			}
			if l == nil || l.Name != r.Name {
				l = nil
				for _, m := range p.Methods {
					if m.Name == r.Name {
						l = m
						break
					}
				}
			}

			if l == nil {
				panic(fmt.Errorf("Trying to move method %s, but does not exist in source %v", r.Name, p))
			}

			if !movableTo(l, r, state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *WildcardPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	return false
}

func (p *TuplePermission) isAssignableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *TuplePermission:
		if len(p.Elements) != len(p2.Elements) || !assignableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}
		for i := range p.Elements {
			if !assignableTo(p.Elements[i], p2.Elements[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *NilPermission) isAssignableTo(p2 Permission, state assignableState) bool {
	switch p2.(type) {
	case *ChanPermission, *FuncPermission, *InterfacePermission, *MapPermission, *NilPermission, *PointerPermission, *SlicePermission:
		return true
	default:
		return false
	}
}
