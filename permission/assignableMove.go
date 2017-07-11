// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

// This file defines when permissions are considered movable from one
// reference to another.
//
// TODO: Annotate functions with logic rules

// movableTo is a map to help with recursive types.
//
// TODO: We want a local map, a global one is ugly (or it needs a lock)
var movableTo = make(map[struct{ A, B Permission }]bool)

// MovableTo checks that a capability of permission A can be moved to
// a capability with permission B.
//
// A move is only allowed if the permissions of the target are narrower
// than the permission of the source. Values can be copied, however: A
// copy of a value might have a larger set of permissions - see CopyableTo()
func MovableTo(A, B Permission) bool {
	// Oh dear, this is our entry point. We need to ensure we can do recursive
	// permissions correctly.
	isMovable, ok := movableTo[struct{ A, B Permission }{A, B}]

	if !ok {
		movableTo[struct{ A, B Permission }{A, B}] = true
		isMovable = A.isMovableTo(B)
		movableTo[struct{ A, B Permission }{A, B}] = isMovable
	}

	return isMovable
}

func (perm BasePermission) isMovableTo(p2 Permission) bool {
	perm2, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	return perm2&^perm == 0
}

func (p *PointerPermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *PointerPermission:
		return MovableTo(p.BasePermission, p2.BasePermission) && MovableTo(p.Target, p2.Target)
	default:
		return false
	}
}

func (p *ChanPermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *ChanPermission:
		return MovableTo(p.BasePermission, p2.BasePermission) && MovableTo(p.ElementPermission, p2.ElementPermission)
	default:
		return false
	}
}

func (p *ArrayPermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		return MovableTo(p.BasePermission, p2.BasePermission) && MovableTo(p.ElementPermission, p2.ElementPermission)
	default:
		return false
	}
}

func (p *SlicePermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *SlicePermission:
		return MovableTo(p.BasePermission, p2.BasePermission) && MovableTo(p.ElementPermission, p2.ElementPermission)
	default:
		return false
	}
}

func (p *MapPermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *MapPermission:
		return MovableTo(p.BasePermission, p2.BasePermission) && MovableTo(p.KeyPermission, p2.KeyPermission) && MovableTo(p.ValuePermission, p2.ValuePermission)
	default:
		return false
	}
}

func (p *StructPermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *StructPermission:
		if !MovableTo(p.BasePermission, p2.BasePermission) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Fields); i++ {
			if !MovableTo(p.Fields[i], p2.Fields[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *FuncPermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *FuncPermission:
		// Ownership needs to be respected
		if !MovableTo(p.BasePermission&Owned, p2.BasePermission&Owned) {
			return false
		}
		// We cannot assign a mutable function to a value function, but vice
		// versa. This is reversed, because essentially
		if !MovableTo(p2.BasePermission&^Owned, p.BasePermission&^Owned) {
			return false
		}
		// Receivers and parameters are contravariant: If f2 takes argument
		// of a permission p2, and we want to assign f1 with permission p1,
		// then that permission may be more narrow.
		for i := 0; i < len(p.Receivers); i++ {
			if !MovableTo(p2.Receivers[i], p.Receivers[i]) {
				return false
			}
		}
		for i := 0; i < len(p.Params); i++ {
			if !MovableTo(p2.Params[i], p.Params[i]) {
				return false
			}
		}
		// Results are covariant
		for i := 0; i < len(p.Results); i++ {
			if !MovableTo(p.Results[i], p2.Results[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *InterfacePermission) isMovableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *InterfacePermission:
		if !MovableTo(p.BasePermission, p2.BasePermission) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Methods); i++ {
			if !MovableTo(p.Methods[i], p2.Methods[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
